-- Asset Database System — 初始数据库 Schema
-- 对应架构文档 §5 数据模型
-- Phase 1: Foundation Migration

-- 扩展
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS btree_gist;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Schema
CREATE SCHEMA IF NOT EXISTS assets;

-- ===================================================================
-- 组织表 (ltree 物化路径, 深度≤20)
-- ===================================================================
CREATE TABLE assets.organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    parent_id   UUID REFERENCES assets.organizations(id),
    depth       INTEGER NOT NULL DEFAULT 0 CHECK (depth <= 20),
    path        LTREE NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orgs_path_gist ON assets.organizations USING GIST (path);
CREATE INDEX idx_orgs_parent ON assets.organizations (parent_id);

-- ===================================================================
-- 用户表 (5 种角色)
-- ===================================================================
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

-- ===================================================================
-- 资产类型 (JSON Schema 驱动扩展)
-- ===================================================================
CREATE TABLE assets.asset_types (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name     VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL
             CHECK (category IN ('hardware','software','network','cloud_resource','license','other')),
    schema   JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===================================================================
-- 核心资产表 (软删除 + 乐观锁 version)
-- ===================================================================
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
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      UUID REFERENCES assets.users(id),
    updated_by      UUID REFERENCES assets.users(id)
);

-- org_id-leading 复合索引 (多租户查询)
CREATE INDEX idx_assets_org_status ON assets.assets (org_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_type ON assets.assets (org_id, type_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_updated ON assets.assets (org_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_lifecycle ON assets.assets (org_id, lifecycle_state) WHERE deleted_at IS NULL;
CREATE INDEX idx_assets_org_location ON assets.assets (org_id, location_id) WHERE deleted_at IS NULL;

-- JSONB GIN 索引
CREATE INDEX idx_assets_properties ON assets.assets USING GIN (properties jsonb_path_ops) WHERE deleted_at IS NULL;

-- 全文搜索
CREATE INDEX idx_assets_search ON assets.assets USING GIN (
    to_tsvector('english', name || ' ' || COALESCE(manufacturer,'') || ' ' || COALESCE(model,'') || ' ' || COALESCE(serial_number,'') || ' ' || asset_tag)
);

CREATE INDEX idx_assets_deleted ON assets.assets (deleted_at) WHERE deleted_at IS NOT NULL;

-- ===================================================================
-- 位置表 (树结构)
-- ===================================================================
CREATE TABLE assets.locations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(255) NOT NULL,
    parent_id  UUID REFERENCES assets.locations(id),
    org_id     UUID NOT NULL REFERENCES assets.organizations(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_locations_org ON assets.locations (org_id);
CREATE INDEX idx_locations_parent ON assets.locations (parent_id);

-- ===================================================================
-- 领用表
-- ===================================================================
CREATE TABLE assets.assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id     UUID NOT NULL REFERENCES assets.assets(id),
    assigned_to  UUID NOT NULL REFERENCES assets.users(id),
    assigned_by  UUID NOT NULL REFERENCES assets.users(id),
    status       VARCHAR(20) NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active','returned','lost','transferred')),
    notes        TEXT,
    reason       TEXT,  -- 审批理由字段 (对应的架构文档补充)
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    returned_at  TIMESTAMPTZ,
    version      INTEGER NOT NULL DEFAULT 1
);

CREATE UNIQUE INDEX idx_active_assignment
    ON assets.assignments (asset_id) WHERE status = 'active';

-- ===================================================================
-- 收集代理表
-- ===================================================================
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

CREATE INDEX idx_agents_status ON assets.collection_agents (status, last_heartbeat);
CREATE INDEX idx_agents_org ON assets.collection_agents (org_id);

-- ===================================================================
-- Enrollment Token 表
-- ===================================================================
CREATE TABLE assets.enrollment_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash  VARCHAR(64) UNIQUE NOT NULL,
    created_by  UUID NOT NULL REFERENCES assets.users(id),
    org_id      UUID NOT NULL REFERENCES assets.organizations(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    used_by_agent UUID REFERENCES assets.collection_agents(id),
    max_uses    INTEGER NOT NULL DEFAULT 1,
    use_count   INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===================================================================
-- Refresh Token 表 (Vault 全故障应急)
-- ===================================================================
CREATE TABLE assets.refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES assets.users(id),
    token_hash  CHAR(64) NOT NULL,
    family_id   UUID NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user ON assets.refresh_tokens (user_id, expires_at);
CREATE INDEX idx_refresh_tokens_family ON assets.refresh_tokens (family_id);

-- ===================================================================
-- JWT 吊销表 (Redis 故障兜底)
-- ===================================================================
CREATE TABLE assets.revoked_tokens (
    jti         VARCHAR(36) PRIMARY KEY,
    revoked_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_revoked_tokens_expiry ON assets.revoked_tokens (expires_at);

-- ===================================================================
-- 审计日志 (不可变 + 链式哈希)
-- ===================================================================
CREATE TABLE assets.audit_log (
    id         BIGSERIAL,
    asset_id   UUID,
    user_id    UUID REFERENCES assets.users(id),
    agent_id   UUID REFERENCES assets.collection_agents(id),
    org_id     UUID REFERENCES assets.organizations(id),
    action     VARCHAR(50) NOT NULL,
    field      VARCHAR(255),
    old_value  TEXT,
    new_value  TEXT,
    metadata   JSONB DEFAULT '{}' CHECK (octet_length(metadata::text) <= 4096),
    prev_hash  CHAR(64),
    hash       CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- 按月分区 (初始)
CREATE TABLE assets.audit_log_2026_07
    PARTITION OF assets.audit_log
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');

CREATE INDEX idx_audit_asset_time ON assets.audit_log (asset_id, created_at DESC);
CREATE INDEX idx_audit_user_time ON assets.audit_log (user_id, created_at DESC);
CREATE INDEX idx_audit_action_time ON assets.audit_log (action, created_at DESC);
CREATE INDEX idx_audit_org_time ON assets.audit_log (org_id, created_at DESC);

-- ===================================================================
-- 审计不可变性: 三层防御
-- ===================================================================

-- 1. 数据库角色
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'app_writer') THEN
        CREATE ROLE app_writer WITH LOGIN;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'audit_reader') THEN
        CREATE ROLE audit_reader WITH LOGIN;
    END IF;
END
$$;

GRANT INSERT ON assets.audit_log TO app_writer;
REVOKE UPDATE, DELETE ON assets.audit_log FROM app_writer;
GRANT SELECT ON assets.audit_log TO audit_reader;

-- 2. RLS
ALTER TABLE assets.audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_insert_only ON assets.audit_log
    FOR INSERT TO app_writer WITH CHECK (true);
CREATE POLICY audit_log_no_update ON assets.audit_log
    FOR UPDATE TO app_writer USING (false) WITH CHECK (false);
CREATE POLICY audit_log_no_delete ON assets.audit_log
    FOR DELETE TO app_writer USING (false);
CREATE POLICY audit_log_select_only ON assets.audit_log
    FOR SELECT TO audit_reader USING (true);

-- 3. 触发器 (最后一层防线)
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

-- 链式哈希触发器 (含 advisory lock 序列化)
CREATE OR REPLACE FUNCTION assets.audit_log_set_hash()
RETURNS trigger AS $$
DECLARE
    prev CHAR(64);
BEGIN
    PERFORM pg_advisory_xact_lock(hashtext(NEW.asset_id::text));
    
    SELECT hash INTO prev FROM assets.audit_log
    WHERE asset_id = NEW.asset_id ORDER BY id DESC LIMIT 1;
    
    NEW.prev_hash := COALESCE(prev, '');
    NEW.hash := encode(
        digest(COALESCE(prev, '') || NEW.id::text || NEW.action || NEW.created_at::text, 'sha256'),
        'hex'
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_hash
    BEFORE INSERT ON assets.audit_log
    FOR EACH ROW EXECUTE FUNCTION assets.audit_log_set_hash();

-- ===================================================================
-- 审批表
-- ===================================================================
CREATE TABLE assets.approval_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    action      VARCHAR(50) NOT NULL,
    target_id   UUID,
    requestor   UUID NOT NULL REFERENCES assets.users(id),
    approver    UUID REFERENCES assets.users(id),
    reason      TEXT,  -- 审批理由
    status      VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    meta        JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);

-- ===================================================================
-- 种子数据
-- ===================================================================
INSERT INTO assets.organizations (id, name, depth, path) VALUES
    (gen_random_uuid(), 'Demo Corp', 0, 'root');

INSERT INTO assets.asset_types (id, name, category, schema) VALUES
    (gen_random_uuid(), 'Laptop', 'hardware',
     '{"type":"object","properties":{"cpu":{"type":"string"},"ram_gb":{"type":"integer"},"os":{"type":"string"}}}'),
    (gen_random_uuid(), 'Server', 'hardware',
     '{"type":"object","properties":{"cpu_cores":{"type":"integer"},"ram_gb":{"type":"integer"},"disk_tb":{"type":"number"}}}'),
    (gen_random_uuid(), 'Software License', 'license',
     '{"type":"object","properties":{"vendor":{"type":"string"},"seats":{"type":"integer"},"expires":{"type":"string"}}}');
