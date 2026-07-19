-- 013_asset_parent_and_data_scope.sql
-- Wave 2 技术-B:
--   G8  资产关系/外设挂载 — assets 表新增 parent_asset_id (自引用, ON DELETE SET NULL)
--   G9  部门级行级数据权限 — 无 schema 变更 (基于 organizations.path ltree 在查询期匹配)
--       本迁移仅为可观测性加一个注释列; 行级权限由应用层 OrgScope + SQL 子查询实现。
-- 幂等: 全部 IF NOT EXISTS。

SET search_path TO assets;

-- ============================================================
-- G8: 资产自引用父子关系 (外设挂载)
-- ============================================================
ALTER TABLE assets.assets
    ADD COLUMN IF NOT EXISTS parent_asset_id UUID;

-- 自引用外键: 父资产被删除时, 子资产的 parent_asset_id 置空 (不级联删除子资产)
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_assets_parent_asset'
          AND conrelid = 'assets.assets'::regclass
    ) THEN
        ALTER TABLE assets.assets
            ADD CONSTRAINT fk_assets_parent_asset
            FOREIGN KEY (parent_asset_id) REFERENCES assets.assets(id)
            ON DELETE SET NULL;
    END IF;
END $$;

-- 查询某资产的直接子资产 (外设列表) 常用索引
CREATE INDEX IF NOT EXISTS idx_assets_parent
    ON assets.assets(parent_asset_id) WHERE parent_asset_id IS NOT NULL AND deleted_at IS NULL;

-- 注: 防止循环引用 (parent 不能是自身或其后代) 由应用层在写入前校验,
-- 不在 DB 加 CHECK (递归校验难以用约束表达)。
