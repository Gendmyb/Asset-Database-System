-- 011_ldap_and_user_import.sql
-- Wave 1 G1/G2: 支持 AD/LDAP 用户同步与批量导入
-- 1. users 表新增 source / external_id / display_name / dn 列
--    - source: 'local' | 'ldap' (默认 'local', 现有用户保持本地)
--    - external_id: 外部目录唯一标识 (LDAP 用户为 sAMAccountName 或 DN; 本地用户为 NULL)
--    - display_name: 显示名 (AD 同步的 displayName / CSV 导入的 display_name)
--    - dn: LDAP 用户的完整 DN (便于 bind 校验时直接复用)
-- 2. 唯一索引 (source, external_id) — 同一目录内不重复
-- 3. username 仍保持 UNIQUE (登录名), 软删除用户重命名以允许后续重建

SET search_path TO assets;

ALTER TABLE assets.users
    ADD COLUMN IF NOT EXISTS source       VARCHAR(20) NOT NULL DEFAULT 'local'
        CHECK (source IN ('local','ldap')),
    ADD COLUMN IF NOT EXISTS external_id  VARCHAR(255),
    ADD COLUMN IF NOT EXISTS display_name VARCHAR(255),
    ADD COLUMN IF NOT EXISTS dn           VARCHAR(512);

-- 同一目录内 external_id 唯一; 允许 NULL (本地用户)
CREATE UNIQUE INDEX IF NOT EXISTS uq_users_source_external
    ON assets.users(source, external_id) WHERE external_id IS NOT NULL;

-- 同步查询常用: 按 source 列活跃用户
CREATE INDEX IF NOT EXISTS idx_users_source_active
    ON assets.users(source) WHERE deleted_at IS NULL;
