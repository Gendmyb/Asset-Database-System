-- 003_drop_unused.sql
-- Phase B: 清理未使用的/实验性表
SET search_path TO assets;

DROP TABLE IF EXISTS assets.asset_snapshots_2026_07 CASCADE;
DROP TABLE IF EXISTS assets.asset_snapshots_2026_08 CASCADE;
DROP TABLE IF EXISTS assets.asset_snapshots CASCADE;
DROP TABLE IF EXISTS assets.enrollment_tokens CASCADE;
DROP TABLE IF EXISTS assets.collection_agents CASCADE;
DROP TABLE IF EXISTS assets.approval_requests CASCADE;
DROP TABLE IF EXISTS assets.archive_manifest CASCADE;
DROP TABLE IF EXISTS assets.audit_meta CASCADE;
DROP TABLE IF EXISTS assets.revoked_tokens CASCADE;
