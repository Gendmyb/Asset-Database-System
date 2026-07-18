SET search_path TO assets;

-- 系统设置表（settings_repo 已在查此表，但表不存在）
CREATE TABLE IF NOT EXISTS assets.system_settings (
    key        VARCHAR(100) PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO assets.system_settings (key, value) VALUES
    ('asset_tag_prefix', 'AST-'),
    ('org_name', 'Demo Corp')
ON CONFLICT (key) DO NOTHING;

-- 单据序列（原子取号，替代 COUNT+1 的并发重号问题）
CREATE TABLE IF NOT EXISTS assets.doc_sequences (
    org_id   UUID NOT NULL REFERENCES assets.organizations(id),
    scope    VARCHAR(20) NOT NULL,
    next_seq BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (org_id, scope)
);
INSERT INTO assets.doc_sequences (org_id, scope, next_seq)
    SELECT id, 'asset', 1 FROM assets.organizations
ON CONFLICT (org_id, scope) DO NOTHING;
