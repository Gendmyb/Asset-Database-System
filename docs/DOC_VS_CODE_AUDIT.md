# 文档 ↔ 代码 逐条对照审计报告

> 生成日期: 2026-07-18
> 方法: 以 `assetserver/`、`web/`、`demo/`、`migrations/` 的**实际代码**为准，逐条核对 `README.md`、`docs/architecture.md`、`docs/progress.md`、`demo/README.md` 的描述。
> 图例: ✅ 代码与文档一致 · ⚠️ 部分实现/有缺陷 · ❌ 文档声称但代码未实现 · 📄 仅文档设计（无代码）

---

## 0. 总体结论

| 维度 | 实际情况 |
|---|---|
| **可运行主链路** | DEMO 模式下：登录 → 资产 CRUD → 生命周期转换 → 领用/归还 → 仪表盘，**真实可用** |
| **文档 vs 代码差距** | 巨大。`architecture.md`(2913 行) 描述的 Vault/mTLS/OCSP/Patroni HA/Redis Sentinel/S3 归档/K8s 等**绝大多数无代码实现** |
| **死代码** | `cache`/`event`/`webhook`/`ingest` 包完整但 `server.go` 从未 import；`location.go`、`dashboard.go` handler 未注册 |
| **高危契约 bug** | 前端生命周期转换、token 刷新两处调用与后端路由不匹配，功能直接失效（见 §5） |
| **测试** | 实为 **40 个 test 函数**（含 fuzz），非 README 所称 "47 项" |
| **本机限制** | 未安装 Go，编译/测试结论基于静态审查 |

---

## 1. 模块级对照（文档承诺 ↔ 代码实现）

| 文档/架构承诺 | 对应章节 | 代码位置 | 状态 | 说明 |
|---|---|---|---|---|
| Ed25519 JWT 签发/验证 | §7.1 | `crypto/jwt.go` | ✅ | 实现规范，校验 iss/aud/exp/alg/kid |
| 登录认证 | §6.4 | `server.go:513` | ⚠️ | **硬编码 `admin/admin`**，无用户表校验、无密码哈希比对 |
| Refresh Token 轮换 + 重用检测 | §6.4 / §7.1 | 无 | ❌ | 无 `/auth/refresh` 端点、无 `refresh_tokens` 写入、无 family 吊销 |
| Vault/KMS 密钥管理 | §2.3 / §7.1 | `crypto/jwt.go:37` | ❌ | 每次启动随机生成密钥，重启后所有 token 失效；`config.VaultConfig` 仅定义未使用 |
| 多租户隔离 (org scope) | §7.2 | `middleware/middleware.go:92` | ⚠️ | `OrgScope` 无 org 时默认注入固定 org；role 存入 context 但**从不校验** |
| RBAC 权限模型 | §7.3 | 无 | ❌ | role 仅展示，无任何权限检查代码 |
| MFA / 双人审批 | §7.3 | 无 | ❌ | 仅 schema 有 `approval_requests` 表，无代码 |
| 三层锁策略 | §8 | `internal/lock/lock.go` | ✅(代码) | 乐观/悲观/advisory 锁完整；但实际仅资产/领用用到乐观+悲观 |
| 乐观锁重试 | §8.2 | `repository/asset_repo.go:198` | ✅ | `UpdateWithRetry` 最多 3 次 |
| 悲观锁防死锁 (字典序) | §8.3 | `repository/asset_repo.go:276` | ✅ | `LockAssetsSorted` + `lock.SortedAssetIDs` |
| Advisory 锁 + 碰撞检测 | §8.4 | `internal/lock/lock.go:84` | ⚠️ | 代码完整但**无生产调用方** |
| 生命周期状态机 | §5.4 | `domain/lifecycle.go` | ✅ | 转换矩阵 + 终态判断 + fuzz 测试，质量最高模块 |
| 资产 CRUD + 游标分页 | §6.4 | `repository/asset_repo.go:58` | ✅ | 游标分页、软删除、If-Match 乐观锁均实现 |
| 中文/英文搜索分流 | §6.5 | `repository/asset_repo.go:70` | ✅ | CJK→ILIKE，英文→`to_tsvector` |
| 缓存三防 (雪崩/击穿/穿透) | §11 | `internal/cache/cache.go` | ❌(未接线) | `CacheAside` 实现完整，**`server.go` 未 import** |
| Redis 缓存 / Sentinel HA | §11 / §3 | 无 Redis 代码 | ❌ | `go.mod` **无 Redis 依赖**；仅 docker-compose 有 Redis 容器，代码不连接 |
| 事件总线 (Pub/Sub + Outbox) | §10.1 | `internal/event/bus.go` | ❌(未接线) | 仅内存实现，未 import；无 Redis Stream |
| Webhook 引擎 + HMAC + SSRF 防护 | §10.2 / §7.4 | `internal/webhook/` | ❌(未接线) | 实现质量高（SSRF 私有网段 + DNS rebinding 双校验），但未 import |
| Agent 摄入管道 (背压/去重/序列号) | §6.3 / §9.5 | `internal/ingest/` | ❌(未接线) | api-server 未使用；仅 `collection-agent` main 引用 |
| Agent 系统采集 (Linux/macOS) | §9.4 | `internal/agent/collector/` | ⚠️ | 采集真实（读 /proc、df、sysctl），但 `main.go` 注册与上报被注释跳过 |
| Agent 离线队列 + DLQ | §9.6 | `internal/agent/store/offline.go` | ⚠️ | 内存队列实现，未接入 main |
| Agent 证书续期 | §9.7 / §7.1 | `internal/agent/certs/renewal.go` | 📄 | 占位实现（RenewCertificate 空函数） |
| Agent 自更新 + 金丝雀 | §9.7 | `internal/agent/updater/updater.go` | 📄 | CheckUpdate 返回固定 mock；Rollback 未真正 exec |
| mTLS 双向认证 | §7.1 / §13 | `deploy/nginx.conf` | 📄 | 仅 nginx 配置模板，Go 代码无 mTLS |
| 审计链 (不可变 + 链式哈希) | §7.6 | `migrations/001_init.sql:126` | ⚠️ | **表结构 + trigger + RLS 完整，但无任何代码 INSERT**；审计功能空转 |
| 组织树 (ltree 物化路径) | §7.2 | `handler/organization.go` | ⚠️ | **生产模式也用内存 `OrgStore`**（非 PG/ltree），重启即丢；且无锁 |
| 位置管理 (Locations) | Phase 5 | `handler/location.go` | ❌(未注册) | handler 存在但 `server.go` 未注册 `/locations` 路由 |
| Dashboard 聚合 | §12 / Phase 5 | `repository/dashboard_repo.go` | ✅ | 生产走 PG 聚合（status/category/lifecycle） |
| Grafana 只读通道 | §12 | `grafana/asset-overview.json` | 📄 | 仅 JSON + docker-compose 容器，无授权 CronJob |
| HA 部署 (Patroni/PgBouncer/K8s) | §3 / §13 | 无 | 📄 | 仅文档与 runbook 命令 |
| S3 冷热分层归档 | §14.3 | 无 | ❌ | 仅 schema 有 `archive_manifest` 表 |
| 限流策略 | §8.6 | `middleware/middleware.go:105` | ❌ | `RateLimit()` 空函数且未注册 |

---

## 2. API 端点逐条对照

### 2.1 架构文档 §6.4 路由表 ↔ 实际注册路由

**认证**

| 文档端点 | 实际状态 | 证据 |
|---|---|---|
| `POST /auth/login` | ✅ 实现（硬编码 admin/admin） | `server.go:513` |
| `POST /auth/refresh` | ❌ 不存在 | 前端 `client.ts:42` 却依赖它 → **bug** |
| `POST /auth/register-agent` | ❌ 不存在 | — |
| `POST /auth/logout` | ❌ 不存在 | — |

**资产**

| 文档端点 | 实际状态 | 证据 |
|---|---|---|
| `GET /assets` | ✅ | `asset_v2.go:57` / demo `asset.go:73` |
| `POST /assets` | ✅ | `asset_v2.go:105` |
| `GET /assets/:id` | ✅ | `asset_v2.go:95` |
| `PUT /assets/:id` | ✅ | `asset_v2.go:150`（If-Match） |
| `DELETE /assets/:id` | ✅ | `asset_v2.go:193`（软删除） |
| `GET /assets/:id/history` | ⚠️ 仅 DEMO 有 | `server.go:304`；生产 assetV2 无端点 |
| `GET /assets/:id/snapshots` | ❌ | 无 |
| `GET /assets/:id/snapshots/latest` | ❌ | 无 |
| `GET /assets/:id/relationships` | ❌ | 无 |

**生命周期 / 领用**

| 文档端点 | 实际状态 | 证据 |
|---|---|---|
| `POST /assets/:id/transition` | ✅ | `asset_v2.go:202`（前端却调 PUT `/lifecycle` → bug） |
| `POST /assets/:id/assign` | ✅ | `assignment.go:21` |
| `POST /assets/:id/release` | ✅ | `assignment.go:53` |
| `POST /assets/:id/transfer` | ✅ | `assignment.go:67` |
| `GET /assets/:id/assignment` | ⚠️ 实为 `/assignments`（复数） | `server.go:475` |
| `GET /assets/:id/assignment/history` | ❌ | 无 |
| `GET /users/:id/assignments` | ❌ | 无 |

**Agent**

| 文档端点 | 实际状态 |
|---|---|
| `POST /agents/sync` | ❌ |
| `POST /agents/heartbeat` | ❌ |
| `GET /agents` | ⚠️ 返回硬编码空数组 `server.go:501` |
| `GET /agents/:id` · `PUT` · `DELETE` · `update-check` | ❌ 均无 |

**管理 (super_admin)**

| 文档端点 | 实际状态 |
|---|---|
| `/admin/users` 系列、`/admin/asset-types`、`/admin/enrollment-tokens`、`/admin/approvals` | ❌ 全部不存在；前端 `/admin` 路由是占位 `<div>` |

### 2.2 实际存在但文档未列出的端点

| 端点 | 实现 | 说明 |
|---|---|---|
| `GET /asset-types` | 硬编码 6 类型 | `server.go:217` |
| `GET /users` · `GET /users/:id` | DEMO 硬编码 / 生产部分 PG | `server.go:229` |
| `GET /settings` · `PUT /settings` · `GET /settings/next-tag` | 内存/PG | `server.go:256` |
| `GET /dashboard/overview` | ✅ | DEMO 内存 / 生产 PG |
| `GET /dashboard/agents` | ⚠️ 硬编码 `{0,0,0}` | `server.go:496` |
| `GET /organizations` 系列 | 内存 OrgStore | `organization.go` |
| `GET /healthz` · `GET /readyz` | ✅ | `health.go` |

---

## 3. 数据库表对照（schema 有 ↔ 代码使用）

`migrations/001_init.sql` 定义的表及实际读写情况：

| 表 | 代码读 | 代码写 | 状态 |
|---|---|---|---|
| `organizations` | ❌(用内存) | ❌(用内存) | ⚠️ 表建而不用 |
| `users` | ✅ `user_repo.go` | ✅ 种子写入 | ✅ |
| `asset_types` | ✅ `dashboard_repo.go` | ❌(仅种子) | ⚠️ |
| `assets` | ✅ | ✅ | ✅ 核心表 |
| `assignments` | ✅ | ✅ | ✅ |
| `audit_log` | ❌ | **❌ 无人写入** | ❌ 审计空转 |
| `collection_agents` | ❌ | ❌ | ❌ |
| `asset_snapshots` (分区) | ❌ | ❌ | ❌ |
| `enrollment_tokens` | ❌ | ❌ | ❌ |
| `revoked_tokens` | ❌ | ❌ | ❌ |
| `refresh_tokens` | ❌ | ❌ | ❌ |
| `approval_requests` | ❌ | ❌ | ❌ |
| `archive_manifest` | ❌ | ❌ | ❌ |
| `audit_meta` | ❌ | ❌ | ❌ |

> 结论: 14 张表中 **5 张真正使用**，其余 9 张建而未用。

---

## 4. 前端实现 ↔ 后端契约

前端（React18+TS+Vite+Zustand）完成度较高：登录、路由守卫、资产表格（搜索/筛选/游标分页/详情侧滑/新建/领用/归还）、仪表盘、代理监控页。

### 前端实际调用的端点 ↔ 后端是否存在

| 前端调用 | 位置 | 后端匹配 | 状态 |
|---|---|---|---|
| `POST /auth/login` | `Login.tsx:19` | ✅ `server.go:513` | ✅ |
| `GET /assets` | `Assets.tsx:37` | ✅ | ✅ |
| `POST /assets` | `Assets.tsx:205` | ✅ | ✅ |
| `GET /assets/:id` | `Assets.tsx:59` | ✅ | ✅ |
| `PUT /assets/:id` (If-Match) | `Assets.tsx:415` | ✅ | ✅ |
| `POST /assets/:id/assign` | `Assets.tsx:748` | ✅ | ✅ |
| `POST /assets/:id/release` | `Assets.tsx:447` | ✅ | ✅ |
| `GET /assets/:id/assignments` | `Assets.tsx:388` | ✅ | ✅ |
| `GET /users` · `/users/:id` | `Assets.tsx:735,395` | ✅ | ✅ |
| `GET /dashboard/overview` | `Dashboard.tsx:9` | ✅ | ✅ |
| `GET /agents` · `/dashboard/agents` | `Agents.tsx:13` | ⚠️ 返回空/硬编码 | ⚠️ |

---

## 5. ⚠️ 高危契约 Bug（功能直接失效）

| # | 严重度 | 问题 | 证据 | 后果 |
|---|---|---|---|---|
| B1 | 🔴 | **生命周期转换 404**：前端调 `PUT /assets/:id/lifecycle`，后端是 `POST /assets/:id/transition` | `Assets.tsx:431` ↔ `server.go:467` | "可转换状态"按钮点击即失败 |
| B2 | 🔴 | **Token 刷新链断裂**：前端依赖 `POST /auth/refresh`，后端无此端点 | `client.ts:42` ↔ (无) | access token 15min 过期后刷新 404 → 强制登出 |
| B3 | 🟡 | **login 缺 refresh_token**：前端读 `data.refresh_token`，后端只返回 `access_token` | `Login.tsx:20` ↔ `server.go:532` | localStorage 存入 `undefined` |
| B4 | 🟡 | **组织内存存储 + 无锁**：生产也用内存 OrgStore，重启丢数据；map 无锁有竞争 | `organization.go:84` | 多租户数据不可靠 |
| B5 | 🟡 | **审计链空转**：audit_log 有 trigger/RLS 但无写入方，生产无 history 端点 | §3 | 审计功能不可用 |
| B6 | 🟢 | **Agents 健康字段不匹配**：前端期望 `degraded/error`，后端只返回 `{online,offline,total}` | `Agents.tsx:52` ↔ `server.go:498` | KPI 显示 undefined |
| B7 | 🟢 | **DEMO transfer 不改状态**：demo 转移只返回响应不更新资产 | `server.go:389` | demo 行为不一致 |

---

## 6. 文档 / 构建 / 脚本的具体错误

| # | 位置 | 文档声称 | 实际 | 严重度 |
|---|---|---|---|---|
| E1 | `assetserver/Dockerfile:4` | `golang:1.23-alpine` | `go.mod` 要求 `go 1.25.0` | 🔴 Docker 构建失败 |
| E2 | `Makefile:32` | migrate 用 `migrations/000001_init_schema.sql` | 实际文件 `migrations/001_init.sql` | 🟡 |
| E3 | `Makefile:19` | 注释 "SQLite 内存模式" | 实际是 Go map 内存，非 SQLite | 🟢 |
| E4 | `README.md:57` | "47 项测试全 PASS" | 静态统计 **40 个 test 函数** + 3 子测试 | 🟡 |
| E5 | `demo/README.md:11` | `python3 main.py` + FastAPI/SQLite | 实际 `python3 demo.py` + **纯 stdlib** http.server | 🟡 |
| E6 | `README.md` 技术栈表 | "Redis 7 + Sentinel, Cache-Aside, Pub/Sub" | go.mod 无 Redis 依赖，代码不连 Redis | 🟡 |
| E7 | `README.md` Phase 表 | 全部 11 Phase "✅ 完成" | 多数 Phase 仅有未接线代码或无代码 | 🟡 |
| E8 | `demo/README.md:3` | "FastAPI 验证架构" | demo.py 无第三方依赖 | 🟢 |

---

## 7. 安全问题对照（架构 §7 承诺 ↔ 实现）

| 架构承诺 | 实现 | 状态 |
|---|---|---|
| Ed25519 + kid + 密钥轮换 | Ed25519 实现；kid 有；轮换无（随机密钥每次重生） | ⚠️ |
| iss/aud/exp 校验 | ✅ 完整 | ✅ |
| 登录防暴力破解 / 锁定 | 无（硬编码凭据） | ❌ |
| 密码 bcrypt 哈希存储 | schema 有字段，登录不校验 | ❌ |
| 组织级 IDOR 防护 | OrgScope 中间件有，但可回退默认 org | ⚠️ |
| RBAC (super_admin/admin/operator/viewer) | role 存 context 不校验 | ❌ |
| MFA (super_admin) | 无 | ❌ |
| 双人审批 | 无 | ❌ |
| Webhook SSRF 防护 | 代码完整但未接线 | ❌(未用) |
| 审计不可变 (trigger+RLS+链式哈希) | schema 完整，无写入 | ⚠️ |
| 限流 | 空函数 | ❌ |
| mTLS / 证书吊销 CRL/OCSP | 仅 nginx 模板 | 📄 |

> 说明: 多数安全缺项在 DEMO/演示场景可接受，但**绝不可直接用于生产**。

---

## 8. 真实亮点（代码确实优秀处）

1. **生命周期状态机** `domain/lifecycle.go` — 转换矩阵 + 终态判断 + fuzz 测试，全库测试覆盖最好。
2. **pgx Repository** `repository/asset_repo.go` — 游标分页、CJK/英文搜索分流、乐观锁重试，工程质量高。
3. **三层锁** `internal/lock/lock.go` — 乐观/悲观/advisory + 字典序防死锁 + 碰撞检测，实现完整。
4. **audit_log schema** — 不可变 trigger + RLS + insert-only 策略，设计到位（仅缺写入方）。
5. **Webhook SSRF 防护** `internal/webhook/ssrf.go` — 私有网段 + DNS rebinding 连接后双校验，虽死代码但写得好。
6. **前端 UI** — Linear 风格暗色设计系统，组件完整度全库最高。

---

## 9. 修复优先级建议

| 优先级 | 事项 | 工作量 |
|---|---|---|
| **P0** | 修 B1/B2/B3 三个前后端契约 bug（生命周期路由、refresh 端点、login 返回） | 小 |
| **P1** | 决策死代码：cache/event/webhook/ingest 要么接线要么删除；同步修订文档 | 中 |
| **P1** | 修 Dockerfile Go 版本 (1.23→1.25)、Makefile migrate 路径 | 小 |
| **P2** | 组织树落 PG/ltree + 加锁；生产补 `/assets/:id/history` + audit_log 写入 | 中 |
| **P2** | RBAC 校验中间件（真正检查 role） | 中 |
| **P3** | 实现 refresh token 轮换、登录用户表校验 + bcrypt | 大 |
| **P3** | Agent 上报链路接线（ingest + agents/sync 端点） | 大 |

---

*本报告基于静态代码审查；本机未安装 Go，编译与测试结论以静态统计为准。*
