# IT 资产管理系统

中小企业 IT 资产全生命周期管理平台。Go + React，PostgreSQL 单库 + Docker Compose 部署。

## 功能

| 模块 | 功能 |
|---|---|
| 资产台账 | 编号自动生成、CRUD、搜索（中文/英文分流）、游标分页、If-Match 乐观锁 |
| 入库登记 | 单条入库 + 批量入库（含采购价格/日期/供应商/保修/折旧参数） |
| 领用管理 | 长期领用分配、临时借用（应还日期 + 逾期检测）、归还、转移 |
| 维修工单 | 报修/保养工单，状态流转（开单→进行中→完成/取消），恢复资产原状态 |
| 盘点 | 盘点计划 → 快照 → 逐项核对（found/missing/moved/surplus）→ 盘盈登记 → 报告 CSV |
| 折旧 | 直线法实时计算（原值−残值÷月数），明细 + 汇总报表 |
| 报废 | 终态锁定（校验无活跃领用/工单），不可逆 |
| 报表 | 资产汇总/折旧明细/维修成本/应收逾期，Dashboard 图表 |
| 导入导出 | CSV 模板下载 + dry-run 预检 + 逐行错误报告 + 事务导入；CSV 导出（UTF-8 BOM） |
| Webhook | 事件订阅（asset.* 等）+ HMAC 签名 + SSRF 防护 + 自动重试 + 投递记录 |
| 认证 | JWT Ed25519（15min）+ Refresh Token 轮换 + 复用检测（全族吊销） |
| 权限 | 4 角色 RBAC：viewer/manager/admin/super_admin，org 级 IDOR 防护 |
| 审计 | 不可变审计链（SHA-256 链式哈希），所有写操作自动记录 |

## 快速开始

**Docker Compose（推荐）**：
```bash
git clone git@github.com:Gendmyb/Asset-Database-System.git
cd Asset-Database-System
docker compose up -d --build
# 访问 http://localhost:8080
```
默认账号：**admin** / **admin123**

**DEMO 模式**（免 PostgreSQL，仅演示基础功能）：
```bash
cd assetserver && DEMO=true go run ./cmd/api-server
# 浏览器打开 http://localhost:8080
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go + Gin + pgx/v5（直接 SQL）+ Ed25519 JWT |
| 前端 | React 18 + TypeScript + Vite + TanStack Query + recharts |
| 数据库 | PostgreSQL（单库，schema: assets） |
| 部署 | Docker Compose（postgres + app 两个服务，单二进制 embed 前端） |
| CI | GitHub Actions（go vet/build/test + tsc/vite + docker build + integration） |

## 角色权限

| 角色 | 说明 | 权限 |
|---|---|---|
| viewer | 只读 | 查看资产/仪表盘/报表/盘点报告 |
| manager | 资产管理员 | viewer + 创建编辑资产、领用/借用/归还/转移、执行盘点、导入 |
| admin | 管理员 | manager + 用户管理、系统设置、位置管理、盘点计划、报废、导出、Webhook |
| super_admin | 超级管理员 | admin + 所有管理操作 |

## API 概览

| 域 | 端点 |
|---|---|
| 认证 | POST /auth/login, /auth/refresh, /auth/logout, GET /me, PUT /me/password |
| 资产 | GET/POST /assets, POST /assets/batch, GET/PUT/DELETE /assets/:id, POST /assets/:id/transition/assign/release/transfer/borrow/retire |
| 领用 | GET /assignments, GET /users/:id/assignments, GET /assets/:id/assignments |
| 维修 | GET/POST /maintenance-orders, GET /maintenance-orders/:id, POST .../:id/start/complete/cancel |
| 盘点 | GET/POST /stocktakes, GET /stocktakes/:id, POST .../:id/start/complete, PUT .../items/:id, POST .../items, GET .../report |
| 报表 | GET /reports/summary/depreciation/maintenance-cost/assignments-due |
| 导入导出 | GET /assets/export, GET /assets/import/template, POST /assets/import |
| Webhook | GET/POST /admin/webhooks, GET/PUT/DELETE /admin/webhooks/:id, GET .../deliveries |

完整 API 文档见 [docs/api.md](docs/api.md)。

## 运维

备份恢复、升级、管理员找回等见 [docs/runbook.md](docs/runbook.md)。

## 项目状态

v0.2.0 — 核心业务闭环完成，生产可用性待验证。
实施规划见 [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md)。
