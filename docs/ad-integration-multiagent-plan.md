# AD 域控接入 · 多 Agent 编排实施方案（可执行版）

> **状态**：已批准，待执行
> **编排者**：Claude(PM,主循环，统一把控所有 agent)
> **用法**：下个会话按本文档拆任务、逐波派 agent 执行。本文档自包含——无需翻阅历史对话即可开工。
> **创建日期**：2026-07-19

---

## 1. 背景与目标

企业画像：超两千万 IT 资产、约 1000 人的中型企业。本系统用于日常设备管理、设备出入库登记、资产盘点、与域控同步人员名单。

**目标**：把"系统用户由 AD 域控同步"从现状的"可用但粗放"升级为"企业可管控"——**按安全组圈人、按组定角色、默认个人只读、超管可控、存量可合并**。

**关键认知（避免返工）**:
- AD/LDAP 集成本系统**已建好一版**(Wave 1 G1)，本次是增强而非从零新建。核心代码在 `assetserver/internal/auth/ldap/`(`client.go`/`auth.go`/`sync.go`)。
- README/提交里的"**SSO**"**不是真单点登录**，代码里没有任何 OIDC/SAML/Kerberos/OAuth，实际是 **LDAP bind 密码认证**（员工在登录页输域账号密码，系统去 AD 校验）。本期保持此方式，不做真 SSO。

---

## 2. 已确认的 4 项决策（产品口径，驱动全部设计）

| 决策点 | 用户拍板 | 设计含义 |
|---|---|---|
| **登录方式** | 走域账号登录（LDAP bind)；保留超管控制账号开启/关闭；后续再接 Lark | 主登录=现有 LDAP bind；新增 `manual_override` 保证**AD 同步不覆盖超管手动禁用/改权**。Lark（飞书）列后续，本期只留扩展点（G6 已支持飞书 webhook 通知） |
| **同步范围** | **按安全组成员**同步 | `SearchUsers` 改为按 `memberOf=<组DN>` 过滤（多组）+ **`ControlPaging` 分页**（堵 AD `MaxPageSize=1000` 静默截断，正好卡在 1000 人规模） |
| **角色分配** | 后台给每个安全组配角色；**默认 = 只读自己名下设备** | 新建"安全组→角色"映射表 + admin 界面。**默认角色是新数据范围（个人只读 self-scope)**：现有 `viewer` 能看全组织，需新增"仅见分配给自己的设备"范围 |
| **存量账号** | 留 admin 兜底 + **账号链接合并** | 保留本地 admin（现状已满足：同步只碰 `source='ldap'`，同名冲突跳过）；新增"同名本地账号链接到 AD"机制，保留 id/角色/领用历史 |

---

## 3. 现状关键事实（执行前必读，均已核实）

### 数据模型
- `assets.users`:id, org_id(FK), username(UNIQUE), password_hash, role, email, status(active/disabled/locked), last_login_at, deleted_at（软删）。迁移 `011` 已加 `source('local'|'ldap')`、`external_id`、`display_name`、`dn`，含 `(source, external_id)` 唯一索引。
- role CHECK 约束经迁移 `005` 统一为 `('super_admin','admin','manager','viewer')`。
- `assets.organizations`:ltree 层级树（parent_id + path + depth),G9 部门行权限已基于此。
- `assets.assignments`:`assigned_to`(user id)+ `status='active'` 表示活跃领用；**"我名下设备" = 有活跃领用且 assignee=我**（有索引 `idx_assignments_user ON assigned_to WHERE status='active'`)。

### 认证 / RBAC
- 登录 `service/auth_service.go::Login`：**本地优先 + LDAP 兜底**；本地 admin 永不被 AD 覆盖；JWT Ed25519 双 token;bcrypt；按用户名限流（内存，单实例）。
- RBAC `middleware/rbac.go`:4 级，`RequireRole(min)` 中间件。
- 数据范围 `repository/org_scope.go`:`OrgScope{OrgID,Role,Mode}.ClauseFor(col,nextIdx)` 生成**参数化** SQL;`Mode ∈ {ScopeOrg, ScopeDepartment}`。资产各查询（`asset_repo.go`)都走它——扩展新模式侵入面小。`handler/asset_v2.go::orgScopeFromCtx` 从 gin 上下文（JWT claims)构建。

### 现有 AD/LDAP（要增强的点）
- `client.go::SearchUsers`:**硬编码 `(objectClass=user)` 全量拉，不吃 `UserFilter`，无分页**（→ 范围不可控 + 1000 截断风险）。
- 不读 `memberOf`（→ 无法组映射）、不读 `userAccountControl`（→ AD 禁用不同步）。
- `auth.go::EnsureUserRow` / `sync.go`：新 AD 用户**硬编码 `viewer`**；本地同名冲突跳过。
- 触发：手动 `POST /admin/ldap/sync`(admin+，未启用返回 503)；定时（调度器 `SCHEDULER_LDAP_SYNC=true` + `SCHEDULER_INTERVAL`)。
- **前端零入口**：无同步按钮 / 无 LDAP 配置 / 用户列表无 source 列 / 无 CSV 导入 UI。

### 测试 / 构建约定（给测试 agent 的契约）
- 后端测试：`cd assetserver && go test ./...`（即 `make test`)。**单测用 fake/mock**（如 `DirectoryClient` 是接口可 mock),**不依赖真实 DB/LDAP**。无 build tag，只有 `testing.Short()` 用于 fuzz。
- 前端 `web/`:**无测试框架**,`package.json` 仅 `"build": "tsc && vite build"`。前端"测试" = tsc 类型检查 + 构建通过 + 逻辑走查。

---

## 4. 多 Agent 协作模型

**每个任务包（Task）的执行管线，由 PM 驱动并门禁：**

| 角色 | 职责 | 产出 |
|---|---|---|
| **PM（主循环）** | 拆任务、写任务简报（上下文/契约/验收/审计点）、派 agent、裁决审计与测试结果、合并代码、跑全量回归、更新进度文档 | 任务简报、合并与门禁决定 |
| **开发**（小任务 PM 亲自，较大任务派 1 个 dev agent) | 按简报实现，**新逻辑一律进新文件** | 代码 + 变更摘要 |
| **审计 agent**(1，独立，只读） | 审 diff：正确性、安全（凭据/LDAP 注入/IDOR)、契约与规范符合度、向后兼容 | 结构化发现（severity + file:line + 建议） |
| **测试 agent**(1，独立） | 补/写测试、跑 `go test ./...` 与构建、逐条核对验收标准 | 通过/失败 + 证据 |

**门禁（Definition of Done)**：开发完成 → 审计 + 测试 agent **并行**验证 → PM 裁决；任一 P0/P1 审计发现或测试失败 → 打回开发 agent 重做，循环至全绿 → PM 合并并跑全量 `go test ./...` + `npm run build` → 标记完成。

> 实现默认 PM 亲自（小任务）或另派 dev agent（大任务）；如需实现也独立成 agent，每任务升级为 3 个，按需裁剪。

---

## 5. 并行波次与依赖

```
Wave0  T0 契约冻结(必须先合入,串行)
Wave1  T1 LDAP客户端 │ T2 组映射+解析 │ T5 个人数据范围      ← 文件互不重叠,可并行
Wave2  T3 同步引擎(需T1,T2) │ T4 登录(需T1,T2) │ T6 资产self-scope(需T5) │ T7 目录API(需T0,T2)
Wave3  T8 用户API(需T0) │ T9 目录集成前端页(需T7)        ← T8与T7共改routes.go, PM合并
Wave4  T10 用户页前端(需T8) │ 集成回归+E2E+文档
```

**冲突规则**：共享文件 `routes.go` / `App.tsx` / `AdminLayout.tsx` 只做**追加式注册**；并行任务若同改一个共享文件，由 PM 做文本级合并。页面/服务/handler 等**新文件天然无冲突**，这是能并行的前提。

---

## 6. 共享契约（T0 冻结，所有 agent 对齐的唯一事实源）

- **DB(migration `assetserver/migrations/014_ad_enterprise.sql`，全部新增、默认安全）**:
  - `assets.ad_group_mappings(id UUID PK, group_dn VARCHAR(512) UNIQUE NOT NULL, group_name VARCHAR(255), role VARCHAR(50) NOT NULL DEFAULT 'viewer' CHECK(role IN ('super_admin','admin','manager','viewer')), data_scope VARCHAR(20) NOT NULL DEFAULT 'inherit', sync_enabled BOOLEAN NOT NULL DEFAULT true, created_at, updated_at)`
  - `assets.users` 增列：`data_scope VARCHAR(20) NOT NULL DEFAULT 'inherit'`（值 `'inherit'|'self'`)、`manual_override BOOLEAN NOT NULL DEFAULT false`
- **Go 签名**:
  - `resolveRoleForGroups(memberOf []string, maps []GroupMapping) (role, scope string)`（多组取最高角色，无命中→默认映射）
  - `OrgScope` 增 `UserID string` 与 `ScopeSelf` 模式
  - `DirectoryClient.SearchUsers` 改为支持注入 filter + 分页，并返回 `memberOf` / `userAccountControl`
- **API 形状**:`GET/POST/PUT/DELETE /admin/ad-groups`;`GET /admin/ldap/status`;`POST/DELETE /admin/users/:id/link-ad`;`POST /admin/ldap/sync`（已有）。
- **配置键**:`LDAP_PAGE_SIZE=500`、`LDAP_SYNC_RECURSIVE=false`、`LDAP_LINK_EXISTING=false`、`LDAP_GROUP_ATTR=memberOf`;`.env.example` 补齐全部 LDAP/调度变量（当前缺失）。
- **前端**:`/admin/directory` 路由 + `AdminLayout` 增"目录集成"页签。

---

## 7. 任务包清单（每包可直接派工）

> 格式：**目标 / 依赖 / 文件 / 要点 / 验收（测试 agent) / 审计重点（审计 agent) / 工作量**

### T0 契约冻结
- 依赖：无 | 文件：`migrations/014_ad_enterprise.sql`、`config.go`、`.env.example`
- 要点：建映射表 + users 加两列 + 新配置键，默认全关向后兼容
- 验收：迁移幂等可重复跑；纯本地（LDAP 未启用）模式行为不变
- 审计：新增项不破坏现有 CHECK/索引；无凭据入库；默认值安全 | **0.5d**

### T1 LDAP 客户端加固
- 依赖：T0 | 文件：`internal/auth/ldap/client.go`
- 要点：`SearchUsers` 加 `ControlPaging(500)` + 可注入 group filter（可选递归链 OID `1.2.840.113556.1.4.1941`，默认直接成员）；增读 `memberOf`、`userAccountControl`
- 验收：构造 >1000 条 mock 验证分页不漏不重；filter 正确拼接且转义
- 审计：防 LDAP 注入；凭据不入日志；TLS 不弱化 | **1.5d**

### T2 组映射存储 + 角色解析
- 依赖：T0 | 文件：新 `repository/ad_group_repo.go`、新 `ldap/resolve.go`
- 要点：映射 CRUD repo + `resolveRoleForGroups`（最高角色优先，无命中→默认映射，禁用组忽略）
- 验收：单测覆盖多组/无组/默认/禁用组各分支
- 审计：角色提升只能来自显式映射；默认最小权限 | **1d**

### T3 同步引擎重写
- 依赖：T1,T2 | 文件：`internal/auth/ldap/sync.go`
- 要点：按启用组圈人；角色/范围来自 T2;`manual_override` 跳过 role/status/scope（仍刷新 profile);AD 禁用→本地禁；移出所有组→禁/软删；审计摘要
- 验收：mock 目录跑全量同步，核对 增/改/禁/审计 计数
- 审计：不覆盖本地用户；`manual_override` 生效；无凭据落审计 | **1.5d**

### T4 登录路径
- 依赖：T1,T2 | 文件：`internal/auth/ldap/auth.go`、`service/auth_service.go`
- 要点：`SearchOne` 返回 memberOf/userAccountControl;`EnsureUserRow` 用 T2 定角色/范围并尊重 override;AD 禁用拒绝登录
- 验收：登录新建/更新/禁用/override 各路径正确
- 审计：本地优先兜底不破；统一错误不泄露用户存在性 | **1d**

### T5 个人数据范围 ScopeSelf
- 依赖：T0 | 文件：`repository/org_scope.go`、JWT claims、`handler/asset_v2.go`
- 要点：`OrgScope` 加 `UserID` + `ScopeSelf` 模式（`asset.id IN (SELECT asset_id FROM assets.assignments WHERE assigned_to=$uid AND status='active')`);`orgScopeFromCtx` 读 JWT 的 data_scope
- 验收：`ClauseFor` 对 self 生成正确参数化子查询
- 审计：无注入；super_admin/manager 不受影响 | **1d**

### T6 资产查询 self-scope 集成
- 依赖：T5 | 文件：`asset_repo.go` + 测试
- 要点：核对所有走 scope 的查询（列表/详情/子资产）在 self 下正确
- 验收：员工只见名下设备；他人设备不可见（防 IDOR)
- 审计：跨用户数据隔离无旁路 | **0.5d**

### T7 目录集成 API
- 依赖：T0,T2 | 文件：新 `handler/ad_directory.go` + `routes.go` 注册
- 要点：组映射 CRUD + LDAP status（连通/启用/上次同步结果）+ link/unlink-ad + 默认角色设置
- 验收：端点 RBAC(admin+）与 CRUD 行为正确；link 保留 id/历史
- 审计：仅 admin 可达；输入校验；link 不丢历史 | **1.5d**

### T8 用户 API 增强
- 依赖：T0 | 文件：`repository/user_repo.go` + `routes.go`
- 要点：列表返回 source/display_name/部门/last_login/data_scope;admin 编辑置 `manual_override=true`
- 验收：字段齐全；编辑后 override=true
- 审计：不暴露敏感字段；override 只增不悄改 | **1d**

### T9 目录集成前端页
- 依赖：T7 | 文件：新 `web/src/pages/admin/Directory.tsx` + api client
- 要点：状态卡 / 手动同步按钮 / 上次同步结果 / 组映射 CRUD 表
- 验收：`npm run build`(tsc）过；交互逻辑走查
- 审计：错误态/loading 态；不硬编码凭据 | **1.5d**

### T10 用户页前端增强
- 依赖：T8 | 文件：`web/src/pages/admin/Users.tsx` + api
- 要点：source 徽标 / display_name / 部门 / 最后登录 / "链接到 AD"操作
- 验收：tsc 构建过；列表与操作正确
- 审计：操作有确认；权限按钮按角色显隐 | **1d**

### 集成回归 + E2E + 文档
- 依赖：全部 | 全量 `go test ./...` + `npm run build` + 起服务手工 E2E + 更新 `docs/PROGRESS.md`/`DEPLOYMENT.md`/`CHANGELOG.md`
- 验收关键链路：同步 → 登录 → 组赋角色 → self 可见 → link，端到端通；回归无红；文档与代码一致 | **1d**

---

## 8. 环境与运行（执行会话必读）

> 本机 `node`/`npm`/`go` **不在默认 PATH**，调试前必须先加：
> ```bash
> export PATH=$HOME/.local/node/bin:$HOME/.local/go/bin:$PATH
> ```
> (node v22 在 `~/.local/node/bin`;go 在 `~/.local/go/bin`)

**本地起后端**(production 模式，PG 已在 5432):
```bash
cd assetserver
export DATABASE_URL="postgres://app_user:app_pass@localhost:5432/assetdb?sslmode=disable&search_path=assets"
export DB_USER=app_user DATABASE_PASSWORD=app_pass JWT_ED25519_SEED=$(openssl rand -hex 32)
nohup ./bin/api-server > /tmp/assetdb-api.log 2>&1 &
```
默认登录 `admin/admin123`。`setup-and-run.sh` 一键构建+启动。

**⚠️ pkill 陷阱**:`pkill -f 'bin/api-server'` 会误杀当前 bash（命令行含该串）。用 `pkill -x api-server`（精确进程名）或按端口 PID 杀。

**前端构建**:`cd web && npm run build`(node_modules 已存在），然后 `rm -rf assetserver/web/dist && cp -r web/dist assetserver/web/dist` 再 `go build`。dist 提交占位符，CI/setup 按需重建。

**构建/测试命令**:
- 后端构建：`cd assetserver && go build -o api-server ./cmd/api-server`（或 `make build`)
- 后端测试：`cd assetserver && go test ./...`（或 `make test`)
- 前端构建：`cd web && npm run build`

---

## 9. 质量门禁 / Definition of Done

单任务完成需同时满足：
1. 代码按简报实现，新逻辑进新文件；共享文件仅追加式注册。
2. **测试 agent**：相关测试通过 + 全量 `go test ./...` 无红 +（前端任务）`npm run build` 过；验收标准逐条核对通过。
3. **审计 agent**：无 P0/P1 发现（安全/正确性/兼容/契约）;P2 记录待办。
4. PM 复核 diff，确认符合 §6 契约，未引入计划外依赖。

每波结束 PM 出进度小结并更新 `docs/PROGRESS.md`。

---

## 10. 回滚与兼容

- 全部 schema 变更**新增列/新增表、默认关闭**；未启用 LDAP 时系统行为与现状完全一致（向后兼容）。
- 每任务按文件归属可独立 revert；契约（T0）一旦合入不改签名，后续任务只增量。
- 安全基线（不可破）：密码/服务账号凭据**绝不入日志/审计**;LDAP filter 转义防注入；默认角色最小权限；本地 admin 兜底永存。

---

## 11. 本期明确不做（列入后续单独立项）

真 SSO(OIDC/SAML/Kerberos)、Lark 登录集成（仅留扩展点）、AD OU 层级→组织树多级同步（现为一级拍平）、增量/delta 同步（现为全量）。

---

## 12. 执行检查清单（下个会话开工顺序）

1. 读本文档 + `docs/PROGRESS.md`，确认起点。
2. `export PATH=$HOME/.local/node/bin:$HOME/.local/go/bin:$PATH`。
3. 确认在分支 `feat/enterprise-adaptation`（或按计划新建特性分支）。
4. 执行 **T0** 并合入 → 冻结契约。
5. 按 §5 波次逐波派 agent（Wave1 起可并行），每任务走"开发→审计+测试→PM 门禁"管线。
6. 每波结束跑全量回归 + 更新进度文档。
7. 全部完成后做集成 E2E，更新文档，收尾。

**规模**：开发量约 12.5 人日 + 每任务审计/测试开销；并行后**日历工期约 5–7 天**。
