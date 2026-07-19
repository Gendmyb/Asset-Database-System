# CHANGELOG

## 未发布 (2026-07-19 之后)

### 企业化适配（Wave 1+2）

> 缺口 G1–G9 全部交付，双门禁通过；提交 `967324f`（Wave 1）、`a600eca`（Wave 2）。所有新功能默认关闭，向后兼容 v0.2.0 行为。

- **G1 AD/LDAP 同步 + SSO**（`internal/auth/ldap/`）：登录本地优先，本地未命中走 LDAP bind 兜底；admin 账号永不被 LDAP 覆盖。配置 env：`LDAP_HOST`/`LDAP_PORT`/`LDAP_USE_TLS`(starttls)/`LDAP_USE_SSL`(ldaps)/`LDAP_BIND_DN`/`LDAP_BIND_PASSWORD`/`LDAP_BASE_DN`/`LDAP_USER_FILTER`(默认 `(&(objectClass=user)(sAMAccountName=%s))`)；`LDAP_HOST`+`LDAP_BASE_DN`+`LDAP_BIND_DN` 三者齐全才自动启用，否则纯本地模式。`LDAP_SYNC_DISABLE_ONLY` 控制仅禁用而不创建账号。API：`POST /admin/ldap/sync`（admin+）。
- **G2 用户批量导入**：`POST /admin/users/import?dry_run=true`（dry-run 预检）、`GET /admin/users/import/template`（CSV 模板下载）。CSV 列：`username,display_name,email,role,org_path,password`。
- **G3 扫码 + 移动盘点**：`GET /assets/:id/qrcode`（PNG 二维码，默认内容为 `asset_tag`；`?content=url` 模式基于受信 `EXTERNAL_URL` 拼详情页 URL，未配置则回退 400）。前端盘点页支持扫码录入与响应式布局。
- **G4 到期提醒**（`internal/scheduler/`）：env `SCHEDULER_INTERVAL`（默认 `off` 不启动；支持 `30m`/`1h` 等duration 或纯数字秒）、`SCHEDULER_WARRANTY_DAYS`(默认 30)、`SCHEDULER_LDAP_SYNC`(bool，定时触发 LDAP 同步)。扫描保修到期 / 领用逾期并发布事件。
- **G5 Excel 导出**：`GET /reports/assets.xlsx`；盘点报告、折旧报表支持 `?format=xlsx` 切换 Excel 输出。
- **G6 通知渠道**（`internal/notify/`）：SMTP 邮件 + 钉钉 + 企微 + 飞书机器人。env：`NOTIFY_ENABLE`、`SMTP_HOST`/`SMTP_PORT`(默认 587)/`SMTP_USER`/`SMTP_PASSWORD`/`SMTP_FROM`、`NOTIFY_DINGTALK_WEBHOOK`/`NOTIFY_WECOM_WEBHOOK`/`NOTIFY_FEISHU_WEBHOOK`。API：`/admin/notify/rules`、`/admin/notify/deliveries`。机器人 webhook 强制 HTTPS + SSRF 防护。
- **G7 审批流**：领用 / 报废 / 维修可配置审批门，系统设置 `approval.assignment.enabled` / `approval.retirement.enabled` / `approval.maintenance.enabled`（默认全部关闭，向后兼容）。API：`/admin/approvals`、`/admin/approvals/:id/approve|reject`。
- **G8 资产关系 / 外设挂载**：`POST /assets/:id/mount`、`POST /assets/:id/unmount`（manager+），资产详情返回 parent + children 外设树。防循环 + 跨 org 禁止。
- **G9 部门级行级权限**：env `DATA_SCOPE_DEPARTMENT`(默认 off)。开启后 super_admin 全局可见、manager 仅见本部门及子孙（ltree 子树）；关闭时行为同 v0.2.0（org 级）。
- **迁移**：011(ldap + user_import)、012(notify + approvals)、013(asset_parent + data_scope)、**014(ad_group_mappings + users.data_scope + users.manual_override)**，启动自动执行，多实例 EXCLUSIVE 锁。

### 企业化适配（Wave 3 — AD 域控增强）

> 交付 T0–T10，解决企业化 AD 部署中暴露的 truncation、角色粒度不足、权限过度放大、误覆盖 admin 调整四大问题。提交 `TODO-commit-hash`。所有新功能默认关闭，向后兼容 v0.2.0 行为。

- **T0–T2 组同步 + ControlPaging**（`internal/auth/ldap/`）：LDAP 搜索增加 `ControlPaging`，防止 AD 默认 MaxPageSize=1000 导致的**无声截断**。换回原版 `go-ldap/v3`，仅开启翻页控制扩展 (`ControlTypePaging`)。新增 env：`LDAP_PAGE_SIZE`(默认 1000)、`LDAP_SYNC_RECURSIVE`(默认 true，递归搜索 OU 嵌套组)、`LDAP_GROUP_ATTR`(默认 `memberOf`，组属性字段)。修复 `base_dn` 拼写检查在合理值上的误报。
- **T3–T5 组到角色映射**：新增 `ad_group_mappings` 表，将 AD 安全组 DN 映射到系统角色（`super_admin`/`admin`/`manager`/`viewer`）与数据范围（`inherit`/`self`）。系统同步时枚举所有启用的映射组，查询组成员并按**最高角色**确定用户角色。API：`GET/POST /admin/ldap/group-mappings`、`DELETE /admin/ldap/group-mappings/:id`（admin+）。Admin UI 新增「LDAP 组映射」管理卡片。
- **T6–T8 个人数据范围（ScopeSelf）**：`users` 表新增 `data_scope` 列（`inherit` 默认 / `self` 仅见自己）。当值为 `self` 时，用户只能查看分配给自己（`assignments.assigned_to_user_id = current_user`）的资产，即使拥有 manager/admin 角色也无法越权查看同部门/同 org 其他人的资产。范围优先级：`users.data_scope = 'self'` 时忽略其他范围策略；`'inherit'` 时回退到组映射或系统默认。
- **T9–T10 LDAP 状态页 + 用户列表增强**：Admin UI 新增「LDAP 状态」页面，展示同步状态、组映射列表 CRUD、最后一次同步时间/结果统计。用户管理页面增强：列表新增 `source` 列（Local / LDAP 徽标）、`display_name` 列展示 AD displayName；详情/行操作增加「Link AD Account」/「Unlink AD Account」按钮。
- **manual_override 防覆盖**：`users` 表新增 `manual_override` 布尔列（默认 false）。AD 同步前由 admin 对特定用户开启此标记，随后**任何 AD 同步都不会覆盖该用户的 role、status、data_scope 字段**（display_name、email、department 仍正常刷新）。适用于 admin 手动调整了某用户权限/范围后需要保护不被 AD 批量覆盖的场景。
- **迁移**：014_ad_enterprise，新增 `ad_group_mappings` 表（唯一 DN 约束 + `sync_enabled` 索引），`users` 表新增 `data_scope` 列（默认 `'inherit'`）、`manual_override` 列（默认 `false`）+ 索引。全部 `IF NOT EXISTS`，幂等安全。

### 体验与门控
- 前端 UI 改为**亮色主题**（Linear 风格），并修复 `index.css` 未被 `main.tsx` 引入导致生产构建样式全丢的根因。
- 补齐 **viewer 角色对资产操作按钮的门控**（此前仅门控 admin+）；新增 **Webhooks 管理页**（此前仅 API）。

### 业务修复
- 资产创建重号 (SQLSTATE 23505)：`NextAssetTag`/`NextBatchTags` 改用 `doc_sequences` 原子取号 + 已存在最大编号回填，兼容软删除遗留编号；支持自定义资产编号。
- 盘点报告导出 500：`COALESCE` 修复 NULL 字符串扫描。
- **用户软删除**（保留记录）：`users` 表加 `deleted_at`，`DELETE /admin/users/:id` 仅置位，行保留以维系审计/领用历史；禁止删自己。
- **报废门控**：移除生命周期转换直达 `retirement` 的无弹窗旁路，报废统一走确认弹窗；有活跃领用（领用/借用中）时隐藏报废按钮并提示先归还。
- **借用归还**：归还按钮对 `borrowed` 状态也显示，借用中资产可从详情页归还。
- **移除「采购中」(procurement) 生命周期状态**：入库即进入 `deployment`，迁移 010 回填历史数据并收紧 CHECK。
- **领用/借用列表显示名称**：`ListAssignments` JOIN `assets`+`users` 返回 `asset_name`/`asset_tag`/`assigned_to_name`，不再回退 UUID。

### 部署/可靠性
- 修复 **SPA 客户端路由直访/刷新返回 404**：`web.Handler()` 此前用裸 `http.FileServer` 无回退，直接访问 `/login`、`/assets/:id` 等返回 `404 page not found`；改为文件不存在时回退 `index.html`，`/api/*` 404 不受影响。
- 新增 **部署手册** `docs/DEPLOYMENT.md`。

## v0.2.0 (2026-07-19) — 核心业务闭环完成

### Phase A: 止血
- 修复种子用户 SQL bug（`'$4'`→`$4`，生产领用必失败）
- 修复生产模式 next-tag nil panic
- 修复前后端三处契约断裂（lifecycle 路由 404、If-Match 引号 428、refresh 端点缺失）
- 新增 `system_settings` + `doc_sequences` 表（002 迁移）
- 删除 2,729 行死代码（cache/ingest/agent/collection-agent/nginx/grafana）
- Dockerfile 三阶段重写 + WebFS embed 单二进制
- GitHub Actions CI（go vet/build/test + tsc/vite + docker build）

### Phase B: 数据层地基
- DBTX 接口 + 全 repo 方法签名改造（Pool 与 Tx 天然满足）
- Service 层事务包裹（pgx.Begin/Commit/Rollback）
- IDOR 加固（全单行操作补 org_id 过滤）
- 审计链 Recorder（SHA-256 链式哈希，事务内写入 audit_log）
- 自研迁移执行器（embed + EXCLUSIVE 锁 + 存量 baseline）
- 003 迁移（DROP 9 张未用表）+ 004 迁移（locations 表）
- org/location 落 PG 替代内存 Store
- EventBus 接线（服务层 commit 后 Publish）
- server.go 拆分（360 行 routes + routes_demo）

### Phase C: 认证 + RBAC
- bcrypt 登录（users 表校验）+ x/time/rate 限速（5 次/15min 锁定）
- Refresh Token 轮换 + 复用检测（全族吊销）
- JWT_ED25519_SEED 密钥持久化
- RequireRole 中间件（viewer/manager/admin/super_admin）
- /admin/users CRUD + 重置密码
- 005 迁移（admin bcrypt hash、refresh_tokens.revoked_at、role 去 agent）

### Phase D: 前端基建
- API 模块层（6 文件）+ UI 组件库（12 组件，Linear 暗色设计系统）
- TanStack Query + react-hook-form + sonner + recharts
- Assets.tsx 拆分（856→150 行，5 组件）
- 导航重组（7 项）+ 占位页面
- 管理页（用户管理 + 系统设置）
- Dashboard recharts 图表

### Phase E: 入库 + 领借还闭环
- 006 迁移（采购字段 + borrowed 状态 + assignment_type/due_date）
- POST /assets/batch（原子取号批量创建）
- POST /assets/:id/borrow（临时借用 + 应还日期）
- GET /assignments（逾期过滤 + 游标分页）
- AssignDialog borrow 模式 + AssignmentsPage（4 tabs）

### Phase F: 维修工单 + 报废
- 007 迁移（maintenance_orders + 唯一活跃索引）
- 工单状态流转（open→in_progress→completed/canceled，恢复 prev_status）
- POST /assets/:id/retire（校验无活跃领用/工单）
- MaintenancePage（列表 + start/complete/cancel）

### Phase G: 盘点
- 008 迁移（stocktake_plans + items）
- 盘点计划 + Start 快照 + 逐项核对 + 盘盈登记 + Complete（apply_moves）
- 报告 JSON + CSV 导出
- StocktakesPage + StocktakeDetail（进度条 + 行内 Select 即时保存）

### Phase H: 折旧 + 报表 + 导入导出
- 直线法折旧（SQL 内联实时计算）
- 4 报表 API（summary/depreciation/maintenance-cost/assignments-due）
- CSV 导出（UTF-8 BOM，12 列）
- CSV 导入（模板下载 + dry_run 预检 + all-or-nothing 事务）
- ReportsPage（KPI + 折旧明细 + 导出）+ ImportWizard（3 步）
- Dashboard 增强（7 KPI + 趋势）

### Phase I: Webhook + 文档 + CI
- 009 迁移（webhook_endpoints + webhook_deliveries）
- /admin/webhooks CRUD + 投递记录查询
- WebhookDispatcher（订阅 EventBus → SSRF 防护引擎异步投递）
- README 重写（功能矩阵 + 技术栈 + 角色表 + API 索引）
- 真实架构文档（docs/architecture.md）
- 运维手册重写（docs/runbook.md）
- API 参考（docs/api.md）
- CHANGELOG（本文档）
- 旧文档归档（docs/archive/）
- CI integration job（PostgreSQL service + smoke test）
- 删除 demo/ Python 概念验证
