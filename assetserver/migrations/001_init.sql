-- ============================================================
-- Asset Database System — 初始数据库迁移
-- PostgreSQL 16
-- 对应架构文档 §5 数据模型
-- ============================================================

-- 1. Schema
CREATE SCHEMA IF NOT EXISTS assets;
SET search_path TO assets;

-- 2. 角色
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'app_writer') THEN
        CREATE ROLE app_writer WITH LOGIN PASSWORD 'devpassword';
    END IF;
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'audit_reader') THEN
        CREATE ROLE audit_reader WITH LOGIN PASSWORD 'devpassword';
    END IF;
END
$$;

GRANT USAGE ON SCHEMA assets TO app_writer, audit_reader;
GRANT CREATE ON SCHEMA assets TO app_writer;

-- 3. 扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "ltree";

-- ============================================================
-- 4. 核心表
-- ============================================================

-- 4.1 组织
CREATE TABLE assets.organizations (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(255) NOT NULL,
    parent_id  UUID REFERENCES assets.organizations(id),
    path       LTREE NOT NULL DEFAULT 'root',
    depth      INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_orgs_parent ON assets.organizations(parent_id);
CREATE INDEX idx_orgs_path_gist ON assets.organizations USING GIST(path);

-- 4.2 用户
CREATE TABLE assets.users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id        UUID NOT NULL REFERENCES assets.organizations(id),
    username      VARCHAR(100) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50) NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('super_admin','admin','operator','viewer')),
    email         VARCHAR(255),
    status        VARCHAR(20) NOT NULL DEFAULT 'active'
                  CHECK (status IN ('active','disabled','locked')),
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_org ON assets.users(org_id);

-- 4.3 资产类型
CREATE TABLE assets.asset_types (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name       VARCHAR(100) NOT NULL UNIQUE,
    category   VARCHAR(50) NOT NULL,
    schema     JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.4 资产
CREATE TABLE assets.assets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asset_tag       VARCHAR(100) NOT NULL UNIQUE,
    name            VARCHAR(255) NOT NULL,
    type_id         UUID NOT NULL REFERENCES assets.asset_types(id),
    org_id          UUID NOT NULL REFERENCES assets.organizations(id),
    serial_number   VARCHAR(255),
    manufacturer    VARCHAR(255),
    model           VARCHAR(255),
    location_id     UUID,
    lifecycle_state VARCHAR(50) NOT NULL DEFAULT 'procurement'
                    CHECK (lifecycle_state IN ('procurement','deployment','utilization','maintenance','retirement')),
    status          VARCHAR(50) NOT NULL DEFAULT 'available'
                    CHECK (status IN ('available','assigned','maintenance','retired')),
    properties      JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    version         INTEGER NOT NULL DEFAULT 1,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 索引
CREATE INDEX idx_assets_org_status ON assets.assets(org_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_type ON assets.assets(org_id, type_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_updated ON assets.assets(org_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_lifecycle ON assets.assets(org_id, lifecycle_state) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_tag ON assets.assets(asset_tag) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_properties ON assets.assets USING GIN(properties jsonb_path_ops) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_search ON assets.assets USING GIN(
    to_tsvector('english', name || ' ' || COALESCE(manufacturer,'') || ' ' || COALESCE(model,'') || ' ' || COALESCE(serial_number,'') || ' ' || asset_tag)
);

-- 4.5 领用表
CREATE TABLE assets.assignments (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    asset_id    UUID NOT NULL REFERENCES assets.assets(id),
    org_id      UUID NOT NULL REFERENCES assets.organizations(id),
    assigned_to UUID NOT NULL REFERENCES assets.users(id),
    assigned_by UUID NOT NULL REFERENCES assets.users(id),
    status      VARCHAR(20) NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','returned','transferred')),
    notes       TEXT,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    returned_at TIMESTAMPTZ,
    version     INTEGER NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX idx_active_assignment ON assets.assignments(asset_id) WHERE status = 'active';
CREATE INDEX idx_assignments_user ON assets.assignments(assigned_to) WHERE status = 'active';
CREATE INDEX idx_assignments_asset_time ON assets.assignments(asset_id, assigned_at DESC);

-- 4.6 审计日志 (不可变)
CREATE TABLE assets.audit_log (
    id         BIGSERIAL PRIMARY KEY,
    asset_id   UUID,
    org_id     UUID NOT NULL REFERENCES assets.organizations(id),
    user_id    UUID REFERENCES assets.users(id),
    agent_id   UUID,
    action     VARCHAR(50) NOT NULL,
    field      VARCHAR(255),
    old_value  TEXT,
    new_value  TEXT,
    metadata   JSONB DEFAULT '{}' CHECK (octet_length(metadata::text) <= 4096),
    prev_hash  CHAR(64),
    hash       CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_asset_time ON assets.audit_log(asset_id, created_at DESC);
CREATE INDEX idx_audit_org_time ON assets.audit_log(org_id, created_at DESC);
CREATE INDEX idx_audit_action_time ON assets.audit_log(action, created_at DESC);
CREATE INDEX idx_audit_recent ON assets.audit_log(created_at DESC);

-- 审计日志不可变保护
REVOKE UPDATE, DELETE ON assets.audit_log FROM app_writer;
ALTER TABLE assets.audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_insert_only ON assets.audit_log FOR INSERT TO app_writer WITH CHECK (true);
CREATE POLICY audit_log_select_only ON assets.audit_log FOR SELECT TO audit_reader USING (true);

CREATE OR REPLACE FUNCTION assets.audit_log_immutable_guard()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only: % not permitted on row %', TG_OP, OLD.id;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_immutable
    BEFORE UPDATE OR DELETE ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_immutable_guard();

-- 4.7 采集 Agent
CREATE TABLE assets.collection_agents (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
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
CREATE INDEX idx_agents_status_heartbeat ON assets.collection_agents(status, last_heartbeat);
CREATE INDEX idx_agents_org ON assets.collection_agents(org_id);

-- 4.8 资产快照 (按月分区)
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

CREATE TABLE assets.asset_snapshots_2026_08
    PARTITION OF assets.asset_snapshots
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');

CREATE INDEX idx_snapshots_agent_time ON assets.asset_snapshots(agent_id, created_at DESC);
CREATE INDEX idx_snapshots_asset_time ON assets.asset_snapshots(asset_id, created_at DESC);

-- 4.9 注册令牌
CREATE TABLE assets.enrollment_tokens (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    token_hash    VARCHAR(64) UNIQUE NOT NULL,
    created_by    UUID NOT NULL REFERENCES assets.users(id),
    org_id        UUID NOT NULL REFERENCES assets.organizations(id),
    expires_at    TIMESTAMPTZ NOT NULL,
    used_at       TIMESTAMPTZ,
    used_by_agent UUID REFERENCES assets.collection_agents(id),
    max_uses      INTEGER NOT NULL DEFAULT 1,
    use_count     INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 4.10 JWT 吊销表
CREATE TABLE assets.revoked_tokens (
    id          BIGSERIAL PRIMARY KEY,
    jti         VARCHAR(128) NOT NULL UNIQUE,
    user_id     UUID REFERENCES assets.users(id),
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    reason      VARCHAR(100),
    revoked_by  UUID REFERENCES assets.users(id)
);
CREATE INDEX idx_revoked_tokens_expires ON assets.revoked_tokens(expires_at);

-- 4.11 Refresh Token 表
CREATE TABLE assets.refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES assets.users(id),
    token_hash CHAR(64) NOT NULL,
    family_id  UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user ON assets.refresh_tokens(user_id, expires_at);
CREATE INDEX idx_refresh_tokens_family ON assets.refresh_tokens(family_id);

-- 4.12 审批请求
CREATE TABLE assets.approval_requests (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    action      VARCHAR(50) NOT NULL,
    target_id   UUID,
    requestor   UUID NOT NULL REFERENCES assets.users(id),
    approver    UUID REFERENCES assets.users(id),
    status      VARCHAR(20) DEFAULT 'pending'
                CHECK (status IN ('pending','approved','rejected')),
    reason      TEXT,
    meta        JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

-- 4.13 归档清单
CREATE TABLE assets.archive_manifest (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    archive_id      UUID UNIQUE NOT NULL,
    table_name      VARCHAR(100) NOT NULL,
    partition_name  VARCHAR(100) NOT NULL,
    status          VARCHAR(20) DEFAULT 'pending'
                    CHECK (status IN ('pending','exporting','uploading','verifying','detaching','completed','failed','terminal_failed')),
    row_count       BIGINT,
    s3_key          VARCHAR(1024),
    s3_checksum     VARCHAR(64),
    error_message   TEXT,
    retry_count     INTEGER DEFAULT 0,
    max_retries     INTEGER NOT NULL DEFAULT 5,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(table_name, partition_name)
);

-- 4.14 归档审计
CREATE TABLE assets.audit_meta (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    batch_id        UUID NOT NULL,
    table_name      VARCHAR(100) NOT NULL,
    partition_name  VARCHAR(100),
    row_count       BIGINT NOT NULL,
    operated_by     VARCHAR(100) NOT NULL,
    trigger_status  BOOLEAN NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 5. 自动更新 updated_at 触发器
-- ============================================================
CREATE OR REPLACE FUNCTION assets.update_updated_at()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_orgs_updated_at BEFORE UPDATE ON assets.organizations
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON assets.users
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();
CREATE TRIGGER trg_assets_updated_at BEFORE UPDATE ON assets.assets
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();
CREATE TRIGGER trg_agents_updated_at BEFORE UPDATE ON assets.collection_agents
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();

-- ============================================================
-- 6. 种子数据
-- ============================================================

-- 默认组织
INSERT INTO assets.organizations (id, name, path, depth) VALUES
    ('00000000-0000-4000-a000-000000000001', 'Demo Corp', 'root.Demo_Corp', 1);

-- Admin 用户 (password_hash = bcrypt of "admin123")
INSERT INTO assets.users (id, org_id, username, password_hash, role, email) VALUES
    ('00000000-0000-4000-a000-000000000010', '00000000-0000-4000-a000-000000000001', 'admin', '$2a$10$placeholder_hash_for_admin123', 'super_admin', 'admin@demo.local');

-- 资产类型
INSERT INTO assets.asset_types (id, name, category, schema) VALUES
    ('10000000-0000-4000-a000-000000000001', 'Laptop',  'hardware', '{"type":"object","properties":{"cpu":{"type":"string"},"ram":{"type":"string"},"storage":{"type":"string"},"os":{"type":"string"}}}'),
    ('10000000-0000-4000-a000-000000000002', 'Server',  'hardware', '{"type":"object","properties":{"cpu":{"type":"string"},"ram":{"type":"string"},"storage":{"type":"string"},"rack":{"type":"string"},"os":{"type":"string"}}}'),
    ('10000000-0000-4000-a000-000000000003', 'Monitor', 'hardware', '{"type":"object","properties":{"size":{"type":"string"},"resolution":{"type":"string"},"panel":{"type":"string"}}}'),
    ('10000000-0000-4000-a000-000000000004', 'Network', 'hardware', '{"type":"object","properties":{"ports":{"type":"integer"},"speed":{"type":"string"},"firmware":{"type":"string"}}}');

-- ============================================================
-- 7. 权限授予
-- ============================================================
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA assets TO app_writer;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA assets TO app_writer;
GRANT SELECT ON ALL TABLES IN SCHEMA assets TO audit_reader;
