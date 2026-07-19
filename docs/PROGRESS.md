# 实施进度表（PROGRESS）

> 规则：每个关键修复完成即更新本表并独立 commit（小步提交，中断可恢复）。
> 状态：⬜ 待办 · 🔵 进行中 · ✅ 已完成 · ✔️ 验证通过（CI 绿/验收清单过）
> 本表只由主会话（总控）更新，子代理不直接改，避免并发写冲突。
> 完整规划见 [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)。

## 阶段总览

| 阶段 | 内容 | 状态 | 完成时间 |
|---|---|---|---|
| 0 | 环境准备 + 规划落库 | ✅ | 2026-07-19 |
| A | 止血：契约修复 + 可构建可部署 + CI | ✅ | 2026-07-19 |
| B | 数据层地基：goose 迁移/事务/审计/租户隔离 | ✅ | 2026-07-19 |
| C | 真实认证与 RBAC | ✅ | 2026-07-19 |
| D | 前端基建重构 + 管理页 | ✅ | 2026-07-19 |
| E | 入库增强 + 领用/借用/归还闭环 | ✅ | 2026-07-19 |
| F | 维修/保养工单 + 报废 | ✅ | 2026-07-19 |
| G | 盘点 | ✅ | 2026-07-19 |
| H | 折旧、报表、导入导出 | ✅ | 2026-07-19 |
| I | Webhook 接线、文档校准、CI 完整化 | ✅ | 2026-07-19 |
| J | 终验双门禁（PM 代理 + 逻辑审计代理） | ✅ | 2026-07-19 |

## 明细

| 阶段 | 任务 | 状态 | 提交 hash | 验证方式与结果 | 时间 |
|---|---|---|---|---|---|
| 0 | 规划写入 docs/IMPLEMENTATION_PLAN.md + 本进度表落库 | ✅ | 647089c | git commit 成功 | 2026-07-19 |
| 0 | 用户执行 docker 组授权（usermod + 重登录） | ⚠️ | | docker ps 仍 permission denied（需重新登录/newgrp），不影响 Phase A 代码修改 | 2026-07-19 |
| 0 | 安装 Go 1.25.0 → ~/.local/go | ✅ | | go version go1.25.0 linux/amd64 | 2026-07-19 |
| 0 | 安装 Node 22.19.0 LTS → ~/.local/node | ✅ | | node v22.19.0 + npm 11.16.0 | 2026-07-19 |
| 0 | 首次真实验证：`go build ./... && go test ./...` | ✅ | | 编译通过，全部 40 测试 PASS（lock/webhook 包 ok） | 2026-07-19 |
| 0 | 首次真实验证：`npm ci && npm run build` | ✅ | | tsc 0 errors + vite build ✓（107 modules, 256KB gzip 82KB） | 2026-07-19 |
| A | 前端：lifecycle 路由 404（PUT→POST /transition） | ✅ | | api.post `/transition` body `{to}` | 2026-07-19 |
| A | 前端：If-Match 引号修复（"1" → 1） | ✅ | | `String(asset.version)` 无引号 | 2026-07-19 |
| A | 前端：getApiError 统一错误提取 + 全部 catch 接入 | ✅ | | 兼容 {error:str}/{error:{message}} | 2026-07-19 |
| A | 前端：client.ts 401 直接登出（标注 TODO Phase C） | ✅ | | 注释旧 refresh 队列代码 | 2026-07-19 |
| A | 前端：Login refresh_token 兜底 'placeholder-phase-c' | ✅ | | 防 localStorage 存 undefined | 2026-07-19 |
| A | 前端：删 Agents 页+路由+导航+类型+Dashboard KPI | ✅ | | Agents.tsx 已删，NotFound 页新增 | 2026-07-19 |
| A | 前端：新增 404 兜底路由 NotFound 页 | ✅ | | `path="*"` catch-all | 2026-07-19 |
| A | 前端：模式徽标真实化（fetch /healthz 读 mode） | ✅ | | demo 时才显示绿点"演示模式" | 2026-07-19 |
| A | 前端：tsc --noEmit 通过 | ✅ | | 0 type errors | 2026-07-19 |
| A | 后端：种子 SQL bug `'$4'`→`$4` + next-tag nil check + If-Match Trim + DEMO 分页/transfer | ✅ | | 全部修完，go build 通过 | 2026-07-19 |
| A | 后端：002 迁移（system_settings + doc_sequences） | ✅ | | 新建 | 2026-07-19 |
| A | 后端：死代码删除（cache/ingest/agent/collection-agent/nginx/grafana） | ✅ | | 2,729 行死代码删除 | 2026-07-19 |
| A | 部署：Dockerfile 三阶段（node→go→alpine）+ webfs embed | ✅ | | 仓库根 Dockerfile，context=./ | 2026-07-19 |
| A | 部署：docker-compose 精简（仅 pg+app）+ .github/ci.yml | ✅ | | 删 redis/grafana，3-job CI | 2026-07-19 |
| B | Step1: 自研 migration runner（embed+EXCLUSIVE锁+schema_migrations） | ✅ | | 99 行，避免 goose 依赖链 | 2026-07-19 |
| B | Step2: 003 DROP 9 未用表 + 004 locations 表 | ✅ | | 新建迁移文件 | 2026-07-19 |
| B | Step3: DBTX 接口 + 全 repo 方法签名改造 | ✅ | | asset/assignment/user/dashboard/settings repo | 2026-07-19 |
| B | Step4: Service 层（Begin/Commit/Rollback 事务包裹) | ✅ | | asset_service(296行)+assignment_service(147行) | 2026-07-19 |
| B | Step5: IDOR 修复（全单行操作补 AND org_id=$n） | ✅ | | GetByID/Update/Delete/ForUpdate 等 | 2026-07-19 |
| B | Step6: 审计 Recorder（链式哈希 SHA256） | ✅ | | 事务内 INSERT audit_log | 2026-07-19 |
| B | Step7: org_repo + location_repo 落 PG | ✅ | | 替换内存 Store，/locations 路由注册 | 2026-07-19 |
| B | Step8: Event bus 接线（DefaultBus + log consumer） | ✅ | | service commit 后 Publish | 2026-07-19 |
| B | Step9: server.go 拆分（routes/routes_demo）+ config 清理 | ✅ | | 800+→360行；删 Redis/Vault 配置 | 2026-07-19 |
| B | go build + go test + go vet 全部通过 | ✅ | | 0 errors 0 regressions | 2026-07-19 |
| C | 005_auth.sql (bcrypt admin+revoked_at+role去agent) | ✅ | | 迁移自动执行 | 2026-07-19 |
| C | JWT_ED25519_SEED 密钥持久化 | ✅ | | crypto/jwt.go NewKeyManager(seedHex) | 2026-07-19 |
| C | auth_service: Login(bcrypt+限速5次15min)+Refresh(轮换+复用检测)+Logout(全族吊销) | ✅ | | 新建文件 | 2026-07-19 |
| C | 路由: /auth/login重写, /auth/refresh, /auth/logout, /me, /me/password | ✅ | | server.go 生产分支 | 2026-07-19 |
| C | middleware/rbac.go RequireRole(min) + routes挂载 | ✅ | | viewer/manager/admin/super_admin四层 | 2026-07-19 |
| C | /admin/users CRUD + reset-password (admin+) | ✅ | | 用户管理4端点 | 2026-07-19 |
| C | 前端: client.ts 启用 refresh 队列, Login admin/admin123 | ✅ | | tsc 通过 | 2026-07-19 |
| C | go build/test/vet + npm build 通过 | ✅ | | 全部通过 | 2026-07-19 |
| D | Step1: API 模块层（auth/assets/assignments/users/settings/lookup） | ✅ | | 6 文件，类型化 Promise 函数 | 2026-07-19 |
| D | Step2: UI 组件库（Button/Input/Select/Modal/Drawer/ConfirmDialog/Badge/EmptyState/Spinner/DataTable/FormField/Toaster） | ✅ | | 12 文件，沿用 Linear 暗色设计系统 | 2026-07-19 |
| D | Step3: QueryClientProvider + sonner Toaster 注入 App.tsx | ✅ | | staleTime:30s retry:1 | 2026-07-19 |
| D | Step4: Assets.tsx 拆分（856行→150行，5个组件） | ✅ | | AssetTable/Filters/DetailPanel/CreateAssetModal/AssignDialog | 2026-07-19 |
| D | Step5: 导航重组（7项+路由占位） | ✅ | | 含5个占位页（E/F/G/H阶段填充） | 2026-07-19 |
| D | Step6: 管理页 admin/Users + admin/Settings | ✅ | | 用户CRUD+重置密码+系统设置+位置 | 2026-07-19 |
| D | Step7: Dashboard recharts（PieChart+Barchart） | ✅ | | 状态 donut + 类型 bar | 2026-07-19 |
| D | npm run build（tsc+vite）通过 | ✅ | | 156 modules, 315KB (99KB gzip)，+recharts 738KB chunk | 2026-07-19 |
| E | 后端: 006迁移(采购字段+borrowed+assignment_type/due_date+逾期索引) | ✅ | | 迁移自动执行 | 2026-07-19 |
| E | 后端: CreateAssetBatch(原子取号) + BorrowAsset(事务+审计) + Release unify + Transfer限borrowed | ✅ | | service层全部事务包裹 | 2026-07-19 |
| E | 后端: GET /assignments (overdue+cursor) + GET /users/:id/assignments | ✅ | | viewer+ 路由 | 2026-07-19 |
| E | 前端: CreateAssetModal批量+采购字段, AssignDialog borrow模式+due_date, AssetDetailPanel借用按钮 | ✅ | | tsc 0 errors | 2026-07-19 |
| E | 前端: AssignmentsPage全改写 (4tabs+逾期红标+行内归还), Badge assignment类型 | ✅ | | tsc 0 errors | 2026-07-19 |
| F | 后端: 007迁移(maintenance_orders+唯一活跃索引), service+handler+8路由 | ✅ | | go build/test/vet 全通过 | 2026-07-19 |
| F | 后端: RetireAsset(校验无活跃领用/工单→终态) | ✅ | | admin+ 路由 | 2026-07-19 |
| F | 前端: MaintenancePage全页(列表+新建+start/complete/cancel), AssetDetailPanel报修+报废+Badge扩展 | ✅ | | tsc 0 errors, vite build 成功 | 2026-07-19 |
| G | 后端: 008迁移(stocktake_plans+items), repo+service+handler+9路由 | ✅ | | go build/test/vet 全通过 | 2026-07-19 |
| G | 后端: start快照批量生成items + complete(apply_moves批量更新位置+审计) + CSV报告 | ✅ | | STK- 取号 | 2026-07-19 |
| G | 前端: StocktakesPage完整页面 + StocktakeDetail(进度条+行内Select+盘盈Modal+完成盘点) | ✅ | | tsc 0 errors, vite build 成功 | 2026-07-19 |
| H | 后端: depreciation_service(SQL直线法) + report_service(summary+cost+due) + export_service(CSV+BOM) + import_service(template+dry_run+all-or-nothing) | ✅ | | 4 service 新文件 | 2026-07-19 |
| H | 后端: 10新路由 (reports 4个 + export 3个 + import 3个) | ✅ | | routes.go 注册 | 2026-07-19 |
| H | 前端: reports API + ReportsPage(KPI+折旧明细+导出) + ImportWizard(3步骤) + Dashboard增强(7KPI) | ✅ | | tsc 0 errors, vite build 成功 | 2026-07-19 |
| I | 后端: 009迁移(webhooks) + webhook_repo+handler+dispatcher(EventBus订阅→异步投递+SSRF) + 6 管理路由 | ✅ | | go build/vet 全通过 | 2026-07-19 |
| I | 文档: README重写 + 真实architecture.md + CHANGELOG + runbook重写 + api.md + 旧文档归档 + DOC_VS_CODE_AUDIT标注 | ✅ | | 文档诚实准确 | 2026-07-19 |
| I | CI: integration job (postgres service + smoke test) | ✅ | | ci.yml 新增 | 2026-07-19 |
| I | 清理: 删除 demo/ Python 概念验证目录 | ✅ | | -3610 行 | 2026-07-19 |
| D | Step1: API 模块层（auth/assets/assignments/users/settings/lookup） | ✅ | | 6 文件，类型化 Promise 函数 | 2026-07-19 |
| D | Step2: UI 组件库（Button/Input/Select/Modal/Drawer/ConfirmDialog/Badge/EmptyState/Spinner/DataTable/FormField/Toaster） | ✅ | | 12 文件，沿用 Linear 暗色设计系统 | 2026-07-19 |
| D | Step3: QueryClientProvider + sonner Toaster 注入 App.tsx | ✅ | | staleTime:30s retry:1 | 2026-07-19 |
| D | Step4: Assets.tsx 拆分（856行→150行，5个组件） | ✅ | | AssetTable/Filters/DetailPanel/CreateAssetModal/AssignDialog | 2026-07-19 |
| D | Step5: 导航重组（7项+路由占位） | ✅ | | 含5个占位页（E/F/G/H阶段填充） | 2026-07-19 |
| D | Step6: 管理页 admin/Users + admin/Settings | ✅ | | 用户CRUD+重置密码+系统设置+位置 | 2026-07-19 |
| D | Step7: Dashboard recharts（PieChart+Barchart） | ✅ | | 状态 donut + 类型 bar | 2026-07-19 |
| D | npm run build（tsc+vite）通过 | ✅ | | 156 modules, 315KB (99KB gzip)，+recharts 738KB chunk | 2026-07-19 |
| F | 后端: 007迁移(maintenance_orders)+service+handler+8路由 | ✅ | | go build/test/vet 全通过 | 2026-07-19 |
| F | 前端: MaintenancePage+AssetDetailPanel报修报废+Badge扩展+ConfirmDialog children | ✅ | | tsc 0 errors, vite build 成功 | 2026-07-19 |

## 部署后修复 (K)

| 任务 | 状态 | 说明 | 时间 |
|---|---|---|---|
| 后端: audit.QueryHistory 变量名 bug (tx.Query → q.Query) | ✅ | 导致编译失败，recorder.go:104 | 2026-07-19 |
| 后端: 自研 migration runner 替换 goose CLI | ✅ | embed+EXCLUSIVE锁+schema_migrations | 2026-07-19 |
| 后端: SPA fallback 重定向循环修复 | ✅ | http.FS → http.FileServer + fs.Sub | 2026-07-19 |
| 部署: 本地 deploy script (setup-and-run.sh) | ✅ | PostgreSQL + Go 二进制一键启动 | 2026-07-19 |
| 前端: 登录密码错误 404 闪白页修复 | ✅ | client.ts login 401 不再触发 clearAuth() | 2026-07-19 |
| 前端: 资产新建表单透明度过低 | ✅ | rgba(255,255,255,0.02) → 0.06 | 2026-07-19 |
| 前端: 资产新建窗口溢出 → 可滚动 | ✅ | Modal max-height:90vh + overflow-y:auto | 2026-07-19 |
| 构建: 前端 tsc+vite + 后端 go build 验证 | ✅ | 756 modules + 27MB 静态二进制 | 2026-07-19 |
| Git: 全部修改提交推送 GitHub | ✅ | 主分支，含 deploy script | 2026-07-19 |
