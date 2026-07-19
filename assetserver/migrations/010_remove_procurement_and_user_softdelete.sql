-- 010_remove_procurement_and_user_softdelete.sql
-- 1. 移除 lifecycle_state 的 'procurement' (采购中) 状态
--    资产入库即进入 deployment; 历史 procurement 数据迁移到 deployment。
-- 2. 用户软删除 (保留记录): 新增 deleted_at, 删除仅置位, 行保留以维系审计/领用历史。

-- 1.1 资产 lifecycle_state: 回填历史 procurement -> deployment (含软删除行, 否则新 CHECK 会报错)
UPDATE assets.assets SET lifecycle_state = 'deployment'
 WHERE lifecycle_state = 'procurement';

-- 1.2 收紧 CHECK (移除 procurement) + 调整默认值为 deployment
ALTER TABLE assets.assets DROP CONSTRAINT IF EXISTS assets_lifecycle_state_check;
ALTER TABLE assets.assets ADD CONSTRAINT assets_lifecycle_state_check
    CHECK (lifecycle_state IN ('deployment','utilization','maintenance','retirement'));
ALTER TABLE assets.assets ALTER COLUMN lifecycle_state SET DEFAULT 'deployment';

-- 2. 用户软删除: 新增 deleted_at 列 + 索引
ALTER TABLE assets.users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_users_org_active
    ON assets.users(org_id) WHERE deleted_at IS NULL;
