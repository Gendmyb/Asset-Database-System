# 企业资产管理系统 — 补全实施规划

## Context（背景）

- **现状**：Go1.25+Gin+pgx 后端 + React18+Vite 前端（Linear 暗色设计系统）。文档（architecture.md 2913 行）描述的是 236 人天企业级蓝图（Vault/K8s/Patroni/Redis/mTLS），README/progress.md 声称「11 Phase 全部完成」，但三个探索代理 + 自带审计报告（docs/DOC_VS_CODE_AUDIT.md）核实：**真实可用的只有 DEMO 模式下的资产 CRUD/领用归还/仪表盘主链路，约占目标功能 25%**。
- **确证的关键坏损**：生产模式领用必失败（种子 SQL 把 role 写成字面量 `'$4'`，user_repo.go:78）；生产 `GET /settings/next-tag` 必 panic（nil demoRepo，server.go:281）；前端「编辑保存」「生命周期转换」两个按钮 404/428（契约断裂 + If-Match 带引号）；15 分钟强制登出（依赖不存在的 /auth/refresh）；Docker 构建两条路径全坏；**全库无一个事务**（FOR UPDATE autocommit 下不持锁）；`system_settings` 表不存在但代码在查；编号 COUNT+1 并发重号；租户隔离仅 List 有（GetByID/Update/Delete 全 IDOR）；RBAC 零校验；audit_log 零写入；users/settings/organizations 生产模式也是硬编码/内存。
- **业务缺失**：入库、借用（区别于领用）、维修工单、盘点、折旧、导入导出完全没有；前端盘点/报表/用户管理/设置等 8 大块 UI 不存在。

## 用户决策

1. **定位：实用可部署优先**——中小企业能用：PG 单库 + Docker Compose。砍掉 Vault/K8s/Patroni/Redis Sentinel/mTLS/S3。
2. **业务全闭环**：入库、领用、**借用（临时+应还日期）**、归还、维修/保养、盘点、报废、折旧与报表、CSV/Excel 导入导出。
3. **API-First**：所有功能有完整 REST API。死代码按此裁决：event/webhook 接线，cache/ingest/agent 删除。
4. **Agent 硬件采集出范围**；前端 Agents 页删除。
5. **环境**：本机装 Go+Node（$HOME 免 root）；Docker 已装（需用户 `sudo usermod -aG docker gendmyb` + 重登录）；GitHub Actions CI 为权威门禁。
6. **DEMO 模式冻结保留**：修好现有 bug，新业务功能只做生产模式，前端 DEMO 下灰置未支持项。

## 执行要求（用户补充，2026-07-19）

1. **规划落库**：实施第一步即把本规划写入仓库 `docs/IMPLEMENTATION_PLAN.md`、创建 `docs/PROGRESS.md` 进度表并 git commit——会话中断不丢失。
2. **多代理并行 + 进度纪律**：主会话总控，按文件域派发并行子代理；**每个关键修复完成即更新 PROGRESS.md 并小步 commit**，中断随时可恢复续做。
3. **复用优先，禁止造轮子**：每个功能块动手前先调研成熟库/现成实现，拿来改造合并；仓库内已有优质模块（lifecycle 状态机、webhook 引擎、lock 包）直接复用不重写。
4. **终验双门禁**：全部完成后必须通过「项目经理代理」+「代码运行逻辑审计代理」检查（Phase J），发现问题回环修复直至通过。

## 执行模式

- **总控**：主会话负责派单、汇总、更新 PROGRESS.md、提交 git；子代理不直接改进度表（避免并发写冲突）。
- **并行分流**（按文件域隔离，天然低冲突）：
  - 后端流（assetserver/）、前端流（web/）、部署与 CI 流（Dockerfile/compose/.github/）、文档流（docs/）可并行
  - 同一文件域内的深度改造（如 Phase B 事务重构）串行单代理，避免自我冲突
  - 典型并行对：Phase A 的后端修复 ∥ 前端修复 ∥ CI 搭建；Phase C 后端认证 ∥ Phase D 前端基建（admin 页在 C 完成后接 API）；E/F/G 各自的后端 ∥ 前端
- **进度表格式**（docs/PROGRESS.md）：`| 阶段 | 任务 | 状态(待办/进行中/已完成/验证通过) | 提交 hash | 验证方式与结果 | 时间 |`
- **提交纪律**：每个关键修复独立 commit；每阶段结束 push 触发 CI，CI 绿 + 验收清单过 = 阶段完成。

## 复用优先——成熟库选型（代替从零搭建）

| 需求 | 采用 | 说明 |
|---|---|---|
| 数据库迁移执行 | **pressly/goose v3**（库方式） | embed.FS 迁移 + 版本表 + 锁，经 pgx stdlib 适配器接入；替代原方案的自研 runner |
| 密码哈希 | golang.org/x/crypto/bcrypt | 标准实现 |
| 登录限速 | golang.org/x/time/rate | per-key limiter，不自写计数器 |
| JWT | 既有 golang-jwt/v5 | 已在依赖树，继续用 |
| CSV | stdlib encoding/csv + UTF-8 BOM | 零依赖 |
| Excel 导出 | qax-os/excelize | 纯 Go 成熟库 |
| 金额计算（若 Go 侧需要） | shopspring/decimal | 折旧优先 SQL 内联 NUMERIC 计算，Go 侧兜底用 decimal |
| Go 测试断言 | stretchr/testify | 集成测试可读性 |
| 前端数据请求/缓存 | **TanStack Query v5** | 替代手写 loading/error/refetch 状态，全部新页面统一用 |
| 前端表单 | **react-hook-form** | 替代自写校验 helper；复杂 schema 可加 zod |
| Toast | **sonner** | 成熟、可主题化贴合 Linear 暗色设计系统；替代自写 ToastProvider |
| 图表 | recharts | React 声明式，tree-shake 后 ~50KB gz |
| 图标 | lucide-react（可选） | 替换手写 SVG/Unicode 字符 |
| 复用既有仓内模块 | domain/lifecycle.go、internal/webhook/（HMAC+SSRF）、internal/lock、audit_log 表设计（trigger+RLS+链哈希）、前端设计系统 CSS 变量 | 直接接线/扩展，不重写 |

原则：动手每个功能块前先花 5-10 分钟调研（pkg.go.dev / npm），维护活跃的成熟实现优先；只在「无合适库或库明显过重」时才手写。

## 总体设计决策

| 主题 | 决策 |
|---|---|
| 借用 vs 领用 | 扩展 `assignments` 表：`assignment_type('permanent'/'temporary')` + `due_date`，复用 `idx_active_assignment` 部分唯一索引（每资产同时仅一条 active，天然防「又领又借」） |
| 维修 | 新表 `maintenance_orders`，记录 `prev_status`，完成/取消时恢复资产原状态 |
| 盘点 | `stocktake_plans` + `stocktake_items`，start 时事务快照生成 items（冻结账面基线才能出盘亏结论） |
| 折旧 | assets 加采购/折旧字段，直线法 SQL 实时计算（不做快照表） |
| 入库 | 轻量：POST /assets 扩展采购字段（价格/日期/供应商/保修）+ `POST /assets/batch` 批量；不建独立入库单表（audit_log 的 create 即入库凭证） |
| 位置 | `locations` 落 PG 真表（盘点按位置圈范围的硬依赖），删内存 LocationStore |
| 组织 | 单组织默认可用 + 保留 org_id 管道 + 修 IDOR；OrgStore 换 PG 薄 repo，不做组织管理 UI |
| 角色 | **沿用 DB 现有 CHECK 枚举** `{super_admin, admin, manager, viewer}`（005 迁移去掉 'agent'），不引入 operator；等级 viewer(0)<manager(1)<admin(2)<super_admin(3)；UI 标签：只读/资产管理员/管理员/超级管理员 |
| 认证 | users 表 + bcrypt；refresh token 轮换 + 复用检测（family 全族吊销）；Ed25519 种子从 `JWT_ED25519_SEED` env 读（缺省随机+警告） |
| 审计 | service 层**事务内**写 audit_log（001 已有 BEFORE INSERT 触发器算链式哈希——实现时核实，Recorder 不重复算哈希） |
| 事务 | repo 方法签名改收 `DBTX` 接口（Pool 与 pgx.Tx 都满足）；service 层 `pgx.BeginFunc` 包裹多语句操作 |
| 导入导出 | CSV 为主（dry_run 预检+逐行错误+all-or-nothing 事务）；xlsx 导出仅报表处可选 |
| 部署 | **Go embed 单二进制**（web/dist 嵌入，NoRoute 回落 index.html）；compose = postgres + app 两服务 |

## 死代码裁决表

| 对象 | 裁决 | 阶段 |
|---|---|---|
| internal/cache、internal/ingest、internal/agent/*、cmd/collection-agent、deploy/nginx.conf、grafana/、web Agents 页、config 中 Redis/Vault 段、demo/(Python) | **删** | A（demo/ 在 I） |
| internal/event/bus.go | **接线**（service commit 后 Publish） | B |
| internal/webhook/（HMAC+SSRF 质量高） | **接线**（管理 API + 订阅 event bus） | I |
| handler/location.go、organization.go 内存 Store | **替换为 PG repo** | B |
| handler/dashboard.go（未注册） | 删（dashboard_repo 已承担职责） | B |
| internal/lock、domain/lifecycle.go、DemoAssetRepo | **留**（lifecycle 是核心；DEMO 冻结） | — |
| 表 collection_agents/asset_snapshots(+分区)/enrollment_tokens/approval_requests/archive_manifest/audit_meta/revoked_tokens | **DROP**（003 迁移） | B |
| 表 refresh_tokens/audit_log/organizations/asset_types | **用起来** | B/C |

## 阶段计划

依赖链：0→A→B→C→D 严格顺序；之后 E→F/G（可调序）→H（依赖 E 字段）→I→J 终验。**每阶段结束系统均处于可部署、可演示、CI 绿状态，PROGRESS.md 同步更新。**

### Phase 0 — 环境准备 + 规划落库（0.5 会话）
- **将本规划写入 `docs/IMPLEMENTATION_PLAN.md` + 创建 `docs/PROGRESS.md` 进度表，git commit**（执行要求 #1）
- 用户执行：`sudo usermod -aG docker gendmyb` + 重新登录（docker.sock 权限）
- 安装 Go 1.25.x → `~/.local/go`、Node 20 LTS → `~/.local/node`（官方 tarball，PATH 写入 ~/.bashrc）
- 首次真实验证：`cd assetserver && go build ./... && go test ./...`（验证「可编译」传闻）；`cd web && npm ci && npm run build`；`docker compose config`

### Phase A — 止血：契约修复 + 可构建可部署 + CI（2 会话，三流并行：后端 ∥ 前端 ∥ 部署CI）
后端：
- `user_repo.go:78`：`'$4'` → `$4`
- `server.go`：next-tag/settings/users/asset-types 生产分支改走 PG repo（settingsRepo/userRepo/dashRepo.ListAssetTypes），DEMO 保留内存（修 nil panic）；DEMO 分页实现 offset 游标（修重复加载）；DEMO transfer 落实资产状态
- `asset_v2.go:151`：If-Match 先 `strings.Trim(h, "\"")` 再 Atoi
- 新增 `migrations/002_settings_sequences.sql`：`system_settings(key,value,updated_at)` 表 + `doc_sequences(org_id,scope,next_seq)` 表（upsert-returning 原子取号，scope: asset/maintenance/stocktake）
- 删死代码（见裁决表）+ `go mod tidy` + Makefile 清理（删 agent-*、修 migrate 路径）
部署与 CI：
- Dockerfile 三阶段重写（node 构建 web → golang:1.25 构建含 embed → alpine 运行，context=仓库根）；新增 `assetserver/internal/webfs/`（`//go:embed dist` + NoRoute 回落 index.html）；docker-compose.yml 精简为 postgres+app（删 redis/grafana），app 环境含 `JWT_ED25519_SEED`
- 新增 `.github/workflows/ci.yml`：go vet/build/test + web tsc/build + docker build
前端：
- `Assets.tsx:431`：`PUT .../lifecycle` → `POST .../transition`，body `{to}`；`Assets.tsx:416`：If-Match 发不带引号版本号
- 新增 `web/src/lib/errors.ts` `getApiError()`（兼容 `{error:"s"}`/`{error:{code,message}}`），所有 catch 接入（修「错误永不显示」）
- `client.ts`：401 暂直接登出（refresh 端点 C 阶段才有）；`Login.tsx` refresh_token 兜底
- 删 /agents 路由+页面+类型；新增 404 兜底路由；顶栏徽标从 `/readyz` 读真实 mode
验收：CI 三 job 绿；`docker compose up -d --build` 后 curl 全链路（登录→建资产自动编号→If-Match 编辑 200→transition 200→assign/release 200→dashboard）；DEMO 裸二进制同链路；生产 next-tag 返回 200

### Phase B — 数据层地基：迁移器/事务/审计/租户隔离（3 会话，深度重构串行单代理）
- 迁移执行接入 **goose v3**（库方式 + embed.FS + pgx stdlib 适配器，存量库 baseline 处理）；main 启动调用；compose 移除 initdb.d 挂载（存量开发卷 `down -v` 重建，无生产数据）
- `003_drop_unused.sql`（DROP 7 张未用表）；`004_locations.sql`（locations 表：org_id/name/code/parent_id + UNIQUE(org_id,name)；assets.location_id 补 FK）
- 事务改造：新增 `internal/repository/db.go` `DBTX` 接口；全 repo 方法签名改造；新增 `internal/service/` 层（asset_service/assignment_service），`pgx.BeginFunc` 包裹 Create(取号)/Update/SoftDelete/Transition/Assign/Release/Transfer；Transfer 补旧 assignment 存在性 + 资产状态校验
- IDOR 修复：GetByID/UpdateWithRetry/SoftDelete/LockForUpdate/GetActiveAssignment 等全部补 `AND org_id=$n`
- 审计：新增 `internal/audit/recorder.go`，事务内写 audit_log（哈希交给已有触发器）；覆盖资产全操作；生产路由 `GET /assets/:id/history` + `GET /admin/audit-logs`
- `repository/org_repo.go`、`location_repo.go` 落 PG；注册 `GET/POST/PUT/DELETE /locations`；删内存 Store；种子演示用户改 `scripts/seed_demo.sql`
- event bus 接线：commit 后 Publish（asset.created/updated/assigned/...）+ log consumer
- `server.go` 拆分 routes.go / routes_demo.go；config.go 删 Redis/Vault
验收（本地 docker PG + CI postgres service）：并发 50 建资产编号无重复；Assign 注入失败无半写；org B token 查 org A 资产 404；每操作 audit_log +1 且链哈希连续；重启后 settings/orgs/locations 仍在

### Phase C — 真实认证与 RBAC（2 会话，可与 D 前端基建并行）
- `005_auth.sql`：admin 密码置 bcrypt("admin123") 常量 + `must_change_password`；refresh_tokens 加 `revoked_at`+UNIQUE(token_hash)；users.role CHECK 去掉 'agent'
- `crypto/jwt.go` 支持 `JWT_ED25519_SEED`（hex 32B）派生，缺省随机+启动警告
- 新增 `service/auth_service.go`：Login（users 表+bcrypt+active 校验+x/time/rate 限速：同用户名 5 次失败锁 15 分钟）；Refresh（轮换：旧 token 置 revoked 发新对；复用检测：已 revoked → 全 family 吊销+401）；Logout（吊销当前 family）
- 路由：重写 `POST /auth/login`（返回 access+refresh）、新增 `/auth/refresh`、`/auth/logout`、`GET /me`、`PUT /me/password`
- 新增 `middleware/rbac.go` `RequireRole(min)`：读 viewer+；资产写/领用借用/维修/盘点执行/导入 manager+；admin 域/设置/位置/盘点计划/报废/删除/导出 admin+
- 用户管理 API：`GET/POST /admin/users`、`PUT /admin/users/:id`、`POST /admin/users/:id/reset-password`（全写审计）
- 前端：client.ts 启用 refresh 队列（真端点）；Login 文案 admin/admin123
验收：错密码 5 次锁定；旧 refresh 复用→全族失效；viewer POST /assets 403；固定 seed 重启后旧 access token 有效；admin 建用户→新用户登录成功

### Phase D — 前端基建重构 + 管理页（2.5 会话）
- 引入 **TanStack Query**（数据请求统一）+ **react-hook-form**（表单）+ **sonner**（toast，主题化贴合设计系统）+ recharts（+可选 lucide-react 图标）
- `api/` 按域拆模块（auth/assets/assignments/users/settings/lookup）；`components/ui/`（Button/Input/Select/Modal/Drawer/ConfirmDialog/Badge/EmptyState/Spinner/FormField/DataTable）；`components/assets/` 七件套拆解 → `pages/Assets.tsx` 收敛 ~150 行
- 导航重组：仪表盘/资产管理/领用与借用/维修保养/盘点/报表/管理（admin+ 可见）；demo 模式灰置未支持项
- 新增 `pages/admin/Users.tsx`（列表/新建/改角色/禁用/重置密码）、`pages/admin/Settings.tsx`（系统设置+资产类型+位置 CRUD）
- 修遗留：类型/制造商下拉改 API 驱动；详情使用人首开即拉取；managed_by 接用户下拉
- Dashboard：recharts 状态 donut + 类型 bar
验收：tsc 0 error；所有失败操作有 toast；viewer 不见管理导航、直连 /admin/users 得 403 页；CI 绿

### Phase E — 入库增强 + 领用/借用/归还闭环（2 会话，后端 ∥ 前端）
- `006_asset_finance_assignments.sql`：assets 加 purchase_price/purchase_date/supplier/warranty_until/depreciation_method('none'/'straight_line')/useful_life_months/salvage_value/managed_by/retired_at/retire_reason；status CHECK 加 'borrowed'；assignments 加 assignment_type/due_date/return_notes + `CHECK(permanent OR due_date IS NOT NULL)` + 逾期部分索引
- `POST /assets/batch`（事务内连续取号建 N 条）；`POST /assets/:id/borrow`（due_date 必填→type=temporary+status='borrowed'）；Release 增强（return_notes，服务 assigned/borrowed）；Transfer 限 permanent（borrowed 409）
- `GET /assignments`（filter：status/type/assigned_to/overdue/cursor）+ `GET /users/:id/assignments`
- 前端：入库分组表单（基本/采购/管理归属+批量数量）；新增 `pages/Assignments.tsx`（tabs：全部/领用/借用/已逾期，行内归还、逾期红标）；BorrowDialog（应还日期必填）；「借用中」badge
验收：借用缺 due_date 400；并发 assign+borrow 仅一成功；overdue 过滤只含逾期 active temporary；归还后可再领用；批量 20 条编号连续唯一

### Phase F — 维修/保养工单 + 报废（1.5 会话）
- `007_maintenance.sql`：maintenance_orders（order_no MNT- 取号/category repair|upkeep/status open|in_progress|completed|canceled/prev_status/cost/唯一活跃索引 per asset）
- API：`POST /maintenance-orders`（校验无活跃工单、非 retired→记 prev_status→资产 maintenance）；`/:id/start` `/complete{resolution,cost}` `/cancel`（恢复 prev_status）；列表+详情；全程审计
- `POST /assets/:id/retire {reason 必填}`（校验无 active assignment/活跃工单→retirement 终态+retired_at；admin+）
- 前端：`pages/Maintenance.tsx`（列表+流转+新建）；资产详情报修/保养/报废入口（ConfirmDialog 填原因）
验收：开单后资产维护中不可领用；完成恢复开单前状态（含 assigned 场景）；有活跃领用报废 409；报废终态按钮全消失

### Phase G — 盘点（2 会话）
- `008_stocktake.sql`：stocktake_plans（plan_no STK-/scope_location_id/scope_type_id/status draft|in_progress|completed|canceled）+ stocktake_items（result pending|found|missing|moved|surplus/actual_location_id/surplus_note/UNIQUE(plan_id,asset_id)）
- API：建计划（admin+）；`/:id/start`（事务快照圈定非 retired 资产批量生成 items）；items 勾选 `PUT /:id/items/:itemId`（manager+）；盘盈登记 `POST /:id/items`；`/:id/complete{apply_moves}`（moved 项批量更新资产位置+审计）；`/:id/report`（JSON 汇总+`?format=csv`）
- 前端：`pages/Stocktakes.tsx` + `pages/StocktakeDetail.tsx`（进度条、逐项核对、盘盈登记、报告视图+CSV 下载）
验收：快照数=范围内资产数；start 后新建资产不入本次；重复 complete 409；报告计数与 items 一致；apply_moves 位置已更新且有审计

### Phase H — 折旧、报表、导入导出（2.5 会话）
- 折旧直线法 SQL 内联：月折旧=(price−salvage)/months；净值=GREATEST(price−月折旧×已提月数, salvage)
- API：`GET /reports/summary`（原值/累计折旧/净值/按类型/状态/位置）、`/reports/depreciation?as_of=`、`/reports/maintenance-cost?from&to`、`/reports/assignments-due?days=`
- 导出：`GET /assets/export?format=csv`（透传当前 filter，BOM）+ 报表导出；可选 xlsx（excelize）
- 导入：`GET /assets/import/template`；`POST /assets/import?dry_run=true`（multipart，逐行解析+名称解析类型/位置/用户+重复检测→`{total,valid,errors:[{row,field,message}]}`）；正式导入单事务 all-or-nothing
- 前端：`pages/Reports.tsx`（KPI 卡+折旧明细+导出按钮）；资产页导入向导（上传→dry-run 错误预览→确认）；Dashboard 补净值 KPI + 30 天趋势线
验收：算例 12000/残值0/60月/已用13月→净值 9400；超期净值=残值；dry-run 行号准确；含错文件正式导入 0 写入；CSV Excel 打开中文无乱码

### Phase I — Webhook 接线、文档校准、CI 完整化（2 会话，后端 ∥ 文档流并行）
- `009_webhooks.sql`：webhook_endpoints（url/secret AES-256-GCM 加密存储 `APP_SECRET`/events[]/active）+ webhook_deliveries；`/admin/webhooks` CRUD + `/test` + `/deliveries`；engine 订阅 event bus 异步投递（复用现有重试+SSRF 防护）
- 文档校准：README 按真实功能重写（功能矩阵/快速开始/默认账号/角色表/API 索引）；architecture.md → `docs/archive/` 加「历史设计稿，未按此实现」横幅 + 新写 ~400 行真实架构文档；progress.md → CHANGELOG.md；runbook.md 重写为 compose 运维手册（pg_dump 备份恢复/升级/管理员找回 SQL）；新增 `docs/api.md`；DOC_VS_CODE_AUDIT.md 加已修复注记
- CI 完整化：integration job（postgres service+迁移+`scripts/e2e.sh`）+ compose 冒烟（up→healthz→登录→建资产）；删 `demo/` Python 目录
验收：新读者按 README 从零 compose 十分钟完成全业务演示；文档无一处声称未实现功能；CI 全绿含 e2e

### Phase J — 终验双门禁（1 会话，用户执行要求 #4）
- **项目经理代理**：对照本规划逐项验收——功能矩阵完成度、各阶段验收标准复核、PROGRESS.md 完整性、文档与代码一致性，输出验收报告
- **代码运行逻辑审计代理**：沿用 DOC_VS_CODE_AUDIT.md 方法论全库重审——前后端契约逐条对照、事务/并发正确性、IDOR/RBAC 覆盖、死代码残留、DEMO 冻结边界，输出 `docs/FINAL_AUDIT.md`
- 两代理发现的问题 → 回环修复 → 复审，直至双通过；最终 CI 全绿 + compose 全业务演示通过收官

## 验证方式（贯穿）

- 本地：Go+Node 编译/单测/`tsc`；`docker compose up` 起 PG 全栈手动+curl 链路验证；集成测试 `DATABASE_URL` gate（本地指向 docker PG）
- CI（权威门禁）：每阶段结束 push，vet/build/test/tsc/docker build/integration/e2e 全绿才算完成
- 关键并发正确性（编号取号、assign 竞争、refresh 复用）必须有集成测试覆盖
- 每个关键修复 → PROGRESS.md 更新 + 小步 commit（中断可恢复）

## 风险

1. 事务改造（Phase B）是正确性关键路径——现有所有「锁」是安慰剂，改造前不做并发承诺
2. 存量 docker 卷需 `down -v` 重建（项目未上线无生产数据，零风险）
3. `JWT_ED25519_SEED` 丢失=全员重登录（可接受）；compose .env 模板给生成命令
4. 单管理员锁死：runbook 提供 SQL 找回流程
5. Docker 组权限需用户执行一次 usermod + 重登录
6. 并行代理冲突：以文件域分流规避；同域深改串行；进度表只由主会话写

## 工作量估计

| 阶段 | 0 环境 | A 止血 | B 地基 | C 认证 | D 前端基建 | E 入库领借还 | F 维修报废 | G 盘点 | H 折旧报表导入导出 | I Webhook+文档 | J 终验 | 合计 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 会话 | 0.5 | 2 | 3 | 2 | 2.5 | 2 | 1.5 | 2 | 2.5 | 2 | 1 | **~21** |
