-- 014_ad_enterprise.sql
-- AD 域控接入企业化增强 (Wave 3 T0 契约冻结)
--
-- 1. 新建 ad_group_mappings 表: 安全组 DN → 角色 + 数据范围映射
-- 2. users 表新增 data_scope 列 (个人只读 / 继承组映射)
-- 3. users 表新增 manual_override 列 (防止 AD 同步覆盖超管手动调整)
--
-- 安全基线:
--   - 全部新增列/新增表、默认关闭/安全, 向后兼容
--   - 未启用 LDAP 时系统行为与现状完全一致
--   - 无凭据入库
-- 幂等: 全部 IF NOT EXISTS / ADD COLUMN IF NOT EXISTS。

SET search_path TO assets;

-- ============================================================
-- 1. 安全组 → 角色映射表
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.ad_group_mappings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_dn    VARCHAR(512) UNIQUE NOT NULL,          -- AD 组 distinguishedName (唯一)
    group_name  VARCHAR(255),                          -- 显示名 (sAMAccountName / CN)
    role        VARCHAR(50) NOT NULL DEFAULT 'viewer'
                CHECK (role IN ('super_admin','admin','manager','viewer')),
    data_scope  VARCHAR(20) NOT NULL DEFAULT 'inherit'
                CHECK (data_scope IN ('inherit','self')),
    sync_enabled BOOLEAN NOT NULL DEFAULT true,         -- false 时该组成员不参与同步
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 索引: 快速查所有启用的映射
CREATE INDEX IF NOT EXISTS idx_ad_group_mappings_enabled
    ON assets.ad_group_mappings(sync_enabled) WHERE sync_enabled = true;

-- ============================================================
-- 2. users 表新增两列 (默认安全, 向后兼容)
-- ============================================================

-- data_scope: 个人数据可见范围
--   'inherit' = 沿用组的 data_scope 或系统默认 (历史行为, 向后兼容)
--   'self'    = 仅见分配给自己的资产
ALTER TABLE assets.users
    ADD COLUMN IF NOT EXISTS data_scope VARCHAR(20) NOT NULL DEFAULT 'inherit'
        CHECK (data_scope IN ('inherit','self'));

-- manual_override: admin 手动标记, 防止 AD 同步覆盖
--   false (默认) = AD 同步可更新此用户的 role/status/scope
--   true  = AD 同步跳过 role/status/scope 字段 (仍刷新 display_name/dn/email)
ALTER TABLE assets.users
    ADD COLUMN IF NOT EXISTS manual_override BOOLEAN NOT NULL DEFAULT false;

-- 索引: 快速查所有被手动覆盖的用户 (运维审计用)
CREATE INDEX IF NOT EXISTS idx_users_manual_override
    ON assets.users(manual_override) WHERE manual_override = true;

-- 注释 (可观测性; 非功能约束)
COMMENT ON TABLE assets.ad_group_mappings IS 'AD/LDAP 安全组到系统角色的映射。按组成员同步时, 用户角色取所有命中组中的最高角色。';
COMMENT ON COLUMN assets.users.data_scope IS '个人数据可见范围: inherit=沿用组/系统默认, self=仅见分配给自己的资产';
COMMENT ON COLUMN assets.users.manual_override IS 'admin 手动标记: true 时 AD 同步不覆盖此用户的 role/status/scope';
