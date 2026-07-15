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
| `github.com/golang-jwt/jwt/v5` | JWT 鉴权 |
| `github.com/golang-migrate/migrate/v4` | 数据库迁移 |
| `github.com/shirou/gopsutil/v4` | Agent 跨平台系统信息采集 |
| `modernc.org/sqlite` | Agent 离线队列 (纯 Go 无 CGO) |
| `github.com/rs/zerolog` | 结构化日志 |
| `golang.org/x/crypto` | bcrypt 密码哈希, ed25519 签名 |

---

## 3. 系统拓扑

```
                       ┌─────────────┐
                       │   Grafana   │
                       │  (port 3000)│
                       └──────┬──────┘
                              │ PostgreSQL read-only (PgBouncer port 6432)
                       ┌──────▼──────┐
                       │  PgBouncer  │
                       │  (pool=25)  │
                       └──────┬──────┘
                              │
                       ┌──────▼──────┐
                       │ PostgreSQL  │◄──── write ─────────────────────┐
                       │    :5432    │                                │
                       └──────┬──────┘                                │
                              │                                       │
                       ┌──────▼──────┐                        ┌───────┴──────┐
                       │    Redis    │                        │  API Server  │
                       │    :6379    │                        │  (Go + Gin)  │
                       │ cache / MQ  │                        │   :8080      │
                       └─────────────┘                        └──────┬───────┘
                                                                    │
                                                            ┌───────┴──────┐
                                                            │    Nginx     │
                                                            │  :443/:80   │
                                                            │ TLS + proxy │
                                                            └──────┬───────┘
                                                                   │
                    ┌──────────────────────────────────────────────┼──────────────────────────────┐
                    │                                              │                              │
             ┌──────▼──────┐                               ┌──────▼──────┐               ┌──────▼──────┐
             │  React Web  │                               │ Collection  │               │ Collection  │
             │    UI       │                               │ Agent (Go)  │               │ Agent (Go)  │
             │  (Vite dev  │                               │ Linux       │               │ Windows     │
             │   :5173)    │                               │ (binary ~10MB)              │ (binary ~12MB)
             └─────────────┘                               └─────────────┘               └─────────────┘
```

### 3.1 数据流

1. **用户操作**: Browser → Nginx (TLS) → API Server → Service → Repository → PostgreSQL
2. **Agent 上报**: Agent (mTLS) → Nginx → API Server → Ingest Buffer → Processor → Engine → PostgreSQL
3. **Grafana 查询**: Grafana → PgBouncer → PostgreSQL (read-only user, SELECT only)
4. **缓存**: Service 层查 Redis → 命中返回 / 未命中查 DB 并回填
5. **事件**: Service 发布事件 → Event Bus → Webhook Dispatcher (异步外发)

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
    metadata   JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_asset_time ON assets.audit_log (asset_id, created_at DESC);
```

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

-- Grafana 面板优化
CREATE INDEX idx_assets_lifecycle_org ON assets.assets (org_id, lifecycle_state);
CREATE INDEX idx_agents_status_heartbeat ON assets.collection_agents (status, last_heartbeat);
CREATE INDEX idx_audit_recent ON assets.audit_log (created_at DESC);
CREATE INDEX idx_assignments_active_user ON assets.assignments (assigned_to) WHERE status = 'active';
CREATE INDEX idx_assets_loc_state ON assets.assets (location_id, lifecycle_state);
CREATE INDEX idx_snapshots_agent_time ON assets.asset_snapshots (agent_id, created_at DESC);
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
| GET | `/assets/:id/snapshots` | 资产 Agent 快照历史 |
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

| 参数 | 类型 | 示例 |
|---|---|---|
| `search` | string | `?search=thinkpad` (全文搜索) |
| `type_id` | UUID | `?type_id=xxx` |
| `category` | string | `?category=hardware` |
| `lifecycle_state` | string | `?lifecycle_state=utilization` |
| `status` | string | `?status=available` |
| `org_id` | UUID | `?org_id=xxx` |
| `location_id` | UUID | `?location_id=xxx` |
| `assigned_to` | UUID | `?assigned_to=xxx` |
| `cursor` | string | 分页游标 |
| `limit` | int | 默认 50, 最大 200 |
| `sort` | string | `?sort=updated_at:desc` |

### 6.6 中间件链

```
Request ID → Recovery (panic) → Structured Logging → Rate Limit (Redis) → Auth (JWT validation) → Handler
```

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

### 7.3 悲观锁实现 (`internal/lock/pessimistic.go`)

在事务中锁定目标行，阻止并发修改。**所有悲观锁操作必须在 5 秒内超时**。

```go
func (s *AssignmentService) Assign(ctx context.Context, assetID, userID, byUserID uuid.UUID) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)

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

### 7.4 Advisory 锁实现 (`internal/lock/advisory.go`)

用于跨多行的批量操作，避免锁升级阻塞普通读写。

```go
func (s *AssetService) BulkRetireByLocation(ctx context.Context, locationID uuid.UUID) error {
    lockID := hashUUIDToInt64(locationID)
    s.db.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID)
    defer s.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)
    return s.repo.BulkUpdateLifecycleByLocation(ctx, locationID, domain.StateRetirement)
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

### 8.3 增量同步协议

**首次运行 (全量):**
1. 运行所有 collector → 计算 checksum
2. 构建 `SyncPayload{full_snapshot: true, modules: [...]}`
3. POST `/api/v1/agents/sync`
4. 本地持久化 checksums

**后续运行 (增量, 默认 5 分钟):**
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
│   Buffer    │  环形缓冲区 (容量 10,000)
│  (channel)  │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  Processor  │  Worker goroutines 从 buffer 取 payload:
│  (workers)  │  - 验证 Ed25519 签名
│             │  - 检查 sequence_number 连续性
│             │  - 去重 (已处理的 sequence 跳过)
│             │  - 转换为 domain 对象
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Engine    │  按资产分组 → 批量事务写入:
│             │  - SELECT ... FOR UPDATE 锁定目标 asset
│             │  - INSERT/UPDATE asset_snapshots
│             │  - 如有属性变化 → UPDATE assets (乐观锁)
│             │  - INSERT audit_log
└─────────────┘
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
| `agent` | 仅自身 | 仅 `/agents/sync`, `/agents/heartbeat` |

### 9.3 组织范围查询

**Repository 层强制过滤 (非 Middleware 层，防止绕过):**

```sql
-- 获取用户可访问的所有组织 ID
WITH RECURSIVE org_tree AS (
    SELECT id FROM assets.organizations WHERE id = $user_org_id
    UNION ALL
    SELECT o.id FROM assets.organizations o
    JOIN org_tree ot ON o.parent_id = ot.id
)
SELECT * FROM assets.assets WHERE org_id IN (SELECT id FROM org_tree);
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

每个组件可独立部署在不同主机，通过网络互联：

| 组件 | 部署方式 | 网络要求 |
|---|---|---|
| PostgreSQL | 独立主机 / 托管服务 (RDS) | API Server 可写, PgBouncer 可读 |
| Redis | 独立主机 / 托管服务 (ElastiCache) | API Server 访问 |
| API Server | Go 二进制 + systemd / Docker | Nginx 代理 `/api/*` |
| Nginx | 反向代理 + 静态文件 | 80/443 对外 |
| PgBouncer | 独立主机 / sidecar | 仅 Grafana 访问 |
| Grafana | Docker / 独立部署 | 访问 PgBouncer:6432 |
| Agent | Go 二进制 + systemd / Windows Service | 出站 HTTPS 到 Nginx |

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
