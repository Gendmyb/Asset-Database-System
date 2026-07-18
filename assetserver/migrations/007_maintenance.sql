-- 007_maintenance.sql
-- Phase F: 维修/保养工单 + 报废
SET search_path TO assets;

-- ============================================================
-- 1. 维修/保养工单表
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.maintenance_orders (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_no    VARCHAR(50) NOT NULL UNIQUE,
    asset_id    UUID NOT NULL REFERENCES assets.assets(id),
    org_id      UUID NOT NULL REFERENCES assets.organizations(id),
    category    VARCHAR(20) NOT NULL CHECK (category IN ('repair','upkeep')),
    status      VARCHAR(20) NOT NULL DEFAULT 'open'
                CHECK (status IN ('open','in_progress','completed','canceled')),
    title       VARCHAR(255) NOT NULL,
    description TEXT,
    reported_by UUID NOT NULL REFERENCES assets.users(id),
    assignee    UUID REFERENCES assets.users(id),
    vendor      VARCHAR(255),
    cost        NUMERIC(12,2),
    resolution  TEXT,
    prev_status VARCHAR(50) NOT NULL,
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    version     INTEGER NOT NULL DEFAULT 1
);

-- 每资产同时只能有一张活跃工单
CREATE UNIQUE INDEX IF NOT EXISTS idx_active_maintenance
    ON assets.maintenance_orders(asset_id)
    WHERE status IN ('open','in_progress');

CREATE INDEX IF NOT EXISTS idx_maintenance_org ON assets.maintenance_orders(org_id, status, created_at DESC);

-- updated_at 触发器
DROP TRIGGER IF EXISTS trg_mo_updated_at ON assets.maintenance_orders;
CREATE TRIGGER trg_mo_updated_at
    BEFORE UPDATE ON assets.maintenance_orders
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();
