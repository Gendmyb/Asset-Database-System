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
| **企业化（Wave 1+2，默认关闭）** | |
| AD/LDAP (G1) | 本地优先登录 + LDAP bind 兜底 + 定时/手动账号同步，admin 永不被覆盖 |
| 用户导入 (G2) | CSV 批量导入用户（dry-run 预检 + 模板下载） |
| 扫码盘点 (G3) | 资产二维码 PNG + 移动端响应式盘点扫码录入 |
| 到期提醒 (G4) | 定时扫描保修到期 / 领用逾期并发布事件，可触发 LDAP 同步 |
| Excel 导出 (G5) | 资产清单 `.xlsx`；盘点 / 折旧报表支持 `?format=xlsx` |
| 通知渠道 (G6) | SMTP 邮件 + 钉钉 + 企微 + 飞书，机器人 webhook 强制 HTTPS + SSRF 防护 |
| 审批流 (G7) | 领用 / 报废 / 维修可配置审批门（系统设置开关，默认关闭向后兼容） |
| 资产关系 (G8) | 外设挂载（parent/children 树），防循环 + 跨 org 禁止 |
| 部门行级权限 (G9) | `DATA_SCOPE_DEPARTMENT` 开启后 manager 仅见本部门及子孙（ltree 子树） |

## 快速开始

**Docker Compose（推荐）**：
```bash
git clone git@github.com:Gendmyb/Asset-Database-System.git
cd Asset-Database-System
docker compose up -d --build
# 访问 http://localhost:8080
```
默认账号：**admin** / **admin123**（首次登录后请立即改密）

**源码本地部署**（一键脚本，含 PostgreSQL 初始化）：
```bash
bash setup-and-run.sh
```

**DEMO 模式**（免 PostgreSQL，仅演示基础功能）：
```bash
cd assetserver && DEMO=true go run ./cmd/api-server
# 浏览器打开 http://localhost:8080
```

**环境变量示例**（生产请用强密码；数据库连接使用 `DB_*` 单项变量，`DATABASE_URL` 不被识别）：
```bash
export DB_HOST=localhost DB_PORT=5432 DB_NAME=assetdb DB_USER=app_user
export DATABASE_PASSWORD='强密码'
export DB_SSLMODE=require                 # 可选，生产建议 require
export JWT_ED25519_SEED=$(openssl rand -hex 32)
# 企业化（可选，默认关闭）
export LDAP_HOST=ldap.corp.local LDAP_BASE_DN=OU=Staff,DC=corp,DC=local LDAP_BIND_DN=CN=svc-ldap,DC=corp,DC=local LDAP_BIND_PASSWORD='***'
export NOTIFY_ENABLE=true SMTP_HOST=smtp.corp.local SMTP_FROM=assets@corp.local SMTP_USER=assets SMTP_PASSWORD='***'
export SCHEDULER_INTERVAL=1h SCHEDULER_WARRANTY_DAYS=30
export EXTERNAL_URL=https://assets.example.com
```

> 完整部署流程（三种方式、环境变量、反向代理 HTTPS、备份恢复、升级与排障、企业化功能配置）见 **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)**。

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

部署手册（推荐先读）：[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)
备份恢复、升级、管理员找回等速查：[docs/runbook.md](docs/runbook.md)。

## 项目状态

v0.2.0 — 核心业务闭环完成；此后持续迭代（亮色主题、用户软删除、报废门控、移除采购中状态、领用列表显示名称、SPA 路由 404 修复等，详见 [docs/CHANGELOG.md](docs/CHANGELOG.md)）。

**企业化适配 Wave 1+2**（未发布，供上线测试）：G1 AD/LDAP 同步+SSO、G2 用户批量导入、G3 扫码+移动盘点、G4 到期提醒、G5 Excel 导出、G6 通知渠道、G7 审批流、G8 资产关系/外设挂载、G9 部门级行级权限已交付，默认全部关闭，向后兼容 v0.2.0。Wave 3（G11 PDF 报表 / G12 PWA）待启动，G10 合同管理已跳过。

实施规划见 [docs/IMPLEMENTATION_PLAN.md](docs/IMPLEMENTATION_PLAN.md)，进度跟踪见 [docs/PROGRESS.md](docs/PROGRESS.md)。
