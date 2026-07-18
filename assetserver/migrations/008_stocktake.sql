-- 008_stocktake.sql
-- Phase G: 盘点管理 (Stocktake)
SET search_path TO assets;

-- ============================================================
-- 1. 盘点计划表
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.stocktake_plans (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plan_no           VARCHAR(50) NOT NULL UNIQUE,
    org_id            UUID NOT NULL REFERENCES assets.organizations(id),
    name              VARCHAR(255) NOT NULL,
    scope_location_id UUID REFERENCES assets.locations(id),
    scope_type_id     UUID REFERENCES assets.asset_types(id),
    status            VARCHAR(20) NOT NULL DEFAULT 'draft'
                      CHECK (status IN ('draft','in_progress','completed','canceled')),
    created_by        UUID NOT NULL REFERENCES assets.users(id),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_stocktake_plans_org
    ON assets.stocktake_plans(org_id, status, created_at DESC);

-- updated_at trigger
DROP TRIGGER IF EXISTS trg_stp_updated_at ON assets.stocktake_plans;
CREATE TRIGGER trg_stp_updated_at
    BEFORE UPDATE ON assets.stocktake_plans
    FOR EACH ROW EXECUTE FUNCTION assets.update_updated_at();

-- ============================================================
-- 2. 盘点明细表
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.stocktake_items (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plan_id              UUID NOT NULL REFERENCES assets.stocktake_plans(id) ON DELETE CASCADE,
    asset_id             UUID REFERENCES assets.assets(id),
    expected_location_id UUID REFERENCES assets.locations(id),
    expected_status      VARCHAR(50),
    result               VARCHAR(20) NOT NULL DEFAULT 'pending'
                         CHECK (result IN ('pending','found','missing','moved','surplus')),
    actual_location_id   UUID REFERENCES assets.locations(id),
    surplus_note         TEXT,
    checked_by           UUID REFERENCES assets.users(id),
    checked_at           TIMESTAMPTZ,
    notes                TEXT,
    UNIQUE(plan_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_stocktake_items_plan
    ON assets.stocktake_items(plan_id, result);

CREATE INDEX IF NOT EXISTS idx_stocktake_items_asset
    ON assets.stocktake_items(asset_id);
