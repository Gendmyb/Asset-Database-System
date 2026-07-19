# 部署手册 (DEPLOYMENT)

本手册覆盖 IT 资产管理系统的三种部署方式、环境变量、数据库初始化、反向代理、备份恢复、升级与排障。
运维速查（备份/恢复/管理员找回）见 [runbook.md](runbook.md)；架构与数据模型见 [architecture.md](architecture.md)。

---

## 目录

- [1. 部署方式总览](#1-部署方式总览)
- [2. 前置条件](#2-前置条件)
- [3. 方式 A：Docker Compose（推荐）](#3-方式-adocker-compose推荐)
- [4. 方式 B：源码本地部署（裸机）](#4-方式-b源码本地部署裸机)
- [5. 方式 C：单镜像 Docker](#5-方式-c单镜像-docker)
- [6. 环境变量参考](#6-环境变量参考)
- [6.x 企业化功能配置](#6x-企业化功能配置)
- [7. 数据库说明](#7-数据库说明)
- [8. 反向代理与 HTTPS](#8-反向代理与-https)
- [9. 首次登录与初始化](#9-首次登录与初始化)
- [10. 迁移机制](#10-迁移机制)
- [11. 健康检查与日志](#11-健康检查与日志)
- [12. 备份与恢复](#12-备份与恢复)
- [13. 升级](#13-升级)
- [14. 生产部署检查清单](#14-生产部署检查清单)
- [15. 常见问题排障](#15-常见问题排障)

---

## 1. 部署方式总览

| 方式 | 适用场景 | 说明 |
|---|---|---|
| **A. Docker Compose** | 生产/测试推荐 | 一条命令拉起 PostgreSQL 16 + 应用，单二进制内嵌前端 |
| **B. 源码本地部署** | 开发/内网裸机 | `setup-and-run.sh` 一键脚本，或手动 `go build` + 本机 PostgreSQL |
| **C. 单镜像 Docker** | 已有外部 PG | 只构建应用镜像，连接外部 PostgreSQL |

应用为**单二进制**：Go 编译时通过 `//go:embed` 把前端 `dist/` 与全部 SQL 迁移嵌入二进制，运行时无需额外静态文件。生产模式默认连接 PostgreSQL；`DEMO=true` 时使用内存存储（仅演示，数据不持久）。

---

## 2. 前置条件

| 组件 | 版本 | 备注 |
|---|---|---|
| PostgreSQL | ≥ 13（推荐 16） | 需要 `uuid-ossp`、`ltree` 扩展（迁移自动创建） |
| Go | ≥ 1.25（仅源码部署） | 编译后端 |
| Node.js | ≥ 22（仅源码部署） | 构建前端 |
| Docker + Compose | 任意现代版本（方式 A/C） | |

**数据库用户权限**：执行迁移的数据库用户必须能够 `CREATE EXTENSION` 与 `CREATE ROLE`，因此**需要 SUPERUSER**（或由 DBA 预建扩展/角色后降权）。原因：`001_init.sql` 会创建 `uuid-ossp`、`ltree` 扩展及 `app_writer`、`audit_reader` 两个角色。

> Docker Compose 方式中 `POSTGRES_USER=app_user` 生成的用户即超级用户，满足要求；裸机方式请参考脚本中的 `SUPERUSER` 授权。

---

## 3. 方式 A：Docker Compose（推荐）

```bash
git clone https://github.com/Gendmyb/Asset-Database-System.git
cd Asset-Database-System

# （强烈建议）生成并写入 JWT 种子，编辑 docker-compose.yml 的 JWT_ED25519_SEED
openssl rand -hex 32

docker compose up -d --build
```

启动后：

- 前端/UI：`http://<服务器IP>:8080`
- API：`http://<服务器IP>:8080/api/v1`
- 健康检查：`http://<服务器IP>:8080/healthz`
- 默认账号：**admin / admin123**（首次登录后请立即在「个人设置」改密）

**关键配置（编辑 `docker-compose.yml`）**：

1. `JWT_ED25519_SEED`：务必填入 `openssl rand -hex 32` 生成的 64 位 hex。留空则每次重启随机生成，会导致所有已签发 token 失效、全员被踢下线。
2. `POSTGRES_PASSWORD` / `DATABASE_PASSWORD`：生产环境改成强密码（两处需一致）。
3. `ports`：默认暴露 5432 与 8080；生产建议仅暴露 8080，5432 限定内网/仅 app 容器访问。

数据卷 `pgdata` 持久化数据库，删除容器不会丢数据（除非 `docker compose down -v`）。

---

## 4. 方式 B：源码本地部署（裸机）

### 4.1 一键脚本

```bash
git clone https://github.com/Gendmyb/Asset-Database-System.git
cd Asset-Database-System
bash setup-and-run.sh
```

脚本自动完成：安装/启动 PostgreSQL → 创建 `app_user`/`assetdb`/`assets` schema（授权 SUPERUSER）→ 构建前端 → 构建后端 → 生成 JWT 种子 → 后台启动 → 健康检查。

完成后访问 `http://localhost:8080`，默认 `admin / admin123`。

脚本默认使用 `app_user / app_pass`，仅适合内网/演示。生产请改密码并固化 `JWT_ED25519_SEED`（脚本未持久化种子，重启会重新生成）。

### 4.2 手动构建

```bash
# 1. 前端
cd web
npm ci
npm run build          # 产物在 web/dist
cd ..

# 2. 把前端产物放到 Go embed 路径
rm -rf assetserver/web/dist && cp -r web/dist assetserver/web/dist

# 3. 后端
cd assetserver
go build -ldflags="-s -w" -o bin/api-server ./cmd/api-server

# 4. 配置环境变量并运行（示例）
export DB_HOST=localhost DB_PORT=5432 DB_NAME=assetdb DB_USER=app_user
export DATABASE_PASSWORD=app_pass
export JWT_ED25519_SEED=$(openssl rand -hex 32)
export DEMO=false
./bin/api-server
```

> ⚠️ `dist/` 在仓库里以**占位符**提交。`go build` 要求该目录存在（embed 编译期需要），但占位符不含真实前端。**修改前端后必须重新 `npm run build` 并拷贝到 `assetserver/web/dist/` 再编译后端**，否则界面是空的。CI 与 `setup-and-run.sh` 已自动处理。

### 4.3 数据库手动初始化（可选）

应用启动时会自动执行迁移，通常无需手动建表。若要预建库与用户：

```bash
sudo -u postgres psql <<'SQL'
CREATE USER app_user WITH PASSWORD '改成强密码' SUPERUSER CREATEROLE;
CREATE DATABASE assetdb OWNER app_user;
\c assetdb
CREATE SCHEMA IF NOT EXISTS assets AUTHORIZATION app_user;
SQL
```

---

## 5. 方式 C：单镜像 Docker

适合已有外部 PostgreSQL 的场景。镜像构建同 Dockerfile（三阶段：node 构建前端 → go 构建后端 → alpine 运行时）。

```bash
git clone https://github.com/Gendmyb/Asset-Database-System.git
cd Asset-Database-System
docker build -t asset-db .

docker run -d --name asset-db \
  -p 8080:8080 \
  -e DB_HOST=pg.internal -e DB_PORT=5432 -e DB_NAME=assetdb \
  -e DB_USER=app_user -e DATABASE_PASSWORD='强密码' \
  -e JWT_ED25519_SEED=$(openssl rand -hex 32) \
  asset-db
```

外部 PostgreSQL 需提前创建 `assetdb` 库、`assets` schema，并授予 `app_user` 足够权限（见 [第 2 节](#2-前置条件) 与 [第 7 节](#7-数据库说明)）。

---

## 6. 环境变量参考

应用实际读取的环境变量如下（`DATABASE_URL` **不被使用**，连接串由 `DB_*` 变量拼装）：

| 变量 | 默认 | 说明 |
|---|---|---|
| `SERVER_HOST` | `0.0.0.0` | 监听地址 |
| `SERVER_PORT` | `8080` | 监听端口 |
| `DB_HOST` | `localhost` | PostgreSQL 主机 |
| `DB_PORT` | `5432` | PostgreSQL 端口 |
| `DB_NAME` | `assetdb` | 数据库名 |
| `DB_USER` | `app_user` | 数据库用户（需 SUPERUSER/CREATEROLE，见第 2 节） |
| `DATABASE_PASSWORD` | （空） | 数据库密码 |
| `DB_SSLMODE` | `disable` | PG sslmode，生产建议 `require` 或 `verify-full` |
| `JWT_ED25519_SEED` | （空→随机） | Ed25519 种子，64 位 hex。**生产必须固定**，否则重启全员掉线 |
| `DEMO` | `false` | `true` 走内存存储，无需 PG（功能冻结，仅演示） |

连接串最终形如：
```
postgres://app_user:<PASSWORD>@<DB_HOST>:<DB_PORT>/assetdb?sslmode=<DB_SSLMODE>&search_path=assets
```

完整模板见仓库根 `.env.example`。

---

## 6.x 企业化功能配置

> Wave 1+2 引入的企业化能力（G1–G9）全部默认关闭，行为与 v0.2.0 一致；按需开启下列开关。所有 env 均为可选项。

### AD/LDAP（G1）

`LDAP_HOST` + `LDAP_BASE_DN` + `LDAP_BIND_DN` 三者齐全时自动启用 LDAP；任一缺失则系统以纯本地模式运行（不报错）。

| 变量 | 默认 | 说明 |
|---|---|---|
| `LDAP_HOST` | （空） | AD/LDAP 主机，例如 `ldap.corp.local` |
| `LDAP_PORT` | `389` | 端口；LDAPS 一般用 `636` |
| `LDAP_USE_TLS` | `false` | `true` 启用 StartTLS |
| `LDAP_USE_SSL` | `false` | `true` 启用 LDAPS（ldaps://） |
| `LDAP_BIND_DN` | （空） | 服务账号 DN，例如 `CN=svc-ldap,OU=Service,DC=corp,DC=local` |
| `LDAP_BIND_PASSWORD` | （空） | 服务账号密码（不入日志/审计） |
| `LDAP_BASE_DN` | （空） | 搜索基 DN，例如 `OU=Staff,DC=corp,DC=local` |
| `LDAP_USER_FILTER` | `(&(objectClass=user)(sAMAccountName=%s))` | 用户查找过滤器，`%s` 占位用户名 |
| `LDAP_SYNC_DISABLE_ONLY` | `false` | `true` 时同步仅禁用不存在账号，不创建新账号 |

登录策略：本地账号优先，本地未命中走 LDAP bind 兜底；`admin` 账号永不被 LDAP 覆盖。手动同步：`POST /admin/ldap/sync`（admin+）。

### 定时任务（G4 到期提醒）

| 变量 | 默认 | 说明 |
|---|---|---|
| `SCHEDULER_INTERVAL` | `off` | 调度间隔；`off` 不启动。支持 `30m`/`1h`/`24h` 等 Go duration，或纯数字秒 |
| `SCHEDULER_WARRANTY_DAYS` | `30` | 保修到期提前预警天数 |
| `SCHEDULER_LDAP_SYNC` | `false` | `true` 时调度循环中触发 LDAP 同步 |

调度器扫描保修到期 / 领用逾期并发布事件（接入 G6 通知渠道时即会推送）。

### 通知渠道（G6）

总开关 `NOTIFY_ENABLE` 默认关闭。SMTP 渠道在 `SMTP_HOST` 配置后启用；任一机器人 webhook 配置即启用该渠道。机器人 webhook 强制 HTTPS 并做 SSRF 防护（拒绝内网/回环地址）。

| 变量 | 默认 | 说明 |
|---|---|---|
| `NOTIFY_ENABLE` | `false` | 通知总开关 |
| `SMTP_HOST` | （空） | SMTP 主机 |
| `SMTP_PORT` | `587` | SMTP 端口 |
| `SMTP_USER` | （空） | SMTP 登录用户 |
| `SMTP_PASSWORD` | （空） | SMTP 密码（不入日志/审计） |
| `SMTP_FROM` | （空） | 发件人地址 |
| `NOTIFY_DINGTALK_WEBHOOK` | （空） | 钉钉机器人 webhook（必须 https） |
| `NOTIFY_WECOM_WEBHOOK` | （空） | 企业微信机器人 webhook（必须 https） |
| `NOTIFY_FEISHU_WEBHOOK` | （空） | 飞书机器人 webhook（必须 https） |

管理 API：`/admin/notify/rules`（规则 CRUD）、`/admin/notify/deliveries`（投递记录）。

### 资产二维码（G3）

| 变量 | 默认 | 说明 |
|---|---|---|
| `EXTERNAL_URL` | （空） | 受信对外基础 URL，例如 `https://assets.example.com`。仅用于 `GET /assets/:id/qrcode?content=url` 模式拼详情页 URL；未配置时 url 模式返回 400，默认 QR 内容为 `asset_tag` |

### 部门级行级权限（G9）

| 变量 | 默认 | 说明 |
|---|---|---|
| `DATA_SCOPE_DEPARTMENT` | `false` | `true` 启用部门级可见范围：super_admin 全局可见，manager 仅见本部门及子孙（ltree 子树）。`false` 时行为同 v0.2.0（org 级） |

### 审批流（G7）

无需 env，通过系统设置开关：`approval.assignment.enabled` / `approval.retirement.enabled` / `approval.maintenance.enabled`，默认全部关闭（领用/报废/维修直接执行，向后兼容）。开启后对应操作生成审批单，经 `/admin/approvals/:id/approve|reject` 流转后方可生效。

### Wave 3: AD 组同步与企业级权限

> Wave 3 引入的增强能力（T0–T10）全部默认关闭，行为与 v0.2.0 一致；所有 env 均为可选项。

#### ControlPaging 与同步控制

AD/LDAP 搜索新增分页控制，防止 AD 默认 MaxPageSize=1000 导致的无声截断（超过 1000 用户时仅返回前 1000 条，无任何错误提示）。此行为自动生效，无需额外配置。

| 变量 | 默认 | 说明 |
|---|---|---|
| `LDAP_PAGE_SIZE` | `1000` | 单页返回条目数，与 AD MaxPageSize 匹配。降低可减少每页负载，但不能超过 AD 服务器 `MaxPageSize` 限制 |
| `LDAP_SYNC_RECURSIVE` | `true` | `true` 时递归搜索 `LDAP_BASE_DN` 下所有 OU 嵌套组；`false` 仅查直属对象 |
| `LDAP_LINK_EXISTING` | `false` | `true` 时 AD 同步会尝试将现有用户与 AD 账号关联（匹配 username）；仅影响首次关联，不覆盖已有关联 |
| `LDAP_GROUP_ATTR` | `memberOf` | LDAP 属性字段名，用于读取用户所属组列表。AD 默认 `memberOf`，标准 LDAP 可能需改为 `memberof` 或自定义属性 |

#### 组到角色映射（ad_group_mappings）

通过 `ad_group_mappings` 表将 AD 安全组 DN 映射到系统角色。同步时系统枚举所有 `sync_enabled=true` 的映射，查询对应组成员并按**最高角色**确定每个用户的最终角色。

**配置方式**（API + Admin UI）：

1. 在 Admin UI「LDAP 状态」→「组映射」卡片中创建映射，或通过 API：
   ```
   POST /admin/ldap/group-mappings
   { "group_dn": "CN=IT-Admins,OU=Groups,DC=corp,DC=local",
     "group_name": "IT-Admins",
     "role": "admin",
     "data_scope": "inherit",
     "sync_enabled": true }
   ```
2. `group_dn` 必须精确匹配 AD 中组的 `distinguishedName`（表中 UNIQUE 约束）。
3. `data_scope` 设为 `self` 时，该组所有成员将只能看到分配给自己个人的资产（见下文）。
4. 如果用户属于多个映射组，取角色层级最高的（`super_admin > admin > manager > viewer`）。
5. 删除或设置 `sync_enabled=false` 即停止该组映射。

手动同步：`POST /admin/ldap/sync`（admin+），或配置 `SCHEDULER_LDAP_SYNC=true` 定时触发。

#### 个人数据范围（data_scope）

`users.data_scope` 控制单个用户的数据可见范围：

| 值 | 行为 | 安全说明 |
|---|---|---|
| `inherit`（默认） | 沿用组映射的 `data_scope`，或系统默认（org/部门级） | 历史行为，向后兼容 |
| `self` | 用户只能查看 **分配给自己**（`assigned_to_user_id = current_user`）的资产 | **最高限制优先级**：即使拥有 manager/admin 角色也无法越权查看他人的资产。适合外部审计员、临时承包商等敏感角色 |

范围优先级判定：
1. 若 `users.data_scope = 'self'` → 忽略所有其他范围策略，仅返回自有资产
2. 若 `users.data_scope = 'inherit'` → 回退到组映射的 `data_scope`，若组也未指定则使用系统默认（`DATA_SCOPE_DEPARTMENT` env 或 v0.2.0 org 级）

**安全影响**：`self` 模式是硬限制，不受角色提升影响。不要将其分配给需要跨用户管理的 admin/manager——这些角色应保持 `inherit` 并依靠部门权限（G9）控制范围。建议仅对审计/承包商/临时访问场景使用 `self`。

#### manual_override 保护

`users.manual_override` 标记（默认 `false`）用于保护 admin 手动调整不被 AD 同步覆盖：

- **未开启**（`false`）：AD 同步正常更新用户的 role、status、data_scope 等字段
- **开启**（`true`）：AD 同步**跳过**该用户的 role、status、data_scope 字段更新，但 display_name、email、department 等基本属性仍正常刷新

典型场景：
1. Admin 手动将某用户的角色从 `viewer` 提升为 `manager`
2. Admin 对该用户开启 `manual_override`（Admin UI 用户管理页 → 行操作）
3. 后续 AD 同步不会再将其降级回 `viewer`
4. 用户的 display_name/email 变化仍会从 AD 同步

#### 迁移 014

`014_ad_enterprise.sql` 随应用启动自动执行（见第 10 节迁移机制），包含：
- `ad_group_mappings` 表（group_dn UNIQUE、sync_enabled 索引）
- `users.data_scope` 列（默认 `'inherit'`，CHECK `IN ('inherit','self')`）
- `users.manual_override` 列（默认 `false`，带 partial index）

全部使用 `IF NOT EXISTS`，对已运行的实例安全幂等，不会影响现有数据。

---

## 7. 数据库说明

- **schema**：所有表位于 `assets` schema（非 public）。
- **迁移**：`assetserver/migrations/001-014*.sql`，启动时自动执行（见第 10 节）。当前最新为 `014_ad_enterprise.sql`。
- **扩展**：`uuid-ossp`（UUID 生成）、`ltree`（组织树）。
- **角色**：迁移创建 `app_writer`、`audit_reader`（预留，当前应用统一用 `app_user`）。
- **连接池**：默认 5–25 连接，生命周期 1h、空闲 10m。PostgreSQL `max_connections` 建议 ≥ 50。

核心表：`assets`、`assignments`、`users`、`asset_types`、`locations`、`maintenance_orders`、`stocktake_plans`/`stocktake_items`、`audit_log`、`system_settings`、`doc_sequences`、`webhook_endpoints`/`webhook_deliveries`、`refresh_tokens`、`schema_migrations`。详见 [architecture.md](architecture.md#数据模型实际使用表)。

---

## 8. 反向代理与 HTTPS

应用本身只监听 HTTP:8080，生产应在前面挂反向代理终止 TLS。

### Nginx 示例

```nginx
server {
    listen 443 ssl http2;
    server_name assets.example.com;

    ssl_certificate     /etc/ssl/assets.crt;
    ssl_certificate_key /etc/ssl/assets.key;

    client_max_body_size 20m;          # CSV 导入可能较大

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 60s;
    }
}
# HTTP 80 跳转 HTTPS
server {
    listen 80;
    server_name assets.example.com;
    return 301 https://$host$request_uri;
}
```

要点：

- `X-Forwarded-Proto` 让应用识别真实协议（若后续启用信任代理逻辑）。
- 静态资源（`/assets/*.js`、`/assets/*.css`）由应用直接服务，无需单独配置；可对 `/assets/` 路径加长缓存。
- 不要把 5432 端口暴露到公网。

---

## 9. 首次登录与初始化

1. 浏览器访问部署地址。
2. 用 **admin / admin123** 登录。
3. 立即修改密码（右上角 / 个人设置）。
4. 建议在「系统设置」中确认资产编号前缀（默认 `AST-`）、组织名称等。
5. 按需在「用户管理」创建经办人/只读账号（4 角色 RBAC 见 README）。

种子用户：`admin`(super_admin)、`张三`(manager)、`李四`(manager)、`王五`(viewer)。

---

## 10. 迁移机制

- 迁移文件位于 `assetserver/migrations/`，**嵌入二进制**，启动时由自研执行器（`internal/db/migrate.go`）按文件名顺序执行。
- 已执行版本记录在 `assets.schema_migrations` 表。
- 执行前对 `schema_migrations` 加 `EXCLUSIVE` 锁，**多实例同时启动不会重复执行**。
- 迁移在**独立事务**中运行；某条失败则启动中止，已执行的不受影响。
- **升级即重启**：拉新代码/镜像后 `docker compose up -d --build`，新增迁移会自动应用，无需手动操作。

当前迁移清单：

| 文件 | 内容 |
|---|---|
| 001_init | schema、核心表、扩展、角色、审计链触发器 |
| 002_settings_sequences | system_settings + doc_sequences |
| 003_drop_unused | 清理未用表 |
| 004_locations | 位置树 |
| 005_auth | bcrypt admin、refresh_tokens.revoked_at、role 收敛 |
| 006_asset_finance_assignments | 采购/折旧字段、borrowed 状态、借用 due_date |
| 007_maintenance | 维修工单 |
| 008_stocktake | 盘点计划/明细 |
| 009_webhooks | webhook 订阅/投递记录 |
| 010_remove_procurement_and_user_softdelete | 移除 procurement 状态、用户软删除(deleted_at) |
| 011_ldap_and_user_import | LDAP 同步状态字段、用户导入批次记录 |
| 012_notify_and_approvals | 通知规则/投递记录表、审批单表 |
| 013_asset_parent_and_data_scope | 资产 parent_id 外设树、部门数据范围 ltree 索引 |
| 014_ad_enterprise | AD group-to-role 映射表 (ad_group_mappings)、users.data_scope、users.manual_override |

---

## 11. 健康检查与日志

```bash
curl -s http://localhost:8080/healthz     # 存活（进程在跑）
curl -s http://localhost:8080/readyz      # 就绪（含 DB 连通性）
```

- Docker Compose：`docker compose logs -f app`、`docker compose logs -f postgres`。
- 裸机一键脚本：`tail -f /tmp/assetdb-api.log`。
- systemd：`journalctl -u asset-db -f`（需自行编写 unit 文件，见下）。

### systemd unit 示例（裸机生产）

```ini
# /etc/systemd/system/asset-db.service
[Unit]
Description=Asset Database System API
After=network.target postgresql.service

[Service]
Type=simple
User=asset
WorkingDirectory=/opt/asset-db
EnvironmentFile=/opt/asset-db/.env
ExecStart=/opt/asset-db/bin/api-server
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## 12. 备份与恢复

### 备份（仅 `assets` schema，自定义格式）

```bash
docker compose exec postgres pg_dump -U app_user -d assetdb --schema=assets -Fc \
  -f /tmp/assetdb_$(date +%Y%m%d).dump
docker compose cp postgres:/tmp/assetdb_$(date +%Y%m%d).dump ./backups/
```

裸机：`pg_dump -U app_user -d assetdb --schema=assets -Fc -f backups/assetdb_$(date +%Y%m%d).dump`

建议加入 cron 每日备份，并保留多份。

### 恢复

```bash
docker compose stop app
docker compose exec -T postgres pg_restore -U app_user -d assetdb \
  --clean --if-exists --schema=assets < ./backups/assetdb_YYYYMMDD.dump
docker compose up -d app     # 启动时自动补齐缺失迁移
```

> 恢复会按 dump 重建数据；若 dump 早于新迁移，应用启动时会自动补跑后续迁移。

---

## 13. 升级

```bash
git pull
docker compose up -d --build      # 自动重建镜像 + 应用新迁移
```

裸机：重新 `npm run build` → 拷贝 dist → `go build` → 重启进程（迁移自动执行）。

升级前建议先备份（第 12 节）。回滚：用备份恢复 + 退回旧代码/镜像。

---

## 14. 生产部署检查清单

- [ ] `JWT_ED25519_SEED` 已固定为 `openssl rand -hex 32`（写进 compose env / systemd EnvironmentFile）
- [ ] 数据库密码已改成强密码（`POSTGRES_PASSWORD`/`DATABASE_PASSWORD` 两处一致）
- [ ] PostgreSQL `max_connections` ≥ 50
- [ ] 5432 端口未对公网暴露（仅 app 可达）
- [ ] 前置反向代理终止 HTTPS
- [ ] 配置每日 `pg_dump` 备份并验证可恢复
- [ ] 默认 `admin/admin123` 已改密
- [ ] `DB_SSLMODE` 按需设为 `require`/`verify-full`
- [ ] 服务器时区正确（影响折旧月份计算与时间戳显示）
- [ ] 企业化开关按需开启：`LDAP_HOST`+`LDAP_BASE_DN`+`LDAP_BIND_DN`（G1）、`NOTIFY_ENABLE`+对应渠道（G6）、`SCHEDULER_INTERVAL`（G4）
- [ ] `EXTERNAL_URL` 已配置为对外受信域名（启用 G3 url 模式二维码所需）
- [ ] `DATA_SCOPE_DEPARTMENT` 按组织治理需要决定是否开启（G9）
- [ ] 审批门 `approval.*.enabled` 按业务流程在系统设置中开启（G7，默认关闭）
- [ ] `LDAP_PAGE_SIZE` 与 AD 服务器 MaxPageSize 匹配（Wave 3，默认 1000）
- [ ] `LDAP_SYNC_RECURSIVE` / `LDAP_GROUP_ATTR` 按 LDAP 结构验证默认值（Wave 3）
- [ ] 组到角色映射（`ad_group_mappings`）已按组织架构配置（Wave 3，Admin UI → LDAP 状态）
- [ ] 敏感用户（承包商/审计员）已按需设置 `data_scope=self` 限制可见范围（Wave 3）
- [ ] 手动调整过权限的用户已开启 `manual_override` 防止 AD 同步覆盖（Wave 3）

---

## 15. 常见问题排障

### 15.1 直接访问 `/login` 等页面返回 404 page not found

已修复（SPA 回退）。若仍出现，说明运行的是旧二进制——重新构建（前端 dist + 后端）并重启。应用现在对不存在的非 `/api` 路径回退到 `index.html`，由前端路由器接管；`/api/*` 仍返回 JSON 404。

### 15.2 界面空白 / 只有背景色

前端 `dist/` 是占位符或未更新。重新 `npm run build` 并把 `web/dist` 拷到 `assetserver/web/dist` 后再 `go build`。CI 与 `setup-and-run.sh` 已自动处理。

### 15.3 重启后所有人被踢下线 / 频繁要求重新登录

`JWT_ED25519_SEED` 未固定，每次重启随机生成。设置固定的 64 位 hex 种子。

### 15.4 迁移失败：permission denied / must be owner / cannot create extension

数据库用户权限不足。`app_user` 需要 SUPERUSER（或由 DBA 预建 `uuid-ossp`/`ltree` 扩展与 `app_writer`/`audit_reader` 角色后降权）。一键脚本已自动授权 SUPERUSER。

### 15.5 启动失败：Migration error ... check constraint violated

通常是跳版本升级或手工改过数据导致状态非法。查看 `/tmp/assetdb-api.log` 或容器日志定位具体迁移与约束，修正数据后重启（迁移失败不会记录版本，可安全重试）。

### 15.6 管理员密码遗忘 / 账户被禁用

```bash
docker compose exec postgres psql -U app_user -d assetdb <<'SQL'
-- admin123 的 bcrypt hash, 同时要求下次登录改密
UPDATE assets.users
  SET password_hash='$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
      must_change_password=true, status='active', deleted_at=NULL
  WHERE username='admin';
SQL
# 用 admin / admin123 登录后立即改密
```

> 注：`deleted_at=NULL` 用于应对管理员被误删（软删除）的情况——记录仍在，恢复即可登录。

### 15.7 端口被占用

`8080`/`5432` 被占：改 `SERVER_PORT` / compose `ports`，或停掉占用进程。

### 15.8 Docker 构建慢 / 拉镜像失败

前端 `npm ci` 与 Go `go mod download` 首次较慢，后续有缓存层。国内网络可配置 npm/Go 镜像源。

---

更多 API 细节见 [api.md](api.md)，架构与数据模型见 [architecture.md](architecture.md)。
