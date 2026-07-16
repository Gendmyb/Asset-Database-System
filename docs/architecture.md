# Asset Database System — 正式架构文档

> **版本**: 2.0 | **更新**: 2026-07-16 | **状态**: 生产就绪 (Production-Ready)

---

## 文档信息

### 版本历史

| 版本 | 日期 | 变更内容 |
|---|---|---|
| 1.0 | 2026-07-05 | 初始架构设计，14 章 |
| 1.1 | 2026-07-15 | 补充加固设计 (15.1–15.8)，+446 行 |
| 1.2 | 2026-07-16 | 7 板块修复 (安全/权限/并发/审计/可靠性/性能/部署)，37 个问题，+1,872 行 |
| 1.3 | 2026-07-16 | 新增风险修复 (N1–N10)，+2,443 行 |
| 2.0 | 2026-07-16 | **正式重构**：整合散落修补、统一格式、新增术语表/评审记录/检查清单 |

### 术语表

| 缩写 | 全称 | 说明 |
|---|---|---|
| SOT | Source of Truth | 唯一数据源 |
| HA | High Availability | 高可用 |
| RTO | Recovery Time Objective | 恢复时间目标 |
| RPO | Recovery Point Objective | 恢复点目标 |
| CMDB | Configuration Management Database | 配置管理数据库 |
| DCIM | Data Center Infrastructure Management | 数据中心基础设施管理 |
| IPAM | IP Address Management | IP 地址管理 |
| mTLS | Mutual TLS | 双向 TLS 认证 |
| CRL | Certificate Revocation List | 证书吊销列表 |
| OCSP | Online Certificate Status Protocol | 在线证书状态协议 |
| MFA | Multi-Factor Authentication | 多因素认证 |
| RLS | Row-Level Security | 行级安全 |
| IDOR | Insecure Direct Object Reference | 不安全直接对象引用 |
| SSRF | Server-Side Request Forgery | 服务端请求伪造 |
| JWT | JSON Web Token | JSON Web 令牌 |
| RBAC | Role-Based Access Control | 基于角色的访问控制 |
| KMS | Key Management Service | 密钥管理服务 |
| HPA | Horizontal Pod Autoscaler | 水平 Pod 自动扩缩 |
| DLQ | Dead Letter Queue | 死信队列 |

### 审计评审记录

| 轮次 | 日期 | Agent 类型 | 数量 | 发现问题 | 结果 |
|---|---|---|---|---|---|
| 第 1 轮 | 2026-07-15 | 安全审计 + 可靠性审计 + PM 评估 | 3 | 37 问题 (12🔴 + 21🟡 + 4🟢) | 全部分类归档 |
| 第 2 轮 | 2026-07-16 | 分板块修复 (A–G) | 7 | 修复 37 问题 | 7/7 板块完成 |
| 第 3 轮 | 2026-07-16 | 审计复查 + PM 复查 | 2 | 发现 10 新增风险 | 全部记录 |
| 第 4 轮 | 2026-07-16 | 风险修复 (H–J) | 3 | 修复 10 风险 | 10/10 完成 |
| 第 5 轮 | 2026-07-16 | 文档重构 | 1 | 统一格式化 | 完成，产出 v2.0 |

---

## 目录

1. [系统概述](#1-系统概述)
2. [技术选型](#2-技术选型)
3. [系统拓扑](#3-系统拓扑)
4. [项目结构](#4-项目结构)
5. [数据模型](#5-数据模型)
6. [API 设计](#6-api-设计)
7. [安全架构](#7-安全架构)
8. [并发控制与锁策略](#8-并发控制与锁策略)
9. [Agent 采集架构](#9-agent-采集架构)
10. [事件与 Webhook](#10-事件与-webhook)
11. [缓存策略](#11-缓存策略)
12. [Grafana 集成](#12-grafana-集成)
13. [部署架构](#13-部署架构)
14. [可靠性设计](#14-可靠性设计)
15. [实施计划](#15-实施计划)
16. [附录：实施检查清单](#16-附录实施检查清单)

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
| PostgreSQL | PostgreSQL 16 | 唯一数据源 (SOT) |
| Redis | Redis 7 + Sentinel | 缓存、限流、事件总线 (Pub/Sub + Stream) |
| PgBouncer | PgBouncer 1.21+ | 数据库连接池 (Grafana 只读通道) |
| Nginx | Nginx | TLS 终结、反向代理、负载均衡 |
| Vault / KMS | HashiCorp Vault / 云 KMS | JWT 签名密钥管理 |
| Patroni + etcd | Patroni + etcd | PostgreSQL HA 自动故障转移 |

### 1.3 核心设计原则

- **API-First**: 所有功能通过 REST API 暴露，Web UI 和 Agent 是 API 的消费者
- **单一写路径**: API Server 是 PostgreSQL 的唯一写入者
- **读写分离**: Grafana 经 PgBouncer 读 Replica，不影响 API Server 写性能
- **解耦扩展**: 新增资产类型只需 INSERT 一行 `asset_types`，零代码改动
- **零信任 Agent**: Agent 纯出站 HTTPS+mTLS，不需要入站端口，双向证书认证
- **安全纵深防御**: 认证链、多租户隔离、审计不可变、Webhook 防重放多层保护

---

## 2. 技术选型

### 2.1 Go vs Java Spring Boot

| 维度 | Go (Gin) | Spring Boot (JVM) |
|---|---|---|
| 吞吐量 | **125,700 req/s** | 54,600 req/s |
| 内存空闲 | **24 MB** | 717 MB |
| 冷启动 | **~100 ms** | 3,200 ms |
| GC 延迟 | <1ms (可预测) | 10-50ms (G1GC 压力下) |
| 百万并发 | goroutine (2KB/个) | 需 NIO 深度调优 |
| 编译产物 | 单一静态二进制 | JAR + JVM 运行时 |

**选择 Go 的核心原因**: 后端和 Agent 共享同一技术栈；高性能低开销；交叉编译一键产出全平台 Agent 二进制。

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
| `golang.org/x/crypto` | bcrypt 密码哈希, Ed25519 签名 |
| `github.com/hashicorp/vault/api` | Vault KMS — JWT 密钥/Webhook secret/Token 密钥管理 |

### 2.3 新增基础设施依赖

| 组件 | 版本 | 用途 | 替代方案 |
|---|---|---|---|
| Patroni + etcd | 3.x | PostgreSQL HA 集群 (RTO<30s) | RDS Multi-AZ |
| Redis Sentinel | 7.x | Redis HA 自动故障转移 | ElastiCache |
| HashiCorp Vault | 1.15+ | 密钥管理 (JWT/Webhook/Token) | AWS/GCP KMS |
| Kubernetes | 1.28+ | 生产容器编排 (可选) | Docker Swarm / systemd |
| S3 对象存储 | — | 冷数据归档 | MinIO 自建 |
| ltree (PG 扩展) | — | 组织树物化路径 | — |

---

## 3. 系统拓扑

### 3.1 高可用架构

```
                       ┌─────────────┐
                       │   Grafana   │
                       │  (port 3000)│
                       └──────┬──────┘
                              │ read-only (PgBouncer :6432 → Replica)
                       ┌──────▼──────────────┐
                       │     PgBouncer ×2     │
                       │  (pool=25, HA pair)  │
                       │  Primary + Replica   │
                       └──────┬──────────────┘
                              │
              ┌───────────────┼─────────────────────┐
              │               │                     │
     ┌────────▼────────┐ ┌───▼──────────┐ ┌─────────▼─────────┐
     │ PostgreSQL      │ │ PostgreSQL   │ │ PostgreSQL        │
     │ Primary (R/W)   │ │ Replica (R/O)│ │ Replica (R/O)     │
     │ :5432           │ │ :5432        │ │ :5432             │
     └────────┬────────┘ └──────────────┘ └───────────────────┘
              │ Streaming Replication (同步/异步)
              │ Patroni + etcd (3节点) — 自动故障转移 RTO<30s
              │
     ┌────────┼──────────┐
     │        │          │
┌────▼───┐ ┌──▼───┐ ┌───▼──┐
│API Svr │ │API   │ │API   │  (Go+Gin, 无状态, 水平扩展)
│:8080   │ │:8080 │ │:8080 │
└────┬───┘ └──┬───┘ └──┬───┘
     │        │        │
     │   ┌────┴────────┴───┐           ┌──────────────────────┐
     │   │   Nginx :443   │           │  Redis Sentinel ×3   │
     └──►│  TLS + upstream │◄──────────│  (自动故障转移)       │
         │  health check + │           │  cache / MQ /        │
         │  active eject   │           │  Pub-Sub / Stream    │
         └───────┬─────────┘           └──────────────────────┘
                 │
     ┌───────────┴────────────┐
     │                        │
┌────▼──────┐          ┌──────▼──────┐
│ React Web │          │Collection   │
│    UI     │          │Agent (mTLS) │
│ :5173 dev │          │Linux/Win/Mac│
└───────────┘          └─────────────┘
```

### 3.2 数据流

1. **用户操作**: Browser → Nginx (TLS) → API Server (upstream 池, 健康检查剔除故障实例) → Service → Repository → PostgreSQL Primary
2. **Agent 上报**: Agent (mTLS) → Nginx → API Server → Redis Stream (持久化) → Processor → Engine → PostgreSQL Primary
3. **Grafana 查询**: Grafana → PgBouncer → PostgreSQL Replica (read-only user, SELECT only)
4. **缓存**: Service 查 Redis Sentinel → 命中返回 / 未命中查 DB 并回填；Redis 故障时熔断降级
5. **事件**: Service 发布事件 → Redis Pub/Sub → 所有 API Server 实例订阅 → Webhook 异步外发

---

## 4. 项目结构

```
asset-database-system/
├── docs/
│   ├── architecture.md          # 本文档
│   └── progress.md              # 项目进度报告
│
├── assetserver/                  # Go 后端 (monorepo)
│   ├── cmd/
│   │   ├── api-server/main.go
│   │   ├── collection-agent/main.go
│   │   └── migrate/main.go
│   │
│   ├── internal/
│   │   ├── api/
│   │   │   ├── middleware/       # auth, ratelimit, logging, recover, requestid
│   │   │   ├── handler/          # Gin handler (auth, asset, assignment, agent, dashboard, ...)
│   │   │   ├── router.go
│   │   │   └── server.go
│   │   ├── domain/               # 领域模型 (asset, agent, assignment, audit, user, webhook, ...)
│   │   ├── service/              # 业务逻辑层
│   │   │   └── ingest/           # 摄入管道 (buffer → processor → engine)
│   │   ├── repository/           # 数据访问层 (pgx)
│   │   ├── cache/                # Redis 缓存层
│   │   ├── lock/                 # 锁策略 (optimistic, pessimistic, advisory)
│   │   ├── job/                  # 后台任务 (worker, scheduler)
│   │   ├── event/                # 事件总线 (Redis Pub/Sub)
│   │   ├── webhook/              # Webhook 外发引擎 (HMAC 签名 + 重试)
│   │   └── config/               # 配置加载
│   │
│   ├── pkg/                      # 共享库 (Agent 和 Server 共用)
│   │   ├── agentproto/           # DeltaPayload, SnapshotPayload, crypto
│   │   ├── apierror/             # 统一错误类型
│   │   ├── pagination/           # 游标分页
│   │   └── validator/            # 输入校验
│   │
│   ├── agent/                    # Collection Agent 应用代码
│   │   ├── collector/            # Collector 接口 + 平台实现 (linux, windows, darwin)
│   │   ├── comm/                 # HTTPS 客户端 (mTLS) + sync
│   │   ├── store/                # 离线队列 (SQLite)
│   │   ├── updater/              # 自更新 + Ed25519 签名验证
│   │   └── identity/             # 硬件指纹生成
│   │
│   ├── migrations/               # golang-migrate SQL 文件
│   ├── grafana/                  # 仪表盘 JSON + 数据源配置
│   ├── deploy/                   # Docker Compose, Dockerfile, Nginx, PgBouncer 配置
│   ├── k8s/                      # Kubernetes Helm Chart (生产部署)
│   ├── Makefile
│   ├── go.mod
│   └── go.sum
│
└── web/                          # React 前端
    ├── src/
    │   ├── api/                  # API 客户端 (Axios, JWT 注入, 401/403 拦截)
    │   ├── components/           # shadcn/ui, layout, assets, assignments, agents, dashboard
    │   ├── pages/                # 路由页面 (Login, Dashboard, Assets, Agents, Admin, AuditLog)
    │   ├── hooks/                # useAuth, usePagination, useDebounce
    │   ├── store/                # Zustand (authStore, assetStore)
    │   ├── types/                # TypeScript 类型定义
    │   └── lib/                  # utils, constants
    ├── vite.config.ts
    ├── tailwind.config.ts
    ├── tsconfig.json
    └── package.json
```

---

## 5. 数据模型

所有表位于 `assets` schema 下。

### 5.1 核心实体关系

```
organizations (树: parent_id, ltree path)
       │
       ├── users (5 种角色)
       │     │
       │     └── assignments ────┐
       │                         │
       ├── asset_types ── assets ─────────────┘
       │     (JSON schema)   │
       │                     ├── audit_log (不可变 + hash chain)
       │                     ├── asset_snapshots (分区分区)
       │                     ├── asset_relationships (自引用)
       │                     ├── webhooks
       │                     └── collection_agents (含 org_id)
       │
       └── locations (树: parent_id)
```

### 5.2 核心表 DDL

#### organizations

```sql
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE assets.organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    parent_id   UUID REFERENCES assets.organizations(id),
    depth       INTEGER NOT NULL DEFAULT 0 CHECK (depth <= 20),
    path        LTREE NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 物化路径索引 (替代递归 CTE)
CREATE INDEX idx_orgs_path_gist ON assets.organizations USING GIST (path);
CREATE INDEX idx_orgs_parent ON assets.organizations (parent_id);
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
    mfa_enabled   BOOLEAN NOT NULL DEFAULT false,
    mfa_secret    VARCHAR(64),
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
    schema   JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### assets (核心表，含软删除和乐观锁)

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
    properties      JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    version         INTEGER NOT NULL DEFAULT 1,
    deleted_at      TIMESTAMPTZ,               -- 软删除
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID REFERENCES assets.users(id),
    updated_by      UUID REFERENCES assets.users(id)
);

CREATE INDEX idx_assets_deleted ON assets.assets (deleted_at) WHERE deleted_at IS NOT NULL;
```

#### assignments (领用表)

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

CREATE UNIQUE INDEX idx_active_assignment
    ON assets.assignments (asset_id) WHERE status = 'active';
```

#### audit_log (不可变 + 链式哈希)

```sql
CREATE TABLE assets.audit_log (
    id         BIGSERIAL PRIMARY KEY,
    asset_id   UUID,
    user_id    UUID REFERENCES assets.users(id),
    agent_id   UUID REFERENCES assets.collection_agents(id),
    action     VARCHAR(50) NOT NULL,
    field      VARCHAR(255),
    old_value  TEXT,
    new_value  TEXT,
    metadata   JSONB DEFAULT '{}' CHECK (octet_length(metadata::text) <= 4096),
    prev_hash  CHAR(64),
    hash       CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 索引
CREATE INDEX idx_audit_asset_time ON assets.audit_log (asset_id, created_at DESC);
CREATE INDEX idx_audit_user_time ON assets.audit_log (user_id, created_at DESC);
CREATE INDEX idx_audit_action_time ON assets.audit_log (action, created_at DESC);
CREATE INDEX idx_audit_agent_time ON assets.audit_log (agent_id, created_at DESC);
CREATE INDEX idx_audit_recent ON assets.audit_log (created_at DESC);
```

**三层不可变性保护**:

```sql
-- 1. 数据库角色分离
CREATE ROLE app_writer WITH LOGIN;
GRANT INSERT ON assets.audit_log TO app_writer;
REVOKE UPDATE, DELETE ON assets.audit_log FROM app_writer;

CREATE ROLE audit_reader WITH LOGIN;
GRANT SELECT ON assets.audit_log TO audit_reader;

-- 2. 行级安全 (RLS)
ALTER TABLE assets.audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_insert_only ON assets.audit_log
    FOR INSERT TO app_writer WITH CHECK (true);
CREATE POLICY audit_log_no_update ON assets.audit_log
    FOR UPDATE TO app_writer USING (false) WITH CHECK (false);
CREATE POLICY audit_log_no_delete ON assets.audit_log
    FOR DELETE TO app_writer USING (false);
CREATE POLICY audit_log_select_only ON assets.audit_log
    FOR SELECT TO audit_reader USING (true);

-- 3. 触发器最后一层防线
CREATE OR REPLACE FUNCTION assets.audit_log_immutable_guard()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % not permitted on row %',
        TG_OP, OLD.id;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_immutable
    BEFORE UPDATE OR DELETE ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_immutable_guard();
```

#### collection_agents (含 org_id 绑定)

```sql
CREATE TABLE assets.collection_agents (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_key       VARCHAR(64) UNIQUE NOT NULL,
    org_id          UUID NOT NULL REFERENCES assets.organizations(id),
    hostname        VARCHAR(255) NOT NULL,
    ip_address      INET,
    os_type         VARCHAR(50) NOT NULL,
    os_version      VARCHAR(100),
    agent_version   VARCHAR(20) NOT NULL,
    last_heartbeat  TIMESTAMPTZ,
    status          VARCHAR(20) NOT NULL DEFAULT 'registered'
                    CHECK (status IN ('registered','online','offline','disabled')),
    public_key      TEXT NOT NULL,
    cert_serial     VARCHAR(64),
    cert_revoked    BOOLEAN NOT NULL DEFAULT false,
    cert_expires_at TIMESTAMPTZ,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### asset_snapshots (按月分区)

```sql
CREATE TABLE assets.asset_snapshots (
    id          BIGSERIAL,
    asset_id    UUID NOT NULL,
    agent_id    UUID NOT NULL REFERENCES assets.collection_agents(id),
    snapshot    JSONB NOT NULL,
    checksum    VARCHAR(64) NOT NULL,
    is_delta    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE TABLE assets.asset_snapshots_2026_07
    PARTITION OF assets.asset_snapshots
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
```

> **注意**: `asset_id` 外键移除，改用应用层校验 + 定时孤儿检测 + 物理删除时显式清理。

#### enrollment_tokens

```sql
CREATE TABLE assets.enrollment_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  VARCHAR(64) UNIQUE NOT NULL,      -- SHA-256 哈希
    created_by  UUID NOT NULL REFERENCES assets.users(id),
    org_id      UUID NOT NULL REFERENCES assets.organizations(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    used_by_agent UUID REFERENCES assets.collection_agents(id),
    max_uses    INTEGER NOT NULL DEFAULT 1,
    use_count   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

#### approval_requests (双人审批)

```sql
CREATE TABLE assets.approval_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action      VARCHAR(50) NOT NULL,              -- create_user, delete_org, issue_token
    target_id   UUID,
    requestor   UUID NOT NULL REFERENCES assets.users(id),
    approver    UUID REFERENCES assets.users(id),
    status      VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    meta        JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
```

#### archive_manifest (归档管道幂等)

```sql
CREATE TABLE assets.archive_manifest (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    archive_id      UUID UNIQUE NOT NULL,
    table_name      VARCHAR(100) NOT NULL,
    partition_name  VARCHAR(100) NOT NULL,
    status          VARCHAR(20) DEFAULT 'pending'
                    CHECK (status IN ('pending','exporting','uploading','verifying','detaching','completed','failed')),
    row_count       BIGINT,
    s3_key          VARCHAR(1024),
    s3_checksum     VARCHAR(64),
    error_message   TEXT,
    retry_count     INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(table_name, partition_name)
);
```

#### audit_meta (归档操作审计)

```sql
CREATE TABLE assets.audit_meta (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id        UUID NOT NULL,
    table_name      VARCHAR(100) NOT NULL,
    partition_name  VARCHAR(100),
    row_count       BIGINT NOT NULL,
    operated_by     VARCHAR(100) NOT NULL,       -- archive_runner
    trigger_status  BOOLEAN NOT NULL,            -- 触发器是否被 DISABLE
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 5.3 资产类型扩展机制

新增资产类型只需一条 SQL，服务端零代码改动：

```sql
INSERT INTO assets.asset_types (name, category, schema) VALUES (
    'software_license',
    'license',
    '{"type":"object","properties":{"license_key":{"type":"string"},"vendor":{"type":"string"},"seats":{"type":"integer","minimum":1},"expiration_date":{"type":"string","format":"date"},"license_type":{"enum":["perpetual","subscription","trial"]}},"required":["license_key","vendor","seats"]}'::jsonb
);
```

### 5.4 资产生命周期状态机

```
procurement → deployment → utilization → maintenance → retirement (终态)
                  ↑              ↑  ↑            ↑
                  └──────────────┘  └────────────┘
```

合法转换矩阵：

| 当前状态 | 可转换到 |
|---|---|
| procurement | deployment, retirement |
| deployment | utilization, maintenance, retirement |
| utilization | maintenance, retirement |
| maintenance | utilization, retirement |
| retirement | — (终态) |

### 5.5 关键索引

```sql
-- 资产查询复合索引 (均以 org_id 为前导列)
CREATE INDEX idx_assets_org_status ON assets.assets (org_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_type ON assets.assets (org_id, type_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_updated ON assets.assets (org_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_lifecycle ON assets.assets (org_id, lifecycle_state) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_location ON assets.assets (org_id, location_id) WHERE deleted_at IS NULL;

-- JSONB GIN 索引
CREATE INDEX idx_assets_properties ON assets.assets USING GIN (properties jsonb_path_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_metadata ON assets.assets USING GIN (metadata jsonb_path_ops) WHERE deleted_at IS NULL;

-- 全文搜索
CREATE INDEX idx_assets_search ON assets.assets USING GIN (
    to_tsvector('english', name || ' ' || COALESCE(manufacturer,'') || ' ' || COALESCE(model,'') || ' ' || COALESCE(serial_number,'') || ' ' || asset_tag)
);

-- 领用表索引
CREATE INDEX idx_assignments_active_user ON assets.assignments (assigned_to) WHERE status = 'active';
CREATE INDEX idx_assignments_asset_time ON assets.assignments (asset_id, assigned_at DESC);
CREATE INDEX idx_assignments_assigned_by ON assets.assignments (assigned_by) WHERE status = 'active';

-- Agent 表索引
CREATE INDEX idx_agents_status_heartbeat ON assets.collection_agents (status, last_heartbeat);
CREATE INDEX idx_agents_org ON assets.collection_agents (org_id);

-- 快照分区索引
CREATE INDEX idx_snapshots_agent_time ON assets.asset_snapshots (agent_id, created_at DESC);
CREATE INDEX idx_snapshots_asset_time ON assets.asset_snapshots (asset_id, created_at DESC);
```

---

## 6. API 设计

### 6.1 通用约定

- 基础路径: `/api/v1/`
- 鉴权: `Authorization: Bearer <JWT>` (用户) / mTLS (Agent)
- 乐观锁: 客户端发送 `If-Match: "<version>"` header
- 分页: 游标分页，参数 `cursor` + `limit` (默认 50, 最大 200)
- JWT 签名: **EdDSA (Ed25519)** 非对称签名，私钥由 Vault/KMS 管理
- JWT Claims 全量校验: `iss`, `aud`, `exp`, `iat`, `jti` 全部强制验证
- 算法降级防护: `jwt.Parse` 显式设置 `ValidMethods: []string{"EdDSA"}`

### 6.2 统一响应格式

**成功**:
```json
{
    "data": { ... },
    "pagination": {
        "next_cursor": "eyJsYX...zIn0=",
        "has_more": true,
        "total": 1042
    },
    "request_id": "req_a1b2c3d4"
}
```

**错误**:
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
| 204 | 无内容 |
| 400 | 参数校验失败 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 409 | 冲突 (版本过期、重复领用、锁竞争) |
| 429 | 限流 |
| 500 | 服务器内部错误 |
| 503 | 服务不可用 (Redis/Vault 故障降级) |

### 6.4 API 路由表

#### 认证

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/auth/login` | 用户登录，返回 JWT access + refresh token |
| POST | `/auth/refresh` | Refresh token 轮换 (旧 token 立即失效) |
| POST | `/auth/register-agent` | Agent 注册 (需 enrollment token) |
| POST | `/auth/logout` | 登出，Redis 标记 refresh token 失效 |

#### 资产

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/assets` | 资产列表 (org_id 由服务端根据 JWT 自动注入) |
| POST | `/assets` | 创建资产 |
| GET | `/assets/:id` | 资产详情 (含 version 号) |
| PUT | `/assets/:id` | 更新 (需 If-Match header) |
| DELETE | `/assets/:id` | 软删除 (设置 deleted_at) |
| GET | `/assets/:id/history` | 审计日志 (?include_archive=true) |
| GET | `/assets/:id/snapshots?from=&to=` | Agent 快照 (强制时间范围, 最大 90 天) |
| GET | `/assets/:id/snapshots/latest` | 最新快照 |
| GET | `/assets/:id/relationships` | 资产关联关系 |

#### 生命周期

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/assets/:id/transition` | 状态转换 (悲观锁, 5s 超时) |

#### 领用

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/assets/:id/assign` | 分配资产 (悲观锁) |
| POST | `/assets/:id/release` | 归还 |
| POST | `/assets/:id/transfer` | 转移 (按 UUID 字典序锁定, 防死锁) |
| GET | `/assets/:id/assignment` | 当前领用 |
| GET | `/assets/:id/assignment/history` | 领用历史 |
| GET | `/users/:id/assignments` | 用户的领用列表 |

#### Agent

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/agents/sync` | Delta/全量快照 (mTLS, Ed25519 签名预检) |
| POST | `/agents/heartbeat` | 心跳 (mTLS) |
| GET | `/agents` | Agent 列表 |
| GET | `/agents/:id` | Agent 详情 |
| PUT | `/agents/:id` | 更新元数据 |
| DELETE | `/agents/:id` | 注销 (吊销证书 + cert_revoked=true) |
| POST | `/agents/:id/update-check` | 检查新版本 |

#### 管理 (仅 super_admin, 需 MFA + 双人审批)

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/admin/users` | 用户列表 |
| POST | `/admin/users` | 创建用户 (双人审批) |
| PUT | `/admin/users/:id` | 更新用户 |
| DELETE | `/admin/users/:id` | 禁用用户 |
| GET | `/admin/asset-types` | 资产类型列表 |
| POST | `/admin/asset-types` | 创建资产类型 |
| POST | `/admin/enrollment-tokens` | 生成 enrollment token (双人审批) |
| GET | `/admin/enrollment-tokens` | Token 列表 |
| DELETE | `/admin/enrollment-tokens/:id` | 撤销 token |
| GET | `/admin/assets?org_id=xxx` | 跨组织资产查询 (super_admin 专用) |
| GET | `/admin/approvals` | 审批队列 |

### 6.5 查询参数 (资产列表)

| 参数 | 类型 | 示例 | 说明 |
|---|---|---|---|
| `search` | string | `?search=thinkpad` | 全文搜索 (plainto_tsquery 参数化) |
| `type_id` | UUID | `?type_id=xxx` | 资产类型 |
| `category` | string | `?category=hardware` | 资产类别 |
| `lifecycle_state` | string | `?lifecycle_state=utilization` | 生命周期 |
| `status` | string | `?status=available` | 状态 |
| `location_id` | UUID | `?location_id=xxx` | 位置 |
| `assigned_to` | UUID | `?assigned_to=xxx` | 领用人 |
| `cursor` | string | `?cursor=xxx` | 分页游标 |
| `limit` | int | `?limit=50` | 默认 50, 最大 200 |
| `sort` | string | `?sort=updated_at:desc` | **白名单校验**允许的列名 |

> **多租户安全**: `org_id` 不由用户传入，服务端根据 JWT 中 `user.org_id` 自动注入。Repository 层对所有查询强制加 `org_id IN (用户可访问组织树)` 过滤。

### 6.6 中间件链

```
Request ID → Recovery (panic) → Structured Logging → Rate Limit (Redis + 本地令牌桶兜底)
  → Auth (JWT EdDSA 校验 / mTLS CN 校验 + cert_revoked 检查)
  → Org Scope (自动注入 org_id)
  → Handler
```

---

## 7. 安全架构

### 7.1 认证链

#### JWT 签名与密钥管理

- **签名算法**: EdDSA (Ed25519)，禁止 HS256/RS256
- **密钥管理**: 私钥由 Vault/KMS 托管，API Server 启动时读取并缓存在内存+磁盘
- **密钥轮换**: 支持 `kid` header，定期轮换
- **Vault 不可用降级**: 已运行实例用缓存的公钥继续验证 JWT，无法签发新 token；新实例启动时指数退避重试 (2s→30s, 最多 10 次)，超时拒绝启动

```go
// JWT 签发
func IssueAccessToken(ctx context.Context, userID string, role string) (string, error) {
    privKey := keyManager.GetPrivateKey() // 从 Vault/缓存 获取
    now := time.Now()
    claims := jwt.RegisteredClaims{
        Issuer:    "asset-db-api",
        Subject:   userID,
        Audience:  jwt.ClaimStrings{"asset-db"},
        ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
        IssuedAt:  jwt.NewNumericDate(now),
        ID:        uuid.New().String(),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
    token.Header["kid"] = keyManager.GetCurrentKeyID()
    return token.SignedString(privKey)
}

// JWT 验证
func VerifyJWT(tokenString string) (*jwt.RegisteredClaims, error) {
    pubKey := keyManager.GetPublicKey()
    token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{},
        func(t *jwt.Token) (interface{}, error) { return pubKey, nil },
        jwt.WithValidMethods([]string{"EdDSA"}),          // 拒绝 none/HS256/RS256
        jwt.WithIssuer("asset-db-api"),
        jwt.WithAudience("asset-db"),
        jwt.WithExpirationRequired(),
        jwt.WithIssuedAt(),
    )
    if err != nil { return nil, err }
    claims := token.Claims.(*jwt.RegisteredClaims)
    if isRevoked(ctx, claims.ID) { return nil, ErrTokenRevoked }
    return claims, nil
}
```

#### mTLS 证书管理

- **证书有效期**: ≤90 天
- **自动续期**: Agent 到期前 7 天自动请求续期
- **吊销**: Nginx CRL 1h 刷新 + OCSP Stapling (秒级) + DB `cert_revoked` 实时校验 (双保险)
- **CN 绑定**: 证书 CN = agent_id，API Server 校验 CN 与 JWT 中 agent_id 一致

```nginx
# Nginx mTLS + CRL + OCSP
ssl_verify_client on;
ssl_verify_depth 2;
ssl_client_certificate /etc/nginx/ca.crt;
ssl_crl /etc/nginx/ca.crl;
ssl_stapling on;
ssl_stapling_verify on;

proxy_set_header X-SSL-Client-Verify $ssl_client_verify;
proxy_set_header X-SSL-Client-CN $ssl_client_s_dn_cn;
```

### 7.2 多租户隔离

- **org_id 注入**: 服务端根据 JWT 的 `user.org_id` 自动注入，用户不可控
- **组织树**: ltree 物化路径替代递归 CTE，深度限制 ≤20 层
- **Repository 层强制过滤**: 所有 SQL 注入 `org_id IN (SELECT path FROM org_tree WHERE depth <= 20)`
- **跨组织查询**: `super_admin` 专用端点 `/admin/assets?org_id=xxx`

```sql
-- 组织树查询 (ltree, 无递归)
SELECT * FROM assets.organizations
WHERE path @> (SELECT path FROM assets.organizations WHERE id = $user_org_id);
```

### 7.3 权限模型 (RBAC + 双人审批 + MFA)

| 角色 | 权限范围 | 典型能力 |
|---|---|---|
| `super_admin` | 全部组织 | 用户管理、AssetType、Agent、全局配置、跨组织查询 |
| `admin` | 所属组织+子组织 | 创建用户(组织内)、管理资产、配置 Webhook |
| `manager` | 所属组织+子组织 | 资产 CRUD、领用/归还、仪表盘 |
| `viewer` | 所属组织+子组织 | 只读查看 |
| `agent` | 仅自身 (绑定 org_id) | `/agents/sync`, `/agents/heartbeat` |

**super_admin 敏感操作双人审批**:

| 操作 | 审批要求 | MFA |
|---|---|---|
| 创建用户 | 另一 super_admin 审批 | 强制 |
| 签发 enrollment token | 另一 super_admin 审批 | 强制 |
| 删除组织 | 另一 super_admin 审批 | 强制 |
| 物理删除资产 | 另一 super_admin 审批 | 强制 |
| 跨组织查询 | 无需审批 | 强制 |

**MFA**: super_admin 强制 TOTP (Google Authenticator 兼容)，MFA 服务需 HA (≥2 实例)。MFA 故障时启用 break-glass 流程：双人物理验证 + 15 分钟临时 token + 事后审计。

### 7.4 Webhook 安全

- **HMAC 签名**: `HMAC-SHA256(secret, event_id + delivered_at + raw_body)`
- **防重放**: `event_id` (UUID) + `delivered_at` (≤5 分钟窗口)
- **Secret 加密**: AES-256-GCM 加密存储，解密仅在内存中
- **SSRF 防护**: 强制 HTTPS，DNS 解析后校验目标 IP 不在私有网段

### 7.5 Agent 注册安全

- **Enrollment token**: SHA-256 哈希存储，明文仅创建时返回一次
- **原子并发**: `UPDATE ... SET use_count = use_count + 1 WHERE use_count < max_uses RETURNING`
- **org_id 绑定**: Agent 注册时从 enrollment token 继承 org_id，Engine 层校验 `asset.org_id = agent.org_id`

### 7.6 审计不可变性

三层防御 + 链式哈希完整性保证：

1. **角色分离**: app_writer INSERT-only, audit_reader SELECT-only
2. **RLS**: 阻止 UPDATE/DELETE
3. **触发器**: BEFORE UPDATE OR DELETE RAISE EXCEPTION
4. **链式哈希**: `hash = SHA256(prev_hash || record)`，BEFORE INSERT 触发器自动计算，定时校验链完整性
5. **归档保护**: audit_log_archive 表受相同三层保护，归档函数仅 archive_runner 角色可调用

---

## 8. 并发控制与锁策略

### 8.1 三层锁策略

| 层级 | 机制 | 适用场景 | 占比 |
|---|---|---|---|
| 乐观锁 | version 列 + `If-Match` header | 资产元数据更新、属性修改、位置变更 | ~90% |
| 悲观锁 | `SELECT ... FOR UPDATE` + `SET lock_timeout=5s` | 资产领用/归还、生命周期转换 | ~8% |
| Advisory 锁 | `pg_try_advisory_lock` (非阻塞) | 批量退役、归档操作 | ~2% |

### 8.2 乐观锁

```go
func (r *AssetRepo) UpdateWithRetry(ctx context.Context, a *domain.Asset, maxRetries int) error {
    for attempt := 1; attempt <= maxRetries; attempt++ {
        // 读取当前版本
        current, _ := r.GetByID(ctx, a.ID)
        // 应用修改
        a.Version = current.Version
        // 尝试更新
        err := r.db.QueryRow(ctx, updateSQL,
            a.ID, a.Name, a.TypeID, a.LocationID,
            a.State, a.Status, a.Properties,
            a.UpdatedBy, a.Version,
        ).Scan(&a.Version)
        if err == pgx.ErrNoRows {
            if attempt >= maxRetries {
                return apierror.NewConflict("asset", a.ID, a.Version)
            }
            continue // 重试
        }
        return err
    }
    return ErrMaxRetriesExceeded
}
```

**重试上限**: 3 次，超过返回 409 + `retry_exhausted: true`。

### 8.3 悲观锁 (死锁预防)

**全局锁排序规范**: 所有事务按 `asset_id` UUID 字典序依次锁定，禁止以业务语义顺序锁定。

```go
func LockAssetsSorted(ctx context.Context, tx pgx.Tx, assetIDs []uuid.UUID) error {
    // 1. 排序
    sort.Slice(assetIDs, func(i, j int) bool {
        return assetIDs[i].String() < assetIDs[j].String()
    })
    // 2. 逐一锁定
    for _, id := range assetIDs {
        tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'")
        tx.QueryRow(ctx, "SELECT id FROM assets.assets WHERE id = $1 FOR UPDATE", id)
    }
    return nil
}
```

**死锁检测集成测试**: 两个 goroutine 反向 transfer，验证不出现 `40P01 deadlock_detected`。

### 8.4 Advisory 锁

```go
func (s *AssetService) BulkRetireByLocation(ctx context.Context, locationID uuid.UUID) error {
    lockID := hashUUIDToInt64(locationID)
    // 非阻塞获取
    acquired, _ := s.db.Exec(ctx, "SELECT pg_try_advisory_lock($1)", lockID)
    if !acquired {
        return apierror.NewConflict("location", locationID, 0) // 409 稍后重试
    }
    defer s.db.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)
    return s.repo.BulkUpdateLifecycleByLocation(ctx, locationID, "retirement")
}
```

**碰撞检测**: 启动时扫描所有 UUID，检测 `hashUUIDToInt64` 碰撞，发现碰撞拒绝启动并告警。

### 8.5 乐观/悲观锁路径分离

| 路径 | 锁类型 | 操作 |
|---|---|---|
| Agent 上报 | 乐观锁 (INSERT snapshots/audit) | 属性变化用 `UpdateWithRetry`，不持有悲观锁 |
| 人工操作 | 悲观锁 | assign/transfer/transition 使用 `FOR UPDATE` |

两条路径不竞争同一行锁，Agent 上报不阻塞人工操作。

### 8.6 限流策略

滑动窗口 (Redis + 本地令牌桶兜底)：

| 用户层级 | 限制 | 窗口 |
|---|---|---|
| Admin (人工) | 300 req/min | 60s |
| User (人工) | 100 req/min | 60s |
| Agent (程序) | 30 req/min + 10 req/s 突发 | 60s |
| `/auth/login` | 5 req/min/IP | 60s |

---

## 9. Agent 采集架构

### 9.1 架构概览

```
┌─────────────────────────────────┐
│        Collection Agent         │
│  ┌───────────────────────────┐  │
│  │   Monitor Service         │  │
│  │   - 分级采集 (关键5min/标准15min/低优30min) │
│  │   - 跨平台 OS 原生命令   │  │
│  │   - 计算 checksum        │  │
│  └───────────┬───────────────┘  │
│  ┌───────────▼───────────────┐  │
│  │   Communication Service   │  │
│  │   - mTLS 出站 HTTPS       │  │
│  │   - Delta 增量推送        │  │
│  │   - 失败入离线队列        │  │
│  └───────────────────────────┘  │
│  ┌───────────────────────────┐  │
│  │   Local SQLite Queue      │  │
│  │   - 离线缓存 (≤10,000)    │  │
│  │   - 溢出到本地文件        │  │
│  │   - attempts≥100 → DLQ    │  │
│  └───────────────────────────┘  │
└─────────────────────────────────┘
```

### 9.2 注册与认证

1. Agent 启动 → 生成硬件指纹 (`SHA256(/etc/machine-id + MAC + hostname)`)
2. 生成 Ed25519 密钥对
3. `POST /auth/register-agent` 携带 enrollment token + 指纹 + 公钥
4. 服务器验证 token (原子 UPDATE) → 签发 mTLS 客户端证书 + JWT
5. Agent `org_id` 从 enrollment token 继承
6. 证书 CN = agent_id, 有效期 90 天, 到期前 7 天自动续期

**吊销 Agent 时**: 同时设置 `cert_revoked=true` (DB 实时) + 更新 CRL 文件 + Redis Pub/Sub 触发 Nginx reload

### 9.3 增量同步协议

- **首次运行 (全量)**: 所有 collector → checksum → `SyncPayload{full_snapshot: true}`
- **后续 (增量, 按分级频率)**: 比较 checksum → 仅打包变化模块 → `SyncPayload{full_snapshot: false, delta_modules: [...]}`
- **全量快照频率**: 每小时一次 (Delta 压缩)
- **分级采集频率**: critical=5min, standard=15min, low_priority=30min

### 9.4 采集模块

| 模块 | Linux | Windows | macOS |
|---|---|---|---|
| CPU | `/proc/cpuinfo` | `Win32_Processor` | `sysctl -n machdep.cpu` |
| 内存 | `/proc/meminfo` | `Win32_PhysicalMemory` | `sysctl hw.memsize` |
| 磁盘 | `lsblk -J` | `Win32_DiskDrive` | `diskutil list` |
| 网络 | `/sys/class/net/*` | `Win32_NetworkAdapter` | `ifconfig` |
| OS | `/etc/os-release` | `Win32_OperatingSystem` | `sw_vers` |
| BIOS | `dmidecode -t bios` | `Win32_BIOS` | `system_profiler SPHardwareDataType` |

### 9.5 Server 端摄入管道

```
Agent POST /sync → Redis Stream (持久化, 消费者组)
       → Pre-Check (Ed25519 签名预检, 模块数≤200, 背压满载 503)
       → Processor (验证 sequence 连续性, 去重, 转换 domain)
       → Engine (INSERT snapshots + audit_log, 乐观锁更新 assets 属性)
```

### 9.6 离线队列

- SQLite 存储 (`modernc.org/sqlite`, 零 CGO)
- 队列上限 10,000 条，满时**停止采集+告警**，不删旧记录
- 溢出到本地文件 (`queue_overflow.db`)
- 最大重试 100 次，超过移入 Dead-Letter Queue
- 服务器端 `sequence gap` 检测，缺失时通知 Admin
- 重连后按时间顺序清空队列后发新数据

### 9.7 自更新机制

- 每 6 小时 `POST /agents/:id/update-check`
- 服务器返回版本号 + 下载 URL + SHA-256 + Ed25519 签名
- Agent 验证签名 → 下载 → 校验 SHA-256
- `.new` 文件 + 可执行权限 → `syscall.Exec` (Linux/macOS) 或批处理 (Win) 原地替换
- 启动失败 30 秒内自动回滚 `.old` → 当前二进制
- 灰度发布: 10% → 50% → 100%

### 9.8 资源预算

| 指标 | 目标值 |
|---|---|
| CPU | <1% (采集期间共享单核) |
| RAM | <50 MB |
| 磁盘 | <15 MB (含离线队列) |
| 网络 | 纯出站 HTTPS :443 |
| 二进制大小 | ~10-12 MB |

---

## 10. 事件与 Webhook

### 10.1 事件总线

基于 **Redis Pub/Sub** (多实例跨实例传播) + **Outbox Pattern** 保证事件不丢失：

```go
// 服务层: 在同一事务后置 hook 中写入 outbox 表 + 发布 Redis 事件
func (s *AssetService) Create(ctx context.Context, asset *domain.Asset) error {
    tx, _ := s.db.Begin(ctx)
    defer tx.Rollback(ctx)
    s.repo.Insert(ctx, tx, asset)
    // Outbox: 持久化事件
    s.eventRepo.InsertOutbox(ctx, tx, EventAssetCreated, asset.ID, payload)
    tx.Commit(ctx)
    // 事务成功后发布到 Redis (所有实例订阅)
    s.eventBus.Publish(ctx, EventAssetCreated, payload)
    return nil
}
```

**事件类型**:

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

### 10.2 Webhook 引擎

```go
// Webhook 防重放 payload
type WebhookPayload struct {
    EventID     string          `json:"event_id"`
    EventType   string          `json:"event_type"`
    DeliveredAt time.Time       `json:"delivered_at"`
    Data        json.RawMessage `json:"data"`
    Signature   string          `json:"-"` // X-Signature-256 header
}

// HMAC 签名
func SignPayload(secret []byte, eventID string, deliveredAt time.Time, body []byte) string {
    mac := hmac.New(sha256.New, secret)
    mac.Write([]byte(eventID))
    mac.Write([]byte(deliveredAt.Format(time.RFC3339)))
    mac.Write(body)
    return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
```

- 重试: 指数退避 (1m → 2m → 4m → 8m → 16m)，最多 5 次
- SSRF 防护: 强制 HTTPS + 拒绝内网 CIDR
- Secret: AES-256-GCM 加密存储

---

## 11. 缓存策略

### 11.1 缓存内容

| 缓存项 | Key 模式 | TTL | 策略 |
|---|---|---|---|
| 资产详情 | `asset:{id}` | 5 min | 写入时失效 |
| 资产列表 (热门) | `asset:list:{hash(query)}` | 2 min | LRU |
| Agent 在线状态 | `agent:status:{id}` | 1 min | 心跳刷新 |
| 用户 session | `session:{user_id}` | 同 JWT | 登出删除 |
| 限流计数 | `ratelimit:{tier}:{user_id}:{window}` | 窗口时长 | 滑动窗口 |

### 11.2 缓存模式

- **Cache-Aside**: 查缓存 → 命中返回 / 未命中查 DB → 回填
- **Write-Invalidate**: 更新时删除缓存 key，延迟双删 (先删→写 DB→500ms 再删)
- **TTL 兜底**: 所有缓存均有 TTL

### 11.3 Redis 高可用

- **Sentinel 3 节点** (跨 AZ)：自动故障转移，RTO<10s
- **本地令牌桶兜底**: Redis 不可用时限流中间件降级为本地域流
- **缓存熔断**: Redis 不可用时直接查 DB + 限流保护
- **refresh token fallback**: Redis 不可用时查询 PostgreSQL 作为备选

### 11.4 fail-closed 分级策略

| 操作类型 | Redis 故障时行为 | 配置项 |
|---|---|---|
| 写操作 (POST/PUT/DELETE) | fail-closed, 返回 503 | — |
| 读操作 (GET) | 可选 fail-open (跳过黑名单) | `auth.fail_open_get=true` |
| Agent 上报 (/agents/sync) | fail-open (优先数据采集) | `auth.fail_open_agent_sync=true` |

---

## 12. Grafana 集成

### 12.1 只读通道

```sql
CREATE ROLE grafana_reader WITH LOGIN;
GRANT CONNECT ON DATABASE assetdb TO grafana_reader;
GRANT USAGE ON SCHEMA assets TO grafana_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA assets TO grafana_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA assets GRANT SELECT ON TABLES TO grafana_reader;
```

Grafana 通过 PgBouncer → PostgreSQL Replica 只读查询。

### 12.2 PgBouncer 配置

```ini
[databases]
assetdb = host=primary.db.internal port=5432 dbname=assetdb
assetdb_ro = host=replica.db.internal port=5432 dbname=assetdb

[pgbouncer]
pool_mode = transaction
max_client_conn = 200
default_pool_size = 25
reserve_pool_size = 10
max_prepared_statements = 0       # transaction mode: 禁用 prepared statements
```

> **pgx 配置**: `default_query_exec_mode = QueryExecModeSimpleProtocol` (禁用 implicit prepared statements)
> **Grafana**: `preparedStatements: false`

**PgBouncer HA**: 2 实例 + 多数据源或 HA proxy / Keepalived VIP。

### 12.3 仪表盘

| 仪表盘 | 面板 |
|---|---|
| **资产概览** | KPI(总数)、Bar Gauge(生命周期分布)、Pie(类别)、Time Series(增长)、Bar(Top 制造商)、Table(位置分布) |
| **Agent 健康** | Stat(在线/离线)、Pie(版本分布)、Time Series(心跳)、Table(离线>24h) |
| **生命周期追踪** | Histogram(滞留时长)、Table(各阶段平均天数)、Table(今日状态转换) |

---

## 13. 部署架构

### 13.1 开发环境 (Docker Compose)

```yaml
services:
  postgres:       # PostgreSQL 16, port 5432
  redis:          # Redis 7, port 6379
  pgbouncer:      # PgBouncer, port 6432
  migrate:        # 一次性迁移 (depends_on: postgres, condition: service_healthy)
  api-server:     # Go binary, port 8080 (depends_on: migrate)
  grafana:        # Grafana OSS, port 3000
  web:            # Vite HMR, port 5173
```

### 13.2 生产环境 (Multi-AZ + K8s)

| 组件 | 部署方式 | 资源配置 |
|---|---|---|
| API Server | Deployment (replicas: 2+), HPA | 512Mi-2Gi mem, 0.5-2 CPU |
| PostgreSQL | Patroni + etcd 3节点 (Multi-AZ) | Primary: 4vCPU/16GB, Replica×2: 2vCPU/8GB |
| Redis | Sentinel 3节点 (Multi-AZ) | 2vCPU/4GB per node |
| PgBouncer | Deployment (replicas: 2) | 256Mi/0.25 CPU |
| Nginx | Deployment / Ingress Controller | 256Mi/0.25 CPU |
| Vault | StatefulSet (3节点, Raft) | 512Mi/0.5 CPU |
| MFA Service | Deployment (replicas: 2) | 128Mi/0.1 CPU |

**网络约束**: 同区域多 AZ (延迟 <2ms)，禁止跨区域；Sentinel/Patroni 通信走内网；复制延迟监控 (超 5s 告警)。

**云托管备选 (降低运维)**:
- Patroni → RDS Multi-AZ
- Redis Sentinel → ElastiCache
- Vault → AWS KMS / GCP KMS

### 13.3 健康检查

```go
GET /healthz → 200 (存活探针: 进程存活)
GET /readyz  → 200 (就绪探针: PG + Redis 可达) / 503 (不可用)
```

```yaml
# K8s probe
livenessProbe:
  httpGet: { path: /healthz, port: 8080 }
  initialDelaySeconds: 5, periodSeconds: 10
readinessProbe:
  httpGet: { path: /readyz, port: 8080 }
  initialDelaySeconds: 5, periodSeconds: 5
```

### 13.4 数据库迁移与种子数据

- 工具: `golang-migrate/migrate`
- 开发环境: `--auto-migrate=true`
- 生产环境: init container + advisory lock 防并发迁移
- 种子数据: 初始资产类型 + 默认组织 + 管理员账号 (Phase 0)

---

## 14. 可靠性设计

### 14.1 故障转移

| 组件 | 故障转移方案 | RTO | RPO |
|---|---|---|---|
| PostgreSQL | Patroni 自动切换 (etcd 共识) | <30s | 同步: 0; 异步: <5s |
| Redis | Sentinel 自动故障转移 | <10s | 缓存: 无数据丢失 (重建); Token: PG fallback |
| API Server | Nginx 健康检查剔除 → 新实例接手 | <5s | 无状态, 无数据丢失 |
| PgBouncer | HA Proxy / Keepalived VIP | <10s | 连接中断需重连 |
| Vault | Raft 共识 + API Server 公钥缓存 | <10s | 已运行实例不受影响 |

### 14.2 容错降级

| 故障场景 | 降级策略 | 影响 |
|---|---|---|
| Redis 故障 | 本地令牌桶 + 缓存熔断 + 写操作 503 | 读可降级, 写不可 |
| Vault 故障 | 公钥缓存验证 JWT + 无法签发新 token | 存量用户不受影响 |
| PostgreSQL Replica 故障 | PgBouncer 仅路由到 Primary | Grafana 只读通道不可用 |

### 14.3 数据保护

- **离线队列**: 满时停止采集不丢数据，溢出本地文件，DLQ 兜底
- **摄入管道**: Redis Stream 持久化，at-least-once 语义
- **S3 归档**: 幂等 + 状态机 + archive_manifest + checksum 验证 + 3 次重试
- **审计日志**: 三层不可变保护 + 链式哈希完整性
- **snapshots 治理**: 采样降频 + Delta 压缩 + 冷热分层 (576GB→48GB/天)

### 14.4 故障转移演练

每月第一个周六 02:00 自动执行 Patroni 切换演练：
1. 记录演练前状态 → 2. 优雅切换 → 3. 验证写入 → 4. 验证 Replica 升级 → 5. 恢复原状 → 6. 发送报告

---

## 15. 实施计划

### 15.1 Phase 规划 (标准团队 9 人, ~34 周)

| Phase | 名称 | 工作量 | 依赖 | 关键交付 |
|---|---|---|---|---|
| **0** | 安全基础设施 (Vault/KMS, MFA, CRL/OCSP) | 12 人天 | — | 密钥管理就绪, MFA 可验证 |
| **1** | Foundation | 15 人天 | P0 | JWT EdDSA + Vault 集成, ltree, 基础 Migration |
| **2** | Core CRUD + Locking | 24 人天 | P1 | 资产 CRUD, 乐观/悲观锁, 状态机, 死锁检测测试 |
| **3** | Ingestion + Agent | 30 人天 | P1-P2 | mTLS 生命周期, Buffer 预检, 跨平台采集器, 离线队列 |
| **4** | Caching + Events + Webhooks | 18 人天 | P2 | Redis Sentinel, Webhook AES+HMAC, SSRF 防护 |
| **5** | Dashboard + Locations + Orgs | 10 人天 | P2 | 聚合查询 API, 位置/组织 CRUD |
| **6** | Frontend | 22 人天 | P2-P5 | React 全量页面, 路由守卫, 403/401 拦截 |
| **7** | Agent Polish | 12 人天 | P3 | 证书续期, 降频配置, 交叉编译 |
| **8a** | Grafana + Docker Compose | 12 人天 | P5 | 3 仪表盘, 开发环境一键启动 |
| **8b** | 生产 HA 部署 (K8s/Patroni) | 18 人天 | P8a | Multi-AZ K8s, Patroni, Sentinel, 监控 |
| **9** | Testing | 20 人天 | P1-P8 | 集成/负载/E2E, 死锁/链式哈希/fail-closed 测试 |
| **9.5** | 数据管道 (S3 归档, 聚合表) | 15 人天 | P8 | 归档管道, 分区维护, 冷热分层 |
| **10** | Hardening & Ops | 20 人天 | P1-P9 | 软删除, 链式哈希校验, refresh 轮换 |
| **10.5** | 安全审计与渗透测试 | 8 人天 | P10 | IDOR/SSRF/重放渗透, mTLS 证书链验证 |
| **11** | HA 混沌测试 + Runbook | 10 人天 | P8b-P10.5 | 故障转移演练, 运维 Runbook |
| **合计** | | **236 人天** | | |

### 15.2 里程碑

| 里程碑 | Phase | 验收标准 |
|---|---|---|
| M0 | P0 | Vault/MFA 可验证, CRL/OCSP 基础设施就绪 |
| M1 | P1 | `/healthz` 200, Migration 成功, JWT EdDSA 签发+验证通过 |
| M2 | P2 | 资产 CRUD 全流程, 乐观锁 409, 领用事务原子性, 死锁测试通过 |
| M3 | P3 | Linux Agent 注册→采集→上报→入库全链路, 离线→重连→清空队列 |
| M4 | P4+P5 | 缓存命中率>80%, Webhook HMAC 验证, Dashboard API 正确 |
| M5 | P6 | 登录→资产列表→详情→领用→仪表盘全流程可操作, 路由守卫生效 |
| M6 | P7 | 全平台 Agent 二进制 (6 矩阵), 自更新+回滚验证 |
| M7 | P8 | Docker Compose 一键启动, K8s Multi-AZ 部署成功 |
| M8 | P9 | API 集成测试通过率>95%, Agent E2E 6 矩阵全通过 |
| M9 | P10+P10.5 | 渗透测试通过, 软删除/分区/归档全量验证 |
| M10 | P11 | 故障转移演练通过, 运维 Runbook 完成 |

### 15.3 团队配置建议

| 角色 | 人数 | 参与阶段 | 技能要求 |
|---|---|---|---|
| 技术负责人/架构师 | 1 | 全程 | Go 5年+, PostgreSQL 深度, 分布式系统 |
| Go 后端工程师 | 2 | P1-P10 | Go 3年+, Gin, pgx, 并发控制, JWT/mTLS |
| Agent/跨平台工程师 | 1 | P3, P7, P9 | Go 3年+, 交叉编译, Linux/Win/macOS 系统编程 |
| 前端工程师 (React) | 1 | P6, P9-P10 | React 18+TypeScript 3年+, shadcn/ui, Zustand |
| DBA/数据库工程师 | 1.5 | 全程 | Patroni, ltree, 分区表, 性能调优 |
| DevOps/SRE | 1.5 | P8b-P11 | Vault, Sentinel, K8s/Helm, Multi-AZ, OCSP/CRL |
| 安全工程师 | 0.5 | P10.5 | PKI/X.509, HMAC, SSRF, 渗透测试 |
| QA/测试工程师 | 1 | P2-P11 | Go 测试, 负载测试 (k6/vegeta), E2E, 混沌测试 |
| 项目经理 | 0.5 | 全程 | 敏捷/Scrum |
| **合计** | **9** | | |

---

## 16. 附录：实施检查清单

### Phase 0: 安全基础设施
- [ ] Vault/KMS 部署 (HA 3 节点或云 KMS 配置)
- [ ] MFA 服务搭建 (TOTP, HA 2 实例)
- [ ] Nginx CRL 刷新脚本 (1h 间隔)
- [ ] OCSP Stapling 配置
- [ ] CA 证书颁发机构搭建

### Phase 1: Foundation
- [ ] Go module 初始化, 项目骨架
- [ ] PostgreSQL migration (000001_init_schema) + ltree 扩展
- [ ] JWT EdDSA + Vault 集成 (签发 + 验证 + kid header)
- [ ] 中间件链 (request ID → recovery → logging → rate limit → auth → org scope)
- [ ] `/healthz`, `/readyz` 端点

### Phase 2: Core CRUD + Locking
- [ ] Asset CRUD + 软删除
- [ ] 乐观锁 `UpdateWithRetry` (3 次上限)
- [ ] 悲观锁 + `SET lock_timeout=5s`
- [ ] 全局锁排序规范
- [ ] 生命周期状态机 (合法转换矩阵校验)
- [ ] Advisory 锁 + 碰撞检测
- [ ] 死锁检测集成测试

### Phase 3: Ingestion + Agent
- [ ] Agent 注册 (enrollment token 原子并发 + SHA-256)
- [ ] mTLS 证书签发 (90 天 + 自动续期)
- [ ] mTLS 证书吊销 (cert_revoked + CRL + OCSP)
- [ ] Linux 采集器 (`/proc`, `dmidecode`, `lshw`)
- [ ] 摄入管道 (Redis Stream → Pre-Check → Processor → Engine)
- [ ] 离线队列 (SQLite, 满时停止采集, DLQ)
- [ ] Buffer 签名预检 + 背压 (满载 503)
- [ ] `org_id` 绑定 (Engine 层校验)

### Phase 4: Caching + Events + Webhooks
- [ ] Redis Sentinel 3 节点部署
- [ ] Cache-Aside + Write-Invalidate + 延迟双删
- [ ] Redis Pub/Sub 事件总线 (跨实例)
- [ ] Outbox Pattern
- [ ] Webhook AES-256-GCM 加密 + HMAC 防重放 + SSRF 防护

### Phase 5: Dashboard + Locations + Orgs
- [ ] 聚合查询 API (按状态/类别/生命周期)
- [ ] Location CRUD (树结构)
- [ ] Organization CRUD (ltree 物化路径)
- [ ] Dashboard 数据接口

### Phase 6: Frontend
- [ ] Vite + React + TailwindCSS + shadcn/ui 脚手架
- [ ] 登录页面 + JWT refresh 自动轮换
- [ ] 资产表格/详情/表单 (含游标分页)
- [ ] Agent 管理页面
- [ ] 24 条路由守卫 (RequireAuth 组件)
- [ ] 403 Forbidden 页面
- [ ] API 客户端 401 refresh 排队 + 403 循环防护
- [ ] 审批队列页面

### Phase 7: Agent Polish
- [ ] Windows 采集器 (WMI)
- [ ] macOS 采集器 (system_profiler)
- [ ] 证书自动续期逻辑
- [ ] 采样降频配置
- [ ] 全平台交叉编译 (Linux/Win/macOS × amd64/arm64)

### Phase 8: Deployment
- [ ] Docker Compose 开发环境
- [ ] Dockerfile + 健康检查探针
- [ ] PgBouncer 配置 (transaction mode, 多后端, HA pair)
- [ ] K8s/Helm Chart 生产部署
- [ ] Patroni + etcd 集群
- [ ] Nginx upstream + 主动健康检查
- [ ] Multi-AZ 网络配置

### Phase 9: Testing
- [ ] API 集成测试 (覆盖率 >95%)
- [ ] 死锁检测集成测试
- [ ] 链式哈希完整性测试
- [ ] 负载测试 (目标 QPS 50%+)
- [ ] Agent E2E (6 平台矩阵)
- [ ] fail-closed 场景测试
- [ ] mTLS 证书续期 E2E

### Phase 10: Hardening
- [ ] 软删除全量验证
- [ ] 链式哈希定时校验 job
- [ ] 归档 SECURITY DEFINER 函数 + archive_runner 权限
- [ ] audit_meta 元日志
- [ ] 分区表自动维护 job
- [ ] JWT refresh token 轮换并发安全验证
- [ ] API 版本兼容 Deprecation 中间件

### Phase 10.5: 安全审计
- [ ] 渗透测试 (IDOR/SSRF/重放/注入)
- [ ] mTLS 证书链验证
- [ ] Webhook HMAC 伪造测试
- [ ] JWT 越权测试
- [ ] Vault 密钥轮换演练

### Phase 11: 混沌测试 + Runbook
- [ ] Patroni 故障转移演练 (自动化脚本)
- [ ] Redis Sentinel 切换测试
- [ ] Redis 故障 fail-closed 验证
- [ ] Vault 故障降级验证
- [ ] 运维 Runbook 编写
- [ ] 备份恢复流程验证

---

> **文档结束** — Asset Database System Architecture v2.0, 2026-07-16
