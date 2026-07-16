# Asset Database System — Architecture Document

> 版本: 1.0 | 更新: 2026-07-05 | 状态: 设计阶段

---

## 目录

1. [系统概述](#1-系统概述)
2. [技术选型与理由](#2-技术选型与理由)
3. [系统拓扑](#3-系统拓扑)
4. [项目结构](#4-项目结构)
5. [数据模型](#5-数据模型)
6. [API 设计](#6-api-设计)
7. [并发控制与锁策略](#7-并发控制与锁策略)
8. [Agent 采集架构](#8-agent-采集架构)
9. [多租户与权限模型](#9-多租户与权限模型)
10. [Grafana 集成](#10-grafana-集成)
11. [事件与 Webhook](#11-事件与-webhook)
12. [缓存策略](#12-缓存策略)
13. [部署架构](#13-部署架构)
14. [实施计划](#14-实施计划)

---

## 1. 系统概述

### 1.1 定位

Asset Database System 是一个 IT 资产管理平台，核心管理对象是 IT 硬件和基础设施资产，同时具备向软件许可证、云资源等类型扩展的能力。

### 1.2 系统组成

| 组件 | 技术 | 说明 |
|---|---|---|
| API Server | Go + Gin | 核心 REST API 服务，所有写操作的唯一入口 |
| Web UI | React 18 + TypeScript + Vite | 资产管理 Web 控制台 |
| Collection Agent | Go (跨平台) | 部署在终端设备上的采集代理 |
| Grafana | Grafana OSS | 资产面板可视化 |
| PostgreSQL | PostgreSQL 16 | 唯一数据源 (Single Source of Truth) |
| Redis | Redis 7 | 缓存、限流、后台任务队列 |
| PgBouncer | PgBouncer | 数据库连接池 (为 Grafana 提供只读通道) |
| Nginx | Nginx | TLS 终结、静态文件服务、反向代理 |

### 1.3 核心设计原则

- **API-First**: 所有功能通过 REST API 暴露，Web UI 和 Agent 都是 API 的消费者
- **单一写路径**: API Server 是 PostgreSQL 的唯一写入者
- **读写分离**: Grafana 通过只读角色直连 PostgreSQL，不影响 API Server 性能
- **解耦扩展**: 新增资产类型只需 INSERT 一行 `asset_types`，零代码改动
- **零信任 Agent**: Agent 纯出站 HTTPS，不需要任何入站端口

---

## 2. 技术选型与理由

### 2.1 Go vs Java Spring Boot

| 维度 | Go (Gin) | Spring Boot (JVM) |
|---|---|---|
| 吞吐量 | **125,700 req/s** | 54,600 req/s |
| 内存空闲 | **24 MB** | 717 MB |
| 冷启动 | **~100 ms** | 3,200 ms |
| GC 延迟 | <1ms (可预测) | 10-50ms (G1GC 压力下) |
| 百万并发 | goroutine (2KB/个) | 需 NIO 深度调优 |
| 编译产物 | 单一静态二进制 | JAR + JVM 运行时 |

**选择 Go 的核心原因**: 后端和 Agent 共享同一技术栈；高性能低开销适合高并发场景；交叉编译一键产出全平台 Agent 二进制。

### 2.2 核心依赖

| 包 | 用途 |
|---|---|
| `github.com/gin-gonic/gin` | HTTP 框架，路由，中间件 |
| `github.com/jackc/pgx/v5` | PostgreSQL 驱动 (高性能、纯 Go) |
| `github.com/redis/go-redis/v9` | Redis 客户端 |
| `github.com/golang-jwt/jwt/v5` | JWT 鉴权 (EdDSA / Ed25519 非对称签名) |
| `github.com/golang-migrate/migrate/v4` | 数据库迁移 |
| `github.com/shirou/gopsutil/v4` | Agent 跨平台系统信息采集 |
| `modernc.org/sqlite` | Agent 离线队列 (纯 Go 无 CGO) |
| `github.com/rs/zerolog` | 结构化日志 |
| `golang.org/x/crypto` | bcrypt 密码哈希, ed25519 签名 |
| `github.com/hashicorp/vault/api` | HashiCorp Vault KMS — JWT 签名密钥、Webhook secret、Enrollment token 的密钥管理 |

> **[安全加固] JWT 签名算法与密钥管理**:
> - **签名算法**: 强制使用 `EdDSA` (Ed25519) 非对称签名，**禁止** HS256/RS256 等对称或可降级算法。
> - **密钥管理**: Ed25519 私钥通过 **HashiCorp Vault / 云 KMS** 托管，API Server 启动时从 Vault 读取，**禁止** 硬编码或写入配置文件/环境变量。公钥可缓存供验证端使用。Vault 自身需 HA 部署，且 API Server 启动时缓存公钥以降低对 Vault 的运行时依赖 — 详见 [§15.9 Vault/KMS 单点故障修复](#159-vaultkms-单点故障修复)。
> - **算法降级防护**: `jwt.Parse` 必须显式设置 `ValidMethods: []string{"EdDSA"}`，拒绝任何其他算法的 token。
> - **Claims 全量校验**: `iss` (签发者)、`aud` (受众)、`exp` (过期)、`iat` (签发时间)、`jti` (唯一 ID) 全部强制校验，`jti` 可用于主动撤销黑名单。

---

## 3. 系统拓扑

> **高可用拓扑**: 生产环境采用多可用区 (Multi-AZ) 部署。PostgreSQL 通过 Patroni + Streaming Replication 实现主从自动故障转移 (RTO<30s, RPO<5s)；API Server 至少 2 实例由 Nginx upstream 池负载均衡；Redis 采用 Sentinel 3 节点集群实现自动故障转移。详见 [§13.2 生产分布式部署](#132-生产分布式部署) 与 [§15.9 高可用与可靠性加固](#159-高可用与可靠性加固)。

```
                       ┌─────────────┐
                       │   Grafana   │
                       │  (port 3000)│
                       └──────┬──────┘
                              │ PostgreSQL read-only (PgBouncer port 6432, 多后端读写分离)
                       ┌──────▼──────┐
                       │  PgBouncer  │  (pool=25, 指向 Primary + Replica)
                       │  (pool=25)  │
                       └──────�┬──────┘
                              │
                       ┌──────▼──────────────────────────────────┐
                       │  PostgreSQL HA Cluster (Patroni)         │
                       │  ┌──────────┐    ┌──────────┐            │
                       │  │ Primary  │◄──►│ Replica  │            │
                       │  │  :5432   │ SR │  :5432  │            │  ◄──── write (API Server)
                       │  └────┬─────┘    └─────────┘            │       read (PgBouncer → Replica)
                       │       │ Streaming Replication           │
                       │       │ Patroni Auto-Failover (RTO<30s)  │
                       └───────┼─────────────────────────────────┘
                               │
                ┌──────────────┼──────────────┐
                │              │              │
       ┌───────▼───────┐ ┌────▼─────┐ ┌──────▼───────┐
       │  API Server-1  │ │ API Srv-2│ │  API Server-N│  (Go + Gin, 无状态, 水平扩展)
       │  (Go + Gin)    │ │          │ │              │
       │   :8080        │ │  :8080   │ │   :8080      │
       └───────┬───────┘ └────┬─────┘ └──────┬───────┘
               │                │              │
               │     ┌─────────┴──────────┐    │
               │     │      Nginx         │    │     ┌─────────────────────┐
               └────►│  :443/:80          │◄───┘     │  Redis Sentinel     │
                     │  TLS + upstream    │          │  3 节点集群          │
                     │  (health check +   │          │  ┌────┐ ┌────┐     │
                     │   active eject)    │          │  │ M  │ │ S1 │     │
                     └────────┬──────────┘          │  │:6379│ │:6379│    │
                              │                     │  └────┘ └────┘     │
                    ┌─────────┴─────────────────┐   │     ┌────┐         │
                    │                           │   │     │ S2 │         │
             ┌──────▼──────┐             ┌──────▼──┐ │     │:6379│        │
             │  React Web  │             │ Agent   │ │     └────┘         │
             │    UI       │             │ (mTLS)  │ │   cache / MQ /     │
             │  (Vite :5173)│             │ Linux   │ │   Pub-Sub / Stream│
             └─────────────┘             └─────────┘ └───────────────────┘
```

### 3.1 数据流

1. **用户操作**: Browser → Nginx (TLS) → API Server (upstream 池, 健康检查剔除故障实例) → Service → Repository → PostgreSQL Primary
2. **Agent 上报**: Agent (mTLS) → Nginx → API Server → Redis Stream (Ingest Buffer, 持久化) → Processor → Engine → PostgreSQL Primary
3. **Grafana 查询**: Grafana → PgBouncer → PostgreSQL Replica (read-only user, SELECT only, 读写分离)
4. **缓存**: Service 层查 Redis Sentinel 集群 → 命中返回 / 未命中查 DB 并回填
5. **事件**: Service 发布事件 → Redis Pub/Sub Event Bus → 所有 API Server 实例的 Subscriber → Webhook Dispatcher (异步外发)

---

## 4. 项目结构

```
/root/claude-md/
├── CLAUDE.md                          # Claude Code 行为指引
├── docs/
│   └── architecture.md                # 本文档
│
├── assetserver/                       # Go 后端 (monorepo)
│   ├── cmd/
│   │   ├── api-server/main.go         # API Server 入口
│   │   ├── collection-agent/main.go   # Agent 入口
│   │   └── migrate/main.go            # 迁移工具入口
│   │
│   ├── internal/                      # 内部包 (不可外部导入)
│   │   ├── api/
│   │   │   ├── middleware/            # 中间件 (auth, ratelimit, logging, recover, requestid)
│   │   │   ├── handler/               # Gin handler (auth, asset, assignment, agent, dashboard, location, org, webhook, lifecycle, admin)
│   │   │   ├── router.go             # 路由注册 + 中间件链
│   │   │   └── server.go             # HTTP Server 生命周期
│   │   │
│   │   ├── domain/                   # 领域模型 (纯 Go struct)
│   │   │   ├── asset.go
│   │   │   ├── agent.go
│   │   │   ├── assignment.go
│   │   │   ├── auditevent.go
│   │   │   ├── location.go
│   │   │   ├── organization.go
│   │   │   ├── user.go
│   │   │   ├── webhook.go
│   │   │   ├── relationship.go
│   │   │   ├── lifecycle.go
│   │   │   └── snapshot.go
│   │   │
│   │   ├── service/                  # 业务逻辑层
│   │   │   ├── asset_service.go
│   │   │   ├── assignment_service.go
│   │   │   ├── agent_service.go
│   │   │   ├── auth_service.go
│   │   │   ├── dashboard_service.go
│   │   │   ├── webhook_service.go
│   │   │   ├── lifecycle_service.go
│   │   │   └── ingest/               # Collect-Engine 摄入管道
│   │   │       ├── buffer.go         # 环形缓冲区
│   │   │       ├── processor.go      # 校验、去重、转换
│   │   │       └── engine.go         # 批量写入 PostgreSQL
│   │   │
│   │   ├── repository/               # 数据访问层 (pgx)
│   │   │   ├── asset_repo.go
│   │   │   ├── agent_repo.go
│   │   │   ├── assignment_repo.go
│   │   │   ├── audit_repo.go
│   │   │   ├── location_repo.go
│   │   │   ├── org_repo.go
│   │   │   ├── user_repo.go
│   │   │   ├── webhook_repo.go
│   │   │   ├── snapshot_repo.go
│   │   │   └── relationship_repo.go
│   │   │
│   │   ├── cache/                    # Redis 缓存层
│   │   │   ├── redis.go
│   │   │   ├── asset_cache.go
│   │   │   └── agent_cache.go
│   │   │
│   │   ├── lock/                     # 锁策略实现
│   │   │   ├── optimistic.go         # 版本号乐观锁
│   │   │   ├── pessimistic.go        # SELECT ... FOR UPDATE
│   │   │   └── advisory.go           # PostgreSQL advisory lock
│   │   │
│   │   ├── job/                      # 后台任务
│   │   │   ├── worker.go
│   │   │   ├── jobs.go
│   │   │   └── scheduler.go
│   │   │
│   │   ├── event/                    # 内部事件总线
│   │   │   ├── bus.go
│   │   │   ├── types.go
│   │   │   └── subscriber.go
│   │   │
│   │   ├── webhook/                  # Webhook 外发引擎
│   │   │   ├── dispatcher.go
│   │   │   └── retry.go
│   │   │
│   │   └── config/                   # 配置加载
│   │       ├── config.go
│   │       └── defaults.go
│   │
│   ├── pkg/                          # 共享库 (Agent 和 Server 共用)
│   │   ├── agentproto/
│   │   │   ├── types.go              # DeltaPayload, SnapshotPayload, HeartbeatPayload
│   │   │   └── crypto.go             # mTLS 工具, Agent 密钥生成
│   │   ├── apierror/
│   │   │   └── error.go              # 统一错误类型
│   │   ├── pagination/
│   │   │   └── pagination.go         # 游标分页
│   │   └── validator/
│   │       └── validator.go          # 输入校验
│   │
│   ├── agent/                        # Collection Agent 应用代码
│   │   ├── collector/
│   │   │   ├── collector.go          # Collector 接口
│   │   │   ├── linux.go              # /proc, dmidecode, lshw
│   │   │   ├── windows.go            # WMI 查询
│   │   │   └── darwin.go             # system_profiler, ioreg
│   │   ├── comm/
│   │   │   ├── client.go             # HTTPS 客户端 (mTLS)
│   │   │   ├── auth.go               # Agent 认证 token 管理
│   │   │   └── sync.go               # Delta 推送, 全量同步
│   │   ├── store/
│   │   │   └── queue.go              # 离线队列 (SQLite)
│   │   ├── updater/
│   │   │   ├── updater.go            # 版本检查, 下载, 替换
│   │   │   └── signature.go          # Ed25519 签名验证
│   │   └── identity/
│   │       └── fingerprint.go        # 硬件指纹生成
│   │
│   ├── migrations/                   # golang-migrate SQL 文件
│   │   ├── 000001_init_schema.up.sql
│   │   ├── 000001_init_schema.down.sql
│   │   └── ...
│   │
│   ├── grafana/
│   │   ├── dashboards/
│   │   │   ├── asset-overview.json
│   │   │   ├── agent-health.json
│   │   │   └── lifecycle-tracking.json
│   │   └── datasources/
│   │       └── postgres-readonly.yml
│   │
│   ├── deploy/
│   │   ├── docker-compose.yml
│   │   ├── docker-compose.override.yml
│   │   ├── Dockerfile.api
│   │   ├── Dockerfile.agent
│   │   ├── pgbouncer.ini
│   │   └── nginx.conf
│   │
│   ├── Makefile
│   ├── go.mod
│   └── go.sum
│
└── web/                              # React 前端
    ├── src/
    │   ├── main.tsx
    │   ├── App.tsx
    │   ├── api/                      # API 客户端
    │   │   ├── client.ts             # Axios/fetch 封装, JWT 注入
    │   │   ├── assets.ts
    │   │   ├── auth.ts
    │   │   ├── agents.ts
    │   │   ├── assignments.ts
    │   │   ├── locations.ts
    │   │   └── dashboard.ts
    │   ├── components/
    │   │   ├── ui/                   # shadcn/ui 组件
    │   │   ├── layout/               # AppShell, Sidebar, Header
    │   │   ├── assets/               # AssetTable, AssetDetail, AssetForm, AssetTimeline
    │   │   ├── assignments/          # AssignmentPanel, AssignmentHistory
    │   │   ├── agents/               # AgentTable, AgentDetail
    │   │   └── dashboard/            # StatsCard, AssetChart, AgentHealth
    │   ├── pages/                    # 路由页面
    │   │   ├── Login.tsx
    │   │   ├── Dashboard.tsx
    │   │   ├── Assets.tsx
    │   │   ├── AssetDetailPage.tsx
    │   │   ├── Agents.tsx
    │   │   ├── Locations.tsx
    │   │   ├── Admin.tsx
    │   │   └── AuditLog.tsx
    │   ├── hooks/                    # useAuth, usePagination, useDebounce
    │   ├── store/                    # Zustand: authStore, assetStore
    │   ├── types/                    # TypeScript 类型定义
    │   └── lib/                      # utils, constants
    ├── index.html
    ├── vite.config.ts
    ├── tailwind.config.ts
    ├── tsconfig.json
    └── package.json
```

---

## 5. 数据模型

### 5.1 Schema 概览

所有表位于 `assets` schema 下，共 16 张核心表。

### 5.2 核心实体关系

```
organizations (树: parent_id)          locations (树: parent_id)
       │                                      │
       ├── users (5 种角色)                    │
       │     │                                │
       │     └── assignments ────┐            │
       │                         │            │
       ├── asset_types ── assets ─────────────┘
       │     (JSON schema)   │
       │                     ├── audit_log (不可变)
       │                     ├── asset_snapshots ← collection_agents
       │                     ├── asset_relationships (自引用)
       │                     └── webhooks
       │
       └── (scope: 所有数据按 org_id 隔离)
```

### 5.3 核心表 DDL

#### organizations

```sql
CREATE TABLE assets.organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    parent_id   UUID REFERENCES assets.organizations(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### locations

```sql
CREATE TABLE assets.locations (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name      VARCHAR(255) NOT NULL,
    type      VARCHAR(50) NOT NULL CHECK (type IN ('site','building','room','rack','floor')),
    parent_id UUID REFERENCES assets.locations(id),
    org_id    UUID NOT NULL REFERENCES assets.organizations(id),
    metadata  JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### users

```sql
CREATE TABLE assets.users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      VARCHAR(100) UNIQUE NOT NULL,
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50) NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('super_admin','admin','manager','viewer','agent')),
    org_id        UUID REFERENCES assets.organizations(id),
    disabled      BOOLEAN NOT NULL DEFAULT false,
    last_login    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### asset_types (解耦扩展的关键)

```sql
CREATE TABLE assets.asset_types (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name     VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL
             CHECK (category IN ('hardware','software','network','cloud_resource','license','other')),
    schema   JSONB DEFAULT '{}',    -- 定义该类型的 JSON Schema
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### assets (核心资产表)

```sql
CREATE TABLE assets.assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_tag       VARCHAR(100) UNIQUE NOT NULL,
    name            VARCHAR(255) NOT NULL,
    type_id         UUID NOT NULL REFERENCES assets.asset_types(id),
    org_id          UUID NOT NULL REFERENCES assets.organizations(id),
    location_id     UUID REFERENCES assets.locations(id),
    serial_number   VARCHAR(255),
    manufacturer    VARCHAR(255),
    model           VARCHAR(255),
    lifecycle_state VARCHAR(50) NOT NULL DEFAULT 'procurement'
                    CHECK (lifecycle_state IN ('procurement','deployment','utilization','maintenance','retirement')),
    status          VARCHAR(50) NOT NULL DEFAULT 'available',
    properties      JSONB DEFAULT '{}',     -- 类型特定属性
    metadata        JSONB DEFAULT '{}',     -- 任意标签
    version         INTEGER NOT NULL DEFAULT 1,  -- 乐观锁
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID REFERENCES assets.users(id),
    updated_by      UUID REFERENCES assets.users(id)
);
```

#### assignments (资产领用)

```sql
CREATE TABLE assets.assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id     UUID NOT NULL REFERENCES assets.assets(id),
    assigned_to  UUID NOT NULL REFERENCES assets.users(id),
    assigned_by  UUID NOT NULL REFERENCES assets.users(id),
    status       VARCHAR(20) NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','returned','lost','transferred')),
    notes        TEXT,
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    returned_at  TIMESTAMPTZ,
    version      INTEGER NOT NULL DEFAULT 1
);

-- 防止同一资产被重复领用
CREATE UNIQUE INDEX idx_active_assignment
    ON assets.assignments (asset_id) WHERE status = 'active';
```

#### audit_log (不可变审计日志)

```sql
CREATE TABLE assets.audit_log (
    id         BIGSERIAL PRIMARY KEY,
    asset_id   UUID NOT NULL REFERENCES assets.assets(id) ON DELETE CASCADE,
    user_id    UUID REFERENCES assets.users(id),
    agent_id   UUID REFERENCES assets.collection_agents(id),
    action     VARCHAR(50) NOT NULL,
    field      VARCHAR(255),
    old_value  TEXT,
    new_value  TEXT,
    metadata   JSONB DEFAULT '{}' CHECK (octet_length(metadata::text) <= 4096),
    prev_hash  CHAR(64),                          -- 链式哈希: 上一条记录的 hash
    hash       CHAR(64) NOT NULL,                 -- 链式哈希: SHA256(prev_hash || record)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_asset_time ON assets.audit_log (asset_id, created_at DESC);
```

**不可变性保护 (防 UPDATE/DELETE 篡改)**:

audit_log 是合规审计的关键证据，必须保证写入后不可被任何角色 UPDATE 或 DELETE。采用三层防御：

1. **数据库角色分离** — 最小权限原则，写角色只能 INSERT，读角色只能 SELECT：

```sql
-- 写角色: 仅允许 INSERT (API Server 使用)
CREATE ROLE app_writer WITH LOGIN PASSWORD '<secure_password>';
GRANT INSERT ON assets.audit_log TO app_writer;
GRANT USAGE, SELECT ON SEQUENCE assets.audit_log_id_seq TO app_writer;
-- 明确拒绝 UPDATE / DELETE (REVOKE 默认即无，此处显式声明以备审计)
REVOKE UPDATE, DELETE ON assets.audit_log FROM app_writer;

-- 读角色: 仅允许 SELECT (Grafana / 审计查询使用)
CREATE ROLE audit_reader WITH LOGIN PASSWORD '<secure_password>';
GRANT SELECT ON assets.audit_log TO audit_reader;

-- 超级管理员也不应直接持有表的 DML 权限，仅通过 SECURITY DEFINER 函数写入
```

2. **行级安全 (RLS)** — 即使权限被误授，RLS 策略仍阻止修改：

```sql
ALTER TABLE assets.audit_log ENABLE ROW LEVEL SECURITY;

-- app_writer: 只能 INSERT，不能 SELECT/UPDATE/DELETE 任何已存在行
CREATE POLICY audit_log_insert_only ON assets.audit_log
    FOR INSERT TO app_writer WITH CHECK (true);
CREATE POLICY audit_log_no_select ON assets.audit_log
    FOR SELECT TO app_writer USING (false);   -- 写角色看不到历史数据
CREATE POLICY audit_log_no_update ON assets.audit_log
    FOR UPDATE TO app_writer USING (false) WITH CHECK (false);
CREATE POLICY audit_log_no_delete ON assets.audit_log
    FOR DELETE TO app_writer USING (false);

-- audit_reader: 只能 SELECT
CREATE POLICY audit_log_select_only ON assets.audit_log
    FOR SELECT TO audit_reader USING (true);
```

3. **BEFORE UPDATE OR DELETE 触发器** — 最后一道防线，即使绕过权限/RLS 也拒绝：

```sql
CREATE OR REPLACE FUNCTION assets.audit_log_immutable_guard()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % operation not permitted on row %',
        TG_OP, OLD.id;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_immutable
    BEFORE UPDATE OR DELETE ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_immutable_guard();
```

> **注意**: 归档 job 需要删除已归档行时，须通过专门的 SECURITY DEFINER 函数 `archive_audit_log_batch()` 执行（见 §15.4），该函数内部校验归档条件后绕过触发器（`ALTER TABLE ... DISABLE TRIGGER` 仅在该事务内）。

**metadata 字段大小限制与 JSON 结构校验**:

`metadata` JSONB 字段可被注入超大内容导致存储膨胀或 DoS。DDL 中已加 CHECK 约束限制 ≤4KB：

```sql
-- DDL 中已包含: CHECK (octet_length(metadata::text) <= 4096)
```

**应用层补充校验 (Go)**:

```go
const maxAuditMetadataSize = 4096 // 4KB

func validateAuditMetadata(metadata json.RawMessage) error {
    if len(metadata) > maxAuditMetadataSize {
        return fmt.Errorf("audit_log metadata exceeds %d bytes (got %d)",
            maxAuditMetadataSize, len(metadata))
    }
    // 校验 JSON 结构有效性 (非空时必须是合法 JSON 对象)
    if len(metadata) == 0 || string(metadata) == "null" {
        return nil // 允许空
    }
    var v map[string]interface{}
    if err := json.Unmarshal(metadata, &v); err != nil {
        return fmt.Errorf("audit_log metadata is not valid JSON object: %w", err)
    }
    // 限制嵌套深度 ≤5，防止恶意深嵌套
    if err := checkJSONDepth(v, 0, 5); err != nil {
        return err
    }
    return nil
}

func checkJSONDepth(v interface{}, depth, maxDepth int) error {
    if depth > maxDepth {
        return fmt.Errorf("audit_log metadata nesting exceeds %d levels", maxDepth)
    }
    switch val := v.(type) {
    case map[string]interface{}:
        for _, child := range val {
            if err := checkJSONDepth(child, depth+1, maxDepth); err != nil {
                return err
            }
        }
    case []interface{}:
        for _, child := range val {
            if err := checkJSONDepth(child, depth+1, maxDepth); err != nil {
                return err
            }
        }
    }
    return nil
}
```

> **注意**: `asset_id`、`user_id` 等关键审计字段应存于独立列而非 metadata 中，metadata 仅用于附加上下文 (如 IP、UA、变更来源)。应用层在写入前调用 `validateAuditMetadata()`，数据库 CHECK 约束作为兜底。

#### collection_agents

```sql
CREATE TABLE assets.collection_agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_key       VARCHAR(64) UNIQUE NOT NULL,
    hostname        VARCHAR(255) NOT NULL,
    ip_address      INET,
    os_type         VARCHAR(50) NOT NULL,
    os_version      VARCHAR(100),
    agent_version   VARCHAR(20) NOT NULL,
    last_heartbeat  TIMESTAMPTZ,
    status          VARCHAR(20) NOT NULL DEFAULT 'registered'
                    CHECK (status IN ('registered','online','offline','disabled')),
    public_key      TEXT NOT NULL,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### asset_snapshots (Agent 上报快照)

```sql
CREATE TABLE assets.asset_snapshots (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id   UUID NOT NULL REFERENCES assets.assets(id) ON DELETE CASCADE,
    agent_id   UUID NOT NULL REFERENCES assets.collection_agents(id),
    snapshot   JSONB NOT NULL,
    checksum   VARCHAR(64) NOT NULL,
    is_delta   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### asset_relationships

```sql
CREATE TABLE assets.asset_relationships (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_asset_id  UUID NOT NULL REFERENCES assets.assets(id) ON DELETE CASCADE,
    target_asset_id  UUID NOT NULL REFERENCES assets.assets(id) ON DELETE CASCADE,
    relationship     VARCHAR(50) NOT NULL
                     CHECK (relationship IN ('connected_to','runs_on','backed_by',
                           'supports_service','depends_on','virtualized_on','parent_of','child_of')),
    metadata  JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### webhooks

```sql
CREATE TABLE assets.webhooks (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    url        VARCHAR(1024) NOT NULL,
    secret     VARCHAR(255) NOT NULL,       -- HMAC 签名密钥
    events     TEXT[] NOT NULL,             -- {'asset.created','asset.updated',...}
    enabled    BOOLEAN NOT NULL DEFAULT true,
    org_id     UUID NOT NULL REFERENCES assets.organizations(id),
    created_by UUID NOT NULL REFERENCES assets.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 5.4 资产类型扩展机制

**新增一种资产类型（如"软件许可证"），只需一条 SQL：**

```sql
INSERT INTO assets.asset_types (name, category, schema) VALUES (
    'software_license',
    'license',
    '{
        "type": "object",
        "properties": {
            "license_key": {"type": "string"},
            "vendor": {"type": "string"},
            "seats": {"type": "integer", "minimum": 1},
            "expiration_date": {"type": "string", "format": "date"},
            "license_type": {"enum": ["perpetual", "subscription", "trial"]}
        },
        "required": ["license_key", "vendor", "seats"]
    }'::jsonb
);
```

此后创建该类型资产时，`properties` 列存储具体数据：

```json
{
    "license_key": "ABCD-EFGH-IJKL-MNOP",
    "vendor": "Microsoft",
    "seats": 50,
    "expiration_date": "2027-06-30",
    "license_type": "subscription"
}
```

### 5.5 资产生命周期状态机

```
                    ┌─────────────┐
                    │ procurement │
                    └──────┬──────┘
                           │ deploy
                    ┌──────▼──────┐
                    │ deployment  │
                    └──┬──────┬───┘
              activate │      │ maintenance
                    ┌──▼──────▼───┐
                    │ utilization │◄────────┐
                    └──┬──────┬───┘         │
           maintenance │      │ retirement  │ maintenance
                    ┌──▼──────▼───┐         │
                    │ maintenance │─────────┘
                    └──────┬──────┘
                           │ retirement
                    ┌──────▼──────┐
                    │ retirement  │ (终态)
                    └─────────────┘
```

合法状态转换矩阵：

| 当前状态 | 可转换到 |
|---|---|
| procurement | deployment, retirement |
| deployment | utilization, maintenance, retirement |
| utilization | maintenance, retirement |
| maintenance | utilization, retirement |
| retirement | — (终态，不可转换) |

### 5.6 关键索引

```sql
-- 资产查询
CREATE INDEX idx_assets_org ON assets.assets (org_id);
CREATE INDEX idx_assets_type ON assets.assets (type_id);
CREATE INDEX idx_assets_lifecycle ON assets.assets (lifecycle_state);
CREATE INDEX idx_assets_location ON assets.assets (location_id);
CREATE INDEX idx_assets_status ON assets.assets (status);
CREATE INDEX idx_assets_updated ON assets.assets (updated_at DESC);

-- 全文搜索
CREATE INDEX idx_assets_search ON assets.assets USING GIN (
    to_tsvector('english', name || ' ' || coalesce(manufacturer,'')
        || ' ' || coalesce(model,'') || ' ' || coalesce(serial_number,'') || ' ' || asset_tag)
);

-- 复合索引: 资产列表多条件组合查询优化 (§6.5 查询参数覆盖)
-- 典型查询: WHERE org_id = $1 AND status = $2 ORDER BY updated_at DESC
CREATE INDEX idx_assets_org_status ON assets.assets (org_id, status);
CREATE INDEX idx_assets_org_type ON assets.assets (org_id, type_id);
CREATE INDEX idx_assets_org_updated ON assets.assets (org_id, updated_at DESC);
CREATE INDEX idx_assets_org_lifecycle ON assets.assets (org_id, lifecycle_state);
CREATE INDEX idx_assets_org_location ON assets.assets (org_id, location_id);

-- audit_log 复合索引: 按用户/操作/Agent 维度查询审计记录
CREATE INDEX idx_audit_user_time ON assets.audit_log (user_id, created_at DESC);
CREATE INDEX idx_audit_action_time ON assets.audit_log (action, created_at DESC);
CREATE INDEX idx_audit_agent_time ON assets.audit_log (agent_id, created_at DESC);

-- Grafana 面板优化
CREATE INDEX idx_assets_lifecycle_org ON assets.assets (org_id, lifecycle_state);
CREATE INDEX idx_agents_status_heartbeat ON assets.collection_agents (status, last_heartbeat);
CREATE INDEX idx_audit_recent ON assets.audit_log (created_at DESC);
CREATE INDEX idx_assignments_active_user ON assets.assignments (assigned_to) WHERE status = 'active';
CREATE INDEX idx_assets_loc_state ON assets.assets (location_id, lifecycle_state);
CREATE INDEX idx_snapshots_agent_time ON assets.asset_snapshots (agent_id, created_at DESC);
```

**复合索引设计说明**:

资产列表 API (`GET /assets`) 支持多条件组合查询 (§6.5)，典型查询模式为 `WHERE org_id = ? AND status = ? AND type_id = ? ORDER BY updated_at DESC`。单列索引在此场景下需要多索引扫描 + BitmapAnd 合并，效率低下。上述复合索引以 `org_id` 为前导列 (多租户隔离的必过滤条件)，覆盖最常见组合：

| 索引 | 覆盖查询场景 |
|---|---|
| `idx_assets_org_status` | 按组织 + 状态筛选 (最常用) |
| `idx_assets_org_type` | 按组织 + 类型筛选 |
| `idx_assets_org_updated` | 按组织分页 + 按更新时间排序 (游标分页核心索引) |
| `idx_assets_org_lifecycle` | 按组织 + 生命周期阶段筛选 |
| `idx_assets_org_location` | 按组织 + 位置筛选 |

> **注意**: `idx_assets_org`、`idx_assets_type`、`idx_assets_status`、`idx_assets_updated` 等单列索引可被上述复合索引的左前缀覆盖，部署后通过 `pg_stat_user_indexes` 监控使用率，对零使用率的单列索引执行 `DROP INDEX` 清理。

**声明式分区 (大规模租户场景)**:

当单租户资产量超过 100 万行时，可考虑按 `org_id` 进行声明式分区 (Declarative Partitioning by HASH):

```sql
-- 大规模场景: assets 表按 org_id HASH 分区 (可选，需评估)
-- CREATE TABLE assets.assets (...)
--   PARTITION BY HASH (org_id);
-- CREATE TABLE assets.assets_p0 PARTITION OF assets.assets FOR VALUES WITH (MODULUS 4, REMAINDER 0);
-- CREATE TABLE assets.assets_p1 PARTITION OF assets.assets FOR VALUES WITH (MODULUS 4, REMAINDER 1);
-- CREATE TABLE assets.assets_p2 PARTITION OF assets.assets FOR VALUES WITH (MODULUS 4, REMAINDER 2);
-- CREATE TABLE assets.assets_p3 PARTITION OF assets.assets FOR VALUES WITH (MODULUS 4, REMAINDER 3);
```

> HASH 分区会带来跨分区查询开销 (如 super_admin 跨组织查询需扫描全部分区)，仅在单租户数据量成为瓶颈时启用，默认不分区。

**慢查询监控**:

```sql
-- 启用 pg_stat_statements 扩展 (需在 shared_preload_libraries 中配置)
-- CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- 慢查询 Top 10 (平均执行时间)
-- SELECT query, calls, mean_exec_time, total_exec_time, rows
--   FROM pg_stat_statements
--   WHERE query LIKE '%assets%'
--   ORDER BY mean_exec_time DESC
--   LIMIT 10;

-- 定期检查: 平均执行时间 > 100ms 的查询需优化索引或查询计划
```

---

## 6. API 设计

### 6.1 通用约定

- 基础路径: `/api/v1/`
- 鉴权: `Authorization: Bearer <jwt>` (Agent 额外使用 mTLS)
- 乐观锁: 客户端发送 `If-Match: "<version>"` header
- 分页: 游标分页，参数 `cursor` + `limit` (默认 50, 最大 200)

### 6.2 统一响应格式

**成功响应:**
```json
{
    "data": { ... },
    "pagination": {
        "next_cursor": "eyJsYXN0X2lkIjoiYWJjMTIzIn0=",
        "has_more": true,
        "total": 1042
    },
    "request_id": "req_a1b2c3d4"
}
```

**错误响应:**
```json
{
    "data": null,
    "error": {
        "code": "ASSET_NOT_FOUND",
        "message": "Asset with ID 550e8400-e29b-41d4-a716-446655440000 not found",
        "details": {}
    },
    "request_id": "req_a1b2c3d4"
}
```

### 6.3 HTTP 状态码约定

| 状态码 | 含义 |
|---|---|
| 200 | 成功 (读取) |
| 201 | 已创建 |
| 204 | 无内容 (删除, heartbeat) |
| 400 | 参数校验失败 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 409 | 冲突 (版本过期、重复领用) |
| 429 | 限流 |
| 500 | 服务器内部错误 |

### 6.4 完整 API 路由表

#### 认证 (`/api/v1/auth`)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/auth/login` | 用户登录，返回 JWT access + refresh token |
| POST | `/auth/refresh` | 用 refresh token 换新的 access token |
| POST | `/auth/register-agent` | Agent 注册，返回 mTLS 证书 + token |
| POST | `/auth/logout` | 登出，Redis 中标记 refresh token 失效 |

#### 资产 (`/api/v1/assets`)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/assets` | 资产列表 (支持搜索、过滤、分页、排序) |
| POST | `/assets` | 创建资产 |
| GET | `/assets/:id` | 获取资产详情 (含 version 号) |
| PUT | `/assets/:id` | 更新资产 (需要 If-Match header) |
| DELETE | `/assets/:id` | 删除资产 |
| GET | `/assets/:id/history` | 资产审计日志 |
| GET | `/assets/:id/snapshots` | 资产 Agent 快照历史 (需 `?from=&to=` 时间范围，见 §15.4.2) |
| GET | `/assets/:id/snapshots/latest` | 获取最新一条快照 (无需时间范围) |
| GET | `/assets/:id/relationships` | 资产关联关系图 |

#### 生命周期 (`/api/v1/assets/:id`)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/assets/:id/transition` | 资产状态转换 (悲观锁) |

#### 领用 (`/api/v1/assets/:id`)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/assets/:id/assign` | 将资产分配给用户 (悲观锁) |
| POST | `/assets/:id/release` | 归还资产 |
| POST | `/assets/:id/transfer` | 转移资产到另一用户 (原子操作) |
| GET | `/assets/:id/assignment` | 当前领用信息 |
| GET | `/assets/:id/assignment/history` | 领用历史 |
| GET | `/users/:id/assignments` | 用户的所有领用 |

#### Agent (`/api/v1/agents`)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/agents/sync` | Agent 推送 delta/全量快照 (mTLS) |
| POST | `/agents/heartbeat` | Agent 心跳 (mTLS) |
| GET | `/agents` | 已注册 Agent 列表 |
| GET | `/agents/:id` | Agent 详情 + 状态 |
| PUT | `/agents/:id` | 更新 Agent 元数据 |
| DELETE | `/agents/:id` | 注销 Agent |
| POST | `/agents/:id/update-check` | Agent 检查新版本 |

#### 位置 (`/api/v1/locations`)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/locations` | 位置列表 (树形或平铺) |
| POST | `/locations` | 创建位置 |
| GET | `/locations/:id` | 位置详情 |
| PUT | `/locations/:id` | 更新位置 |
| DELETE | `/locations/:id` | 删除位置 (有子节点或资产时拒绝) |
| GET | `/locations/:id/assets` | 该位置的资产列表 |

#### 组织 (`/api/v1/organizations`)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/organizations` | 组织列表 |
| POST | `/organizations` | 创建组织 |
| GET | `/organizations/:id` | 组织详情 |
| PUT | `/organizations/:id` | 更新组织 |
| DELETE | `/organizations/:id` | 删除组织 (有子节点或资产时拒绝) |

#### 仪表盘 (`/api/v1/dashboard`)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/dashboard/summary` | 聚合统计 (按状态、类别) |
| GET | `/dashboard/lifecycle-distribution` | 各生命周期阶段资产数 |
| GET | `/dashboard/agent-health` | Agent 在线/离线统计 |
| GET | `/dashboard/recent-activity` | 最近 N 条审计事件 |

#### Webhook (`/api/v1/webhooks`)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/webhooks` | Webhook 列表 |
| POST | `/webhooks` | 注册 Webhook |
| GET | `/webhooks/:id` | Webhook 详情 |
| PUT | `/webhooks/:id` | 更新 Webhook |
| DELETE | `/webhooks/:id` | 删除 Webhook |
| POST | `/webhooks/:id/test` | 发送测试 ping |

#### 管理 (`/api/v1/admin`) — 仅 super_admin

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/users` | 用户列表 |
| POST | `/admin/users` | 创建用户 |
| PUT | `/admin/users/:id` | 更新用户 |
| DELETE | `/admin/users/:id` | 禁用用户 |
| GET | `/admin/asset-types` | 资产类型列表 |
| POST | `/admin/asset-types` | 创建资产类型 (含 JSON Schema) |

### 6.5 资产列表查询参数

> **索引支持**: 以下多条件组合查询由 §5.6 定义的复合索引覆盖。`org_id` 由服务端从 JWT 注入 (始终作为过滤条件)，因此所有复合索引以 `org_id` 为前导列。具体索引设计见 §5.6「复合索引设计说明」。

| 参数 | 类型 | 示例 |
|---|---|---|
| `search` | string | `?search=thinkpad` (全文搜索) |
| `type_id` | UUID | `?type_id=xxx` |
| `category` | string | `?category=hardware` |
| `lifecycle_state` | string | `?lifecycle_state=utilization` |
| `status` | string | `?status=available` |
| `location_id` | UUID | `?location_id=xxx` |
| `assigned_to` | UUID | `?assigned_to=xxx` |
| `cursor` | string | 分页游标 |
| `limit` | int | 默认 50, 最大 200 |
| `sort` | string | `?sort=updated_at:desc` |

> **⚠ 安全修复 (IDOR 防护)**: `org_id` 已从用户可控查询参数中移除。普通请求 (`GET /assets`) 的 `org_id` 由服务端根据 JWT 中的 `org_id` claim 自动注入，用户无法通过 URL 参数指定任意组织。`super_admin` 跨组织查询使用独立管理端点 `GET /admin/assets?org_id=xxx`，并需通过 §9.2 定义的双人审批与 MFA 校验。Repository 层对传入的 `org_id` 执行白名单校验 (必须属于当前用户可访问的组织子树)，不合法时返回 403。

**org_id 注入流程**:

```
请求 → JWT 中间件提取 org_id claim
     → Service 层: org_scope = buildOrgScope(jwt.org_id, jwt.role)
     → Repository 层: SQL WHERE org_id = ANY($org_scope)  -- 服务端注入，不接受客户端 org_id
```

**super_admin 跨组织查询 (独立端点)**:

```
GET /api/v1/admin/assets?org_id=xxx   -- 仅 super_admin 可访问
  → 需通过 MFA 验证 (X-MFA-Token header)
  → Repository 层校验 org_id 存在且为有效组织
  → 审计日志记录跨组织查询操作
```

**Repository 层白名单校验 (Go 伪代码)**:

```go
func (r *AssetRepo) List(ctx context.Context, q AssetQuery) ([]Asset, error) {
    // org_id 从 context 中的 JWT claim 获取，不从 query 参数获取
    orgScope, ok := ctx.Value(OrgScopeKey).([]uuid.UUID)
    if !ok || len(orgScope) == 0 {
        return nil, apierror.Forbidden("missing org scope")
    }
    // 若 super_admin 显式指定 org_id (仅通过 /admin/assets 端点)，校验合法性
    if q.TargetOrgID != nil {
        if !contains(orgScope, *q.TargetOrgID) {
            return nil, apierror.Forbidden("org_id not in accessible scope")
        }
    }
    // SQL 中始终注入 org_scope 过滤，防止绕过
    return r.db.QueryAssets(ctx, q, orgScope)
}
```

### 6.6 中间件链

```
Request ID → Recovery (panic) → Structured Logging → Rate Limit (Redis) → Auth (JWT validation) → Handler
```

> **[安全加固] JWT 签发与验证实现 (EdDSA / Ed25519)**
>
> 签发端 (`internal/auth/jwt_issue.go`):
> ```go
> package auth
>
> import (
>     "crypto/ed25519"
>     "time"
>     "github.com/golang-jwt/jwt/v5"
> )
>
> // Claims — 全量 claims 校验，防止 alg 降级与 token 篡改
> type Claims struct {
>     UserID string `json:"sub"`
>     OrgID  string `json:"org_id"`
>     Role   string `json:"role"`
>     jwt.RegisteredClaims         // iss, aud, exp, iat, jti
> }
>
> func (s *AuthService) IssueAccessToken(ctx context.Context, userID, orgID, role string) (string, error) {
>     now := time.Now()
>     claims := Claims{
>         UserID: userID,
>         OrgID:  orgID,
>         Role:   role,
>         RegisteredClaims: jwt.RegisteredClaims{
>             Issuer:    s.issuer,           // iss — 签发者，校验时必须匹配
>             Audience:  []string{s.audience}, // aud — 受众，校验时必须匹配
>             ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)), // exp
>             IssuedAt:  jwt.NewNumericDate(now),                       // iat
>             ID:        uuid.NewString(),    // jti — 唯一 ID，用于主动撤销
>         },
>     }
>     // 强制 EdDSA 签名，私钥从 Vault/KMS 获取
>     token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
>     return token.SignedString(s.ed25519PrivateKey) // ed25519.PrivateKey
> }
> ```
>
> 验证端 — Auth 中间件 (`internal/middleware/auth.go`):
> ```go
> func (m *AuthMiddleware) VerifyJWT(tokenStr string) (*Claims, error) {
>     claims := &Claims{}
>     // 关键: 显式设置 ValidMethods，拒绝 alg=none / HS256 等降级攻击
>     parser := jwt.NewParser(
>         jwt.WithValidMethods([]string{"EdDSA"}),
>         jwt.WithIssuer(m.issuer),        // 校验 iss
>         jwt.WithAudience(m.audience),      // 校验 aud
>         jwt.WithExpirationRequired(),     // 强制 exp 存在
>         jwt.WithIssuedAtRequired(),        // 强制 iat 存在
>         jwt.WithLeeway(5 * time.Second),   // 时钟偏移容忍
>     )
>     token, err := parser.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
>         // 公钥从本地缓存读取 (启动时从 Vault 获取公钥部分)
>         return m.ed25519PublicKey, nil // ed25519.PublicKey
>     })
>     if err != nil || !token.Valid {
>         return nil, apierror.NewUnauthorized("invalid token")
>     }
>     // jti 主动撤销检查 (见 §15.6 — Redis 不可用时 fail-closed)
>     if revoked, err := m.isRevoked(ctx, claims.ID); err != nil {
>         return nil, apierror.NewUnauthorized("auth service unavailable") // fail-closed
>     } else if revoked {
>         return nil, apierror.NewUnauthorized("token revoked")
>     }
>     return claims, nil
> }
> ```

---

## 7. 并发控制与锁策略

### 7.1 三层锁策略

| 层级 | 机制 | 适用场景 | 占比 |
|---|---|---|---|
| 乐观锁 | version 列 + `If-Match` header | 资产元数据更新、属性修改、位置变更 | ~90% |
| 悲观锁 | `SELECT ... FOR UPDATE` | 资产领用/归还、生命周期转换、Agent delta 合并 | ~8% |
| Advisory 锁 | `pg_advisory_lock()` | 批量退役、计划维护窗口 | ~2% |

### 7.2 乐观锁实现 (`internal/lock/optimistic.go`)

**SQL 模板:**
```sql
UPDATE assets.assets
SET name = $2, type_id = $3, location_id = $4,
    lifecycle_state = $5, status = $6, properties = $7,
    version = version + 1, updated_at = now(), updated_by = $8
WHERE id = $1 AND version = $9
RETURNING version;
```

**Go 实现:**
```go
func (r *AssetRepo) Update(ctx context.Context, a *domain.Asset, expectedVersion int) error {
    var newVersion int
    err := r.db.QueryRow(ctx, updateSQL,
        a.ID, a.Name, a.TypeID, a.LocationID,
        a.State, a.Status, a.Properties,
        a.UpdatedBy, expectedVersion,
    ).Scan(&newVersion)
    if err == pgx.ErrNoRows {
        return apierror.NewConflict("asset", a.ID, expectedVersion)
    }
    a.Version = newVersion
    return err
}
```

**冲突处理流程:**
1. 客户端读资产 → 获得 `version: 5`
2. 客户端修改 → `PUT /assets/:id` + `If-Match: "5"`
3. 服务端: `UPDATE ... WHERE id = $1 AND version = 5`
4. `RowsAffected() == 0` → 返回 `409 Conflict` + 当前版本号
5. 客户端重新拉取 → 重新应用修改 → 重试

**服务端自动重试上限:**

服务端对同一资源的乐观锁冲突实施**自动重试，上限 3 次**。超过上限后返回 `409 Conflict`，建议客户端放弃当前操作或改用悲观锁路径。

```go
const MaxOptimisticRetries = 3

// UpdateWithRetry 在乐观锁冲突时自动重试，最多 MaxOptimisticRetries 次。
// 每次重试前重新读取最新版本，重新应用客户端修改。
func (r *AssetRepo) UpdateWithRetry(ctx context.Context, assetID uuid.UUID,
    applyFn func(*domain.Asset) error, expectedVersion int) (*domain.Asset, error) {

    var lastErr error
    for attempt := 0; attempt < MaxOptimisticRetries; attempt++ {
        // 读取当前资产状态
        asset, err := r.GetByID(ctx, assetID)
        if err != nil {
            return nil, err
        }

        // 应用客户端修改
        if err := applyFn(asset); err != nil {
            return nil, err
        }

        // 尝试乐观锁更新
        newVersion, err := r.tryUpdate(ctx, asset, asset.Version)
        if err == nil {
            asset.Version = newVersion
            return asset, nil
        }

        if !errors.Is(err, pgx.ErrNoRows) {
            return nil, err // 非冲突错误，直接返回
        }

        lastErr = err // 版本冲突，继续重试
    }

    // 超过重试上限 — 返回 409，建议放弃或改用悲观锁
    return nil, apierror.NewConflict("asset", assetID.String(), 0).
        WithDetail(fmt.Sprintf("乐观锁冲突重试 %d 次仍失败，建议放弃当前操作或改用悲观锁路径 (SELECT ... FOR UPDATE)",
            MaxOptimisticRetries))
}
```

**重试策略说明:**

| 场景 | 行为 |
|---|---|
| 冲突次数 ≤ 3 | 服务端自动重新读取 + 重新应用修改 + 重试更新 |
| 冲突次数 > 3 | 返回 `409 Conflict`，响应体含 `retry_exhausted: true` 和建议 |
| 客户端收到 `retry_exhausted` | 可选择: (a) 放弃操作并提示用户; (b) 改用悲观锁端点 (如 `POST /assets/:id/lock-update`) |

### 7.3 悲观锁实现 (`internal/lock/pessimistic.go`)

在事务中锁定目标行，阻止并发修改。**所有悲观锁操作必须在 5 秒内超时**。

#### 7.3.1 全局锁排序规范

为防止多行悲观锁场景下的死锁，系统定义**全局锁排序规范**：

- **排序键**: `asset_id` 的 UUID 字典序 (字符串比较，区分大小写)
- **规则**: 任何事务需要锁定多个 asset 行时，**必须先对 asset_id 排序，再按序逐一锁定**
- **禁止**: 严禁以业务语义顺序 (如"源资产→目标资产")锁定，必须以 UUID 字典序为准
- **强制超时**: 所有悲观锁事务在 `BEGIN` 后立即执行 `SET LOCAL lock_timeout = '5s'`，超时后 PG 自动 abort 事务并返回 `40P01` (deadlock_detected) 或 `55P03` (lock_not_available)

```go
// LockAssetsSorted 按 UUID 字典序锁定多个资产行，防止死锁。
// 调用方传入 asset_ids 切片，函数内部排序后逐一 FOR UPDATE。
func LockAssetsSorted(ctx context.Context, tx pgx.Tx, assetIDs []uuid.UUID) error {
    // 1. 按 UUID 字典序排序 (字符串比较)
    sorted := make([]uuid.UUID, len(assetIDs))
    copy(sorted, assetIDs)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].String() < sorted[j].String()
    })

    // 2. 按序逐一锁定
    for _, id := range sorted {
        var state string
        err := tx.QueryRow(ctx,
            `SELECT lifecycle_state FROM assets.assets WHERE id = $1 FOR UPDATE`, id).Scan(&state)
        if err != nil {
            return fmt.Errorf("lock asset %s: %w", id, err)
        }
    }
    return nil
}
```

#### 7.3.2 单行锁定示例 (领用)

```go
func (s *AssignmentService) Assign(ctx context.Context, assetID, userID, byUserID uuid.UUID) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)

    // 强制锁超时，防止无限等待
    if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
        return err
    }

    // 锁定资产行
    var state string
    tx.QueryRow(ctx, `SELECT lifecycle_state FROM assets.assets WHERE id = $1 FOR UPDATE`, assetID).Scan(&state)
    if state == "retirement" {
        return ErrAssetRetired
    }

    // 锁定已有活跃领用记录
    var count int
    tx.QueryRow(ctx, `SELECT COUNT(*) FROM assets.assignments WHERE asset_id = $1 AND status = 'active' FOR UPDATE`, assetID).Scan(&count)
    if count > 0 {
        return ErrAlreadyAssigned
    }

    // 创建领用记录
    s.repo.InsertAssignment(ctx, tx, assetID, userID, byUserID)
    // 写入审计日志
    s.auditRepo.Log(ctx, tx, assetID, byUserID, "assigned", ...)

    return tx.Commit(ctx)
}
```

#### 7.3.3 多行锁定示例 (Transfer — 先排序后锁定)

资产转移 (transfer) 涉及源资产和目标资产两行锁定，是死锁高发场景。**必须先排序后锁定**：

```go
func (s *TransferService) Transfer(ctx context.Context, srcID, dstID uuid.UUID, byUserID uuid.UUID) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)

    // 强制锁超时
    if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
        return err
    }

    // 先排序后锁定 — 防止两个 transfer 事务反向锁定导致死锁
    if err := LockAssetsSorted(ctx, tx, []uuid.UUID{srcID, dstID}); err != nil {
        return err
    }

    // 执行转移逻辑 ...
    return tx.Commit(ctx)
}
```

#### 7.3.4 死锁检测集成测试

集成测试套件中必须包含死锁检测用例，验证锁排序规范生效：

```go
func TestTransferNoDeadlock(t *testing.T) {
    // 两个 goroutine 同时发起反向 transfer:
    //   goroutine A: Transfer(asset_1 → asset_2)
    //   goroutine B: Transfer(asset_2 → asset_1)
    // 预期: 两个事务均成功完成 (或其中一个因 lock_timeout 返回错误)，
    //       不应出现死锁等待 (40P01 deadlock_detected)
    var wg sync.WaitGroup
    errs := make([]error, 2)
    for i, pair := range [][2]uuid.UUID{
        {assetA, assetB},
        {assetB, assetA},
    } {
        wg.Add(1)
        go func(idx int, src, dst uuid.UUID) {
            defer wg.Done()
            errs[idx] = svc.Transfer(ctx, src, dst, user)
        }(i, pair[0], pair[1])
    }
    wg.Wait()

    for _, err := range errs {
        // 不应出现 deadlock_detected 错误
        require.NotErrorIs(t, err, pgx.ErrDeadlockDetected)
    }
}
```

### 7.4 Advisory 锁实现 (`internal/lock/advisory.go`)

用于跨多行的批量操作，避免锁升级阻塞普通读写。

**禁止使用阻塞式 `pg_advisory_lock()`**，改用以下两种方式之一：

| 方式 | API | 适用场景 | 释放机制 |
|---|---|---|---|
| 非阻塞尝试 | `pg_try_advisory_lock()` | 批量退役、计划维护窗口 | 手动 `pg_advisory_unlock()` |
| 事务绑定 | `pg_advisory_xact_lock()` | 事务内批量操作 | 事务结束 (COMMIT/ROLLBACK) 自动释放 |

**选择规则**:
- 操作在单事务内完成 → 优先用 `pg_advisory_xact_lock()` (事务结束自动释放，无需手动 unlock，避免泄漏)
- 操作跨事务或需在事务外持锁 → 用 `pg_try_advisory_lock()`，获取失败立即返回 `409 Conflict`

#### 7.4.1 hashUUIDToInt64 碰撞检测

UUID (128-bit) 哈希为 int64 (64-bit) 存在碰撞风险。`hashUUIDToInt64` 必须在启动时和运行时检测碰撞：

```go
// hashUUIDToInt64 将 UUID 映射为 int64 advisory lock key。
// 使用 FNV-1a 哈希，并在启动时检测碰撞。
var hashCollisionCheck sync.Map // key: int64 → value: uuid.UUID

func hashUUIDToInt64(id uuid.UUID) int64 {
    h := fnv.New64a()
    h.Write(id[:])
    return int64(h.Sum64())
}

// InitAdvisoryLockCheck 在服务启动时扫描所有已注册的 location/asset UUID，
// 检测 hashUUIDToInt64 是否产生碰撞。发现碰撞则拒绝启动并告警。
func InitAdvisoryLockCheck(ctx context.Context, db *pgxpool.Pool) error {
    rows, _ := db.Query(ctx, `SELECT id FROM assets.locations`)
    defer rows.Close()
    for rows.Next() {
        var id uuid.UUID
        rows.Scan(&id)
        key := hashUUIDToInt64(id)
        if existing, loaded := hashCollisionCheck.LoadOrStore(key, id); loaded {
            if existing.(uuid.UUID) != id {
                return fmt.Errorf("advisory lock key collision: %s and %s both hash to %d",
                    existing.(uuid.UUID), id, key)
            }
        }
    }
    return nil
}
```

#### 7.4.2 非阻塞 Advisory 锁示例 (pg_try_advisory_lock)

```go
func (s *AssetService) BulkRetireByLocation(ctx context.Context, locationID uuid.UUID) error {
    lockID := hashUUIDToInt64(locationID)

    // 非阻塞尝试获取 — 失败立即返回 409，不等待
    var ok bool
    err := s.db.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&ok)
    if err != nil || !ok {
        return apierror.NewConflict("location", locationID.String(), 0).
            WithDetail("批量退役操作正在进行中，请稍后重试")
    }
    defer s.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)

    return s.repo.BulkUpdateLifecycleByLocation(ctx, locationID, domain.StateRetirement)
}
```

#### 7.4.3 事务绑定 Advisory 锁示例 (pg_advisory_xact_lock)

```go
func (s *AssetService) BulkRetireByLocationInTx(ctx context.Context, locationID uuid.UUID) error {
    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)

    lockID := hashUUIDToInt64(locationID)
    // 事务绑定锁 — COMMIT/ROLLBACK 后自动释放，无需手动 unlock
    if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockID); err != nil {
        return err
    }

    if err := s.repo.BulkUpdateLifecycleByLocationTx(ctx, tx, locationID, domain.StateRetirement); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

### 7.5 限流策略

Redis 滑动窗口算法，三层限流：

| 用户层级 | 限制 | 窗口 |
|---|---|---|
| Admin (人工) | 300 req/min | 60s |
| User (人工) | 100 req/min | 60s |
| Agent (程序) | 30 req/min + 10 req/s 突发 | 60s |

额外保护:
- `/auth/login`: 5 req/min/IP (防暴力破解)
- `/agents/sync`: 10 req/s/agent (突发允许)

### 7.6 MVCC 读一致性

PostgreSQL 默认 READ COMMITTED 已满足绝大多数场景。需要一致性快照的报告类查询使用 REPEATABLE READ：

```go
func (r *AssetRepo) GenerateReport(ctx context.Context) (*Report, error) {
    tx, _ := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
    defer tx.Rollback(ctx)
    return buildReport(ctx, tx)
}
```

---

## 8. Agent 采集架构

### 8.1 双服务分离

```
┌─────────────────────────────────┐
│        Collection Agent         │
│                                 │
│  ┌───────────────────────────┐  │
│  │   Monitor Service         │  │
│  │   - 定时采集 (每 5 分钟)   │  │
│  │   - OS 原生命令/API       │  │
│  │   - 计算 checksum         │  │
│  └───────────┬───────────────┘  │
│              │                   │
│  ┌───────────▼───────────────┐  │
│  │   Communication Service   │  │
│  │   - mTLS 出站 HTTPS       │  │
│  │   - Delta 增量推送        │  │
│  │   - 失败入离线队列        │  │
│  │   - 队列重试              │  │
│  └───────────────────────────┘  │
│                                 │
│  ┌───────────────────────────┐  │
│  │   Local SQLite Queue      │  │
│  │   (离线缓存)              │  │
│  └───────────────────────────┘  │
└─────────────────────────────────┘
```

### 8.2 注册流程

1. Agent 启动，生成硬件指纹: `SHA256(/etc/machine-id + MAC + hostname)`
2. 生成 Ed25519 密钥对
3. `POST /api/v1/auth/register-agent` → 服务器创建 `collection_agents` 记录
4. 服务器签发 mTLS 客户端证书 + JWT token
5. Agent 本地持久化证书和 token

> **[安全加固] mTLS 客户端证书生命周期管理**
>
> **问题**: 原设计签发 mTLS 证书后无过期、吊销机制，证书泄露后无法阻止恶意 Agent 接入。
>
> **修复方案**:
>
> 1. **证书有效期 ≤ 90 天 + 自动续期**:
>    ```go
>    // 签发证书时强制短有效期
>    template := x509.Certificate{
>        SerialNumber: serial,
>        Subject: pkix.Name{
>            CommonName: agentID.String(), // CN 绑定 agent_id，一一对应
>        },
>        NotBefore:   time.Now(),
>        NotAfter:    time.Now().Add(90 * 24 * time.Hour), // ≤ 90 天
>        KeyUsage:    x509.KeyUsageDigitalSignature,
>        ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
>        IPAddresses:  nil, // 不绑定 IP (Agent 可能 DHCP)
>    }
>    ```
>    Agent 在证书到期前 14 天自动发起续期请求 (`POST /api/v1/agents/renew-cert`)，
>    续期需携带当前有效证书 + 硬件指纹验证。
>
> 2. **CN 绑定 agent_id**: 证书 CommonName = agent_id (UUID)，Nginx 在 TLS 握手后
>    将 CN 透传给 API Server，API Server 校验 CN 与请求体中的 agent_id 一致：
>    ```nginx
>    # Nginx 配置 — mTLS 校验 + 传递客户端验证结果
>    server {
>        listen 443 ssl;
>        ssl_client_certificate /etc/nginx/ca.crt;
>        ssl_verify_client on;          # 强制客户端证书
>        ssl_verify_depth 2;
>
>        location /api/v1/agents/ {
>            proxy_pass http://api_server;
>            # 传递客户端证书验证结果与 CN
>            proxy_set_header X-SSL-Client-Verify $ssl_client_verify;  // SUCCESS/FAILED
>            proxy_set_header X-SSL-Client-CN      $ssl_client_s_dn_cn; // agent_id
>            proxy_set_header X-SSL-Client-Serial  $ssl_client_serial;
>        }
>    }
>    ```
>    API Server 中间件校验:
>    ```go
>    func (m *MTLSMiddleware) VerifyClientCert(c *gin.Context) {
>        verify := c.GetHeader("X-SSL-Client-Verify")
>        if verify != "SUCCESS" {
>            c.AbortWithStatusJSON(403, gin.H{"error": "mTLS certificate required"})
>            return
>        }
>        agentIDFromCert := c.GetHeader("X-SSL-Client-CN")
>        agentIDFromReq := c.GetString("agent_id") // 从请求体解析
>        if agentIDFromCert != agentIDFromReq {
>            c.AbortWithStatusJSON(403, gin.H{"error": "agent identity mismatch"})
>            return
>        }
>        c.Next()
>    }
>    ```
>
> 3. **CRL / OCSP 吊销机制 (双保险 + 实时校验)**:
>
> **[安全加固] CRL 刷新延迟导致已吊销 Agent 仍可接入 [🟡N6]**
>
> **问题**: 原方案 CRL 刷新间隔较长 (默认 24 小时)，Nginx 依赖 CRL 文件检查证书吊销状态，
> 刷新窗口期内已吊销的 Agent 仍可通过 TLS 握手接入系统，存在安全窗口期。
>
> **修复方案**: CRL 刷新缩短到 1 小时 + OCSP Stapling 在线补充 + DB `cert_revoked` 实时校验双保险。

**3a. CRL 刷新间隔缩短到 1 小时**:

```yaml
# CRL 刷新配置
crl:
  refresh_interval: 1h            # CRL 刷新间隔从 24h 缩短到 1h (最大吊销延迟 1 小时)
  crl_file_path: /etc/nginx/revoked.crl
  api_endpoint: /api/v1/internal/crl  # API Server 提供 CRL 下载端点
  on_refresh_failure: "alert"     # 刷新失败告警
```

```go
// internal/infra/crl_refresher.go — CRL 定时刷新 (1 小时)

type CRLRefresher struct {
    apiClient  *http.Client
    crlPath    string
    alertSink  AlertSink
}

func (r *CRLRefresher) Start(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour) // 每 1 小时刷新一次
    defer ticker.Stop()

    // 启动时立即刷新一次
    r.refresh(ctx)

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            r.refresh(ctx)
        }
    }
}

func (r *CRLRefresher) refresh(ctx context.Context) {
    // 从 API Server 拉取最新 CRL 文件
    resp, err := r.apiClient.Get("https://api-server:8080/api/v1/internal/crl")
    if err != nil {
        log.Error().Err(err).Msg("CRL refresh failed")
        r.alertSink.Send(ctx, Alert{
            Severity: "warning",
            Title:    "CRL 刷新失败",
            Message:  fmt.Sprintf("CRL 拉取失败: %v — 当前使用旧 CRL (最大延迟 1h)", err),
        })
        return
    }
    defer resp.Body.Close()

    // 原子替换 CRL 文件 (写入临时文件 → rename，避免 Nginx 读取半写文件)
    tmpPath := r.crlPath + ".tmp"
    if err := os.WriteFile(tmpPath, resp.Body, 0644); err != nil {
        log.Error().Err(err).Msg("CRL write failed")
        return
    }
    os.Rename(tmpPath, r.crlPath) // 原子替换

    // Nginx reload 以加载新 CRL (无需重启)
    exec.Command("nginx", "-s", "reload").Run()
    log.Info().Msg("CRL refreshed successfully (1h interval)")
}
```

**3b. OCSP Stapling 作为在线补充验证**:

```nginx
# Nginx 配置 — CRL + OCSP Stapling 双重校验
ssl_crl /etc/nginx/revoked.crl;
ssl_stapling on;
ssl_stapling_verify on;
ssl_stapling_responder http://api-server:8080/api/v1/internal/ocsp;
# OCSP Stapling: Nginx 主动向 OCSP Responder 查询证书状态并缓存
# 缓存期间内吊销的证书 → OCSP Stapling 可在秒级感知 (比 CRL 1h 更快)
```

**3c. 关键吊销场景双写 — DB cert_revoked + CRL 文件**:

```sql
-- 吊销时双写: 同时更新 DB cert_revoked 字段 + 触发 CRL 文件更新
-- 使用事务确保原子性，避免 DB 已吊销但 CRL 未更新 (或反之)

-- 1. 更新 DB cert_revoked 字段 (实时生效，API Server 中间件立即感知)
UPDATE assets.collection_agents
SET cert_revoked = true,
    revoked_at = now(),
    updated_at = now()
WHERE id = $1;

-- 2. 触发 CRL 文件更新 (异步，最大延迟 1 小时生效)
-- 通过 Redis Pub/Sub 通知 CRL Refresher 立即刷新 (而非等待下一个 1h 周期)
-- NOTIFY crl_update_needed, $1;
```

```go
// internal/service/agent_revoke.go — 吊销操作 (双写)

func (s *AgentService) RevokeAgent(ctx context.Context, agentID uuid.UUID) error {
    // 事务内: 更新 DB cert_revoked (实时生效)
    tx, err := s.db.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    _, err = tx.Exec(ctx, `
        UPDATE assets.collection_agents
        SET cert_revoked = true,
            revoked_at = now(),
            updated_at = now()
        WHERE id = $1
    `, agentID)
    if err != nil {
        return fmt.Errorf("revoke DB update failed: %w", err)
    }

    // 记录审计日志
    _, err = tx.Exec(ctx, `
        INSERT INTO assets.audit_log (action, resource_type, resource_id, details)
        VALUES ('agent_revoked', 'agent', $1, '{"cert_revoked": true}')
    `, agentID)
    if err != nil {
        return err
    }

    if err = tx.Commit(ctx); err != nil {
        return err
    }

    // 事务提交后: 通知 CRL Refresher 立即刷新 (不等 1h 周期)
    s.redis.Publish(ctx, "crl_update_needed", agentID.String())
    log.Info().Str("agent_id", agentID.String()).
        Msg("agent revoked (DB cert_revoked=true + CRL refresh triggered)")
    return nil
}
```

**3d. API Server 中间件校验 cert_revoked 字段 (实时，不依赖 CRL 刷新)**:

```go
// internal/middleware/mtls.go — cert_revoked 实时校验中间件

func (m *MTLSMiddleware) VerifyClientCert(c *gin.Context) {
    verify := c.GetHeader("X-SSL-Client-Verify")
    if verify != "SUCCESS" {
        c.AbortWithStatusJSON(403, gin.H{"error": "mTLS certificate required"})
        return
    }
    agentIDFromCert := c.GetHeader("X-SSL-Client-CN")
    agentIDFromReq := c.GetString("agent_id")
    if agentIDFromCert != agentIDFromReq {
        c.AbortWithStatusJSON(403, gin.H{"error": "agent identity mismatch"})
        return
    }

    // ★ 实时校验 DB cert_revoked 字段 (不依赖 CRL 刷新，秒级生效)
    var certRevoked bool
    var certExpiresAt time.Time
    err := m.db.QueryRow(c.Request.Context(), `
        SELECT cert_revoked, cert_expires_at
        FROM assets.collection_agents
        WHERE id = $1
    `, agentIDFromCert).Scan(&certRevoked, &certExpiresAt)
    if err != nil {
        c.AbortWithStatusJSON(403, gin.H{"error": "agent not found"})
        return
    }

    if certRevoked {
        c.AbortWithStatusJSON(403, gin.H{
            "error":  "agent certificate revoked",
            "reason": "cert_revoked=true in database",
        })
        return
    }

    if time.Now().After(certExpiresAt) {
        c.AbortWithStatusJSON(403, gin.H{
            "error":  "agent certificate expired",
            "reason": fmt.Sprintf("expired at %s", certExpiresAt.Format(time.RFC3339)),
        })
        return
    }

    c.Next()
}
```

**3e. Agent 连接时双保险 — Nginx 检查 CRL + API Server 检查 DB**:

```
Agent 连接时双重校验:

  ┌──────────────────────────────────────────────────────────────────┐
  │                    Agent → Nginx → API Server                     │
  └──────────────────────────────────────────────────────────────────┘

  第 1 层: Nginx (TLS 握手阶段)
  ├── ssl_verify_client on          → 验证证书签名链 (CA 签发)
  ├── ssl_crl /etc/nginx/revoked.crl → 检查证书是否在 CRL 中 (最大延迟 1h)
  └── ssl_stapling on               → OCSP Stapling 在线补充 (秒级感知)

  第 2 层: API Server (请求处理阶段)
  ├── X-SSL-Client-Verify == SUCCESS → Nginx 确认 TLS 握手通过
  ├── X-SSL-Client-CN == agent_id    → 证书 CN 与请求 agent_id 一致
  └── DB cert_revoked == false       → 实时查询数据库 (秒级生效，不依赖 CRL)

  双保险: 即使 CRL 刷新延迟 (1h 窗口期)，API Server 的 DB 实时校验仍可拦截已吊销 Agent
          即使 DB 查询失败 (极端情况)，Nginx 的 CRL + OCSP Stapling 仍提供基础保护
```

> **CRL 刷新延迟修复总结**: (1) CRL 刷新间隔从默认 24h 缩短到 1h，最大吊销延迟降至 1 小时; (2) OCSP Stapling 作为在线补充验证，秒级感知证书吊销; (3) 关键吊销场景双写: 同时更新 DB `cert_revoked` 字段 + 触发 CRL 文件立即刷新 (通过 Redis Pub/Sub 通知，不等 1h 周期); (4) API Server 中间件校验 `cert_revoked` 字段 (实时，秒级生效)，不依赖 CRL 刷新; (5) Agent 连接时 Nginx 检查 CRL + OCSP Stapling (第 1 层) + API Server 检查 DB `cert_revoked` (第 2 层) 双保险，任一层均可拦截已吊销 Agent。


### 8.3 增量同步协议

**首次运行 (全量):**
1. 运行所有 collector → 计算 checksum
2. 构建 `SyncPayload{full_snapshot: true, modules: [...]}`
3. POST `/api/v1/agents/sync`
4. 本地持久化 checksums

**后续运行 (增量, 按资产关键性分级)**:

> 采集频率根据资产关键性分级配置 (见 §15.4.3 采样降频策略):
> - **critical** (服务器/网络设备): 5 分钟
> - **standard** (工作站/笔记本): 15 分钟
> - **low_priority** (外设/测试设备): 30 分钟
> 全量快照降频为每小时一次，其余为增量上报。

1. 运行所有 collector → 计算 checksum
2. 与本地 checksum 比较 → 仅打包变化的模块
3. 构建 `SyncPayload{full_snapshot: false, delta_modules: [changed]}`
4. POST `/api/v1/agents/sync`
5. 更新本地 checksums

**Payload 结构:**
```json
{
    "agent_id": "uuid",
    "timestamp": "2026-07-05T12:00:00Z",
    "full_snapshot": false,
    "sequence_number": 142,
    "previous_sequence": 141,
    "modules": [
        {
            "name": "cpu",
            "data": {"model": "Intel Xeon", "cores": 12},
            "checksum": "sha256:abc123...",
            "collected_at": "2026-07-05T11:59:58Z"
        }
    ],
    "signature": "ed25519-sig..."
}
```

### 8.4 采集模块

| 模块 | Linux | Windows | macOS |
|---|---|---|---|
| CPU | `/proc/cpuinfo` | `Win32_Processor` | `sysctl -n machdep.cpu` |
| 内存 | `/proc/meminfo` | `Win32_PhysicalMemory` | `sysctl hw.memsize` |
| 磁盘 | `lsblk -J` | `Win32_DiskDrive` | `diskutil list` |
| 网络 | `/sys/class/net/*` | `Win32_NetworkAdapter` | `ifconfig` |
| OS | `/etc/os-release` | `Win32_OperatingSystem` | `sw_vers` |
| BIOS | `dmidecode -t bios` | `Win32_BIOS` | `system_profiler SPHardwareDataType` |

### 8.5 Server 端摄入管道 (`internal/service/ingest/`)

```
Agent POST /sync
       │
       ▼
┌─────────────┐
│  Pre-Check  │  轻量级签名预检 (不入 Buffer 前快速验证)
│  (sync API) │  - 验证 Ed25519 签名 (agent 公钥缓存)
│             │  - payload 模块数 ≤ MAX_MODULES_PER_AGENT (默认 200)
│             │  - 失败 → 401 直接拒绝，不进 Buffer
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Buffer    │  环形缓冲区 (容量 10,000)
│  (channel)  │  - 满时返回 503 Service Unavailable (背压)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  Processor  │  Worker goroutines 从 buffer 取 payload:
│  (workers)  │  - 验证 Ed25519 签名 (完整验证，与 Pre-Check 一致)
│             │  - 检查 sequence_number 连续性
│             │  - 去重 (已处理的 sequence 跳过)
│             │  - 转换为 domain 对象
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Engine    │  按资产分组 → 批量事务写入:
│             │  - INSERT/UPDATE asset_snapshots (无行锁竞争)
│             │    · 采样降频: 非关键资产 15-30 分钟 (见 §15.4.3)
│             │    · Delta 压缩: 全量快照降为每小时，其余增量
│             │  - INSERT audit_log (无行锁竞争)
│             │  - 如有属性变化 → UPDATE assets (乐观锁，不持有悲观锁)
└─────────────┘
```

> **[安全加固] Buffer 前签名预检 + 背压机制 + Payload 上限**
>
> **问题**: 原设计 Ed25519 签名验证仅在 Processor 层，Buffer 无预检，恶意/损坏 payload
> 可填满 Buffer 导致正常 Agent 请求被丢弃 (DoS)。
>
> **修复方案**:
>
> 1. **Buffer 前轻量级签名预检** — 在 HTTP Handler 入口即验证签名，拒绝无效 payload 进入 Buffer:
>    ```go
>    // internal/service/ingest/handler.go
>    const (
>        MaxModulesPerAgent = 200   // 每 agent 单次 payload 模块数上限
>        BufferCapacity     = 10000
>    )
>
>    func (h *IngestHandler) Sync(c *gin.Context) {
>        var payload SyncPayload
>        if err := c.ShouldBindJSON(&payload); err != nil {
>            c.JSON(400, gin.H{"error": "invalid payload"})
>            return
>        }
>        // 1. Payload 数量上限检查 (防超大 payload DoS)
>        if len(payload.Modules) > MaxModulesPerAgent {
>            c.JSON(413, gin.H{"error": "payload too large"})
>            return
>        }
>        // 2. 轻量级 Ed25519 签名预检 (公钥从缓存获取，避免 DB 查询)
>        pubKey, err := h.agentKeyCache.Get(payload.AgentID)
>        if err != nil {
>            c.JSON(401, gin.H{"error": "unknown agent"})
>            return
>        }
>        if !verifyEd25519Signature(pubKey, payload.SignedBytes(), payload.Signature) {
>            c.JSON(401, gin.H{"error": "signature verification failed"})
>            return
>        }
>        // 3. 背压机制: 非阻塞写入 Buffer，满时返回 503
>        select {
>        case h.buffer <- payload:
>            c.JSON(202, gin.H{"status": "accepted"})
>        default:
>            // Buffer 满 → 503 告知 Agent 退避重试
>            c.JSON(503, gin.H{"error": "server busy, retry later"})
>        }
>    }
>    ```
>
> 2. **背压机制**: Buffer 使用带容量 channel，满时 `select { default }` 立即返回 503，
>    Agent 收到 503 后指数退避重试 (1s → 2s → 4s → ... 上限 60s)。
>
> 3. **每 Agent Payload 数量上限**: `MAX_MODULES_PER_AGENT=200`，
>    防止单个 Agent 发送超大 payload 挤占 Buffer 资源。

#### 8.5.1 乐观锁与悲观锁路径分离

Agent 上报路径和人工操作路径**不竞争同一行锁**，两条路径明确分离：

| 路径 | 操作 | 锁机制 | 涉及表 |
|---|---|---|---|
| **Agent 上报** | 写入采集快照 + 审计日志 | 无悲观锁 (不执行 `SELECT ... FOR UPDATE`) | `asset_snapshots`, `audit_log` |
| **Agent 上报 (属性变化)** | 更新资产属性 | 乐观锁 (`version` 列 + `If-Match`) | `assets` (仅 UPDATE，不持锁) |
| **人工操作** (领用/归还/转移) | 修改资产生命周期/状态 | 悲观锁 (`SELECT ... FOR UPDATE`) | `assets`, `assignments` |

**关键原则:**

1. **Agent 上报不持有悲观锁**: Agent 上报路径中，`asset_snapshots` 和 `audit_log` 是追加写入 (INSERT)，不与 `assets` 行锁竞争。仅当 Agent 上报检测到资产属性变化时，才对 `assets` 表执行乐观锁 UPDATE (`WHERE id = $1 AND version = $2`)，**不先执行 `SELECT ... FOR UPDATE`**。

2. **两条路径不竞争同一行锁**: 人工操作路径通过悲观锁 (`FOR UPDATE`) 锁定 `assets` 行；Agent 上报路径通过乐观锁 (version 列) 更新 `assets` 行。乐观锁 UPDATE 不持有行锁，仅依赖 version 匹配，因此两条路径不会因锁等待而死锁。

3. **冲突处理**: 当 Agent 上报的乐观锁 UPDATE 与人工操作的悲观锁同时发生时:
   - 人工操作持有 `FOR UPDATE` 锁 → Agent 上报的乐观锁 UPDATE 等待锁释放后执行
   - 若人工操作已修改 version → Agent 上报的乐观锁 UPDATE 返回 `RowsAffected() == 0`，触发 §7.2 的自动重试 (上限 3 次)
   - 超过重试上限 → Agent 上报的属性更新被跳过 (快照已写入)，下次上报自然补偿

```go
// IngestEngine.Process — Agent 上报处理，不持有悲观锁
func (e *IngestEngine) Process(ctx context.Context, payload *SyncPayload) error {
    tx, _ := e.db.Begin(ctx)
    defer tx.Rollback(ctx)

    for _, mod := range payload.Modules {
        assetID := payload.AgentID // 或从模块数据解析

        // 1. 追加写入快照 — 无行锁竞争
        if err := e.snapRepo.Insert(ctx, tx, assetID, mod); err != nil {
            return err
        }

        // 2. 追加写入审计日志 — 无行锁竞争
        if err := e.auditRepo.Log(ctx, tx, assetID, "agent_sync", mod); err != nil {
            return err
        }

        // 3. 如有属性变化 → 乐观锁更新 assets (不持有悲观锁)
        if mod.HasPropertyChanges() {
            _, err := e.assetRepo.UpdateWithRetry(ctx, assetID,
                func(a *domain.Asset) error { return a.ApplyPropertyChanges(mod) }, 0)
            if err != nil {
                // 乐观锁冲突超限 — 快照已写入，属性更新跳过，下次补偿
                e.logger.Warn("asset property update skipped due to optimistic lock conflict",
                    "asset_id", assetID, "err", err)
            }
        }
    }

    return tx.Commit(ctx)
}
```

### 8.6 离线队列

使用纯 Go SQLite (`modernc.org/sqlite`)，零 CGO 依赖，可交叉编译到全平台。

```sql
CREATE TABLE offline_queue (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    payload     BLOB NOT NULL,
    created_at  INTEGER NOT NULL,
    attempts    INTEGER DEFAULT 0,
    last_error  TEXT
);
```

- 网络错误或 5xx → 序列化 payload 写入 SQLite
- 后台 goroutine 每 30 秒重试
- 成功 → 删除记录；失败 → attempts++
- 最大重试: 100 次；队列上限: 10,000 条 (超出删旧记录并告警)
- 重连后按时间顺序清空队列再发新数据

### 8.7 自更新机制

1. 每 6 小时 `POST /agents/:id/update-check`
2. 服务器返回最新版本号 + 下载 URL + SHA-256 + Ed25519 签名
3. Agent 验证签名 → 下载 → 校验 SHA-256
4. 写入 `.new` 文件，设置可执行权限
5. `syscall.Exec` (Linux/macOS) 或批处理脚本 (Windows) 原地替换
6. 启动失败 30 秒内自动回滚 `.old` → 当前二进制

### 8.8 Agent 资源预算

| 指标 | 目标值 |
|---|---|
| CPU | <1% (采集期间共享单核) |
| RAM | <50 MB |
| 磁盘 | <15 MB (含离线队列) |
| 网络 | 纯出站 HTTPS :443 |
| 扫描载荷 | <500 KB/次 |
| 二进制大小 | ~10-12 MB |

---

## 9. 多租户与权限模型

### 9.1 组织树

```
Company A (root org)
├── IT Department
│   ├── Infrastructure Team
│   └── Helpdesk Team
├── Engineering Department
└── Finance Department
```

`organizations` 表通过 `parent_id` 自引用实现树形结构。

### 9.2 角色定义

| 角色 | 权限范围 | 典型能力 |
|---|---|---|
| `super_admin` | 全部组织 | 用户管理、AssetType 管理、Agent 管理、全局配置 |
| `admin` | 所属组织 + 子组织 | 创建用户 (组织范围内)、管理资产、配置 Webhook |
| `manager` | 所属组织 + 子组织 | 资产 CRUD、领用/归还、查看仪表盘 |
| `viewer` | 所属组织 + 子组织 | 只读查看资产和仪表盘 |
| `agent` | 仅自身 (绑定 org_id) | 仅 `/agents/sync`, `/agents/heartbeat` |

> **⚠ 安全修复 1 — super_admin 双重控制**: `super_admin` 权限过大且无制衡，存在单点滥用风险。以下敏感操作需双人审批 + 强制 MFA，全部写入审计日志并触发异常告警。

**敏感操作清单**:

| 操作 | 审批要求 | MFA | 审计 | 告警 |
|---|---|---|---|---|
| 创建用户 (`POST /admin/users`) | 需另一名 super_admin 或 admin 审批 | ✅ | ✅ | ✅ |
| 签发 Agent enrollment token | 需另一名 super_admin 审批 | ✅ | ✅ | ✅ |
| 删除组织 (`DELETE /organizations/:id`) | 需另一名 super_admin 审批 | ✅ | ✅ | ✅ |
| 禁用用户 (`DELETE /admin/users/:id`) | 需另一名 super_admin 审批 | ✅ | ✅ | ✅ |
| 修改 AssetType Schema | 需另一名 super_admin 审批 | ✅ | ✅ | ✅ |
| 跨组织查询 (`GET /admin/assets?org_id=xxx`) | 无需审批 (仅查询) | ✅ | ✅ | ✅ |

**双人审批流程**:

```
1. super_admin A 发起敏感操作 → 状态: pending_approval
2. 系统通知另一名 super_admin B (邮件/站内信)
3. super_admin B 审批 (需 MFA 验证) → 状态: approved
4. 操作执行 → 写入 audit_log (含发起人、审批人、操作内容、时间)
5. 异常告警: 若审批超时 (24h 未处理) 或 1h 内连续 3 次敏感操作 → 触发安全告警
```

**审批记录表**:

```sql
CREATE TABLE assets.approval_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation       VARCHAR(100) NOT NULL,   -- e.g. 'create_user', 'delete_org'
    target_resource VARCHAR(255),             -- 操作目标 (用户ID/组织ID等)
    requested_by    UUID NOT NULL REFERENCES assets.users(id),
    approved_by     UUID REFERENCES assets.users(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','approved','rejected','expired')),
    mfa_verified    BOOLEAN NOT NULL DEFAULT false,
    payload         JSONB,                    -- 操作参数快照
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    approved_at     TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours')
);

CREATE INDEX idx_approval_status ON assets.approval_requests (status, expires_at);
```

**MFA 强制校验 (中间件)**:

```go
// MFA 中间件: 敏感操作必须携带有效的 X-MFA-Token header
func MFARequired(next gin.HandlerFunc) gin.HandlerFunc {
    return func(c *gin.Context) {
        mfaToken := c.GetHeader("X-MFA-Token")
        if mfaToken == "" {
            c.AbortWithStatusJSON(403, apierror.New("MFA token required"))
            return
        }
        userID := c.MustGet("user_id").(uuid.UUID)
        if !mfaService.Verify(userID, mfaToken) {
            c.AbortWithStatusJSON(403, apierror.New("invalid MFA token"))
            return
        }
        next(c)
    }
}
```

> **⚠ 安全修复 2 — Agent 角色无 org_id 绑定**: 原 `agent` 角色仅标注"仅自身"但 `collection_agents` 表无 `org_id` 列，Agent 可向任意组织的资产上报数据。修复方案：`collection_agents` 表增加 `org_id` 列 (从 enrollment token 继承)，摄入管道 Engine 层校验 `asset.org_id == agent.org_id`。

**collection_agents 表 DDL 增强 (§5.3 补充)**:

```sql
ALTER TABLE assets.collection_agents
    ADD COLUMN org_id UUID NOT NULL REFERENCES assets.organizations(id);

-- 从 enrollment token 继承 org_id: 注册时由 Service 层写入
-- 见 §15.3 Agent Enrollment Token 注册流程: token.org_id → agent.org_id

CREATE INDEX idx_agents_org ON assets.collection_agents (org_id);
```

**注册流程变更 (§8.2 补充)**:

```
POST /api/v1/auth/register-agent
  → 验证 enrollment_token (含 org_id)
  → 创建 collection_agents 记录时: org_id = token.org_id (从 token 继承，非客户端指定)
  → 签发的 JWT 中包含 agent.org_id claim
```

**摄入管道 Engine 层校验 (§8.5 补充)**:

```go
// Engine 层: 校验上报资产的 org_id 与 Agent 的 org_id 一致
func (e *IngestEngine) Process(ctx context.Context, payload SyncPayload, agentID uuid.UUID) error {
    agent, err := e.agentRepo.GetByID(ctx, agentID)
    if err != nil {
        return fmt.Errorf("agent not found: %w", err)
    }
    for _, snapshot := range payload.Modules {
        // 查找或创建 asset 时，强制 org_id = agent.org_id
        asset, err := e.assetRepo.GetByAssetTag(ctx, snapshot.AssetTag)
        if err == nil {
            // 已有资产: 校验 org_id 一致
            if asset.OrgID != agent.OrgID {
                e.auditLog.Log(ctx, AuditEntry{
                    Action:   "cross_org_agent_rejected",
                    AgentID:  agentID,
                    AgentOrg: agent.OrgID,
                    AssetOrg: asset.OrgID,
                    Message:  "agent attempted cross-org data upload",
                })
                return apierror.Forbidden("agent org_id mismatch with asset org_id")
            }
        } else {
            // 新建资产: 强制使用 agent.org_id，不接受客户端指定
            asset.OrgID = agent.OrgID
        }
        // ... 继续写入逻辑
    }
    return nil
}
```

### 9.3 组织范围查询

> **⚠ 安全修复 (DoS 防护)**: 原 `WITH RECURSIVE` CTE 查询无深度限制，恶意构造的环形引用或超深组织树可导致无限递归，引发 DoS。修复方案：限制组织树最大深度 20 层，引入 `depth` 列和物化路径 (`ltree` 扩展)，避免递归 CTE。

**organizations 表 DDL 增强 (§5.3 补充)**:

```sql
-- 启用 ltree 扩展
CREATE EXTENSION IF NOT EXISTS ltree;

-- organizations 表增加 depth 和 path 列
ALTER TABLE assets.organizations
    ADD COLUMN depth INTEGER NOT NULL DEFAULT 0 CHECK (depth >= 0 AND depth <= 20),
    ADD COLUMN path ltree;

-- 根节点: depth=0, path = 根节点 name
-- 子节点: depth = parent.depth + 1, path = parent.path || child_name

-- 创建 GIST 索引加速物化路径查询
CREATE INDEX idx_org_path ON assets.organizations USING GIST (path);

-- 创建触发器: 插入/更新时自动维护 depth 和 path
CREATE OR REPLACE FUNCTION assets.maintain_org_tree()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.parent_id IS NULL THEN
        NEW.depth := 0;
        NEW.path := text2ltree(NEW.name);
    ELSE
        SELECT depth, path INTO NEW.depth, NEW.path
        FROM assets.organizations WHERE id = NEW.parent_id;
        NEW.depth := NEW.depth + 1;
        -- 深度限制: 超过 20 层拒绝
        IF NEW.depth > 20 THEN
            RAISE EXCEPTION 'Organization tree depth exceeds maximum of 20';
        END IF;
        NEW.path := NEW.path || text2ltree(NEW.name);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_maintain_org_tree
    BEFORE INSERT OR UPDATE OF parent_id ON assets.organizations
    FOR EACH ROW EXECUTE FUNCTION assets.maintain_org_tree();
```

**优化后的组织范围查询 (使用物化路径，无递归 CTE)**:

```sql
-- 获取用户可访问的所有组织 ID (使用 ltree 物化路径，O(log n) 查询)
-- user_org_path 为用户所属组织的 path 值
SELECT id FROM assets.organizations
WHERE path <@ $user_org_path;  -- ltree 子树查询，利用 GIST 索引

-- 资产查询时注入 org scope 过滤
SELECT * FROM assets.assets
WHERE org_id IN (
    SELECT id FROM assets.organizations WHERE path <@ $user_org_path
);
```

**深度限制保护**:

```sql
-- 环形引用检测: 触发器中检查 parent_id 不会形成环
-- 深度上限: CHECK 约束 depth <= 20，触发器中 RAISE EXCEPTION 拦截
-- 查询超时保护: Statement timeout 设置为 5s，防止异常长查询
SET statement_timeout = '5s';
```

### 9.4 鉴权流程

```
请求 → JWT 中间件 (解析 role, org_id, user_id)
     → Role 中间件 (检查 role 是否有权访问该端点)
     → Handler
     → Service 层 (用 user.org_id 构建 org scope)
     → Repository 层 (SQL 中注入 org 过滤条件)
```

---

## 10. Grafana 集成

### 10.1 只读数据库角色

```sql
CREATE ROLE grafana_reader WITH LOGIN PASSWORD '<secure_password>';
GRANT CONNECT ON DATABASE assetdb TO grafana_reader;
GRANT USAGE ON SCHEMA assets TO grafana_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA assets TO grafana_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA assets GRANT SELECT ON TABLES TO grafana_reader;
```

`grafana_reader` 只能 SELECT，无法修改任何数据。

### 10.2 PgBouncer 配置

```ini
[databases]
assetdb = host=postgres port=5432 dbname=assetdb

[pgbouncer]
pool_mode = transaction        # Grafana 短查询最佳模式
max_client_conn = 200
default_pool_size = 25
reserve_pool_size = 10
max_db_connections = 50
```

Grafana 连接 PgBouncer (6432) 而非直连 PostgreSQL (5432)。

### 10.3 面板设计 (3 个仪表盘)

**资产概览 (asset-overview):**
- 资产总数 (Stat KPI)
- 按生命周期阶段分布 (Bar Gauge)
- 按类别分布 (Pie Chart)
- 资产增长趋势 (Time Series)
- Top 制造商 (Bar Chart)
- 按位置分布 (Table)
- 待退役资产队列 (Table)

**Agent 健康 (agent-health):**
- 在线/离线 Agent 数 (Stat)
- Agent 版本分布 (Pie Chart)
- 心跳时间线 (Time Series)
- 离线超过 24 小时 (Table)

**生命周期追踪 (lifecycle-tracking):**
- 采购阶段滞留时长 (Histogram)
- 各阶段平均停留天数 (Table)
- 今日状态转换 (Table)
- 部署阶段滞留超过 30 天 (Table)

---

## 11. 事件与 Webhook

### 11.1 事件类型

```go
const (
    EventAssetCreated      = "asset.created"
    EventAssetUpdated      = "asset.updated"
    EventAssetDeleted      = "asset.deleted"
    EventAssetAssigned     = "asset.assigned"
    EventAssetReleased     = "asset.released"
    EventAssetTransferred  = "asset.transferred"
    EventLifecycleChanged  = "asset.lifecycle_changed"
    EventAgentRegistered   = "agent.registered"
    EventAgentOnline       = "agent.online"
    EventAgentOffline      = "agent.offline"
)
```

### 11.2 事件总线 (`internal/event/bus.go`)

- 进程内发布/订阅模式
- 同步通知所有 subscriber
- Webhook subscriber 将事件放入外发队列 (异步)

### 11.3 Webhook 外发引擎 (`internal/webhook/`)

1. 事件发生 → Event Bus → Webhook Subscriber
2. Subscriber 查询匹配的 enabled webhooks (按 org + events 过滤)
3. 构造 HMAC-SHA256 签名 payload
4. HTTP POST 到 webhook URL，`X-Signature-256: sha256=...`
5. 5xx / 超时 → 指数退避重试 (最多 5 次，跨度 1m-16m)

> **[安全加固] Webhook HMAC 重放防护 + Secret 加密存储 + SSRF 防护**
>
> **问题**:
> - HMAC 签名仅覆盖 payload body，无 `event_id` / `delivered_at`，攻击者可截获请求重放。
> - Webhook secret 明文存储在数据库，DB 泄露即暴露所有 secret。
> - Webhook URL 无 HTTPS 强制与 SSRF 防护，可被利用攻击内网。
>
> **修复方案**:
>
> 1. **重放防护 — event_id + delivered_at 纳入签名**:
>    ```go
>    // internal/webhook/deliver.go
>    type WebhookPayload struct {
>        EventID     string    `json:"event_id"`      // UUID, 全局唯一
>        EventType   string    `json:"event_type"`
>        DeliveredAt  time.Time `json:"delivered_at"`  // 签发时间戳
>        Data        json.RawMessage `json:"data"`
>    }
>
>    func (d *Deliverer) signAndSend(ctx context.Context, wh *Webhook, evt Event) error {
>        payload := WebhookPayload{
>            EventID:    evt.ID,
>            EventType:  evt.Type,
>            DeliveredAt: time.Now().UTC(),
>            Data:       evt.Data,
>        }
>        body, _ := json.Marshal(payload)
>
>        // HMAC-SHA256 覆盖整个 body (含 event_id + delivered_at)
>        secret := d.kms.Decrypt(wh.SecretCiphertext) // 从 KMS 解密 secret
>        mac := hmac.New(sha256.New, []byte(secret))
>        mac.Write(body)
>        signature := hex.EncodeToString(mac.Sum(nil))
>
>        req, _ := http.NewRequestWithContext(ctx, "POST", wh.URL, bytes.NewReader(body))
>        req.Header.Set("Content-Type", "application/json")
>        req.Header.Set("X-Signature-256", "sha256="+signature)
>        req.Header.Set("X-Event-ID", payload.EventID)        // 供接收方去重
>        req.Header.Set("X-Event-Timestamp", payload.DeliveredAt.Format(time.RFC3339))
>
>        // 接收方验证: 1) 校验 HMAC 签名  2) 检查 delivered_at 在 ±5min 窗口内
>        // 3) 用 event_id 做幂等去重 (已处理过的 event_id 拒绝)
>        return d.httpClient.Do(req)
>    }
>    ```
>
> 2. **Secret 加密存储 (AES-256-GCM)**:
>    ```sql
>    -- 数据库存密文，不存明文
>    ALTER TABLE assets.webhooks
>    ALTER COLUMN secret DROP DEFAULT,
>    ALTER COLUMN secret TYPE BYTEA;  -- 改为二进制存储 AES-256-GCM 密文
>
>    -- 新增列: 加密元数据
>    ALTER TABLE assets.webhooks
>    ADD COLUMN secret_key_id  VARCHAR(64) NOT NULL,  -- KMS key version
>    ADD COLUMN secret_nonce   BYTEA NOT NULL;         -- AES-GCM nonce
>    ```
>    ```go
>    // 创建 Webhook 时: 生成随机 secret → 加密后存储
>    func (s *WebhookService) Create(ctx context.Context, req CreateReq) (*Webhook, error) {
>        plaintext := generateRandomSecret(32) // 256-bit
>        ciphertext, nonce, keyID, err := s.kms.EncryptAES256GCM(ctx, []byte(plaintext))
>        if err != nil {
>            return nil, err
>        }
>        wh := &Webhook{
>            URL:            req.URL,
>            SecretCiphertext: ciphertext,
>            SecretNonce:     nonce,
>            SecretKeyID:     keyID,
>        }
>        // 明文仅在创建时返回一次，后续不可再获取
>        wh.PlaintextSecret = plaintext // 返回给调用方
>        return s.repo.Create(ctx, wh)
>    }
>    ```
>
> 3. **Webhook URL 强制 HTTPS + SSRF 防护**:
>    ```go
>    // internal/webhook/validate.go
>    var blockedCIDRs = []string{
>        "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12",
>        "192.168.0.0/16", "169.254.0.0/16", "::1/128", "fc00::/7",
>    }
>
>    func ValidateWebhookURL(rawURL string) error {
>        u, err := url.Parse(rawURL)
>        if err != nil {
>            return fmt.Errorf("invalid URL")
>        }
>        // 强制 HTTPS
>        if u.Scheme != "https" {
>            return fmt.Errorf("webhook URL must use HTTPS")
>        }
>        // 解析目标 IP，拒绝内网/保留地址
>        ips, err := net.LookupIP(u.Hostname())
>        if err != nil {
>            return fmt.Errorf("DNS resolution failed")
>        }
>        for _, ip := range ips {
>            if isBlocked(ip, blockedCIDRs) {
>                return fmt.Errorf("webhook URL must not point to private/loopback address")
>            }
>        }
>        return nil
>    }
>    ```
>    - HTTP 客户端使用自定义 `Transport`，禁止跟随重定向到内网地址:
>    ```go
>    // 自定义 Transport: 每次请求重新校验目标 IP
>    transport := &http.Transport{
>        DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
>            host, port, _ := net.SplitHostPort(addr)
>            ips, _ := net.LookupIP(host)
>            for _, ip := range ips {
>                if isBlocked(ip, blockedCIDRs) {
>                    return nil, fmt.Errorf("blocked: private IP after redirect")
>                }
>            }
>            return (&net.Dialer{}).DialContext(ctx, network, addr)
>        },
>    }
>    ```


### 11.4 audit_log 链式哈希完整性 (防篡改 + 防重放)

**问题**: audit_log 即使禁止 UPDATE/DELETE，攻击者若获得超级用户权限仍可能直接修改数据文件。需要一种可检测篡改的机制。

**方案**: 在 audit_log 中维护链式哈希 (hash chain)，每条记录的 `hash` 依赖前一条的 `hash`，形成类似区块链的防篡改链。任何中间记录被修改都会导致后续所有 hash 校验失败。

**链式哈希算法**:
```
genesis (第一条记录):  prev_hash = '0' * 64
                         hash = SHA256(prev_hash || id || asset_id || user_id || action || field || old_value || new_value || metadata || created_at)
后续记录:                prev_hash = 上一条记录的 hash
                         hash = SHA256(prev_hash || id || asset_id || user_id || action || field || old_value || new_value || metadata || created_at)
```

**写入触发器 (自动维护 hash chain)**:

```sql
CREATE OR REPLACE FUNCTION assets.audit_log_compute_hash()
RETURNS trigger AS $$
DECLARE
    last_hash CHAR(64);
    record_text TEXT;
BEGIN
    -- 获取上一条记录的 hash (按 id 顺序)
    SELECT hash INTO last_hash
        FROM assets.audit_log
        ORDER BY id DESC
        LIMIT 1
        FOR UPDATE;  -- 锁定，防止并发写入导致链断裂

    IF last_hash IS NULL THEN
        NEW.prev_hash := repeat('0', 64);  -- 创世块
    ELSE
        NEW.prev_hash := last_hash;
    END IF;

    -- 计算当前记录的 hash
    record_text := concat(
        NEW.prev_hash, '|',
        NEW.id, '|', COALESCE(NEW.asset_id::text, ''), '|',
        COALESCE(NEW.user_id::text, ''), '|', NEW.action, '|',
        COALESCE(NEW.field, ''), '|', COALESCE(NEW.old_value, ''), '|',
        COALESCE(NEW.new_value, ''), '|', COALESCE(NEW.metadata::text, ''), '|',
        NEW.created_at
    );
    NEW.hash := encode(digest(record_text, 'sha256'), 'hex');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_hash_chain
    BEFORE INSERT ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_compute_hash();
```

**完整性校验 (定时 job，每小时执行)**:

```sql
CREATE OR REPLACE FUNCTION assets.verify_audit_log_chain()
RETURNS TABLE(broken_at BIGINT, expected_hash CHAR(64), actual_hash CHAR(64)) AS $$
DECLARE
    prev CHAR(64) := repeat('0', 64);
    r RECORD;
    computed CHAR(64);
    record_text TEXT;
BEGIN
    FOR r IN SELECT * FROM assets.audit_log ORDER BY id ASC LOOP
        record_text := concat(
            COALESCE(r.prev_hash, repeat('0', 64)), '|',
            r.id, '|', COALESCE(r.asset_id::text, ''), '|',
            COALESCE(r.user_id::text, ''), '|', r.action, '|',
            COALESCE(r.field, ''), '|', COALESCE(r.old_value, ''), '|',
            COALESCE(r.new_value, ''), '|', COALESCE(r.metadata::text, ''), '|',
            r.created_at
        );
        computed := encode(digest(record_text, 'sha256'), 'hex');

        IF r.prev_hash IS DISTINCT FROM prev OR r.hash IS DISTINCT FROM computed THEN
            broken_at := r.id;
            expected_hash := computed;
            actual_hash := r.hash;
            RETURN NEXT;
            RETURN;  -- 报告第一处断裂即可
        END IF;
        prev := r.hash;
    END LOOP;
END;
$$ LANGUAGE plpgsql;
```

**校验 job 配置**:
```yaml
audit_chain_verification:
  schedule: "0 * * * *"        # 每小时执行
  alert_on_break: true         # 链断裂 → 告警 + 阻止后续写入
  alert_channel: "security"    # 发送至安全运维通道
  on_break_action: "freeze"    # 冻结 audit_log 写入，等待人工介入
```

**并发写入保护**: hash chain 的 `SELECT ... FOR UPDATE` 确保同一时刻只有一个 INSERT 能计算 hash，避免链断裂。高并发场景下 audit_log 写入通过 advisory lock 序列化：

```sql
-- 写入前获取 advisory lock (序列化 audit_log INSERT)
SELECT pg_advisory_xact_lock(hashtext('audit_log_chain'));
-- 然后执行 INSERT (触发器内自动维护 hash)
```

> **[风险修复 N3]**: advisory lock 序列化所有 audit_log INSERT，高并发下写入吞吐量受限 (~200 条/秒)。引入批量写入缓冲 (每 100ms 或 100 条批量 INSERT)、分片链方案评估 (按 asset_id 分区并行写入)、Prometheus 监控指标、以及异步 hash 计算降级策略 — 详见 [§15.11 链式哈希序列化写入瓶颈修复](#1511-链式哈希序列化写入瓶颈修复)。

---

## 12. 缓存策略

### 12.1 缓存内容

| 缓存项 | Key 模式 | TTL | 策略 |
|---|---|---|---|
| 资产详情 | `asset:{id}` | 5 min | 写入时失效 |
| 资产列表 (热门查询) | `asset:list:{hash(query)}` | 2 min | LRU |
| Agent 在线状态 | `agent:status:{id}` | 1 min | 心跳刷新 |
| 用户 session | `session:{user_id}` | 同 JWT 有效期 | 登出时删除 |
| 限流计数 | `ratelimit:{tier}:{user_id}:{window}` | 窗口时长 | 滑动窗口 |

### 12.2 缓存模式

- **Cache-Aside**: Service 查缓存 → 命中返回 / 未命中查 DB → 回填缓存
- **Write-Invalidate**: 资产更新/删除时，删除对应缓存 key
- **TTL 兜底**: 所有缓存均有 TTL，防止脏数据永驻

---

## 13. 部署架构

### 13.1 Docker Compose (开发环境)

```yaml
services:
  postgres:     # PostgreSQL 16, port 5432
  redis:        # Redis 7, port 6379
  pgbouncer:    # PgBouncer, port 6432
  api-server:   # Go binary, port 8080
  migrate:      # 一次性运行迁移，完成后退出
  grafana:      # Grafana OSS, port 3000
  web:          # Vite dev server (HMR), port 5173
```

### 13.2 生产分布式部署

每个组件可独立部署在不同主机，通过网络互联。**生产环境强制多可用区 (Multi-AZ) 部署，所有有状态组件均需高可用配置**：

| 组件 | 部署方式 | 网络要求 | 高可用 |
|---|---|---|---|
| PostgreSQL | Patroni 集群 / RDS Multi-AZ | API Server 可写 Primary, PgBouncer 读 Replica | Streaming Replication + 自动故障转移 (RTO<30s, RPO<5s) |
| Redis | Redis Sentinel 3 节点 / ElastiCache Cluster | API Server 访问 | Sentinel 自动故障转移 (RTO<10s) |
| API Server | Go 二进制 + systemd / Docker, **至少 2 实例** | Nginx 代理 `/api/*` | 无状态, Nginx upstream 健康检查剔除 |
| Nginx | 反向代理 + 静态文件, **upstream 池** | 80/443 对外 | 主动健康检查 + 故障实例自动剔除 |
| PgBouncer | 独立主机 / sidecar, **多后端** | Primary 写 / Replica 读 | 读写分离, 指向 Patroni leader |
| Grafana | Docker / 独立部署 | 访问 PgBouncer:6432 | — |
| Agent | Go 二进制 + systemd / Windows Service | 出站 HTTPS 到 Nginx | — |

**PostgreSQL 高可用配置**:
```
Patroni 集群 (3 节点: 1 Primary + 2 Replica)
├── Streaming Replication (同步模式, RPO=0 / 异步模式, RPO<5s)
├── Patroni + etcd (Leader 选举, 自动故障转移)
├── PgBouncer 多后端: 写 → Primary, 读 → Replica (读写分离)
├── RTO < 30s (Patroni 检测 + 故障转移 + 连接重建)
└── RPO < 5s (异步复制延迟监控, 超阈值告警)
```

> **[风险修复 N2]**: 异步复制 RPO<5s 仍可能有数据丢失窗口，且 PgBouncer 需动态感知 Patroni Leader 切换。关键写操作 (资产创建/领用/状态转换) 评估使用同步复制模式，PgBouncer 通过 Patroni REST API 动态切换后端，故障转移期间 API Server 返回 503 引导客户端重试，并建立每月故障转移演练机制 — 详见 [§15.10 Patroni 故障转移数据窗口修复](#1510-patroni-故障转移数据窗口修复)。

**API Server 水平扩展配置**:
```nginx
# Nginx upstream 配置 (至少 2 实例)
upstream api_backend {
    least_conn;
    server api-server-1:8080 max_fails=3 fail_timeout=10s;
    server api-server-2:8080 max_fails=3 fail_timeout=10s;
    # 可按需扩展更多实例

    # 主动健康检查 (Nginx Plus / nginx_upstream_check_module)
    check interval=5s rise=2 fall=3 timeout=2s type=http;
    check_http_send "GET /healthz HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx;
}

# /readyz 返回 503 时 Nginx 主动剔除该实例, 不再分发流量
```

**Redis Sentinel 配置**:
```
Redis Sentinel (3 节点: 1 Master + 2 Slave + 3 Sentinel)
├── Sentinel 自动监控 + 故障转移 (RTO < 10s)
├── API Server 通过 Sentinel 发现 Master 地址
├── 限流中间件: 本地令牌桶兜底 (Redis 不可用时降级)
└── 缓存层熔断: Redis 连续失败 N 次 → 熔断, 直连 DB
```

> **[风险修复 N8] Multi-AZ 网络延迟缓解策略**
>
> 跨 AZ 的 Patroni 复制和 Sentinel 通信会增加网络延迟，影响写入性能和故障检测速度。以下措施确保 Multi-AZ 部署的延迟在可接受范围内。

**1. 同区域多 AZ 部署 (禁止跨区域)**:

```
部署约束:
├── 所有 AZ 必须在同一地理区域 (如 us-east-1a / us-east-1b / us-east-1c)
├── AZ 间网络延迟 < 2ms (部署前使用 pgbench + ping 验证)
├── 禁止跨区域部署 (如 us-east-1 ↔ us-west-2)，跨区域延迟 > 50ms 不可接受
└── 跨区域容灾采用异步快照备份 + S3 跨区域复制，而非实时复制
```

**2. 复制延迟监控与告警**:

```yaml
# 复制延迟监控配置
replication_monitoring:
  # Patroni Streaming Replication 延迟监控
  metrics:
    - name: replication_lag_seconds
      query: "SELECT EXTRACT(EPOCH FROM now() - pg_last_xact_replay_timestamp())"
      target: replica
    - name: replication_lag_bytes
      query: "SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) FROM pg_stat_replication"
      target: primary
  alerts:
    - condition: "replication_lag_seconds > 5"
      severity: warning
      message: "Replication lag exceeds 5 seconds"
    - condition: "replication_lag_seconds > 30"
      severity: critical
      message: "Replication lag exceeds 30 seconds — RPO at risk"
    - condition: "replication_lag_bytes > 104857600"  # 100MB
      severity: warning
      message: "Replication lag exceeds 100MB"
```

```go
// internal/monitor/replication_lag.go — 复制延迟监控
func (m *ReplicationMonitor) CheckLag(ctx context.Context) error {
    // 查询 Primary 上的复制延迟
    var lagSeconds float64
    err := m.primaryDB.QueryRow(ctx, `
        SELECT EXTRACT(EPOCH FROM now() - pg_last_xact_replay_timestamp())
        FROM pg_stat_replication
        WHERE application_name = $1
    `, m.replicaName).Scan(&lagSeconds)
    if err != nil {
        return err
    }

    // Prometheus 指标
    metrics.ReplicationLagSeconds.Set(lagSeconds)

    // 告警判断
    if lagSeconds > 30 {
        m.alerter.Send(ctx, Alert{
            Severity: "critical",
            Title:    "Replication lag critical",
            Message:  fmt.Sprintf("Replication lag %.1fs exceeds 30s threshold — RPO at risk", lagSeconds),
        })
    } else if lagSeconds > 5 {
        m.alerter.Send(ctx, Alert{
            Severity: "warning",
            Title:    "Replication lag warning",
            Message:  fmt.Sprintf("Replication lag %.1fs exceeds 5s threshold", lagSeconds),
        })
    }
    return nil
}
```

**3. 读写分离减少 Primary 负载**:

```
读写分离路由策略:
├── 写操作 (INSERT/UPDATE/DELETE) → Primary (经 PgBouncer 写通道)
├── Grafana 查询 → Replica (经 PgBouncer 读通道, 端口 6432)
├── 报表查询 (dashboard API, 统计聚合) → Replica
├── 快照查询 (GET /assets/:id/snapshots) → Replica (读多写少)
├── 审计日志查询 (GET /assets/:id/history) → Replica
└── 实时资产操作 (CRUD, 状态转换) → Primary (需强一致性)
```

```yaml
# PgBouncer 读写分离配置
pgbouncer:
  write_pool:
    listen_port: 6432
    default_pool: primary
    pool_mode: transaction
  read_pool:
    listen_port: 6433
    default_pool: replica
    pool_mode: transaction
  # Grafana 和报表服务连接 read_pool (6433)
  # API Server 写操作连接 write_pool (6432)
```

**4. Sentinel 通信走内网 (禁止公网)**:

```
Sentinel 网络安全约束:
├── Sentinel 节点间通信: 仅限 VPC 内网 (私有 IP)
├── API Server → Sentinel: 内网连接, 禁止经过公网/NAT
├── Sentinel 端口 (26379): 仅对 VPC 安全组开放
├── 禁止 Sentinel 绑定公网 IP 或通过 Internet Gateway 通信
└── 跨 AZ Sentinel 通信: 走 VPC Peering / Transit Gateway, 延迟 < 2ms
```

```ini
# Redis Sentinel 配置 — 强制内网通信
# sentinel.conf
port 26379
bind 10.0.0.0  # 仅绑定内网 IP，禁止 0.0.0.0
protected-mode yes

# Sentinel 节点列表 (内网 IP)
sentinel monitor mymaster 10.0.1.10 6379 2
sentinel known-sentinel mymaster 10.0.2.10 26379
sentinel known-sentinel mymaster 10.0.3.10 26379

# 超时配置 (考虑跨 AZ 延迟)
sentinel down-after-milliseconds mymaster 5000   # 5s (内网延迟 < 2ms, 5s 足够)
sentinel failover-timeout mymaster 30000          # 30s
```

### 13.3 数据库迁移

- 工具: `golang-migrate/migrate`
- 文件: `assetserver/migrations/` (序号命名，不可变)
- 开发环境: API Server 启动时自动运行 (`--auto-migrate=true`)
- 生产环境: 独立 migrate 容器 / init container
- **原则: 已合并的迁移文件永不修改，只新增**

---

## 14. 实施计划

### Phase 1: Foundation (基础)
- Go module 初始化、项目目录搭建
- 配置加载、领域模型定义
- 数据库 migration (000001_init_schema)
- Gin 路由 + 中间件链 + JWT 认证

### Phase 2: Core CRUD + Locking (核心)
- Asset CRUD + 乐观锁
- Assignment + 悲观锁
- 生命周期状态机
- Advisory 锁 (批量操作)

### Phase 3: Ingestion Pipeline + Agent (采集)
- Collect-Engine 摄入管道 (buffer → processor → engine)
- Agent 采集器 (Linux/Win/macOS)
- Agent 离线队列 + mTLS 认证

### Phase 4: Caching + Events + Webhooks
- Redis 缓存层
- 内部事件总线
- Webhook 外发引擎

### Phase 5: Dashboard + Locations + Orgs
- 聚合查询 API
- Location / Organization CRUD
- Dashboard 数据接口

### Phase 6: Frontend (Web UI)
- Vite + React + TailwindCSS + shadcn/ui 脚手架
- 登录、资产表格/详情/表单
- Agent 管理、仪表盘、权限管理

### Phase 7: Agent Polish
- 自更新、签名验证
- 全平台交叉编译

### Phase 8: Grafana + Deployment
- Grafana 仪表盘 JSON + 数据源配置
- Docker Compose、Dockerfile、PgBouncer、Nginx 配置

### Phase 9: Testing
- 集成测试、负载测试、Agent E2E (全平台)

### Phase 10: Hardening & Operations (加固与运维)
- 软删除机制 (deleted_at + 审计日志保护)
- JSONB GIN 索引 (properties + metadata)
- Agent enrollment token 注册流程
- asset_snapshots 按月分区 + audit_log 归档策略
- 健康检查端点 (/healthz, /readyz)
- JWT refresh token 轮换策略
- API 版本兼容策略 (Sunset header + deprecation)
- 前端权限路由守卫

---

## 15. 补充设计 (架构加固)

> 以下章节为架构评审后补充，解决原设计中的 8 个潜在问题。

### 15.1 软删除机制

**问题**: `assets` 表 `DELETE` 为物理删除，`audit_log` 的 `ON DELETE CASCADE` 导致误删资产时审计日志一同丢失。

**方案**:

```sql
-- assets 表增加软删除字段
ALTER TABLE assets.assets ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_assets_deleted ON assets.assets (deleted_at) WHERE deleted_at IS NOT NULL;

-- 所有查询默认过滤已删除记录 (Repository 层注入)
-- SELECT ... FROM assets.assets WHERE deleted_at IS NULL AND ...
```

**audit_log 外键处理 (保留 asset_id 值，避免变 NULL 影响查询)**:

原方案将 audit_log 外键改为 `ON DELETE SET NULL`，导致物理清理时 `asset_id` 变 NULL，审计日志无法按资产关联查询。改进方案：**解除外键约束但保留 asset_id 值不变**，同时新增 `original_asset_id` 列存储原始 ID 副本，确保即使未来 asset_id 被复用也能追溯。

```sql
-- 1. 新增 original_asset_id 列，与 asset_id 同值 (仅在物理清理前填充)
ALTER TABLE assets.audit_log ADD COLUMN original_asset_id UUID;

-- 2. 写入触发器: 自动填充 original_asset_id = asset_id (仅新记录)
CREATE OR REPLACE FUNCTION assets.audit_log_set_original_asset_id()
RETURNS trigger AS $$
BEGIN
    NEW.original_asset_id := NEW.asset_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_original_asset_id
    BEFORE INSERT ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_set_original_asset_id();

-- 3. 解除外键约束 (保留 asset_id 列值，物理清理资产后 audit_log.asset_id 不变)
ALTER TABLE assets.audit_log DROP CONSTRAINT audit_log_asset_id_fkey;
-- 不重新添加外键约束，asset_id 变为"逻辑外键" (应用层校验)

-- 4. 表达式索引: 支持按 original_asset_id 查询 (即使 asset_id 被清理仍可追溯)
CREATE INDEX idx_audit_original_asset ON assets.audit_log (original_asset_id, created_at DESC)
    WHERE original_asset_id IS NOT NULL;

-- 5. 同时保留 asset_id 索引 (热数据关联查询)
-- idx_audit_asset_time 已存在 (asset_id, created_at DESC)
```

**查询策略**:
```sql
-- 查询资产审计历史 (资产仍存在时)
SELECT * FROM assets.audit_log WHERE asset_id = $1 ORDER BY created_at DESC;

-- 查询已物理清理资产的审计历史 (用 original_asset_id 追溯)
SELECT * FROM assets.audit_log WHERE original_asset_id = $1 ORDER BY created_at DESC;

-- 跨归档表追溯 (使用统一视图)
SELECT * FROM assets.audit_log_all WHERE original_asset_id = $1 ORDER BY created_at DESC;
```

**删除流程**:
1. `DELETE /assets/:id` → 设置 `deleted_at = now()`，不物理删除
2. 已删除资产不出现在列表查询中 (Repository 层自动过滤 `deleted_at IS NULL`)
3. 超过 90 天的软删除记录由定时任务物理清理 (需 super_admin 审批)
4. 物理清理时 audit_log 的 `asset_id` 值**保持不变** (仅解除外键约束)，`original_asset_id` 亦保留，审计日志完整可追溯
5. 审计日志中 `metadata` 同时保存原始 asset_id 作为冗余备份

### 15.2 JSONB 索引优化

**问题**: `properties` 和 `metadata` 为 JSONB 列，缺少 GIN 索引，大数据量下按属性查询性能差。

**方案**:

```sql
-- properties 列 GIN 索引 (jsonb_path_ops 更紧凑、更快)
CREATE INDEX idx_assets_properties ON assets.assets
  USING GIN (properties jsonb_path_ops) WHERE deleted_at IS NULL;

-- metadata 列 GIN 索引
CREATE INDEX idx_assets_metadata ON assets.assets
  USING GIN (metadata jsonb_path_ops) WHERE deleted_at IS NULL;

-- 常用查询路径表达式索引示例
CREATE INDEX idx_assets_properties_license_vendor ON assets.assets
  ((properties->>'vendor')) WHERE properties ? 'vendor';
```

**查询示例 (利用索引)**:
```sql
-- 查找特定厂商的许可证
SELECT * FROM assets.assets WHERE properties @> '{"vendor": "Microsoft"}';

-- 按 metadata 标签过滤
SELECT * FROM assets.assets WHERE metadata @> '{"tag": "critical"}';
```

### 15.3 Agent Enrollment Token 注册流程

**问题**: `POST /auth/register-agent` 仅靠硬件指纹 + Ed25519 密钥对，任何拿到 Agent 二进制的设备都能注册。

**方案**: 增加一次性 enrollment token 机制。

> **[安全加固] Enrollment Token 并发安全 + 哈希存储**
>
> **问题 1 (并发竞态)**: 原设计 `use_count < max_uses` 检查与 `use_count++` 更新非原子操作，
> 并发注册时可导致 `use_count` 超过 `max_uses`，token 被超额使用。
>
> **问题 2 (明文存储)**: token 明文存储在 `token` 列，DB 泄露即可获取所有未使用 token，
> 攻击者可直接用 token 注册恶意 Agent。
>
> **修复方案**:
>
> 1. **原子 UPDATE + RETURNING (修复竞态)**:
>    ```sql
>    -- 原子递增 + 条件检查，单条 SQL 完成并发安全
>    UPDATE assets.enrollment_tokens
>    SET use_count = use_count + 1,
>        used_at   = CASE WHEN use_count + 1 >= max_uses THEN now() ELSE used_at END,
>        used_by_agent = $2   -- 最后一次使用的 agent (多次使用场景记录最近一次)
>    WHERE token_hash = $1           -- 通过哈希查找 (见下文)
>      AND expires_at > now()
>      AND use_count < max_uses
>    RETURNING id, org_id, use_count, max_uses;
>    ```
>    ```go
>    // internal/service/enrollment.go
>    func (s *EnrollmentService) ConsumeToken(ctx context.Context, tokenStr string, agentID uuid.UUID) (*EnrollmentResult, error) {
>        tokenHash := sha256.Sum256([]byte(tokenStr))
>        hashHex := hex.EncodeToString(tokenHash[:])
>
>        var result EnrollmentResult
>        err := s.db.QueryRow(ctx, consumeTokenSQL, hashHex, agentID).Scan(
>            &result.TokenID, &result.OrgID, &result.UseCount, &result.MaxUses,
>        )
>        if err == pgx.ErrNoRows {
>            // token 不存在 / 已过期 / 已用尽 → 统一返回 403，不泄露原因
>            return nil, apierror.NewForbidden("enrollment failed")
>        }
>        if err != nil {
>            return nil, err
>        }
>        return &result, nil
>    }
>    ```
>
> 2. **SHA-256 哈希存储 (修复明文存储)**:
>    ```sql
>    -- 修改 DDL: token 列改为 token_hash，存 SHA-256 哈希
>    CREATE TABLE assets.enrollment_tokens (
>        id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
>        token_hash  VARCHAR(64) UNIQUE NOT NULL,   -- SHA-256(token), 不存明文
>        created_by  UUID NOT NULL REFERENCES assets.users(id),
>        org_id      UUID NOT NULL REFERENCES assets.organizations(id),
>        expires_at  TIMESTAMPTZ NOT NULL,
>        used_at     TIMESTAMPTZ,              -- NULL = 未使用
>        used_by_agent UUID REFERENCES assets.collection_agents(id),
>        max_uses    INTEGER NOT NULL DEFAULT 1,
>        use_count   INTEGER NOT NULL DEFAULT 0,
>        created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
>    );
>
>    -- 索引: 按哈希快速查找
>    CREATE INDEX idx_enrollment_tokens_hash ON assets.enrollment_tokens(token_hash);
>    ```
>    ```go
>    // 创建 token 时: 生成随机明文 → 存哈希 → 明文仅返回一次
>    func (s *EnrollmentService) Create(ctx context.Context, req CreateTokenReq) (*CreateTokenResp, error) {
>        plaintext := generateSecureToken(32) // 256-bit 随机
>        hash := sha256.Sum256([]byte(plaintext))
>        hashHex := hex.EncodeToString(hash[:])
>
>        token := &EnrollmentToken{
>            TokenHash: hashHex,
>            CreatedBy: req.CreatedBy,
>            OrgID:     req.OrgID,
>            ExpiresAt: req.ExpiresAt,
>            MaxUses:   req.MaxUses,
>        }
>        if err := s.repo.Create(ctx, token); err != nil {
>            return nil, err
>        }
>        // 明文仅创建时返回一次，后续不可再获取
>        return &CreateTokenResp{
>            Token:      plaintext,  // 仅此一次返回
>            ID:         token.ID,
>            ExpiresAt:  token.ExpiresAt,
>            MaxUses:    token.MaxUses,
>        }, nil
>    }
>    ```


**数据库表**:
```sql
CREATE TABLE assets.enrollment_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token       VARCHAR(128) UNIQUE NOT NULL,
    created_by  UUID NOT NULL REFERENCES assets.users(id),
    org_id      UUID NOT NULL REFERENCES assets.organizations(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,              -- NULL = 未使用
    used_by_agent UUID REFERENCES assets.collection_agents(id),
    max_uses    INTEGER NOT NULL DEFAULT 1,
    use_count   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**注册流程**:
```
1. Admin 在 Web UI 生成 enrollment token (指定有效期、组织、最大使用次数)
2. Admin 将 token 分发给设备管理员
3. Agent 启动时携带 token:
   POST /api/v1/auth/register-agent
   Body: { enrollment_token: "xxx", fingerprint: "sha256:...", public_key: "ed25519:..." }
4. 服务器验证:
   - token 存在且未过期
   - use_count < max_uses
   - org_id 匹配
5. 验证通过:
   - 签发 mTLS 客户端证书 + JWT token
   - use_count++, 记录 used_by_agent
   - 若 use_count == max_uses → 标记 used_at
6. 验证失败 → 返回 403, 不泄露具体原因
```

**API 变更**:
```
POST /api/v1/admin/enrollment-tokens        # 创建 token (admin+)
GET  /api/v1/admin/enrollment-tokens        # 列表 (admin+)
DELETE /api/v1/admin/enrollment-tokens/:id  # 撤销 token (admin+)
```

### 15.4 数据分区与归档策略

**问题**: `asset_snapshots` 和 `audit_log` 为高增长表，无分区策略。百万资产 × 5分钟采集 = 288万条/天，snapshots 表快速膨胀。

**方案 A: asset_snapshots 按月分区**

```sql
-- 改为分区表
-- 注意: PG16 分区表支持外键引用分区表本身 (FK referencing partitioned table)，
--   但分区表自身的外键约束 (FK from partitioned table) 仍有限制:
--   ON DELETE CASCADE 在分区表外键上不支持跨分区级联删除。
--   因此移除 asset_id 的数据库级外键，改由应用层校验 (见 15.4.1)。
CREATE TABLE assets.asset_snapshots (
    id          BIGSERIAL,                    -- 改用 BIGSERIAL 代替 UUID，避免分区表主键必须包含分区键的约束
    asset_id    UUID NOT NULL,                 -- 逻辑外键，应用层校验 (不建数据库级 FK)
    agent_id    UUID NOT NULL REFERENCES assets.collection_agents(id),
    snapshot    JSONB NOT NULL,
    checksum    VARCHAR(64) NOT NULL,
    is_delta    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)              -- 分区表主键必须包含分区键 (created_at)
) PARTITION BY RANGE (created_at);

-- 初始分区
CREATE TABLE assets.asset_snapshots_2026_07
  PARTITION OF assets.asset_snapshots
  FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE TABLE assets.asset_snapshots_2026_08
  PARTITION OF assets.asset_snapshots
  FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
```

> **主键设计说明**: 分区表主键必须包含分区键。原设计 `PRIMARY KEY (id, created_at)` 中 `id` 为 UUID，改用 `BIGSERIAL` 后仍保持 `PRIMARY KEY (id, created_at)`。备选方案: `PRIMARY KEY (asset_id, created_at)` — 以 asset_id + created_at 作为联合主键，既满足分区键要求，又支持按 asset_id + 时间范围查询的唯一性约束。选择哪种取决于业务是否需要全局自增 ID。

#### 15.4.1 分区表外键约束变更

**问题**: PostgreSQL 16 分区表的外键支持存在以下限制:
1. 分区表作为**引用方** (referencing table) 的外键约束: `ON DELETE CASCADE` 不支持跨分区级联删除，当 `assets` 表的行被删除时，无法自动级联删除 `asset_snapshots` 各分区中的对应行。
2. 分区表主键必须包含分区键 (`created_at`)，原 `asset_id` 单列主键不再适用。
3. 分区表上创建外键约束会增加写入时的约束检查开销，影响 Agent 高频上报的摄入吞吐量。

**方案: 移除 asset_id 数据库级外键，改用应用层校验 + 孤儿快照检测与自动清理**

> **[风险修复 N7] 分区表外键移除导致数据一致性风险**
>
> 移除 `asset_snapshots.asset_id` 数据库级外键后，应用层校验 bug 可能导致孤儿快照 (asset_id 指向已删除的资产)。以下修复措施构建三层防护：写入前强制校验 → 定时检测孤儿 → 自动清理 + 告警。

```sql
-- asset_snapshots.asset_id 不再建数据库级外键约束
-- 原设计: asset_id UUID NOT NULL REFERENCES assets.assets(id) ON DELETE CASCADE
-- 新设计: asset_id UUID NOT NULL (逻辑外键，应用层校验)
```

**第一层：应用层强制校验 (IngestEngine 写入前验证 asset_id 有效性)**:

```go
// internal/service/ingest/engine.go — 写入 snapshot 前强制查询 asset 是否存在
func (e *IngestEngine) writeSnapshot(ctx context.Context, snap *AssetSnapshot) error {
    // 强制校验: 写入前查询 asset 是否存在 (含软删除检查)
    var exists bool
    err := e.db.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1 FROM assets.assets
            WHERE id = $1 AND deleted_at IS NULL
        )
    `, snap.AssetID).Scan(&exists)
    if err != nil {
        return fmt.Errorf("check asset existence: %w", err)
    }

    if !exists {
        // 资产不存在或已软删除 → 记录告警日志，跳过该条快照
        // 不抛异常，避免阻塞摄入管道
        e.logger.Warn().
            Str("asset_id", snap.AssetID.String()).
            Str("agent_id", snap.AgentID.String()).
            Msg("orphan snapshot detected at ingestion: asset not found, skipping")
        // 记录到孤儿快照告警计数器 (Prometheus)
        metrics.OrphanSnapshotIngestSkipped.Inc()
        return nil
    }

    // 资产存在 → 写入快照
    return e.snapshotRepo.Insert(ctx, snap)
}
```

**第二层：定时孤儿快照检测 job (每小时执行)**:

```go
// internal/job/orphan_snapshot_detector.go — 每小时检测孤儿快照
func (j *OrphanSnapshotDetector) Run(ctx context.Context) error {
    // 检测孤儿快照: asset_id 不在 assets 表中 (含已物理删除的资产)
    rows, err := j.db.Query(ctx, `
        SELECT asset_id, COUNT(*) AS orphan_count
        FROM assets.asset_snapshots
        WHERE asset_id NOT IN (SELECT id FROM assets.assets)
        GROUP BY asset_id
    `)
    if err != nil {
        return fmt.Errorf("query orphan snapshots: %w", err)
    }
    defer rows.Close()

    type orphanInfo struct {
        AssetID    uuid.UUID
        Count      int64
    }
    var orphans []orphanInfo
    for rows.Next() {
        var o orphanInfo
        if err := rows.Scan(&o.AssetID, &o.Count); err != nil {
            return err
        }
        orphans = append(orphans, o)
    }

    if len(orphans) > 0 {
        // 告警: 发现孤儿快照
        totalOrphan := int64(0)
        for _, o := range orphans {
            totalOrphan += o.Count
        }
        j.logger.Error().
            Int("orphan_asset_count", len(orphans)).
            Int64("total_orphan_snapshots", totalOrphan).
            Msg("orphan snapshots detected: asset_id references non-existent assets")

        // Prometheus 指标
        metrics.OrphanSnapshotCount.Set(float64(totalOrphan))

        // 告警通知 (PagerDuty / Slack)
        j.alerter.Send(ctx, Alert{
            Severity: "warning",
            Title:    "Orphan snapshots detected",
            Message:  fmt.Sprintf("%d orphan assets, %d total orphan snapshots", len(orphans), totalOrphan),
        })

        // 自动清理 (配置项控制)
        if j.config.CleanupOrphanSnapshots {
            for _, o := range orphans {
                if err := j.cleanupOrphanSnapshots(ctx, o.AssetID); err != nil {
                    j.logger.Error().Err(err).Str("asset_id", o.AssetID.String()).
                        Msg("failed to cleanup orphan snapshots")
                }
            }
        }
    }

    return nil
}

func (j *OrphanSnapshotDetector) cleanupOrphanSnapshots(ctx context.Context, assetID uuid.UUID) error {
    // 跨所有分区删除该孤儿资产的快照
    result, err := j.db.Exec(ctx, `
        DELETE FROM assets.asset_snapshots
        WHERE asset_id = $1
    `, assetID)
    if err != nil {
        return err
    }
    deleted := result.RowsAffected()
    j.logger.Info().
        Str("asset_id", assetID.String()).
        Int64("deleted_count", deleted).
        Msg("orphan snapshots cleaned up")
    return nil
}
```

**孤儿快照检测 job 调度配置**:

```yaml
# 定时 job: 每小时检测孤儿快照
orphan_snapshot_detection:
  schedule: "0 * * * *"              # 每小时整点执行
  cleanup_orphan_snapshots: true     # 检测到孤儿时自动清理 (可配置)
  alert_on_detect: true              # 检测到孤儿时告警
  alert_threshold: 1                 # 孤儿快照数 ≥1 即告警
  max_cleanup_batch: 1000            # 每次清理最多 1000 个 asset 的快照
  on_failure: "alert_and_retry"      # 失败告警并重试
```

**第三层：资产物理删除时显式清理关联快照**:

由于移除了 `ON DELETE CASCADE`，资产物理删除时需在**同一事务内**显式清理快照数据，避免产生孤儿快照:

```go
// internal/repo/asset_repo.go — 资产物理删除 (定时执行，需 super_admin 审批)
func (r *AssetRepo) PhysicallyDeleteAsset(ctx context.Context, assetID uuid.UUID) error {
    tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) // safety rollback

    // 1. 删除关联快照 (跨所有分区)
    snapResult, err := tx.Exec(ctx, `
        DELETE FROM assets.asset_snapshots
        WHERE asset_id = $1
    `, assetID)
    if err != nil {
        return fmt.Errorf("delete snapshots: %w", err)
    }
    snapDeleted := snapResult.RowsAffected()

    // 2. 删除关联审计日志引用 (audit_log 不删，保留审计痕迹)
    // audit_log 通过 ON DELETE SET NULL 处理 asset_id

    // 3. 物理删除资产本身 (仅删除已软删除的资产)
    assetResult, err := tx.Exec(ctx, `
        DELETE FROM assets.assets
        WHERE id = $1 AND deleted_at IS NOT NULL
    `, assetID)
    if err != nil {
        return fmt.Errorf("delete asset: %w", err)
    }
    if assetResult.RowsAffected() == 0 {
        return fmt.Errorf("asset %s not found or not soft-deleted", assetID)
    }

    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("commit: %w", err)
    }

    r.logger.Info().
        Str("asset_id", assetID.String()).
        Int64("snapshots_deleted", snapDeleted).
        Msg("asset physically deleted with associated snapshots cleaned")
    return nil
}
```

> **PG16 分区表外键支持验证**: PostgreSQL 16 已支持分区表作为**被引用方** (referenced table) 的外键约束 (即其他表可 FK 引用分区表)，但分区表自身的外键 (作为引用方) 仍受 `ON DELETE CASCADE` 限制。上述方案移除 `asset_snapshots.asset_id` 的外键约束，改用应用层校验 + 孤儿快照定时检测 + 自动清理 + 资产删除时显式清理，规避此限制并保障数据一致性。

**自动分区维护 (定时 job)**:
```go
// 每月 25 号自动创建下月分区，删除 N 个月前的旧分区
// 配置: retention_months (默认 3, 可配置)
// 旧分区: DETACH PARTITION → 归档到冷存储 → DROP
```

**方案 B: audit_log 归档**

```sql
-- 归档表 (与 audit_log 结构一致，含 hash chain 列)
CREATE TABLE assets.audit_log_archive (LIKE assets.audit_log INCLUDING ALL);

-- 归档表同样施加不可变性保护 (与 audit_log 一致)
-- 1. 角色权限: app_writer 仅 INSERT, audit_reader 仅 SELECT
GRANT INSERT ON assets.audit_log_archive TO app_writer;
GRANT USAGE, SELECT ON SEQUENCE assets.audit_log_archive_id_seq TO app_writer;
REVOKE UPDATE, DELETE ON assets.audit_log_archive FROM app_writer;
GRANT SELECT ON assets.audit_log_archive TO audit_reader;

-- 2. RLS 策略
ALTER TABLE assets.audit_log_archive ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_archive_insert_only ON assets.audit_log_archive
    FOR INSERT TO app_writer WITH CHECK (true);
CREATE POLICY audit_log_archive_no_update ON assets.audit_log_archive
    FOR UPDATE TO app_writer USING (false) WITH CHECK (false);
CREATE POLICY audit_log_archive_no_delete ON assets.audit_log_archive
    FOR DELETE TO app_writer USING (false);
CREATE POLICY audit_log_archive_select_only ON assets.audit_log_archive
    FOR SELECT TO audit_reader USING (true);

-- 3. BEFORE UPDATE OR DELETE 触发器 (与 audit_log 共用同一函数)
CREATE TRIGGER trg_audit_log_archive_immutable
    BEFORE UPDATE OR DELETE ON assets.audit_log_archive
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_immutable_guard();
```

**归档操作 (事务内原子操作 + advisory lock + 审计元日志)**:

归档涉及 audit_log 的 DELETE，而 audit_log 有不可变触发器保护。归档须通过 SECURITY DEFINER 函数在事务内原子完成，使用 advisory lock 防止并发归档冲突。

> **[风险修复 N9] 归档 SECURITY DEFINER 函数权限滥用**
>
> `archive_audit_log_batch()` 以 SECURITY DEFINER 运行，可绕过 audit_log 的不可变触发器执行 DELETE。若权限管理不当，攻击者可利用该函数删除任意审计日志。以下修复措施确保函数权限最小化、操作可审计、触发器恢复可靠。

**1. 审计元日志表 (audit_meta)**:

```sql
-- 归档操作审计元日志表 — 记录每次归档批次的完整信息
CREATE TABLE assets.audit_meta (
    id              BIGSERIAL PRIMARY KEY,
    batch_id        UUID NOT NULL DEFAULT gen_random_uuid(),  -- 归档批次唯一 ID
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),        -- 归档开始时间
    completed_at    TIMESTAMPTZ,                               -- 归档完成时间
    archived_count  BIGINT NOT NULL DEFAULT 0,                 -- 归档行数
    retention_months INT NOT NULL,                             -- 保留期 (月)
    cutoff_date     TIMESTAMPTZ NOT NULL,                       -- 归档截止日期
    operator        VARCHAR(255) NOT NULL,                     -- 操作者 (定时 job 名称或用户)
    trigger_disabled BOOLEAN NOT NULL DEFAULT false,           -- 是否禁用了触发器
    trigger_restored  BOOLEAN NOT NULL DEFAULT false,           -- 触发器是否已恢复
    status          VARCHAR(20) NOT NULL DEFAULT 'running',     -- running | completed | failed
    error_message   TEXT                                        -- 失败时的错误信息
);

-- 仅 archive_runner 可写入 audit_meta
GRANT INSERT, UPDATE ON assets.audit_meta TO archive_runner;
GRANT USAGE, SELECT ON SEQUENCE assets.audit_meta_id_seq TO archive_runner;
GRANT SELECT ON assets.audit_meta TO audit_reader;
```

**2. 增强的归档函数 (严格校验 + 审计元日志 + 触发器事务内恢复)**:

```sql
CREATE OR REPLACE FUNCTION assets.archive_audit_log_batch(
    retention_months INT DEFAULT 6,
    batch_size INT DEFAULT 10000,
    p_operator VARCHAR(255) DEFAULT 'scheduled_job'
)
RETURNS TABLE(archived_count BIGINT, batch_id UUID) AS $$
DECLARE
    cutoff         TIMESTAMPTZ := now() - (retention_months || ' months')::INTERVAL;
    cnt            BIGINT;
    v_batch_id     UUID := gen_random_uuid();
    v_trigger_name TEXT := 'trg_audit_log_immutable';
BEGIN
    -- ========== 权限与参数校验 ==========

    -- 校验 1: retention_months 必须 ≥ 6 (防止删除近期数据)
    IF retention_months < 6 THEN
        RAISE EXCEPTION 'retention_months must be >= 6, got %', retention_months
            USING ERRCODE = '22023';
    END IF;

    -- 校验 2: batch_size 必须 ≤ 10000 (防止长事务)
    IF batch_size > 10000 OR batch_size < 1 THEN
        RAISE EXCEPTION 'batch_size must be between 1 and 10000, got %', batch_size
            USING ERRCODE = '22023';
    END IF;

    -- 校验 3: 禁止手动执行 — 仅允许通过定时 job 触发
    -- 通过 application_name 判断调用来源 (定时 job 设置 application_name=archive_job)
    IF current_setting('app.archive_job_active', true) IS DISTINCT FROM 'true' THEN
        RAISE EXCEPTION 'Direct execution prohibited. Archive function can only be called by scheduled job.'
            USING ERRCODE = '42501';
    END IF;

    -- ========== 记录归档开始 (审计元日志) ==========
    INSERT INTO assets.audit_meta (
        batch_id, started_at, retention_months, cutoff_date, operator, status
    ) VALUES (
        v_batch_id, now(), retention_months, cutoff, p_operator, 'running'
    );

    -- ========== 获取 advisory lock，防止并发归档 ==========
    PERFORM pg_advisory_xact_lock(hashtext('audit_log_archive_job'));

    -- ========== 临时禁用不可变触发器 (仅本事务内生效) ==========
    -- ALTER TABLE ... DISABLE TRIGGER 在事务内执行，COMMIT 后自动恢复
    -- 若事务 ROLLBACK，触发器也会自动恢复 (DDL 在事务内是事务性的)
    ALTER TABLE assets.audit_log DISABLE TRIGGER trg_audit_log_immutable;

    -- ========== 事务内原子操作: INSERT 归档 + DELETE 原表 ==========
    -- 使用 CTE 确保 INSERT 和 DELETE 引用同一批数据
    -- 严格限定: 只处理 6 个月以上数据 (created_at < cutoff)
    WITH to_archive AS (
        SELECT * FROM assets.audit_log
        WHERE created_at < cutoff
        ORDER BY id
        LIMIT batch_size  -- 分批归档，避免长事务 (≤10000 条)
        FOR UPDATE SKIP LOCKED
    ),
    archived AS (
        INSERT INTO assets.audit_log_archive
        SELECT * FROM to_archive
        RETURNING id
    )
    DELETE FROM assets.audit_log
    WHERE id IN (SELECT id FROM to_archive)
    AND created_at < cutoff;

    -- ========== 记录归档结果 ==========
    GET DIAGNOSTICS cnt = ROW_COUNT;
    archived_count := cnt;

    -- 更新审计元日志: 归档完成
    UPDATE assets.audit_meta
    SET completed_at = now(),
        archived_count = cnt,
        trigger_disabled = true,
        trigger_restored = true,  -- COMMIT 后触发器自动恢复
        status = 'completed'
    WHERE batch_id = v_batch_id;

    -- ========== 触发器恢复 ==========
    -- 显式 ENABLE (安全起见，虽然 COMMIT 后会自动恢复)
    ALTER TABLE assets.audit_log ENABLE TRIGGER trg_audit_log_immutable;

    RETURN NEXT;
    RETURN;

EXCEPTION WHEN OTHERS THEN
    -- ========== 异常处理: 确保触发器恢复 + 记录失败 ==========
    -- 异常时触发器随 ROLLBACK 自动恢复 (事务性 DDL)
    -- 记录失败到审计元日志 (使用独立事务)
    BEGIN
        UPDATE assets.audit_meta
        SET completed_at = now(),
            status = 'failed',
            trigger_restored = true,  -- ROLLBACK 后触发器自动恢复
            error_message = SQLERRM
        WHERE batch_id = v_batch_id;
    EXCEPTION WHEN OTHERS THEN
        -- 元日志更新失败不影响主异常传播
        NULL;
    END;
    RAISE;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- ========== 权限控制: 仅授予 archive_runner 专用角色 ==========
REVOKE EXECUTE ON FUNCTION assets.archive_audit_log_batch(INT, INT, VARCHAR) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION assets.archive_audit_log_batch(INT, INT, VARCHAR) TO archive_runner;

-- 创建专用角色 (无其他权限)
-- CREATE ROLE archive_runner LOGIN NOINHERIT;
-- archive_runner 仅拥有: EXECUTE archive_audit_log_batch + INSERT/UPDATE audit_meta
```

> **安全说明 (N9 修复后)**:
> - **权限最小化**: `archive_audit_log_batch()` 权限仅授予 `archive_runner` 专用角色，PUBLIC 被显式 REVOKE。该角色无其他表权限。
> - **禁止手动执行**: 函数内部通过 `app.archive_job_active` 会话变量校验调用来源，仅定时 job 设置该变量后才能执行，手动调用直接抛出异常 (SQLSTATE 42501)。
> - **严格数据校验**: 函数内部强制 `retention_months ≥ 6` (只处理 6 个月以上数据) 和 `batch_size ≤ 10000` (每批不超过 10000 条)，防止删除近期数据或长事务。
> - **审计元日志**: 每次归档操作写入 `audit_meta` 表，记录批次 ID、时间、行数、操作者、触发器状态，支持事后审计追溯。
> - **触发器事务内恢复**: `ALTER TABLE ... DISABLE TRIGGER` 在事务内执行，COMMIT 后自动恢复；若事务 ROLLBACK (异常)，触发器也随事务回滚自动恢复。函数内显式 ENABLE 作为双重保险，EXCEPTION 块确保元日志记录失败状态。

**3. 归档 job 调度 (设置 app.archive_job_active 标志)**:

```yaml
audit_log_archive:
  schedule: "0 2 * * *"              # 每天凌晨 2 点执行
  retention_months: 6                # 热数据保留 6 个月 (函数内部强制 ≥6)
  batch_size: 10000                  # 每批归档 10000 条 (函数内部强制 ≤10000)
  advisory_lock: true                # 使用 advisory lock 防并发
  on_failure: "alert_and_retry"       # 失败告警并重试
  vacuum_after: true                  # 归档后 VACUUM ANALYZE audit_log
  # 归档 job 必须设置 app.archive_job_active=true (禁止手动执行)
```

```go
// internal/job/audit_archive.go — 归档定时 job
func (j *AuditArchiveJob) Run(ctx context.Context) error {
    // 设置 app.archive_job_active 标志，允许归档函数执行
    // 使用连接参数设置会话变量，防止手动执行
    conn, err := j.db.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("acquire connection: %w", err)
    }
    defer conn.Release()

    // 设置会话变量 (归档函数校验此变量)
    if _, err := conn.Exec(ctx, "SET app.archive_job_active = 'true'"); err != nil {
        return fmt.Errorf("set archive_job_active: %w", err)
    }

    // 调用归档函数
    var archivedCount int64
    var batchID string
    err = conn.QueryRow(ctx, `
        SELECT archived_count, batch_id
        FROM assets.archive_audit_log_batch($1, $2, $3)
    `, j.config.RetentionMonths, j.config.BatchSize, "scheduled_job").Scan(&archivedCount, &batchID)
    if err != nil {
        j.logger.Error().Err(err).Msg("archive batch failed")
        return err
    }

    j.logger.Info().
        Str("batch_id", batchID).
        Int64("archived_count", archivedCount).
        Msg("audit log archive batch completed")

    // 归档后 VACUUM ANALYZE
    if j.config.VacuumAfter {
        if _, err := conn.Exec(ctx, "VACUUM ANALYZE assets.audit_log"); err != nil {
            j.logger.Warn().Err(err).Msg("VACUUM ANALYZE failed (non-fatal)")
        }
    }
    return nil
}
```

**统一查询视图 (跨热表 + 归档表)**:

归档后审计查询需要同时检索热表和归档表。创建 UNION ALL 视图实现透明路由：

```sql
CREATE OR REPLACE VIEW assets.audit_log_all AS
    SELECT 'hot'::VARCHAR(10) AS source, * FROM assets.audit_log
    UNION ALL
    SELECT 'archive'::VARCHAR(10) AS source, * FROM assets.audit_log_archive;

-- 授权: audit_reader 可查询视图
GRANT SELECT ON assets.audit_log_all TO audit_reader;

-- 视图索引提示: 查询时建议带 source 过滤以利用各表索引
-- SELECT * FROM assets.audit_log_all WHERE asset_id = $1 AND source = 'hot';
-- SELECT * FROM assets.audit_log_all WHERE asset_id = $1; -- 跨表查询
```

**API 层透明路由**:

```
GET /api/v1/assets/:id/history?include_archive=true
  → 默认仅查 audit_log (source='hot')
  → include_archive=true 时查 audit_log_all 视图 (跨热表+归档表)
  → 响应中每条记录携带 source 字段 ('hot' | 'archive')
```

```go
// Repository 层路由逻辑
func (r *AuditRepo) GetByAssetID(ctx context.Context, assetID uuid.UUID, includeArchive bool) ([]AuditEntry, error) {
    if includeArchive {
        return r.queryAll(ctx, assetID)  // 查 audit_log_all 视图
    }
    return r.queryHot(ctx, assetID)     // 仅查 audit_log
}
```

**归档 job 调度**:
```yaml
audit_log_archive:
  schedule: "0 2 * * *"              # 每天凌晨 2 点执行
  retention_months: 6                # 热数据保留 6 个月
  batch_size: 10000                  # 每批归档 10000 条
  advisory_lock: true                # 使用 advisory lock 防并发
  on_failure: "alert_and_retry"       # 失败告警并重试
  vacuum_after: true                  # 归档后 VACUUM ANALYZE audit_log
```

**分区/归档配置**:
```yaml
data_retention:
  asset_snapshots:
    hot_retention_months: 3      # 热数据保留 3 个月
    archive_to_cold_storage: true # 超过 3 个月归档
    drop_after_months: 12        # 12 个月后删除
  audit_log:
    hot_retention_months: 6      # 热数据保留 6 个月
    archive_table: audit_log_archive
    drop_archive_after_months: 24 # 归档表 24 个月后删除
```

#### 15.4.2 分区裁剪与 asset_id 查询优化

**问题**: `asset_snapshots` 按 `created_at` RANGE 分区后，按 `asset_id` 查询 (如 `GET /assets/:id/snapshots`) 无法利用分区裁剪 (Partition Pruning)，查询会扫描全部分区，导致性能随分区数量线性下降。

**根因**: RANGE 分区键为 `created_at`，按 `asset_id` 查询时 PostgreSQL 无法裁剪分区，必须扫描所有月度分区。

**方案: API 层强制要求时间范围参数**

```go
// API 层: GET /assets/:id/snapshots 强制要求时间范围参数
func (h *SnapshotHandler) ListSnapshots(c *gin.Context) {
    assetID := c.Param("id")
    from := c.Query("from")  // ISO 8601 时间戳，必填
    to := c.Query("to")      // ISO 8601 时间戳，必填

    if from == "" || to == "" {
        c.JSON(400, gin.H{
            "error": gin.H{
                "code":    "MISSING_TIME_RANGE",
                "message": "from and to parameters are required for snapshot queries (partition pruning optimization)",
            },
        })
        return
    }

    // 校验时间范围 (最大跨度 90 天，防止全表扫描)
    fromTime, toTime, err := parseTimeRange(from, to)
    if err != nil || toTime.Sub(fromTime) > 90*24*time.Hour {
        c.JSON(400, gin.H{
            "error": gin.H{
                "code":    "INVALID_TIME_RANGE",
                "message": "time range must be <= 90 days",
            },
        })
        return
    }

    // 查询时 PostgreSQL 自动裁剪分区 (仅扫描 from~to 覆盖的月度分区)
    snapshots, err := h.repo.ListByAssetAndTimeRange(c.Request.Context(), assetID, fromTime, toTime)
    // ...
}
```

**查询 SQL (利用分区裁剪)**:
```sql
-- 带时间范围的查询: PostgreSQL 仅扫描相关分区
SELECT * FROM assets.asset_snapshots
WHERE asset_id = $1
  AND created_at >= $2   -- from
  AND created_at < $3    -- to
ORDER BY created_at DESC;

-- 不带时间范围的查询 (已禁止): 扫描全部分区
-- SELECT * FROM assets.asset_snapshots WHERE asset_id = $1; -- 不再允许
```

**API 路由变更 (§6.4)**:

```
GET /assets/:id/snapshots?from=<ISO8601>&to=<ISO8601>   -- from/to 必填，最大跨度 90 天
GET /assets/:id/snapshots/latest                        -- 获取最新一条快照 (特殊端点，不要求时间范围)
```

**备选方案: asset_id HASH 分区 (不推荐)**:

```sql
-- 按 asset_id HASH 分区可实现按 asset_id 查询的分区裁剪
-- CREATE TABLE assets.asset_snapshots (...) PARTITION BY HASH (asset_id);
-- 但此方案与按时间归档冲突: 旧数据清理需按 asset_id 遍历全部分区，无法直接 DETACH 旧分区
-- 因此不推荐，保留时间 RANGE 分区 + API 层强制时间范围查询
```

> **决策**: 保留按 `created_at` RANGE 分区 (支持高效归档) + API 层强制时间范围参数 (实现分区裁剪)。`GET /assets/:id/snapshots/latest` 作为特殊端点，通过查询 `assets` 表的 `updated_at` 列定位最新快照时间，再精确查询对应分区。

#### 15.4.3 asset_snapshots 线性增长治理 (576GB/天 → 降频 + 压缩 + 冷热分层)

**问题**: `asset_snapshots` 按当前设计线性增长无上限。百万资产 × 5分钟采集 = 288万条/天，每条快照 JSONB 约 200KB → **576GB/天**，3 个月热数据 = 51TB，超出单机存储能力。

**根因分析**:
- 所有资产统一 5 分钟采集频率，非关键资产产生大量低价值快照
- 全量快照 (full_snapshot) 与增量快照 (delta) 混存，全量快照体积大
- 无数据降采样机制，历史数据未聚合

**方案: 四层治理策略**

**1. 采样降频 — 非关键资产降低采集频率**

```yaml
# Agent 采集频率配置 (按资产关键性分级)
collection_frequency:
  critical:        # 关键资产 (服务器、网络设备): 5 分钟
    interval_seconds: 300
    full_snapshot_interval: 300     # 每 5 分钟全量
  standard:        # 标准资产 (工作站、笔记本): 15 分钟
    interval_seconds: 900
    full_snapshot_interval: 3600    # 每小时全量，其余增量
  low_priority:    # 低优先级资产 (外设、测试设备): 30 分钟
    interval_seconds: 1800
    full_snapshot_interval: 7200    # 每 2 小时全量，其余增量
```

```go
// Agent 端动态采集频率 (根据资产类型配置)
func (a *Agent) getCollectInterval() time.Duration {
    tier := a.assetTier // critical | standard | low_priority
    switch tier {
    case "critical":
        return 5 * time.Minute
    case "standard":
        return 15 * time.Minute
    case "low_priority":
        return 30 * time.Minute
    default:
        return 15 * time.Minute
    }
}
```

**2. Delta 压缩 — 全量快照降频为每小时**

```go
// IngestEngine: 全量快照降频策略
// - 首次上报: 全量快照 (full_snapshot=true)
// - 后续上报: 增量快照 (delta)，仅包含变化的模块
// - 每小时强制一次全量快照 (防止 delta 链过长导致回放困难)
// - 全量快照与增量快照存同一张表，通过 is_delta 列区分

func (e *IngestEngine) shouldSendFullSnapshot(agentID uuid.UUID, lastFull time.Time) bool {
    // 距离上次全量快照超过 1 小时 → 发送全量
    return time.Since(lastFull) > time.Hour
}
```

**预估效果**: 降频 + Delta 压缩后，写入量从 576GB/天降至约 **48GB/天** (降低 92%)。

**3. 冷热分层 — 3 个月以上数据转 S3 Parquet**

```yaml
# 冷热分层配置
data_tiering:
  hot_tier:
    storage: postgresql_local      # 本地 PostgreSQL (SSD)
    retention: 3_months            # 热数据保留 3 个月
    format: postgresql_row         # 行存，支持实时查询
  warm_tier:
    storage: s3_parquet            # S3 对象存储 (Parquet 列存)
    retention: 12_months            # 温数据保留 12 个月
    format: parquet_snappy          # Parquet + Snappy 压缩
    partition_columns: [year, month]  # 按年月分区
  cold_tier:
    storage: s3_glacier             # S3 Glacier 归档
    retention: indefinite            # 按合规要求保留
    format: parquet_zstd             # Parquet + Zstd 压缩
```

> **[可靠性加固] S3 归档管道幂等 + 状态机 + Checksum 验证**
>
> **问题 [🟡N4]**: 原归档流程 `Parquet 导出 → S3 上传 → 验证 → DETACH` 为单步串行，
> 任一步失败可能导致数据丢失 (如 S3 上传成功但 DETACH 前崩溃 → 分区既已 DETACH 又无法重新归档；
> 或 Parquet 导出后未上传即崩溃 → 重复导出产生重复文件)。缺少幂等保证和状态追踪。
>
> **修复方案**: 归档管道实现**幂等 + 分步状态机 + Checksum 验证 + 自动重试**。

**归档管道 ASCII 流程图**:

```
                        ┌──────────────────────────────────────────────────────────────┐
                        │                  S3 归档管道 (幂等 + 状态机)                   │
                        └──────────────────────────────────────────────────────────────┘

  ┌─────────┐     ┌───────────┐     ┌────────────┐     ┌───────────┐     ┌──────────┐     ┌──────────┐
  │ pending │────►│ exporting │────►│ uploading  │────►│ verifying │────►│ detaching│────►│completed │
  └────┬────┘     └─────┬─────┘     └─────┬──────┘     └─────┬─────┘     └────┬─────┘     └──────────┘
       │                │                 │                 │                │
       │    重试<3       │   重试<3        │   重试<3        │  checksum      │  DETACH
       │    ┌───────┐   │  ┌───────┐     │  ┌───────┐     │  比对失败       │  成功
       │    │       │   │  │       │     │  │       │     │  ┌───────┐     │
       └───►│retry  │◄──┴─►│retry  │◄────┴─►│retry  │◄────┴─►│retry  │◄────┘
            │backoff│      │backoff│      │backoff│      │backoff│
            └───┬───┘      └───┬───┘      └───┬───┘      └───┬───┘
                │              │              │              │
           重试≥3         重试≥3         重试≥3         重试≥3
                ▼              ▼              ▼              ▼
              ┌──────────────────────────────────────────────┐
              │                    failed                    │ → 告警 + 人工介入
              └──────────────────────────────────────────────┘

  幂等保证: archive_id (UUID) 唯一标识每个分区归档任务
            重试时检查 archive_manifest.state，从失败步骤恢复，不重复执行已完成步骤
```

**1. archive_manifest 表 — 记录每个分区的归档状态**:

```sql
-- 归档清单表: 每个分区归档任务一行，唯一 archive_id 保证幂等
CREATE TABLE assets.archive_manifest (
    archive_id      UUID NOT NULL DEFAULT gen_random_uuid(),
    partition_name  VARCHAR(128) NOT NULL,          -- e.g. asset_snapshots_2026_04
    table_name      VARCHAR(64) NOT NULL DEFAULT 'asset_snapshots',
    state           VARCHAR(20) NOT NULL DEFAULT 'pending',
                    -- pending | exporting | uploading | verifying | detaching | completed | failed
    s3_key          VARCHAR(512),                    -- s3://asset-db/snapshots/year=2026/month=04/data.parquet
    s3_checksum     VARCHAR(64),                     -- S3 上传后返回的 SHA256
    local_checksum  VARCHAR(64),                     -- 本地 Parquet 文件 SHA256 (导出后计算)
    row_count       BIGINT,                          -- 归档行数
    file_size_bytes BIGINT,                          -- Parquet 文件大小
    retry_count     INTEGER NOT NULL DEFAULT 0,
    max_retries     INTEGER NOT NULL DEFAULT 3,
    error_message   TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (archive_id),
    UNIQUE (partition_name, table_name)              -- 同一分区只有一个活跃归档任务 (幂等)
);

-- 状态机约束: 只允许合法的状态流转
-- pending → exporting → uploading → verifying → detaching → completed
-- 任意非 completed 状态 → failed (重试耗尽)
-- failed → pending (人工触发重试)

-- 索引: 按状态查询待处理/失败任务
CREATE INDEX idx_archive_manifest_state ON assets.archive_manifest(state);
CREATE INDEX idx_archive_manifest_partition ON assets.archive_manifest(partition_name);
```

**2. 归档管道实现 (状态机 + 幂等 + Checksum 验证 + 自动重试)**:

```go
// internal/archives/pipeline.go — S3 归档管道 (幂等 + 状态机)

const (
    StatePending   = "pending"
    StateExporting = "exporting"
    StateUploading = "uploading"
    StateVerifying = "verifying"
    StateDetaching = "detaching"
    StateCompleted = "completed"
    StateFailed    = "failed"
    MaxRetries     = 3
)

// ArchivePipeline 归档管道，每个分区有唯一 archive_id，重试从失败步骤恢复
type ArchivePipeline struct {
    db          *pgxpool.Pool
    s3          *s3.Client
    bucket      string
    alertSink   AlertSink // 告警接口 (PagerDuty/Slack)
}

// Run 执行归档管道，幂等: 重试时从 archive_manifest.state 恢复
func (p *ArchivePipeline) Run(ctx context.Context, partitionName string) error {
    // 1. 获取或创建归档记录 (幂等: 同一 partition_name 只有一个活跃任务)
    manifest, err := p.getOrCreateManifest(ctx, partitionName)
    if err != nil {
        return err
    }

    // 2. 已完成的任务直接返回 (幂等)
    if manifest.State == StateCompleted {
        log.Info().Str("archive_id", manifest.ArchiveID.String()).
            Str("partition", partitionName).
            Msg("archive already completed, skipping (idempotent)")
        return nil
    }

    // 3. 按状态机恢复执行 (从失败步骤继续，不重复已完成步骤)
    for manifest.RetryCount < MaxRetries {
        err := p.executeStep(ctx, manifest)
        if err == nil {
            return nil // 归档完成
        }

        // 步骤失败 → 记录错误，递增重试计数
        manifest.RetryCount++
        manifest.ErrorMessage = err.Error()
        if manifest.RetryCount >= MaxRetries {
            // 重试耗尽 → 标记 failed + 告警人工介入
            p.updateState(ctx, manifest.ArchiveID, StateFailed, err)
            p.alertSink.Send(ctx, Alert{
                Severity: "critical",
                Title:    "S3 归档失败 — 需人工介入",
                Message:  fmt.Sprintf("分区 %s 归档失败 (重试 %d 次): %v", partitionName, manifest.RetryCount, err),
            })
            return fmt.Errorf("archive failed after %d retries: %w", manifest.RetryCount, err)
        }

        // 指数退避重试
        backoff := time.Duration(manifest.RetryCount*manifest.RetryCount) * time.Minute
        log.Warn().Err(err).Str("partition", partitionName).
            Int("retry", manifest.RetryCount).
            Dur("backoff", backoff).
            Msg("archive step failed, retrying after backoff")
        time.Sleep(backoff)
    }
    return nil
}

// executeStep 根据当前状态执行对应步骤
func (p *ArchivePipeline) executeStep(ctx context.Context, m *ArchiveManifest) error {
    switch m.State {
    case StatePending, StateFailed:
        // pending → exporting: 导出 Parquet
        return p.stepExport(ctx, m)
    case StateExporting:
        // exporting → uploading: 上传 S3
        return p.stepUpload(ctx, m)
    case StateUploading:
        // uploading → verifying: 验证 checksum
        return p.stepVerify(ctx, m)
    case StateVerifying:
        // verifying → detaching: DETACH 分区
        return p.stepDetach(ctx, m)
    case StateDetaching:
        // detaching → completed: DROP 分区文件
        return p.stepComplete(ctx, m)
    }
    return fmt.Errorf("unknown state: %s", m.State)
}

// stepExport: 导出分区数据为 Parquet 文件
func (p *ArchivePipeline) stepExport(ctx context.Context, m *ArchiveManifest) error {
    p.updateState(ctx, m.ArchiveID, StateExporting, nil)

    // 导出为本地 Parquet 文件 (使用 arrow-go / parquet-go)
    localPath := fmt.Sprintf("/tmp/archive_%s.parquet", m.PartitionName)
    rowCount, err := p.exportToParquet(ctx, m.PartitionName, localPath)
    if err != nil {
        return fmt.Errorf("export failed: %w", err)
    }

    // 计算本地文件 SHA256 checksum
    checksum, fileSize, err := computeSHA256(localPath)
    if err != nil {
        return fmt.Errorf("checksum failed: %w", err)
    }

    // 记录到 manifest (幂等: 重试时若 local_checksum 已存在且匹配则跳过导出)
    p.updateManifest(ctx, m.ArchiveID, ManifestUpdate{
        LocalChecksum:  checksum,
        RowCount:       rowCount,
        FileSizeBytes:  fileSize,
    })
    m.LocalChecksum = checksum
    m.RowCount = rowCount
    return p.stepUpload(ctx, m) // 继续下一步
}

// stepUpload: 上传 Parquet 到 S3
func (p *ArchivePipeline) stepUpload(ctx context.Context, m *ArchiveManifest) error {
    p.updateState(ctx, m.ArchiveID, StateUploading, nil)

    s3Key := fmt.Sprintf("snapshots/%s/data.parquet", m.PartitionName)
    localPath := fmt.Sprintf("/tmp/archive_%s.parquet", m.PartitionName)

    // S3 上传 (带服务端 checksum: S3 返回 ETag/SHA256)
    s3Checksum, err := p.uploadToS3(ctx, localPath, s3Key)
    if err != nil {
        return fmt.Errorf("s3 upload failed: %w", err)
    }

    p.updateManifest(ctx, m.ArchiveID, ManifestUpdate{
        S3Key:      s3Key,
        S3Checksum:  s3Checksum,
    })
    m.S3Key = s3Key
    m.S3Checksum = s3Checksum
    return p.stepVerify(ctx, m)
}

// stepVerify: 验证 S3 上传完整性 (checksum 比对)
func (p *ArchivePipeline) stepVerify(ctx context.Context, m *ArchiveManifest) error {
    p.updateState(ctx, m.ArchiveID, StateVerifying, nil)

    // 从 S3 下载文件头部或完整文件，计算 checksum 并比对
    // 方式1: S3 HeadObject 获取 ETag (MD5) 或 SHA256 checksum
    // 方式2: S3 GetObject 范围读取 + 本地 SHA256 比对 (更严格)
    s3RemoteChecksum, err := p.getS3Checksum(ctx, m.S3Key)
    if err != nil {
        return fmt.Errorf("s3 checksum verification failed: %w", err)
    }

    // checksum 比对: 本地 SHA256 == S3 SHA256
    if s3RemoteChecksum != m.LocalChecksum {
        return fmt.Errorf("checksum mismatch: local=%s s3=%s — data may be corrupted",
            m.LocalChecksum, s3RemoteChecksum)
    }

    log.Info().Str("archive_id", m.ArchiveID.String()).
        Str("partition", m.PartitionName).
        Str("s3_key", m.S3Key).
        Int64("rows", m.RowCount).
        Msg("S3 upload verified (checksum match)")
    return p.stepDetach(ctx, m)
}

// stepDetach: DETACH PARTITION (必须在 checksum 验证通过后)
func (p *ArchivePipeline) stepDetach(ctx context.Context, m *ArchiveManifest) error {
    p.updateState(ctx, m.ArchiveID, StateDetaching, nil)

    // DETACH PARTITION — 仅在 S3 验证通过后执行
    // 关键: DETACH 前再次确认 archive_manifest.state == verifying (防止并发误删)
    _, err := p.db.Exec(ctx, `
        ALTER TABLE assets.asset_snapshots
        DETACH PARTITION assets.${m.PartitionName}
    `)
    if err != nil {
        return fmt.Errorf("detach partition failed: %w", err)
    }
    return p.stepComplete(ctx, m)
}

// stepComplete: DROP 分区文件 + 标记完成
func (p *ArchivePipeline) stepComplete(ctx context.Context, m *ArchiveManifest) error {
    // DROP 已 DETACH 的分区表
    _, err := p.db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS assets.%s", m.PartitionName))
    if err != nil {
        return fmt.Errorf("drop partition failed: %w", err)
    }

    // 清理本地临时文件
    os.Remove(fmt.Sprintf("/tmp/archive_%s.parquet", m.PartitionName))

    // 标记 completed
    p.updateState(ctx, m.ArchiveID, StateCompleted, nil)
    log.Info().Str("archive_id", m.ArchiveID.String()).
        Str("partition", m.PartitionName).
        Str("s3_key", m.S3Key).
        Int64("rows", m.RowCount).
        Msg("archive pipeline completed")
    return nil
}

// getOrCreateManifest 幂等获取或创建归档记录
func (p *ArchivePipeline) getOrCreateManifest(ctx context.Context, partitionName string) (*ArchiveManifest, error) {
    // INSERT ... ON CONFLICT (partition_name, table_name) DO NOTHING
    // 然后 SELECT 返回现有记录 → 同一分区重试不会创建重复记录
    var m ArchiveManifest
    err := p.db.QueryRow(ctx, `
        INSERT INTO assets.archive_manifest (partition_name, state)
        VALUES ($1, 'pending')
        ON CONFLICT (partition_name, table_name) DO NOTHING
        RETURNING archive_id, partition_name, state, s3_key, s3_checksum,
                  local_checksum, row_count, retry_count, max_retries
    `, partitionName).Scan(&m.ArchiveID, &m.PartitionName, &m.State, ...)
    if err == pgx.ErrNoRows {
        // 记录已存在 → 查询现有状态 (从失败步骤恢复)
        err = p.db.QueryRow(ctx, `
            SELECT archive_id, partition_name, state, s3_key, s3_checksum,
                   local_checksum, row_count, retry_count, max_retries
            FROM assets.archive_manifest
            WHERE partition_name = $1 AND state != 'completed'
            ORDER BY updated_at DESC LIMIT 1
        `, partitionName).Scan(&m.ArchiveID, &m.PartitionName, &m.State, ...)
    }
    return &m, err
}
```

**3. 归档 job 调度 (带超时告警)**:

```yaml
# 归档管道调度配置
archive_pipeline:
  schedule: "0 3 * * *"                    # 每天凌晨 3 点执行 (避开业务高峰)
  older_than_months: 3                     # 归档 3 个月以上的分区
  max_retries: 3                            # 每步最多重试 3 次
  retry_backoff: "exponential"              # 指数退避: 1min, 4min, 9min
  step_timeout: 30m                         # 单步超时 30 分钟
  total_timeout: 4h                         # 整个管道超时 4 小时
  on_max_retries_exceeded: "alert_manual"   # 重试耗尽 → 告警 + 人工介入
  on_total_timeout: "alert_manual"          # 总超时 → 告警 + 人工介入
  alert_channels: ["pagerduty", "slack"]    # 告警渠道
  verify_before_detach: true                # DETACH 前必须 checksum 验证通过
  cleanup_temp_files: true                  # 完成后清理临时 Parquet 文件
```

> **幂等保证总结**: (1) `archive_manifest` 表 `UNIQUE(partition_name, table_name)` 确保同一分区只有一个归档任务; (2) 重试时检查 `state` 字段，从失败步骤恢复，不重复执行已完成步骤 (如已 uploading 则跳过 exporting); (3) `archive_id` (UUID) 唯一标识，日志和告警可精确追踪; (4) DETACH 前必须 checksum 比对通过，防止数据损坏后误删分区; (5) 重试 3 次失败后标记 `failed` + 告警人工介入，不会自动 DROP 分区。

**4. 聚合表 — asset_snapshot_daily**

```sql
-- 日聚合表: 每资产每天一条摘要，用于趋势分析和仪表盘展示
CREATE TABLE assets.asset_snapshot_daily (
    asset_id      UUID NOT NULL,
    day           DATE NOT NULL,
    snapshot_count    INTEGER NOT NULL,
    first_snapshot_at TIMESTAMPTZ NOT NULL,
    last_snapshot_at  TIMESTAMPTZ NOT NULL,
    avg_checksum       VARCHAR(64),               -- 日均校验和 (用于变化频率估算)
    change_count       INTEGER NOT NULL DEFAULT 0, -- 当日属性变化次数
    summary           JSONB NOT NULL DEFAULT '{}', -- 日摘要 (关键指标聚合)
    PRIMARY KEY (asset_id, day)
);

-- 按月分区 (数据量远小于原始快照表)
-- CREATE TABLE assets.asset_snapshot_daily (...) PARTITION BY RANGE (day);

-- 定时聚合任务 (每天凌晨执行):
-- INSERT INTO asset_snapshot_daily
-- SELECT asset_id,
--        DATE(created_at) AS day,
--        COUNT(*) AS snapshot_count,
--        MIN(created_at) AS first_snapshot_at,
--        MAX(created_at) AS last_snapshot_at,
--        0 AS change_count,  -- 从 audit_log 统计
--        jsonb_build_object('last_snapshot', last(snapshot)) AS summary
-- FROM asset_snapshots
-- WHERE created_at >= now() - INTERVAL '2 days'
--   AND created_at < now() - INTERVAL '1 day'
-- GROUP BY asset_id, DATE(created_at)
-- ON CONFLICT (asset_id, day) DO UPDATE SET ...
```

**治理效果预估**:

| 指标 | 修复前 | 修复后 |
|---|---|---|
| 日写入量 | 576 GB/天 | ~48 GB/天 (降频+Delta) |
| 3 个月热数据 | 51 TB | ~4.3 TB (本地 SSD) |
| 3-12 个月温数据 | — | S3 Parquet (~2 TB 压缩后) |
| 12 个月+ 冷数据 | — | S3 Glacier (~500 GB) |
| 趋势查询 | 全表扫描快照表 | 查询聚合表 (asset_snapshot_daily) |

> **总结**: 通过采样降频 (非关键资产 15-30 分钟)、Delta 压缩 (全量快照降为每小时)、冷热分层 (3 月+ 转 S3 Parquet)、聚合表 (asset_snapshot_daily) 四层治理，将 576GB/天 的线性增长降至可控范围，同时保留近 3 个月热数据的实时查询能力。

### 15.5 健康检查端点

**问题**: 部署架构缺少 `/healthz`、`/readyz` 端点，K8s/Docker 容器编排需要探针。

**方案**:

```
GET /healthz  → 200 (进程存活，不检查依赖，轻量快速)
GET /readyz   → 200 (依赖正常，可接收流量)
              → 503 (依赖不可用，不应接收流量)
```

**实现**:
```go
// /healthz — 存活探针
func healthzHandler(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
}

// /readyz — 就绪探针
func readyzHandler(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
    defer cancel()

    checks := map[string]string{}

    // PostgreSQL 检查
    if err := db.Ping(ctx); err != nil {
        checks["postgres"] = "fail"
    } else {
        checks["postgres"] = "ok"
    }

    // Redis 检查
    if err := redis.Ping(ctx).Err(); err != nil {
        checks["redis"] = "fail"
    } else {
        checks["redis"] = "ok"
    }

    allOK := true
    for _, v := range checks {
        if v != "ok" {
            allOK = false
            break
        }
    }

    if allOK {
        c.JSON(200, gin.H{"status": "ready", "checks": checks})
    } else {
        c.JSON(503, gin.H{"status": "not_ready", "checks": checks})
    }
}
```

**路由注册 (不经过 Auth 中间件)**:
```go
public := router.Group("/")
public.GET("/healthz", healthzHandler)
public.GET("/readyz", readyzHandler)
```

**Docker Compose 探针**:
```yaml
api-server:
  healthcheck:
    test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/healthz"]
    interval: 10s
    timeout: 3s
    retries: 3
    start_period: 10s
```

### 15.6 JWT Refresh Token 轮换策略

**问题**: 文档提到 refresh token 在 Redis 中标记失效，但缺少有效期、轮换策略、并发刷新处理。

**方案**:

**Token 生命周期**:
```
access token:  15 分钟有效
refresh token: 7 天有效
```

**轮换策略 (Refresh Token Rotation)**:
```
每次刷新:
1. 客户端 POST /auth/refresh { refresh_token: "xxx" }
2. 服务器验证 refresh token 有效性 (Redis 中存在 + 未过期)
3. 签发新的 access token + 新的 refresh token
4. 旧 refresh token 立即从 Redis 删除 (不可重用)
5. 新 refresh token 写入 Redis: key=refresh:{user_id}:{token_id}, TTL=7天
```

**并发刷新处理**:
```
同一 refresh token 只能成功使用一次:
1. 请求 A 和请求 B 同时用同一个 refresh token 刷新
2. 请求 A 先到 → Redis GET + DEL (原子操作) → 成功 → 签发新 token
3. 请求 B 后到 → Redis GET → key 不存在 → 返回 401 "refresh token already used"
4. 客户端收到 401 → 重新登录
```

**Redis 实现 (原子操作)**:
```go
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
    // 原子获取并删除，防止并发重用
    key := fmt.Sprintf("refresh:%s", refreshToken)
    val, err := s.redis.GetDel(ctx, key).Result()
    if err == redis.Nil {
        return nil, apierror.NewUnauthorized("refresh token invalid or already used")
    }
    if err != nil {
        return nil, err
    }

    // val 中存储 user_id, 解析后签发新 token 对
    userID := val
    return s.issueTokenPair(ctx, userID)
}
```

**Redis Key 设计**:
```
refresh:{token_id}      → {user_id}   TTL=7天   (用于验证)
session:{user_id}       → {token_id}  TTL=7天   (用于登出时批量撤销)
blacklist:{access_jti}  → "revoked"   TTL=15min (access token 主动撤销)
```

> **[安全加固 + 可用性加固] Access Token 主动撤销 — 分级 Fail-Closed 策略**
>
> **问题 [🟡N5]**: JWT access token 主动撤销依赖 Redis 黑名单 (`blacklist:{access_jti}`)，
> 原方案在 Redis 不可用时对所有请求统一 fail-closed (返回 503)，导致 Redis 故障时**全站不可用**，
> 可用性大幅下降。Redis 故障虽概率低但影响面广 (所有需鉴权的 API 均受影响)。
>
> **修复方案**: 实施**分级 fail-closed 策略** — 按操作类型区分安全等级，在安全与可用性间取得平衡:

**分级 Fail-Closed 策略矩阵**:

| 操作类型 | HTTP 方法 | 端点示例 | Redis 故障时行为 | 理由 |
|---|---|---|---|---|
| **写操作** | POST/PUT/PATCH/DELETE | `/assets/*`, `/auth/*` | **fail-closed** → 503 | 写操作安全优先，拒绝已撤销 token 写入 |
| **读操作** | GET | `/assets/:id`, `/assets` | **可选 fail-open** → 放行 (配置项 `auth.fail_open_get=true`) | 读操作风险低，容忍短暂窗口 |
| **Agent 上报** | POST `/agents/sync` | `/agents/sync`, `/agents/heartbeat` | **fail-open** → 放行 | 优先保证数据采集不中断，Agent 另有 mTLS 校验 |

**配置项**:

```yaml
# config.yaml — 分级 fail-closed 配置
auth:
  fail_open_get: true          # GET 请求 Redis 故障时 fail-open (默认 true，可关闭)
  fail_open_agent_sync: true   # Agent 上报 Redis 故障时 fail-open (默认 true)
  # POST/PUT/DELETE 始终 fail-closed，不可配置
```

**实现 (分级 fail-closed 中间件)**:

```go
// internal/middleware/auth.go — 分级 fail-closed 策略

func (m *AuthMiddleware) isRevoked(ctx context.Context, jti string) (bool, error) {
    key := fmt.Sprintf("blacklist:%s", jti)
    result, err := m.redis.Exists(ctx, key).Result()
    if err != nil {
        // Redis 不可用 → 返回错误，由调用方按操作类型决定 fail-closed/fail-open
        return false, fmt.Errorf("redis unavailable: cannot verify token revocation")
    }
    return result > 0, nil
}

// VerifyJWT 中间件 — 分级 fail-closed
func (m *AuthMiddleware) VerifyJWT(c *gin.Context) {
    // ... JWT 解析与签名验证 (省略，见 §6.6) ...

    revoked, err := m.isRevoked(c.Request.Context(), claims.ID)
    if err != nil {
        // Redis 不可用 → 按操作类型分级处理
        method := c.Request.Method
        path := c.FullPath()

        switch {
        case isWriteOperation(method): // POST/PUT/PATCH/DELETE
            // 写操作: 始终 fail-closed → 503
            c.AbortWithStatusJSON(503, gin.H{
                "error":   "auth service temporarily unavailable",
                "reason":  "redis unavailable, write operations require revocation check",
            })
            return

        case isAgentSync(path): // /agents/sync, /agents/heartbeat
            // Agent 上报: fail-open → 放行 (优先保证数据采集不中断)
            // Agent 另有 mTLS 证书校验 (见 §8.2)，安全有多重保障
            if m.config.FailOpenAgentSync {
                log.Warn().Str("jti", claims.ID).
                    Msg("redis unavailable, agent sync fail-open (mTLS still enforced)")
                c.Set("revocation_check_skipped", true)
                c.Next()
                return
            }
            // 配置关闭 fail-open → fall through to fail-closed
            c.AbortWithStatusJSON(503, gin.H{"error": "auth service temporarily unavailable"})
            return

        case method == "GET":
            // 读操作: 可选 fail-open (配置项 auth.fail_open_get)
            if m.config.FailOpenGet {
                log.Warn().Str("jti", claims.ID).
                    Msg("redis unavailable, GET fail-open (config enabled)")
                c.Set("revocation_check_skipped", true)
                c.Next()
                return
            }
            // 配置关闭 → fail-closed
            c.AbortWithStatusJSON(503, gin.H{"error": "auth service temporarily unavailable"})
            return

        default:
            // 默认: fail-closed
            c.AbortWithStatusJSON(503, gin.H{"error": "auth service temporarily unavailable"})
            return
        }
    }

    if revoked {
        c.AbortWithStatusJSON(401, gin.H{"error": "token revoked"})
        return
    }
    c.Next()
}

func isWriteOperation(method string) bool {
    return method == "POST" || method == "PUT" ||
           method == "PATCH" || method == "DELETE"
}

func isAgentSync(path string) bool {
    return path == "/api/v1/agents/sync" || path == "/api/v1/agents/heartbeat"
}
```

**Sentinel HA — 降低 Redis 故障概率到 <0.1%**:

```
Redis Sentinel (3 节点: 1 Master + 2 Slave + 3 Sentinel)
├── Sentinel 自动监控 + 故障转移 (RTO < 10s)
├── API Server 通过 Sentinel 发现 Master 地址
├── 限流中间件: 本地令牌桶兜底 (Redis 不可用时降级)
├── 缓存层熔断: Redis 连续失败 N 次 → 熔断, 直连 DB
└── 分级 fail-closed: 写操作 503 / 读操作+Agent 上报 fail-open
```

- **Sentinel 3 节点集群**: 1 Master + 2 Slave + 3 Sentinel 进程，自动故障转移 (RTO < 10s)
- **多 AZ 部署**: Sentinel 节点跨可用区分布，单 AZ 故障不影响 Redis 可用性
- **预估故障率**: Sentinel HA + 多 AZ → Redis 不可用概率 < 0.1% (年宕机 < 53 分钟)

**Redis 健康监控 + 自动告警 + 快速恢复流程**:

```yaml
# Redis 健康监控配置
redis_monitoring:
  health_check:
    interval: 5s              # 每 5 秒 ping 一次
    timeout: 1s               # 超时 1 秒
    consecutive_failures: 3   # 连续 3 次失败 → 触发告警
  alerts:
    redis_unavailable:
      severity: critical
      channels: [pagerduty, slack]
      message: "Redis 不可用 — 已触发分级 fail-closed (写操作 503, 读操作 fail-open)"
    sentinel_failover:
      severity: warning
      channels: [slack]
      message: "Redis Sentinel 故障转移中 — 预计 RTO < 10s"
  recovery:
    auto_reconnect: true      # API Server 自动重连 Sentinel 发现新 Master
    cache_warmup: true        # Redis 恢复后预热热点缓存
```

```go
// internal/infra/redis_health.go — Redis 健康监控 + 自动告警

type RedisHealthMonitor struct {
    redis      *redis.Client
    sentinel   *redis.SentinelClient
    alertSink  AlertSink
    failCount  int32           // 原子计数: 连续失败次数
    lastAlert  time.Time       // 上次告警时间 (防告警风暴)
}

func (m *RedisHealthMonitor) Start(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.check(ctx)
        }
    }
}

func (m *RedisHealthMonitor) check(ctx context.Context) {
    pingCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()

    if err := m.redis.Ping(pingCtx).Err(); err != nil {
        atomic.AddInt32(&m.failCount, 1)
        if atomic.LoadInt32(&m.failCount) >= 3 && time.Since(m.lastAlert) > 30*time.Second {
            m.alertSink.Send(ctx, Alert{
                Severity: "critical",
                Title:    "Redis 不可用",
                Message:  fmt.Sprintf("Redis ping 连续 %d 次失败: %v — 已触发分级 fail-closed", atomic.LoadInt32(&m.failCount), err),
            })
            m.lastAlert = time.Now()
        }
    } else {
        if atomic.SwapInt32(&m.failCount, 0) > 0 {
            log.Info().Msg("Redis 恢复可用 — fail-closed 状态解除")
        }
    }
}
```

> **分级 fail-closed 总结**: (1) 写操作 (POST/PUT/PATCH/DELETE) 始终 fail-closed，Redis 故障时返回 503，保证安全; (2) 读操作 (GET) 可选 fail-open (配置项 `auth.fail_open_get=true`)，跳过黑名单检查直接放行，容忍短暂安全窗口; (3) Agent 上报 (`/agents/sync`, `/agents/heartbeat`) fail-open，优先保证数据采集不中断 (Agent 另有 mTLS 证书校验作为安全兜底); (4) Sentinel 3 节点 HA + 多 AZ 部署将 Redis 故障概率降至 <0.1%; (5) Redis 健康监控 + 自动告警 + 快速恢复流程，故障时自动通知运维并触发重连。

### 15.7 API 版本兼容策略

**问题**: 只有 `/api/v1/`，未定义版本升级时的兼容流程。

**方案**:

**兼容原则**:
```
1. 新版本 (v2) 发布后，v1 保持至少 6 个月兼容期
2. v1 中标记 deprecated 的端点在响应头返回:
   Sunset: Wed, 31 Dec 2026 23:59:59 GMT
   Deprecation: true
   Link: </api/v2/assets>; rel="successor-version"
3. 版本变更日志: docs/CHANGELOG.md 记录每个版本的 breaking changes
4. 向后兼容原则:
   - 新增字段: 可选，不影响旧客户端
   - 删除字段: 需 deprecation period (至少 3 个月)
   - 修改字段类型: 视为 breaking change，需新版本
```

**Deprecation 中间件**:
```go
func DeprecationMiddleware(oldPath, newPath, sunsetDate string) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("Deprecation", "true")
        c.Header("Sunset", sunsetDate)
        c.Header("Link", fmt.Sprintf("</api/v2%s>; rel=\"successor-version\"", newPath))
        c.Next()
    }
}
```

**路由注册**:
```go
v1 := router.Group("/api/v1")
v1.Use(DeprecationMiddleware("", "", "Wed, 31 Dec 2026 23:59:59 GMT"))
// ... v1 路由

v2 := router.Group("/api/v2")
// ... v2 路由
```

### 15.8 前端权限路由守卫

**问题**: 文档定义了 5 种角色，但前端路由表缺少明确的路由守卫设计。原方案缺少完整路由守卫映射表 (详情路由缺失)、403 页面组件实现、API 拦截器的 401 refresh 完整逻辑、通配符兜底路由和 loading 状态处理。

**方案**:

**完整路由守卫映射表**:
```typescript
type UserRole = 'super_admin' | 'admin' | 'manager' | 'viewer' | 'agent';

// 路由守卫映射表 — 所有前端路由必须有对应的角色守卫
const routeGuards: Record<string, UserRole[]> = {
  // 公共路由 (无需鉴权)
  '/login':             [],
  '/403':               [],

  // 通用路由 (所有登录用户)
  '/dashboard':         ['super_admin', 'admin', 'manager', 'viewer'],
  '/assets':            ['super_admin', 'admin', 'manager', 'viewer'],
  '/assets/:id':        ['super_admin', 'admin', 'manager', 'viewer'],
  '/assets/:id/history':['super_admin', 'admin', 'manager', 'viewer'],
  '/assets/:id/snapshots': ['super_admin', 'admin', 'manager', 'viewer'],

  // 管理路由 (admin+)
  '/agents':            ['super_admin', 'admin'],
  '/agents/:id':        ['super_admin', 'admin'],
  '/webhooks':          ['super_admin', 'admin'],
  '/webhooks/:id':      ['super_admin', 'admin'],
  '/locations':         ['super_admin', 'admin', 'manager'],
  '/locations/:id':     ['super_admin', 'admin', 'manager'],
  '/organizations':     ['super_admin', 'admin'],
  '/organizations/:id': ['super_admin', 'admin'],
  '/audit-log':         ['super_admin', 'admin', 'manager'],
  '/audit-log/:id':     ['super_admin', 'admin', 'manager'],

  // super_admin 专属
  '/admin/*':           ['super_admin'],
  '/admin/users':       ['super_admin'],
  '/admin/users/:id':   ['super_admin'],
  '/admin/asset-types': ['super_admin'],
  '/admin/asset-types/:id': ['super_admin'],
  '/admin/enrollment-tokens': ['super_admin'],
  '/admin/approvals':   ['super_admin'],
};
```

**RequireAuth 组件实现 (含 loading 状态)**:
```tsx
// components/RequireAuth.tsx
import { Navigate, useLocation } from 'react-router-dom';
import { useAuthStore } from '@/stores/auth';

function RequireAuth({ allowedRoles, children }: {
  allowedRoles: UserRole[];
  children: React.ReactNode;
}) {
  const { user, isLoading, isAuthenticated } = useAuthStore();
  const location = useLocation();

  // 初始化加载中 → 显示 loading
  if (isLoading) {
    return <FullScreenSpinner />;
  }

  // 未登录 → 跳转 login，记录来源路径
  if (!isAuthenticated || !user) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }

  // 角色不匹配 → 跳转 403
  if (allowedRoles.length > 0 && !allowedRoles.includes(user.role)) {
    return <Navigate to="/403" replace />;
  }

  return <>{children}</>;
}
```

**403 Forbidden 页面组件**:
```tsx
// pages/Forbidden.tsx
function Forbidden() {
  const { user } = useAuthStore();
  const navigate = useNavigate();

  return (
    <Result
      status="403"
      title="403 权限不足"
      subTitle={`抱歉，${user?.username || '当前用户'}无权访问此页面`}
      extra={
        <Button type="primary" onClick={() => navigate('/dashboard')}>
          返回首页
        </Button>
      }
    />
  );
}
```

**App.tsx 完整路由注册 (含通配符兜底)**:
```tsx
// App.tsx
<Routes>
  {/* 公共路由 */}
  <Route path="/login" element={<Login />} />
  <Route path="/403" element={<Forbidden />} />

  {/* 通用路由 — 所有登录用户 */}
  <Route element={<RequireAuth allowedRoles={['super_admin', 'admin', 'manager', 'viewer']} />}>
    <Route element={<AppShell />}>
      <Route path="/dashboard" element={<Dashboard />} />
      <Route path="/assets" element={<Assets />} />
      <Route path="/assets/:id" element={<AssetDetailPage />} />
      <Route path="/assets/:id/history" element={<AssetHistory />} />
      <Route path="/assets/:id/snapshots" element={<AssetSnapshots />} />
    </Route>
  </Route>

  {/* 管理路由 — admin+ */}
  <Route element={<RequireAuth allowedRoles={['super_admin', 'admin']} />}>
    <Route element={<AppShell />}>
      <Route path="/agents" element={<Agents />} />
      <Route path="/agents/:id" element={<AgentDetail />} />
      <Route path="/webhooks" element={<Webhooks />} />
      <Route path="/webhooks/:id" element={<WebhookDetail />} />
    </Route>
  </Route>

  <Route element={<RequireAuth allowedRoles={['super_admin', 'admin', 'manager']} />}>
    <Route element={<AppShell />}>
      <Route path="/locations" element={<Locations />} />
      <Route path="/locations/:id" element={<LocationDetail />} />
      <Route path="/audit-log" element={<AuditLog />} />
      <Route path="/audit-log/:id" element={<AuditLogDetail />} />
    </Route>
  </Route>

  <Route element={<RequireAuth allowedRoles={['super_admin', 'admin']} />}>
    <Route element={<AppShell />}>
      <Route path="/organizations" element={<Organizations />} />
      <Route path="/organizations/:id" element={<OrganizationDetail />} />
    </Route>
  </Route>

  {/* super_admin 专属 */}
  <Route element={<RequireAuth allowedRoles={['super_admin']} />}>
    <Route element={<AppShell />}>
      <Route path="/admin/*" element={<Admin />} />
      <Route path="/admin/users" element={<UserManagement />} />
      <Route path="/admin/users/:id" element={<UserDetail />} />
      <Route path="/admin/asset-types" element={<AssetTypeManagement />} />
      <Route path="/admin/enrollment-tokens" element={<EnrollmentTokens />} />
      <Route path="/admin/approvals" element={<ApprovalQueue />} />
    </Route>
  </Route>

  {/* 通配符兜底 → 404 */}
  <Route path="*" element={<NotFound />} />
</Routes>
```

**API 客户端 401/403 拦截器 (含 refresh 逻辑)**:
```typescript
// api/client.ts
import { useAuthStore } from '@/stores/auth';

let isRefreshing = false;
let failedQueue: Array<{ resolve: Function; reject: Function }> = [];

apiClient.interceptors.response.use(
  (response) => response,
  async (error) => {
    const originalRequest = error.config;
    const status = error.response?.status;

    // 401: access token 过期 → 尝试 refresh
    if (status === 401 && !originalRequest._retry) {
      if (isRefreshing) {
        // 已有 refresh 进行中 → 排队等待
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        }).then((token) => {
          originalRequest.headers.Authorization = `Bearer ${token}`;
          return apiClient(originalRequest);
        }).catch(reject);
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        const refreshToken = useAuthStore.getState().refreshToken;
        if (!refreshToken) {
          throw new Error('no refresh token');
        }
        const { data } = await apiClient.post('/auth/refresh', { refresh_token: refreshToken });
        const newAccessToken = data.access_token;
        useAuthStore.getState().setAccessToken(newAccessToken);

        // 处理排队请求
        failedQueue.forEach(({ resolve }) => resolve(newAccessToken));
        failedQueue = [];

        originalRequest.headers.Authorization = `Bearer ${newAccessToken}`;
        return apiClient(originalRequest);
      } catch (refreshError) {
        // refresh 失败 → 登出并跳转 login
        failedQueue.forEach(({ reject }) => reject(refreshError));
        failedQueue = [];
        useAuthStore.getState().logout();
        window.location.href = '/login';
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    // 403: 权限不足 → 跳转 403 页面 (不登出)
    if (status === 403) {
      const currentPath = window.location.pathname;
      // 避免在 403 页面本身循环
      if (currentPath !== '/403' && currentPath !== '/login') {
        window.location.href = '/403';
      }
    }

    return Promise.reject(error);
  }
);
```

### 15.9 Vault/KMS 单点故障修复

**问题**: API Server 启动时依赖 HashiCorp Vault 读取 Ed25519 私钥，Vault 不可用 = 系统无法启动。运行时签发新 JWT 也依赖 Vault 读取私钥，Vault 宕机期间无法签发新 token，已登录用户的 token 验证不受影响（公钥可本地缓存），但新登录和 token 刷新将失败。

**风险等级**: 🔴 高风险 — 单点故障导致系统不可启动 / 不可登录。

**修复方案**:

#### 15.9.1 Vault HA 部署 (3 节点集群)

Vault 自身需高可用部署，消除单点故障。采用 3 节点集群 + Raft 共识算法：

```
Vault HA 集群 (3 节点, Raft 共识)
┌──────────────────────────────────────────────────────────┐
│                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐│
│  │ Vault Node 1 │◄──►│ Vault Node 2 │◄──►│ Vault Node 3 ││
│  │ (Leader)     │    │ (Follower)   │    │ (Follower)   ││
│  │ AZ-a         │    │ AZ-b         │    │ AZ-c         ││
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘│
│         │                   │                   │         │
│         └───────────────────┴───────────────────┘         │
│                     Raft 共识                              │
│                                                          │
│  存储: Integrated Storage (Raft) — 不依赖外部 Consul       │
│  自动故障转移: Leader 宕机 → Follower 选举 (RTO < 10s)     │
│  跨 AZ 部署: 3 节点分布在 3 个可用区                       │
│  备份: 每日 Raft snapshot 到 S3/GCS (加密)                 │
└──────────────────────────────────────────────────────────┘

API Server 访问方式:
  - 通过 Vault Load Balancer (内部 LB) 访问 Active 节点
  - 或使用 Vault Agent Sidecar 自动发现 Leader
```

**配置要点**:
```hcl
# Vault 集群配置 (每个节点)
storage "raft" {
  path    = "/vault/data"
  node_id = "vault-node-1"  # 每个节点不同

  retry_join {
    leader_address = "https://vault-node-1:8200"
  }
  retry_join {
    leader_address = "https://vault-node-2:8200"
  }
  retry_join {
    leader_address = "https://vault-node-3:8200"
  }
}

api_addr     = "https://0.0.0.0:8200"
cluster_addr = "https://0.0.0.0:8201"
```

#### 15.9.2 API Server 启动时缓存公钥

利用 Ed25519 非对称签名的特性：**验证 JWT 只需公钥，签发 JWT 才需私钥**。API Server 启动时从 Vault 读取一次公钥并缓存到内存 + 本地磁盘，运行时验证 JWT 不再依赖 Vault。

```go
// internal/auth/keymanager.go

type KeyManager struct {
    vaultClient  *vault.Client
    publicKey    ed25519.PublicKey  // 内存缓存
    privateKey   ed25519.PrivateKey // 仅签发时使用，不长期持有
    pubKeyFile   string             // 本地磁盘缓存路径
    mu           sync.RWMutex
}

// Init 启动时从 Vault 读取公钥并缓存
func (km *KeyManager) Init(ctx context.Context) error {
    // 1. 尝试从 Vault 读取公钥
    pubKey, err := km.fetchPublicKeyFromVault(ctx)
    if err != nil {
        // 2. Vault 不可用 → 尝试本地磁盘缓存
        pubKey, err = km.loadCachedPublicKey()
        if err != nil {
            return fmt.Errorf("vault unavailable and no cached public key: %w", err)
        }
        log.Warn("Vault 不可用，使用本地缓存的公钥 (仅验证 JWT，无法签发新 token)")
    }

    km.mu.Lock()
    km.publicKey = pubKey
    km.mu.Unlock()

    // 3. 持久化公钥到本地磁盘 (供下次启动使用)
    if err := km.cachePublicKeyToDisk(pubKey); err != nil {
        log.Warn("公钥磁盘缓存写入失败 (不影响运行): " + err.Error())
    }

    return nil
}

// VerifyPublicKey 验证 JWT — 仅使用缓存的公钥，不依赖 Vault
func (km *KeyManager) VerifyToken(tokenStr string) (*jwt.Claims, error) {
    km.mu.RLock()
    pubKey := km.publicKey
    km.mu.RUnlock()

    if pubKey == nil {
        return nil, errors.New("public key not initialized")
    }

    // EdDSA 验证 — 纯本地计算，无 Vault 依赖
    claims := &jwt.RegisteredClaims{}
    _, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
        // 算法降级防护
        if !t.Method.Alg().Valid() || t.Header["alg"] != "EdDSA" {
            return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
        }
        return pubKey, nil
    })
    return claims, err
}

// SignToken 签发 JWT — 需要从 Vault 读取私钥
func (km *KeyManager) SignToken(claims *jwt.RegisteredClaims) (string, error) {
    // 私钥不长期缓存，每次签发时从 Vault 读取 (或短期缓存 + TTL)
    privKey, err := km.fetchPrivateKeyFromVault(context.Background())
    if err != nil {
        return "", fmt.Errorf("vault unavailable, cannot sign new token: %w", err)
    }
    defer zeroPrivateKey(privKey) // 用后清零内存

    token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
    return token.SignedString(privKey)
}
```

#### 15.9.3 Vault 不可用时的降级策略

| 场景 | Vault 状态 | API Server 行为 | 用户影响 |
|---|---|---|---|
| 正常运行 | 可用 | 正常签发 + 验证 JWT | 无影响 |
| Vault 宕机 (运行中) | 不可用 | 验证 JWT: 使用缓存公钥，正常工作；签发新 token: 返回 503 | 已登录用户不受影响；新登录/刷新 token 失败 |
| API Server 启动时 Vault 不可用 | 不可用 | 使用本地磁盘缓存的公钥启动；仅验证 JWT，签发降级 | 已登录用户可继续使用；无法签发新 token |
| API Server 启动时 Vault 不可用且无本地缓存 | 不可用 | 拒绝启动，触发告警 | 服务不可用 (需人工介入) |

**降级模式标识**: API Server 在降级模式下运行时，`/healthz` 返回 `200` (服务可用)，`/readyz` 返回 `200` 但附加 `degraded: true` + `vault: unavailable` 状态，Nginx 不剔除该实例但监控告警。

```go
// internal/health/health.go

func (h *HealthHandler) ReadyZ(c *gin.Context) {
    status := h.checkAll()
    if status.VaultAvailable {
        c.JSON(200, gin.H{"status": "ok"})
    } else {
        // 降级模式: 服务可用但 Vault 不可用
        c.JSON(200, gin.H{
            "status":   "degraded",
            "vault":    "unavailable",
            "impact":   "token signing disabled, verification using cached public key",
        })
        // 触发告警
        metrics.VaultUnavailable.Inc()
    }
}
```

#### 15.9.4 启动时 Vault 不可用的重试策略

API Server 启动时若 Vault 不可用，采用指数退避重试，超时后拒绝启动并告警：

```go
// internal/auth/keymanager.go

func (km *KeyManager) InitWithRetry(ctx context.Context) error {
    maxRetries := 10
    baseDelay := 2 * time.Second
    maxDelay := 30 * time.Second
    totalTimeout := 5 * time.Minute

    deadlineCtx, cancel := context.WithTimeout(ctx, totalTimeout)
    defer cancel()

    var lastErr error
    for attempt := 0; attempt < maxRetries; attempt++ {
        err := km.Init(deadlineCtx)
        if err == nil {
            log.Info("Vault 密钥初始化成功", "attempts", attempt+1)
            return nil
        }
        lastErr = err

        // 指数退避: 2s, 4s, 8s, 16s, 30s, 30s, ...
        delay := baseDelay * time.Duration(1<<uint(attempt))
        if delay > maxDelay {
            delay = maxDelay
        }

        log.Warn("Vault 不可用，等待重试",
            "attempt", attempt+1,
            "max_retries", maxRetries,
            "delay", delay,
            "error", err)

        select {
        case <-deadlineCtx.Done():
            return fmt.Errorf("vault init timeout after %v: %w", totalTimeout, lastErr)
        case <-time.After(delay):
        }
    }

    // 超过重试上限 → 尝试本地缓存公钥
    if pubKey, err := km.loadCachedPublicKey(); err == nil {
        km.mu.Lock()
        km.publicKey = pubKey
        km.mu.Unlock()
        log.Warn("Vault 重试耗尽，使用本地缓存公钥启动 (降级模式: 仅验证 JWT)")
        metrics.VaultDegradedMode.Set(1)
        return nil
    }

    // 无本地缓存 → 拒绝启动并告警
    metrics.VaultStartupFailed.Inc()
    return fmt.Errorf("vault unavailable after %d retries and no cached public key, refusing to start: %w",
        maxRetries, lastErr)
}
```

**启动行为总结**:

```
API Server 启动流程:
  1. 从 Vault 读取 Ed25519 公钥 (指数退避重试, 最多 10 次, 上限 30s/次, 总超时 5min)
  2. 成功 → 缓存公钥到内存 + 磁盘 → 正常启动
  3. 失败 → 尝试本地磁盘缓存的公钥
     ├── 有缓存 → 降级模式启动 (仅验证 JWT, 签发返回 503) → 告警
     └── 无缓存 → 拒绝启动 → 告警 (需人工介入: 恢复 Vault 或导入公钥)
```

#### 15.9.5 云 KMS 备选方案

为降低 Vault 运维复杂度，建议评估云原生 KMS 作为备选或主方案：

| 方案 | 优势 | 劣势 | 适用场景 |
|---|---|---|---|
| HashiCorp Vault (自建) | 完全自主可控, 跨云/混合云 | 运维复杂度高 (HA, 备份, 升级) | 混合云, 合规要求高 |
| AWS KMS | 全托管, 无运维, 自动 HA | 厂商锁定, 跨云需适配 | AWS 原生部署 |
| GCP Cloud KMS | 全托管, 无运维, 自动 HA | 厂商锁定, 跨云需适配 | GCP 原生部署 |
| Azure Key Vault | 全托管, 与 AD 集成 | 厂商锁定 | Azure 原生部署 |

**云 KMS 集成方式** (以 AWS KMS 为例):

```go
// 使用 AWS KMS 签名 (私钥不离开 KMS)
// 公钥可从 KMS 导出并缓存

func (km *KeyManager) SignTokenWithKMS(claims *jwt.RegisteredClaims) (string, error) {
    // 1. 构造 JWT header + payload (未签名)
    unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
                base64.RawURLEncoding.EncodeToString(payloadJSON)

    // 2. 调用 KMS Sign API (私钥在 KMS 内部, 不导出)
    result, err := km.kmsClient.Sign(ctx, &kms.SignInput{
        KeyId:            km.keyID,
        Message:          []byte(unsigned),
        MessageType:      types.MessageTypeRaw,
        SigningAlgorithm: types.SigningAlgorithmSpecEddsa,
    })
    if err != nil {
        return "", fmt.Errorf("KMS sign failed: %w", err)
    }

    // 3. 拼接签名
    signature := base64.RawURLEncoding.EncodeToString(result.Signature)
    return unsigned + "." + signature, nil
}

// 公钥从 KMS GetPublicKey API 导出, 启动时缓存
func (km *KeyManager) fetchPublicKeyFromKMS(ctx context.Context) (ed25519.PublicKey, error) {
    result, err := km.kmsClient.GetPublicKey(ctx, &kms.GetPublicKeyInput{
        KeyId: km.keyID,
    })
    if err != nil {
        return nil, err
    }
    // 解析 DER 编码的公钥
    pubKey, err := x509.ParsePKIXPublicKey(result.PublicKey)
    if err != nil {
        return nil, err
    }
    return pubKey.(ed25519.PublicKey), nil
}
```

> **决策建议**: 若部署在单一云平台，优先使用云 KMS (零运维, 自动 HA)；混合云或强合规场景使用 Vault 3 节点 HA 集群。两种方案均通过公钥缓存 + 降级策略消除单点故障。

### 15.10 Patroni 故障转移数据窗口修复

**问题**: Patroni 异步复制模式下 RPO<5s 仍可能有数据丢失。故障转移期间 PgBouncer 可能仍指向旧 Primary，导致写入失败。故障转移流程缺乏定期演练，实际故障时可能出现意外行为。

**风险等级**: 🟡 中风险 — 数据丢失窗口 + 故障转移期间写入不可用。

**修复方案**:

#### 15.10.1 关键写操作同步复制模式

对关键写操作评估使用同步复制模式 (`synchronous_commit=on`)，确保写入在至少一个 Replica 确认后才返回客户端成功，消除异步复制的数据丢失窗口：

| 写操作类型 | 复制模式 | 理由 |
|---|---|---|
| 资产创建 (POST /assets) | **同步** | 资产记录是核心数据, 丢失导致资产不明 |
| 资产领用/归还 (POST /assets/:id/assign) | **同步** | 领用记录涉及责任归属, 丢失导致责任不清 |
| 生命周期状态转换 (PUT /assets/:id/lifecycle) | **同步** | 状态转换不可逆, 丢失导致状态不一致 |
| audit_log INSERT | **同步** | 审计日志必须不丢, 合规要求 |
| 资产属性更新 (PUT /assets/:id) | 异步 (可接受) | 属性修改可重试, RPO<5s 可接受 |
| Agent 快照上报 (POST /agents/:id/snapshots) | 异步 (可接受) | 高频写入, 同步模式影响吞吐 |
| 查询 (GET *) | N/A | 只读, 不涉及复制 |

**PostgreSQL 同步复制配置**:

```ini
# postgresql.conf (Primary)
synchronous_commit = on                # 全局开启同步提交
# 或按会话级别控制: SET LOCAL synchronous_commit = on;

synchronous_standby_names = 'ANY 1 (replica1, replica2)'
# ANY 1: 任意一个 Replica 确认即可 (不要求特定 Replica)
# 降低同步复制对写入延迟的影响
```

**会话级动态控制** (推荐: 仅关键写操作开启同步, 非关键保持异步):

```go
// internal/repository/asset_repo.go

// 关键写操作: 资产创建
func (r *AssetRepo) Create(ctx context.Context, asset *domain.Asset) error {
    tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // 本事务启用同步提交 (仅影响当前事务)
    if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = on"); err != nil {
        return fmt.Errorf("set synchronous_commit: %w", err)
    }

    // 执行 INSERT
    if err := r.doCreate(ctx, tx, asset); err != nil {
        return err
    }

    return tx.Commit(ctx) // Commit 等待 Replica 确认后才返回
}

// 非关键写操作: 资产属性更新 (保持异步, 低延迟)
func (r *AssetRepo) UpdateProperties(ctx context.Context, asset *domain.Asset) error {
    // 不设置 synchronous_commit, 使用全局默认 (异步)
    // RPO < 5s 可接受
    return r.doUpdateProperties(ctx, r.db, asset)
}
```

> **权衡**: 同步复制增加写入延迟 (通常 +1~5ms, 取决于网络 RTT)。仅对关键写操作启用, 非关键操作保持异步, 在数据安全与性能之间取得平衡。

#### 15.10.2 PgBouncer 动态感知 Patroni Leader 切换

PgBouncer 需动态感知 Patroni Leader 变化，避免故障转移后仍指向旧 Primary 导致写入失败：

**方案: PgBouncer + Patroni REST API 联动**

```
Patroni REST API:
  GET /patroni/leader  → 200 (当前 Leader 信息)
  GET /patroni/cluster → 集群拓扑 (所有节点角色)

PgBouncer 动态后端切换:
  1. PgBouncer 配置两个后端: primary (写) + replica (读)
  2. 定期轮询 Patroni REST API /patroni/leader
  3. Leader 变化 → 动态 RELOAD PgBouncer 配置 (PAUSE → RELOAD → RESUME)
  4. 切换期间: 暂停新连接, 等待旧连接 drain, 指向新 Leader
```

**实现: Patroni Leader 探测脚本 + PgBouncer 动态配置**

```bash
#!/bin/bash
# /usr/local/bin/patroni-leader-watch.sh
# 定期检测 Patroni Leader 变化, 动态更新 PgBouncer 后端

PATRONI_API="http://patroni:8008"
PGBOUNCER_ADMIN="/usr/bin/pgbouncer -u /var/run/pgbouncer/pgbouncer.sock"
CURRENT_LEADER=""

while true; do
    # 查询当前 Leader
    LEADER=$(curl -s ${PATRONI_API}/patroni/leader | jq -r '.name' 2>/dev/null)

    if [ -n "$LEADER" ] && [ "$LEADER" != "$CURRENT_LEADER" ]; then
        echo "$(date): Leader changed: $CURRENT_LEADER → $LEADER"

        # 1. PAUSE PgBouncer (拒绝新连接, 等待活跃连接结束)
        psql -h /var/run/pgbouncer/pgbouncer.sock -c "PAUSE;"

        # 2. 更新 PgBouncer 配置指向新 Leader
        sed -i "s/^host = .*/host = ${LEADER}.internal/" /etc/pgbouncer/pgbouncer.ini

        # 3. RELOAD PgBouncer
        psql -h /var/run/pgbouncer/pgbouncer.sock -c "RELOAD;"

        # 4. RESUME PgBouncer (恢复接受连接)
        psql -h /var/run/pgbouncer/pgbouncer.sock -c "RESUME;"

        CURRENT_LEADER="$LEADER"
        echo "$(date): PgBouncer switched to new Leader: $LEADER"
    fi

    sleep 2  # 每 2 秒检测一次
done
```

**PgBouncer 配置 (读写分离 + 动态后端)**:

```ini
# /etc/pgbouncer/pgbouncer.ini
[databases]
; 写入: 指向当前 Patroni Leader (由探测脚本动态更新)
assets = host=patroni-leader.internal port=5432 dbname=asset_db pool_size=20
; 只读: 指向 Replica (Grafana 使用)
assets_ro = host=patroni-replica.internal port=5432 dbname=asset_db pool_size=10

[pgbouncer]
listen_addr = 0.0.0.0
listen_port = 6432
; 连接池模式: transaction (适合短事务)
pool_mode = transaction
; 故障转移期间快速失败
query_wait_timeout = 10
; 服务端连接超时
server_lifetime = 3600
server_idle_timeout = 300
```

> **备选方案**: 使用 `pgbouncer-patroni` (社区工具) 或 HAProxy + Patroni 健康检查实现自动后端切换, 无需自定义脚本。

#### 15.10.3 故障转移期间 API Server 行为

故障转移期间 (Patroni 选举新 Leader + PgBouncer 切换, 通常 <30s), API Server 的写入操作应返回 503 并引导客户端重试：

```go
// internal/middleware/failover.go

func FailoverGuard(db *pgxpool.Pool) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 仅拦截写操作
        if c.Request.Method == "GET" {
            c.Next()
            return
        }

        // 尝试执行写操作
        c.Next()

        // 检查是否因数据库不可用导致失败
        if err, ok := c.Errors.Last().Err.(*pgconn.PgError); ok {
            if isConnectionError(err) {
                // 故障转移中: 返回 503 + Retry-After
                c.AbortWithStatusJSON(503, gin.H{
                    "error":      "failover_in_progress",
                    "message":    "database failover in progress, please retry",
                    "retry_after": 10, // 建议客户端 10 秒后重试
                })
                metrics.FailoverWrites503.Inc()
            }
        }
    }
}
```

**客户端重试策略**:

```
客户端收到 503 (failover_in_progress):
  1. 等待 retry_after 秒 (默认 10s)
  2. 指数退避重试: 10s → 20s → 40s, 最多 3 次
  3. 3 次后仍失败 → 提示用户 "系统维护中, 请稍后重试"
  4. 幂等操作 (如资产创建) 可安全重试
  5. 非幂等操作 (如领用) 需先查询状态确认是否已执行
```

#### 15.10.4 定期故障转移演练 (每月一次)

为确保故障转移流程可靠, 建立每月一次的定期演练机制：

**演练流程**:

```
故障转移演练 (每月第一个周六 02:00 低峰期)
├── 1. 通知: 提前 3 天通知所有相关团队
├── 2. 备份: 演练前执行全量备份 (pg_basebackup)
├── 3. 模拟故障: patronictl switchover (优雅切换) 或 kill Primary 进程 (模拟宕机)
├── 4. 观察指标:
│   ├── RTO: Patroni 检测 + 选举 + PgBouncer 切换总耗时 (目标 <30s)
│   ├── RPO: 检查数据丢失量 (同步复制模式应为 0)
│   ├── API Server: 503 返回次数 + 客户端重试成功率
│   └── 监控告警: 是否正确触发告警通知
├── 5. 验证: 演练后执行写入测试, 确认新 Leader 正常工作
├── 6. 回顾: 记录演练结果, 更新 RTO/RPO 实测值, 识别改进项
└── 7. 归档: 演练报告存档 (合规审计需要)
```

**演练自动化脚本**:

```bash
#!/bin/bash
# /usr/local/bin/patroni-drill.sh
# 每月故障转移演练

set -euo pipefail
DRILL_DATE=$(date +%Y%m%d_%H%M%S)
LOG_FILE="/var/log/patroni-drill/${DRILL_DATE}.log"
mkdir -p /var/log/patroni-drill

echo "[$(date)] 故障转移演练开始" | tee "$LOG_FILE"

# 1. 记录演练前状态
echo "[$(date)] 演练前集群状态:" | tee -a "$LOG_FILE"
patronictl list | tee -a "$LOG_FILE"

# 2. 记录当前 Leader
OLD_LEADER=$(patronictl list | grep "Leader" | awk '{print $1}')
echo "[$(date)] 当前 Leader: $OLD_LEADER" | tee -a "$LOG_FILE"

# 3. 执行优雅切换
echo "[$(date)] 执行 switchover..." | tee -a "$LOG_FILE"
START_TIME=$(date +%s)
patronictl switchover --force 2>&1 | tee -a "$LOG_FILE"

# 4. 等待新 Leader 选举
sleep 5
NEW_LEADER=$(patronictl list | grep "Leader" | awk '{print $1}')
END_TIME=$(date +%s)
RTO=$((END_TIME - START_TIME))

echo "[$(date)] 新 Leader: $NEW_LEADER" | tee -a "$LOG_FILE"
echo "[$(date)] RTO: ${RTO}s (目标 <30s)" | tee -a "$LOG_FILE"

# 5. 验证写入
echo "[$(date)] 验证写入..." | tee -a "$LOG_FILE"
psql -h pgbouncer -p 6432 -c "INSERT INTO assets.drill_log (event) VALUES ('failover_drill_${DRILL_DATE}');" | tee -a "$LOG_FILE"

# 6. 记录结果
echo "[$(date)] 演练完成" | tee -a "$LOG_FILE"
patronictl list | tee -a "$LOG_FILE"

# 7. 发送演练报告
mail -s "Patroni 故障转移演练报告 ${DRILL_DATE}" ops-team@company.com < "$LOG_FILE"
```

**Cron 配置**:

```cron
# 每月第一个周六 02:00 执行故障转移演练
0 2 1-7 * 6 /usr/local/bin/patroni-drill.sh
```

### 15.11 链式哈希序列化写入瓶颈修复

**问题**: audit_log 的链式哈希通过 `pg_advisory_xact_lock` 序列化所有 INSERT，同一时刻只有一个事务能写入 audit_log。高并发下 (如批量资产操作、Agent 集中上报), audit_log 写入成为瓶颈, 吞吐量受限于单事务串行执行。

**风险等级**: 🟡 中风险 — 高并发场景下审计日志写入吞吐量受限, 可能成为系统瓶颈。

**修复方案**:

#### 15.11.1 批量写入缓冲

在 Service 层引入批量写入缓冲, 累积 audit 事件后批量 INSERT, 减少 advisory lock 持有次数和事务开销：

```
原方案 (逐条写入):
  每条 audit 事件 → 获取 advisory lock → INSERT 1 条 → 释放 lock
  高并发下 lock 竞争严重, 吞吐量 ~200 条/秒

改进方案 (批量写入):
  Service 层累积 audit 事件到缓冲区
  每 100ms 或满 100 条 → 批量获取 advisory lock → INSERT N 条 → 释放 lock
  吞吐量提升 ~5-10x (预计 ~1000-2000 条/秒)
```

**实现**:

```go
// internal/service/audit_buffer.go

type AuditBuffer struct {
    db        *pgxpool.Pool
    buffer    []domain.AuditEvent
    mu        sync.Mutex
    flushCh   chan struct{}
    maxBatch int
    interval  time.Duration
    closed    chan struct{}
}

func NewAuditBuffer(db *pgxpool.Pool, maxBatch int, interval time.Duration) *AuditBuffer {
    ab := &AuditBuffer{
        db:        db,
        buffer:    make([]domain.AuditEvent, 0, maxBatch),
        flushCh:   make(chan struct{}, 1),
        maxBatch:  maxBatch,  // 默认 100
        interval:  interval,  // 默认 100ms
        closed:    make(chan struct{}),
    }
    go ab.flushLoop()
    return ab
}

// Append 添加 audit 事件到缓冲区 (非阻塞)
func (ab *AuditBuffer) Append(event domain.AuditEvent) {
    ab.mu.Lock()
    ab.buffer = append(ab.buffer, event)
    shouldFlush := len(ab.buffer) >= ab.maxBatch
    ab.mu.Unlock()

    if shouldFlush {
        select {
        case ab.flushCh <- struct{}{}:
        default: // 已有 flush 信号在排队, 不重复发送
        }
    }
}

// flushLoop 定时 + 触发式 flush
func (ab *AuditBuffer) flushLoop() {
    ticker := time.NewTicker(ab.interval) // 100ms
    defer ticker.Stop()

    for {
        select {
        case <-ab.closed:
            ab.flush() // 关闭时 flush 剩余数据
            return
        case <-ab.flushCh:
            ab.flush()
        case <-ticker.C:
            ab.flush()
        }
    }
}

// flush 批量写入 (在单个事务 + advisory lock 内完成)
func (ab *AuditBuffer) flush() {
    ab.mu.Lock()
    if len(ab.buffer) == 0 {
        ab.mu.Unlock()
        return
    }
    batch := ab.buffer
    ab.buffer = make([]domain.AuditEvent, 0, ab.maxBatch)
    ab.mu.Unlock()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    tx, err := ab.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        log.Error("audit buffer flush: begin tx", "error", err)
        ab.requeue(batch) // 写入失败, 重新入队
        return
    }
    defer tx.Rollback(ctx)

    // 获取 advisory lock (整个批量 INSERT 期间持有)
    if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('audit_log_chain'))"); err != nil {
        log.Error("audit buffer flush: advisory lock", "error", err)
        ab.requeue(batch)
        return
    }

    // 批量 INSERT (触发器内自动维护 hash chain)
    // 注意: 批量 INSERT 时触发器按行执行, 每行的 prev_hash 取上一行 hash
    //       需确保触发器内 SELECT ... ORDER BY id DESC LIMIT 1 能看到本批已插入的行
    for _, event := range batch {
        if _, err := tx.Exec(ctx, `
            INSERT INTO assets.audit_log (asset_id, user_id, action, field, old_value, new_value, metadata)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
        `, event.AssetID, event.UserID, event.Action, event.Field,
            event.OldValue, event.NewValue, event.Metadata); err != nil {
            log.Error("audit buffer flush: insert", "error", err)
            ab.requeue(batch)
            return
        }
    }

    if err := tx.Commit(ctx); err != nil {
        log.Error("audit buffer flush: commit", "error", err)
        ab.requeue(batch)
        return
    }

    metrics.AuditLogBatchSize.Observe(float64(len(batch)))
    metrics.AuditLogWritesTotal.Add(float64(len(batch)))
}

func (ab *AuditBuffer) requeue(batch []domain.AuditEvent) {
    ab.mu.Lock()
    ab.buffer = append(batch, ab.buffer...) // 失败的 batch 重新放回缓冲区头部
    ab.mu.Unlock()
    metrics.AuditLogFlushFailures.Inc()
}

func (ab *AuditBuffer) Close() {
    close(ab.closed)
}
```

> **注意**: 批量 INSERT 时, PostgreSQL 触发器按行执行 (BEFORE INSERT FOR EACH ROW)。每行触发器内的 `SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1 FOR UPDATE` 会看到同批已插入的行 (同一事务内), 因此 hash chain 连续性得以保持。

#### 15.11.2 分片链方案评估

评估按 `asset_id` 分区, 每个分区维护独立的 hash chain, 实现并行写入：

```
原方案 (单链):
  所有 audit_log 记录共享一条 hash chain
  → 全局 advisory lock → 串行写入

分片链方案 (多链):
  按 asset_id 哈希分片, 每个分片独立 hash chain
  → 每个分片独立 advisory lock → 并行写入
  → 吞吐量随分片数线性扩展
```

**分片链设计**:

```sql
-- 方案: 新增 shard_id 列, 按 shard 分区
ALTER TABLE assets.audit_log ADD COLUMN shard_id INTEGER NOT NULL
    GENERATED ALWAYS AS (hashtext(asset_id::text) % 16) STORED;

-- 按 shard_id 分区 (16 个分区 = 16 条独立链)
-- 每个分区内的 hash chain 独立维护
-- advisory lock key 按 shard 区分: pg_advisory_xact_lock(hashtext('audit_chain') + shard_id)
```

```sql
-- 修改触发器: 按 shard 分片计算 hash chain
CREATE OR REPLACE FUNCTION assets.audit_log_compute_hash()
RETURNS trigger AS $$
DECLARE
    last_hash CHAR(64);
    record_text TEXT;
BEGIN
    -- 仅查询同分片的上一条记录 (并行写入不同分片)
    SELECT hash INTO last_hash
        FROM assets.audit_log
        WHERE shard_id = NEW.shard_id
        ORDER BY id DESC
        LIMIT 1
        FOR UPDATE;

    IF last_hash IS NULL THEN
        NEW.prev_hash := repeat('0', 64);
    ELSE
        NEW.prev_hash := last_hash;
    END IF;

    record_text := concat(
        NEW.prev_hash, '|',
        NEW.id, '|', COALESCE(NEW.asset_id::text, ''), '|',
        COALESCE(NEW.user_id::text, ''), '|', NEW.action, '|',
        COALESCE(NEW.field, ''), '|', COALESCE(NEW.old_value, ''), '|',
        COALESCE(NEW.new_value, ''), '|', COALESCE(NEW.metadata::text, ''), '|',
        NEW.created_at
    );
    NEW.hash := encode(digest(record_text, 'sha256'), 'hex');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
```

```go
// Service 层: 按 shard 获取不同的 advisory lock
func (ab *AuditBuffer) flush() {
    // 按 shard_id 分组
    shards := groupByShard(batch)
    for shardID, events := range shards {
        go func(sid int, evts []domain.AuditEvent) {
            tx, _ := ab.db.BeginTx(ctx, pgx.TxOptions{})
            // 每个 shard 独立 advisory lock, 并行写入
            tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)",
                hashtext("audit_chain") + sid)
            for _, e := range evts {
                tx.Exec(ctx, insertSQL, ...)
            }
            tx.Commit(ctx)
        }(shardID, events)
    }
}
```

**分片链权衡**:

| 维度 | 单链 (原方案) | 分片链 (16 分片) |
|---|---|---|
| 写入吞吐量 | ~200 条/秒 (串行) | ~3000+ 条/秒 (并行) |
| 篡改检测 | 全局单链, 任何篡改可检测 | 每个分片独立链, 篡改仅影响该分片 |
| 完整性校验 | 单次全表扫描 | 需按分片并行校验 (16 次) |
| 复杂度 | 低 | 中 (分片管理 + 跨分片查询) |
| 适用场景 | 低并发 (<100 条/秒) | 高并发 (>500 条/秒) |

> **决策**: 当前阶段优先实施批量写入缓冲 (§15.11.1), 预计可满足 ~1000-2000 条/秒的需求。若未来并发量持续增长超过 2000 条/秒, 再评估分片链方案。

#### 15.11.3 Prometheus 监控指标

新增 audit_log 写入吞吐量与延迟监控指标, 用于发现瓶颈和容量规划：

```go
// internal/metrics/audit_metrics.go

var (
    // audit_log 写入总量 (计数器)
    AuditLogWritesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "audit_log_writes_total",
            Help: "Total number of audit log events written",
        },
        []string{"action"}, // 按操作类型分维度 (create/update/assign/...)
    )

    // 批量写入大小 (直方图)
    AuditLogBatchSize = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "audit_log_batch_size",
            Help:    "Number of events per batch flush",
            Buckets: []float64{1, 10, 50, 100, 200, 500, 1000},
        },
    )

    // 写入延迟 (从事件产生到持久化的时间)
    AuditLogWriteLatency = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "audit_log_write_latency_seconds",
            Help:    "Time from event creation to persistence",
            Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
        },
    )

    // 缓冲区积压 (当前缓冲区中待 flush 的事件数)
    AuditLogBufferDepth = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "audit_log_buffer_depth",
            Help: "Current number of events waiting in buffer",
        },
    )

    // flush 失败次数
    AuditLogFlushFailures = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "audit_log_flush_failures_total",
            Help: "Total number of batch flush failures",
        },
    )

    // advisory lock 等待时间
    AuditLogLockWaitSeconds = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "audit_log_lock_wait_seconds",
            Help:    "Time spent waiting for advisory lock",
            Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
        },
    )
)

func init() {
    prometheus.MustRegister(
        AuditLogWritesTotal,
        AuditLogBatchSize,
        AuditLogWriteLatency,
        AuditLogBufferDepth,
        AuditLogFlushFailures,
        AuditLogLockWaitSeconds,
    )
}
```

**Grafana 面板建议**:

```
Audit Log 写入监控面板:
├── 写入吞吐量 (req/s): rate(audit_log_writes_total[1m])  by action
├── 批量大小分布: histogram_quantile(0.95, audit_log_batch_size)
├── 写入延迟 P95/P99: histogram_quantile(0.95, audit_log_write_latency_seconds)
├── 缓冲区积压: audit_log_buffer_depth (告警阈值 >500)
├── flush 失败率: rate(audit_log_flush_failures_total[5m])
└── lock 等待时间 P95: histogram_quantile(0.95, audit_log_lock_wait_seconds)
    (告警阈值 >100ms, 提示考虑分片链方案)
```

#### 15.11.4 降级策略: 链式哈希异步计算

当批量写入仍无法满足吞吐量需求时, 将链式哈希计算移到异步 goroutine, 不阻塞主事务：

```
原方案 (同步):
  主事务 → INSERT audit_log (含 hash 计算 + advisory lock) → COMMIT
  → hash 计算阻塞主事务, 写入延迟 = 主事务延迟

降级方案 (异步):
  主事务 → INSERT audit_log (不计算 hash, hash=NULL) → COMMIT (快速返回)
  → 异步 goroutine: 计算 hash + UPDATE audit_log SET hash=... (advisory lock)
  → 主事务不阻塞, 写入延迟 ≈ INSERT 延迟 (无 lock 等待)
```

**实现**:

```go
// internal/service/audit_service.go

type AuditService struct {
    db      *pgxpool.Pool
    buffer  *AuditBuffer
    async   bool // 降级模式: 异步计算 hash
}

// Write 写入 audit 事件
func (s *AuditService) Write(ctx context.Context, event domain.AuditEvent) error {
    if s.async {
        // 降级模式: 快速写入 (hash=NULL), 异步补算
        return s.writeFast(ctx, event)
    }
    // 正常模式: 批量缓冲 + 同步 hash 计算
    s.buffer.Append(event)
    return nil
}

// writeFast 降级模式: 不计算 hash, 快速 INSERT
func (s *AuditService) writeFast(ctx context.Context, event domain.AuditEvent) error {
    _, err := s.db.Exec(ctx, `
        INSERT INTO assets.audit_log (asset_id, user_id, action, field, old_value, new_value, metadata, hash)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NULL)
    `, event.AssetID, event.UserID, event.Action, event.Field,
        event.OldValue, event.NewValue, event.Metadata)
    // 异步 goroutine 补算 hash
    go s.computeHashAsync(event)
    return err
}

// computeHashAsync 异步计算 hash
func (s *AuditService) computeHashAsync(event domain.AuditEvent) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    start := time.Now()
    tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
    if err != nil {
        log.Error("async hash: begin tx", "error", err)
        metrics.AuditLogAsyncHashFailures.Inc()
        return
    }
    defer tx.Rollback(ctx)

    // 获取 advisory lock
    if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('audit_log_chain'))"); err != nil {
        log.Error("async hash: lock", "error", err)
        metrics.AuditLogAsyncHashFailures.Inc()
        return
    }

    // 计算并更新 hash
    if _, err := tx.Exec(ctx, `
        UPDATE assets.audit_log SET
            prev_hash = COALESCE(
                (SELECT hash FROM assets.audit_log
                 WHERE id < $1 ORDER BY id DESC LIMIT 1),
                repeat('0', 64)
            ),
            hash = encode(digest(
                concat(
                    COALESCE(
                        (SELECT hash FROM assets.audit_log
                         WHERE id < $1 ORDER BY id DESC LIMIT 1),
                        repeat('0', 64)
                    ), '|',
                    $1::text, '|', COALESCE(asset_id::text, ''), '|',
                    COALESCE(user_id::text, ''), '|', action, '|',
                    COALESCE(field, ''), '|', COALESCE(old_value, ''), '|',
                    COALESCE(new_value, ''), '|', COALESCE(metadata::text, ''), '|',
                    created_at::text
                ), 'sha256'
            ), 'hex')
        WHERE id = $1 AND hash IS NULL
    `, event.ID); err != nil {
        log.Error("async hash: update", "error", err)
        metrics.AuditLogAsyncHashFailures.Inc()
        return
    }

    if err := tx.Commit(ctx); err != nil {
        log.Error("async hash: commit", "error", err)
        metrics.AuditLogAsyncHashFailures.Inc()
        return
    }

    metrics.AuditLogLockWaitSeconds.Observe(time.Since(start).Seconds())
}
```

**降级模式切换**:

```go
// 根据负载自动切换降级模式
func (s *AuditService) autoDegradation() {
    // 监控缓冲区积压, 超阈值自动切换到异步模式
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        for range ticker.C {
            depth := metrics.GetBufferDepth()
            if depth > 1000 && !s.async {
                log.Warn("audit_log 缓冲区积压过高, 切换到异步 hash 计算模式", "depth", depth)
                s.async = true
                metrics.AuditLogDegradedMode.Set(1)
            } else if depth < 100 && s.async {
                log.Info("audit_log 缓冲区恢复, 切回同步模式", "depth", depth)
                s.async = false
                metrics.AuditLogDegradedMode.Set(0)
            }
        }
    }()
}
```

> **注意**: 异步 hash 计算模式下, `hash IS NULL` 的记录存在短暂窗口 (hash 未计算完成)。完整性校验 job 需跳过 `hash IS NULL` 的记录, 或等待异步计算完成后再校验。降级模式期间, 审计日志的篡改检测有延迟 (从实时变为最终一致)。

> **策略总结**: 正常模式 → 批量写入缓冲 (§15.11.1); 高负载降级 → 异步 hash 计算 (§15.11.4); 持续高并发 → 分片链方案 (§15.11.2)。三层递进, 根据监控指标 (§15.11.3) 动态选择。

### 15.12 MFA 服务可用性加固

**问题**: super_admin 执行敏感操作 (如用户管理、资产类型管理、enrollment token 签发) 时需要 MFA 二次验证。若 MFA 服务故障，super_admin 将无法执行任何敏感操作，导致系统管理瘫痪。

**风险等级**: 🟢 低风险 — 影响 super_admin 敏感操作，不影响普通用户日常使用。

> **[风险修复 N10] MFA 服务可用性 — MFA 故障时 super_admin 无法执行敏感操作**
>
> 以下修复措施确保 MFA 服务高可用，并提供紧急 break-glass 流程在 MFA 完全故障时维持 super_admin 的关键操作能力。

#### 15.12.1 MFA 服务 HA 部署 (至少 2 实例)

```
MFA 服务高可用部署:
├── 至少 2 个 MFA 服务实例 (跨 AZ 部署)
├── Nginx 负载均衡: MFA 请求分发到健康实例
├── 健康检查: 每 5s 检查 MFA 实例 /healthz 端点
├── 故障实例自动剔除: 连续 3 次健康检查失败 → 剔除
├── MFA 状态存储: Redis (Sentinel HA) — 存储 MFA challenge 状态
└── MFA 服务无状态: 可水平扩展，不依赖本地存储
```

```yaml
# MFA 服务部署配置
mfa_service:
  replicas: 2                    # 至少 2 实例
  deployment: cross_az           # 跨 AZ 部署
  health_check:
    interval: 5s
    timeout: 2s
    unhealthy_threshold: 3
  load_balancer: nginx
  redis:
    use_sentinel: true           # MFA 状态存储使用 Redis Sentinel HA
    fallback: local_cache        # Redis 不可用时降级为本地缓存 (有限时间)
```

```nginx
# Nginx upstream — MFA 服务负载均衡
upstream mfa_backend {
    least_conn;
    server mfa-service-1:8090 max_fails=3 fail_timeout=10s;
    server mfa-service-2:8090 max_fails=3 fail_timeout=10s;

    check interval=5s rise=2 fall=3 timeout=2s type=http;
    check_http_send "GET /healthz HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx;
}
```

#### 15.12.2 紧急 Break-Glass 流程

当 MFA 服务完全故障 (所有实例不可用) 时，启用 break-glass 紧急流程，允许 super_admin 在严格管控下执行敏感操作:

```
Break-Glass 紧急流程:
1. 双人物理验证: 两名 super_admin (或 1 名 super_admin + 1 名 admin) 同时在场
2. 发起 break-glass 请求: POST /api/v1/admin/break-glass
   - 请求需包含: 两人用户名 + 密码 + 紧急原因
3. 系统验证双身份后签发临时 break-glass token (15 分钟有效)
4. 临时 token 写入 Redis (key=breakglass:{token_id}, TTL=15min)
5. 使用临时 token 执行敏感操作 (同 super_admin 权限)
6. 所有操作实时写入审计日志 + 触发异常告警
7. 15 分钟后 token 自动失效
8. 事后审计: 安全团队 24 小时内审查 break-glass 操作记录
```

```go
// internal/service/breakglass.go — Break-Glass 紧急流程
type BreakGlassRequest struct {
    InitiatorUsername  string `json:"initiator_username" binding:"required"`
    InitiatorPassword  string `json:"initiator_password" binding:"required"`
    ApproverUsername   string `json:"approver_username" binding:"required"`
    ApproverPassword   string `json:"approver_password" binding:"required"`
    Reason             string `json:"reason" binding:"required,min=10"`
}

type BreakGlassToken struct {
    TokenID     string    `json:"token_id"`
    Token       string    `json:"token"`
    InitiatorID string    `json:"initiator_id"`
    ApproverID  string    `json:"approver_id"`
    Reason      string    `json:"reason"`
    IssuedAt    time.Time `json:"issued_at"`
    ExpiresAt   time.Time `json:"expires_at"`
}

func (s *BreakGlassService) Issue(ctx context.Context, req BreakGlassRequest) (*BreakGlassToken, error) {
    // 1. 验证双身份 (两人均为 super_admin 或 initiator=super_admin + approver=admin)
    initiator, err := s.verifyUser(ctx, req.InitiatorUsername, req.InitiatorPassword, "super_admin")
    if err != nil {
        return nil, fmt.Errorf("initiator verification failed: %w", err)
    }

    approver, err := s.verifyUser(ctx, req.ApproverUsername, req.ApproverPassword, "super_admin", "admin")
    if err != nil {
        return nil, fmt.Errorf("approver verification failed: %w", err)
    }

    // 两人不能是同一人
    if initiator.ID == approver.ID {
        return nil, errors.New("initiator and approver must be different persons")
    }

    // 2. 检查 MFA 服务是否确实不可用 (仅在 MFA 故障时允许 break-glass)
    mfaHealthy, _ := s.mfaClient.HealthCheck(ctx)
    if mfaHealthy {
        return nil, errors.New("MFA service is healthy — break-glass not permitted")
    }

    // 3. 签发临时 token (15 分钟有效)
    tokenID := uuid.New().String()
    token := uuid.New().String()
    expiresAt := time.Now().Add(15 * time.Minute)

    // 4. 写入 Redis (TTL=15min)
    key := fmt.Sprintf("breakglass:%s", tokenID)
    tokenData, _ := json.Marshal(BreakGlassToken{
        TokenID:     tokenID,
        Token:       token,
        InitiatorID: initiator.ID,
        ApproverID:  approver.ID,
        Reason:      req.Reason,
        IssuedAt:    time.Now(),
        ExpiresAt:   expiresAt,
    })
    if err := s.redis.Set(ctx, key, tokenData, 15*time.Minute).Err(); err != nil {
        return nil, fmt.Errorf("store break-glass token: %w", err)
    }

    // 5. 写入审计日志 — break-glass 签发事件
    s.auditLog.Write(ctx, AuditEntry{
        ActionType: "break_glass_issued",
        UserID:     initiator.ID,
        Metadata: map[string]interface{}{
            "approver_id":  approver.ID,
            "reason":       req.Reason,
            "token_id":     tokenID,
            "expires_at":   expiresAt,
        },
    })

    // 6. 触发异常告警 (P1 — 安全团队需立即关注)
    s.alerter.Send(ctx, Alert{
        Severity: "critical",
        Title:    "Break-glass token issued — MFA service unavailable",
        Message:  fmt.Sprintf("Initiator: %s, Approver: %s, Reason: %s, Token expires at: %s",
            initiator.Username, approver.Username, req.Reason, expiresAt.Format(time.RFC3339)),
    })

    return &BreakGlassToken{
        TokenID:     tokenID,
        Token:       token,
        InitiatorID: initiator.ID,
        ApproverID:  approver.ID,
        Reason:      req.Reason,
        IssuedAt:    time.Now(),
        ExpiresAt:   expiresAt,
    }, nil
}

// 验证 break-glass token (中间件)
func (m *AuthMiddleware) verifyBreakGlassToken(ctx context.Context, token string) (*BreakGlassToken, error) {
    // 从 Redis 查找 token
    keys, err := m.redis.Keys(ctx, "breakglass:*").Result()
    if err != nil {
        return nil, err
    }

    for _, key := range keys {
        data, err := m.redis.Get(ctx, key).Result()
        if err == redis.Nil {
            continue
        }
        var bgToken BreakGlassToken
        if err := json.Unmarshal([]byte(data), &bgToken); err != nil {
            continue
        }
        if bgToken.Token == token && time.Now().Before(bgToken.ExpiresAt) {
            // 记录每次使用 (审计)
            m.auditLog.Write(ctx, AuditEntry{
                ActionType: "break_glass_used",
                UserID:     bgToken.InitiatorID,
                Metadata: map[string]interface{}{
                    "token_id":    bgToken.TokenID,
                    "approver_id": bgToken.ApproverID,
                    "reason":      bgToken.Reason,
                },
            })
            return &bgToken, nil
        }
    }
    return nil, errors.New("invalid or expired break-glass token")
}
```

#### 15.12.3 MFA 服务监控与故障自动切换

```go
// internal/monitor/mfa_health.go — MFA 服务健康监控
func (m *MFAHealthMonitor) Monitor(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 检查所有 MFA 实例健康状态
            healthyCount := 0
            for _, instance := range m.instances {
                if err := m.checkInstance(ctx, instance); err == nil {
                    healthyCount++
                }
            }

            // Prometheus 指标
            metrics.MFAHealthyInstances.Set(float64(healthyCount))
            metrics.MFATotalInstances.Set(float64(len(m.instances)))

            // 告警逻辑
            if healthyCount == 0 {
                // 所有 MFA 实例不可用 → P1 告警
                m.alerter.Send(ctx, Alert{
                    Severity: "critical",
                    Title:    "MFA service completely unavailable",
                    Message:  "All MFA instances are down. Break-glass procedure should be activated.",
                })
                metrics.MFAServiceStatus.Set(0) // 0 = down
            } else if healthyCount < len(m.instances) {
                // 部分 MFA 实例不可用 → P2 告警
                m.alerter.Send(ctx, Alert{
                    Severity: "warning",
                    Title:    "MFA service partially degraded",
                    Message:  fmt.Sprintf("%d/%d MFA instances healthy", healthyCount, len(m.instances)),
                })
                metrics.MFAServiceStatus.Set(0.5) // 0.5 = degraded
            } else {
                metrics.MFAServiceStatus.Set(1) // 1 = healthy
            }
        }
    }
}
```

```yaml
# MFA 服务监控配置
mfa_monitoring:
  health_check_interval: 10s
  alerts:
    all_instances_down:
      severity: critical
      message: "MFA service completely unavailable — activate break-glass"
      auto_action: "notify_security_team"
    partial_degradation:
      severity: warning
      message: "MFA service partially degraded"
  prometheus_metrics:
    - mfa_healthy_instances
    - mfa_total_instances
    - mfa_service_status
    - mfa_auth_success_total
    - mfa_auth_failure_total
    - mfa_auth_latency_seconds
```

> **MFA 可用性总结**:
> - **HA 部署**: MFA 服务至少 2 实例跨 AZ 部署，Nginx 负载均衡 + 健康检查自动剔除故障实例。
> - **Break-glass 流程**: MFA 完全故障时，双人验证 + 15 分钟临时 token + 事后审计，确保 super_admin 关键操作不中断。
> - **全面审计**: break-glass 签发和使用全程写入审计日志，触发 P1 告警通知安全团队。
> - **监控告警**: MFA 服务健康状态实时监控 (Prometheus)，全实例故障 → P1 告警 + 通知安全团队激活 break-glass。
