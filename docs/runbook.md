# Asset Database System — 运维 Runbook
# Phase 11: 故障转移演练 + 日常运维

## 日常巡检 (每日)

```bash
# 1. 健康检查
curl -s http://asset-db.internal:8080/healthz
curl -s http://asset-db.internal:8080/readyz

# 2. PostgreSQL 复制延迟
psql -c "SELECT client_addr, state, sync_state, 
  pg_wal_lsn_diff(sent_lsn, write_lsn) AS write_lag_bytes,
  pg_wal_lsn_diff(write_lsn, flush_lsn) AS flush_lag_bytes
  FROM pg_stat_replication;"

# 3. Redis Sentinel 状态
redis-cli -p 26379 SENTINEL MASTER mymaster
redis-cli -p 26379 SENTINEL SLAVES mymaster

# 4. Agent 在线率
curl -s -H "Authorization: Bearer $TOKEN" \
  http://asset-db.internal:8080/api/v1/dashboard/agents

# 5. DLQ 积压
sqlite3 /var/lib/agent/offline.db "SELECT COUNT(*) FROM dlq;"
```

## Patroni 故障转移演练 (每月第一个周六 02:00)

```bash
# 1. 记录演练前状态
patronictl -c /etc/patroni/patroni.yml list > /tmp/pre-failover-$(date +%Y%m%d).txt

# 2. 优雅切换
patronictl -c /etc/patroni/patroni.yml switchover --master postgres1 --candidate postgres2

# 3. 验证写入
psql -h postgres2 -c "INSERT INTO assets.audit_log(asset_id,action,hash) VALUES (NULL,'failover_test','test') RETURNING id;"

# 4. 验证 Replica 已升级
patronictl -c /etc/patroni/patroni.yml list
# 确认 postgres2 角色为 Leader, postgres1 为 Replica

# 5. 恢复原状 (可选)
patronictl -c /etc/patroni/patroni.yml switchover --master postgres2 --candidate postgres1

# 6. 发送报告 → Slack #asset-db-ops
echo "Patroni failover drill completed: RTO=$(calc_rto), all checks passed" \
  | curl -X POST -H "Content-Type: application/json" \
    -d @- $SLACK_WEBHOOK_URL
```

## Redis Sentinel 切换测试

```bash
# 1. 手动触发故障转移
redis-cli -p 26379 SENTINEL FAILOVER mymaster

# 2. 验证新 Master
redis-cli -p 26379 SENTINEL MASTER mymaster | grep -E "ip|port"

# 3. 验证 API 熔断行为
# 停止 Redis → curl POST /api/v1/assets → 期望 503 (fail-closed)
# curl GET /api/v1/assets → 期望 200 (fail-open, 读 PG 直连)
```

## 备份恢复验证 (每月第一个周一)

```bash
# 1. 启动临时 PG 实例
docker run -d --name pg-restore-test -e POSTGRES_PASSWORD=test postgres:16

# 2. WAL-G 恢复
wal-g backup-fetch /var/lib/postgresql/data LATEST

# 3. 启动并验证
echo "restore_command = 'wal-g wal-fetch \"%f\" \"%p\"'" >> recovery.conf
pg_ctl start
psql -c "SELECT count(*) FROM assets.assets;"
psql -c "SELECT count(*) FROM assets.audit_log;"

# 4. 链式哈希校验
psql -c "
  SELECT a.id, a.hash, b.hash AS expected_next
  FROM assets.audit_log a
  LEFT JOIN assets.audit_log b ON b.prev_hash = a.hash
  WHERE b.id IS NULL AND a.id < (SELECT MAX(id) FROM assets.audit_log);
"
# 期望: 0 rows (链完整)

# 5. 清理
docker rm -f pg-restore-test
```

## Vault 故障应急

```bash
# Vault 不可达时 (已运行实例不受影响):
# 1. 验证公钥缓存仍有效
curl http://asset-db.internal:8080/readyz
# 期望: {"status": "degraded", "reason": "vault unreachable"}

# 2. 新实例启动应急 (PG 加密备用密钥)
# 自动降级到 PG 中存储的加密 Ed25519 密钥对
# Access token TTL 缩短至 5min

# 3. Vault 恢复后
# 自动切换回 Vault, 恢复 15min TTL
```

## 紧急操作

### 吊销 Agent 证书
```bash
# 1. DB 标记
psql -c "UPDATE assets.collection_agents SET cert_revoked=true WHERE id='<agent_id>';"

# 2. 更新 CRL
openssl ca -revoke /etc/nginx/ca/certs/<agent_id>.crt
openssl ca -gencrl -out /etc/nginx/ca.crl

# 3. Nginx reload
nginx -s reload

# 4. 验证
curl -k --cert revoked.crt --key revoked.key https://asset-db.internal/healthz
# 期望: 400 SSL certificate error
```

### 强制刷新所有用户 token
```bash
psql -c "DELETE FROM assets.refresh_tokens WHERE expires_at > now();"
redis-cli FLUSHALL
```
