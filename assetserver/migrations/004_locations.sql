-- 004_locations.sql
-- Phase B: 位置管理表
SET search_path TO assets;

CREATE TABLE IF NOT EXISTS assets.locations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id UUID NOT NULL REFERENCES assets.organizations(id),
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50),
    parent_id UUID REFERENCES assets.locations(id),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, name)
);

-- location_id 列已存在于 assets.assets (001_init.sql 第83行), 无需 ALTER TABLE
