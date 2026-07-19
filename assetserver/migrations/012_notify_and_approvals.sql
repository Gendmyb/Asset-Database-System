-- 012_notify_and_approvals.sql
-- Wave 2 G6 通知渠道 + G7 多级审批流
SET search_path TO assets;

-- ============================================================
-- G7: 审批请求 (重建 — 003_drop_unused.sql 曾 DROP 过旧表)
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.approval_requests (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resource_type  VARCHAR(32) NOT NULL
                   CHECK (resource_type IN ('assignment','retirement','maintenance')),
    resource_id    VARCHAR(64) NOT NULL,  -- 业务实体标识 (如 asset_id)
    requester_id   UUID,                   -- 发起人 (manager)
    org_id         UUID NOT NULL REFERENCES assets.organizations(id),
    status         VARCHAR(16) NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','approved','rejected','canceled')),
    current_step   INTEGER NOT NULL DEFAULT 1,  -- 预留多级, 当前恒为 1
    payload        JSONB,                  -- 原始请求载荷, 审批通过后回放
    reason         TEXT,                   -- 拒绝理由 / 审批备注
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at     TIMESTAMPTZ,
    decided_by     UUID
);

CREATE INDEX IF NOT EXISTS idx_approval_requests_org_status
    ON assets.approval_requests(org_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_approval_requests_resource
    ON assets.approval_requests(resource_type, resource_id);

-- ============================================================
-- G6: 通知规则 (系统级: 事件 → 渠道映射, admin 配置)
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.notify_rules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id      UUID REFERENCES assets.organizations(id),  -- NULL = 全局规则
    event_type  VARCHAR(100) NOT NULL,                     -- '*' = 全部事件
    channel     VARCHAR(32) NOT NULL
                CHECK (channel IN ('email','dingtalk','wecom','feishu')),
    target      VARCHAR(512),  -- email: 收件人(逗号分隔); 机器人: 留空用全局 webhook
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notify_rules_org_active
    ON assets.notify_rules(org_id, active);

-- ============================================================
-- G6: 通知投递记录 (便于排查失败)
-- ============================================================
CREATE TABLE IF NOT EXISTS assets.notify_deliveries (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    rule_id       UUID REFERENCES assets.notify_rules(id) ON DELETE CASCADE,
    org_id        UUID REFERENCES assets.organizations(id),  -- 投递所属组织 (全局规则对应 rule.org_id 为 NULL, 此处亦 NULL)
    event_type    VARCHAR(100) NOT NULL,
    channel       VARCHAR(32) NOT NULL,
    target        VARCHAR(512),
    status        VARCHAR(20) NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','success','failed','retrying')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notify_deliveries_status
    ON assets.notify_deliveries(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notify_deliveries_org
    ON assets.notify_deliveries(org_id, created_at DESC);
