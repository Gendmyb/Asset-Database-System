# 真实架构文档 (对应代码 v0.2.0)

> **历史设计稿（Vault/K8s/Redis/mTLS 等）已归档至 archive/architecture-design-v2.md——该文档描述的是 9 人 244 人天企业蓝图，与当前代码严重不符。本文档描述实际实现。**

## 分层架构

```
cmd/api-server/main.go           — 入口：配置→JWT→迁移→Webhook Dispatcher→HTTP server
  └─ internal/api/server.go       — Gin 路由 + DEMO/生产模式分支
       ├─ routes.go               — 生产模式路由注册（RBAC 挂载）
       ├─ routes_demo.go          — DEMO 路由（冻结）
       ├─ middleware/             — AuthRequired + OrgScope + RequireRole
       ├─ handler/                — HTTP handler（解析请求→调 service）
       │    ├─ asset_v2.go        — 资产 CRUD + lifecycle transition + batch
       │    ├─ assignment.go      — assign/release/transfer/borrow
       │    ├─ maintenance.go     — 维修工单
       │    ├─ stocktake.go       — 盘点
       │    ├─ report.go          — 报表/导出
       │    ├─ import.go          — CSV 导入
       │    ├─ webhook.go         — Webhook 管理
       │    ├─ organization.go    — 组织
       │    └─ location.go        — 位置
       └─ service/               — 业务逻辑（事务包裹 + 审计 + 事件发布）
            ├─ asset_service.go   — Create/Update/Delete/Transition/Batch
            ├─ assignment_service.go — Assign/Release/Transfer/Borrow
            ├─ auth_service.go    — Login/Refresh/Logout
            ├─ maintenance_service.go — CreateOrder/Start/Complete/Cancel/Retire
            ├─ stocktake_service.go   — CreatePlan/Start/UpdateItem/Complete/Report
            ├─ depreciation_service.go — 直线法折旧（SQL 内联）
            ├─ report_service.go  — Summary/Cost/Due
            ├─ export_service.go  — CSV 导出（UTF-8 BOM）
            ├─ import_service.go  — CSV 导入（dry_run + all-or-nothing）
            ├─ webhook_dispatcher.go — 订阅 EventBus → 异步投递
            └─ ...
  └─ repository/                 — 数据访问（DBTX 接口）
       ├─ db.go                  — DBTX 接口（Pool 与 Tx 都满足）
       ├─ asset_repo.go          — assets CRUD（org_id IDOR 加固 + 乐观锁重试）
       ├─ assignment_repo.go     — assignments（部分唯一索引防重复）
       ├─ maintenance_repo.go    — maintenance_orders
       ├─ stocktake_repo.go      — stocktake_plans/items
       ├─ webhook_repo.go        — webhook_endpoints/deliveries
       ├─ user_repo.go           — users
       ├─ org_repo.go            — organizations (ltree)
       ├─ location_repo.go       — locations
       ├─ settings_repo.go       — system_settings + doc_sequences
       └─ dashboard_repo.go      — PG 聚合（status/type/lifecycle）
  └─ internal/
       ├─ crypto/jwt.go          — Ed25519 JWT（密钥从 JWT_ED25519_SEED env）
       ├─ event/bus.go           — 内存事件总线（DefaultBus 全局单例）
       ├─ webhook/               — Webhook 引擎（HMAC-SHA256 + SSRF 防护 + 重试）
       ├─ audit/recorder.go      — 审计链写入（SHA-256 链式哈希）
       ├─ lock/lock.go           — 乐观/悲观/advisory 三层锁
       ├─ domain/lifecycle.go    — 生命周期状态机（fuzz 测试）
       ├─ db/migrate.go          — 自研迁移执行器（embed + EXCLUSIVE 锁）
       └─ config/config.go       — 环境变量配置
```

## 数据模型（实际使用表）

迁移文件：`migrations/001-009*.sql`，由 app 启动时自动执行。

| 表 | 用途 |
|---|---|
| organizations | 组织（ltree 物化路径） |
| users | 用户（bcrypt 密码、角色、状态） |
| asset_types | 资产类型 |
| assets | 核心资产表（含采购/折旧/报废字段） |
| assignments | 领用/借用记录（type: permanent/temporary，部分唯一索引防重复） |
| audit_log | 不可变审计链（BEFORE INSERT trigger 计算链式哈希） |
| locations | 位置树（盘点按位置圈范围） |
| system_settings | KV 设置（编号前缀、组织名称等） |
| doc_sequences | 单据序列（scope: asset/maintenance/stocktake，upsert-returning 原子取号） |
| maintenance_orders | 维修/保养工单（唯一活跃索引 per asset） |
| stocktake_plans | 盘点计划 |
| stocktake_items | 盘点明细（每 plan 快照所有资产） |
| webhook_endpoints | Webhook 订阅 |
| webhook_deliveries | 投递记录 |
| schema_migrations | 迁移版本记录（自研 runner 维护） |
| refresh_tokens | Refresh Token（轮换 + 复用检测） |

## 事务模式

全部写操作在 service 层 `pgx.BeginFunc` 包裹：
1. repo 方法签名接受 `DBTX` 接口（*pgxpool.Pool 和 pgx.Tx 都满足）
2. service 层 `tx, _ := pool.Begin(ctx); defer tx.Rollback(ctx)` → repo 方法传 tx → commit
3. 事务内：check-then-act + 审计写入 + 事件发布（commit 成功后 Publish）

## 认证流程

1. Login：bcrypt 验证密码 → 查 users 表 → 成功则 IssueAccessToken(15min Ed25519 JWT) + 生成 refresh token(SHA-256 存表, family_id UUID)
2. Refresh：SHA-256 refresh → 查表 → 若已 revoked 则全 family 吊销(复用检测) → 否则旧行 revoked，新行同 family → 返回新 token 对
3. Logout：吊销整 family
4. 密钥：`JWT_ED25519_SEED` env（hex 32B）派生，缺省随机生成+警告

## RBAC

`RequireRole(min)` 中间件挂载路由组：
- viewer(0): 读接口
- manager(1): 写资产/领用/借用/盘点执行/导入
- admin(2): 用户管理/设置/位置/盘点计划/报废/导出/Webhook
- super_admin(3): 预留

## IDOR 防护

- `OrgScope` 中间件从 JWT claims 提取 org_id 注入 context
- 所有 repo 单行操作 WHERE 子句含 `org_id=$n`
- 跨 org 查询返回 404（不泄露其他 org 数据）

## 部署架构

```
docker compose up -d
  ├─ postgres:16 (端口 5432)
  └─ app (端口 8080)
       ├─ Go 二进制 embed web/dist (React SPA)
       ├─ NoRoute → index.html (SPA 路由回落)
       ├─ 启动时自动执行迁移 (001-009*.sql)
       └─ /healthz /readyz 健康检查
```
