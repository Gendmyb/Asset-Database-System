# CHANGELOG

## 未发布 (2026-07-19 之后)

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
