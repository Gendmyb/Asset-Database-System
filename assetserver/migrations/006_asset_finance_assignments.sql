-- 006_asset_finance_assignments.sql
-- Phase E: 资产采购/折旧字段 + 领用/借用一体化 + borrowed 状态
SET search_path TO assets;

-- ============================================================
-- 1. 资产采购/折旧/报废字段
-- ============================================================
ALTER TABLE assets.assets
    ADD COLUMN IF NOT EXISTS purchase_price      NUMERIC(12,2),
    ADD COLUMN IF NOT EXISTS purchase_date       DATE,
    ADD COLUMN IF NOT EXISTS supplier            VARCHAR(255),
    ADD COLUMN IF NOT EXISTS warranty_until      DATE,
    ADD COLUMN IF NOT EXISTS depreciation_method VARCHAR(20) NOT NULL DEFAULT 'none'
               CHECK (depreciation_method IN ('none','straight_line')),
    ADD COLUMN IF NOT EXISTS useful_life_months  INTEGER,
    ADD COLUMN IF NOT EXISTS salvage_value       NUMERIC(12,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS managed_by          UUID REFERENCES assets.users(id),
    ADD COLUMN IF NOT EXISTS retired_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS retire_reason       TEXT;

-- ============================================================
-- 2. 更新 status CHECK 加入 'borrowed'
-- ============================================================
DO $$ BEGIN
    ALTER TABLE assets.assets DROP CONSTRAINT IF EXISTS assets_status_check;
    ALTER TABLE assets.assets ADD CONSTRAINT assets_status_check
        CHECK (status IN ('available','assigned','borrowed','maintenance','retired'));
EXCEPTION WHEN undefined_object THEN NULL; END $$;

-- ============================================================
-- 3. 领用/借用一体化
-- ============================================================
ALTER TABLE assets.assignments
    ADD COLUMN IF NOT EXISTS assignment_type VARCHAR(20) NOT NULL DEFAULT 'permanent'
               CHECK (assignment_type IN ('permanent','temporary')),
    ADD COLUMN IF NOT EXISTS due_date     DATE,
    ADD COLUMN IF NOT EXISTS return_notes TEXT;

-- 4. 临时借用必须有应还日期
DO $$ BEGIN
    ALTER TABLE assets.assignments DROP CONSTRAINT IF EXISTS chk_temporary_due;
    ALTER TABLE assets.assignments ADD CONSTRAINT chk_temporary_due
        CHECK (assignment_type = 'permanent' OR due_date IS NOT NULL);
EXCEPTION WHEN undefined_object THEN NULL; END $$;

-- ============================================================
-- 5. 逾期查询索引
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_assignments_due ON assets.assignments(due_date)
    WHERE status = 'active' AND assignment_type = 'temporary';
