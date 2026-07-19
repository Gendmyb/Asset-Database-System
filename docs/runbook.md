# 运维 Runbook

适用：Docker Compose 部署的 IT 资产管理系统。

## 日常巡检

```bash
docker compose ps                                 # 服务状态
curl -s http://localhost:8080/healthz             # 健康检查
curl -s http://localhost:8080/readyz              # 就绪检查
docker compose exec postgres psql -U app_user -d assetdb -c "SELECT count(*) FROM pg_stat_activity;"
```

## 备份

```bash
docker compose exec postgres pg_dump -U app_user -d assetdb --schema=assets -Fc \
  -f /tmp/assetdb_$(date +%Y%m%d).dump
docker compose cp postgres:/tmp/assetdb_$(date +%Y%m%d).dump ./backups/
```

## 恢复

```bash
docker compose stop app
docker compose exec -T postgres pg_restore -U app_user -d assetdb \
  --clean --if-exists --schema=assets < ./backups/assetdb_YYYYMMDD.dump
docker compose up -d app   # 启动时自动执行迁移
```

## 镜像升级

```bash
git pull && docker compose up -d --build
```

## 管理员锁死恢复

```bash
docker compose exec postgres psql -U app_user -d assetdb <<'SQL'
UPDATE assets.users SET password_hash = '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
  must_change_password = true WHERE username = 'admin';
SQL
# 用 admin / admin123 登录后改密码
```

## 迁移管理

文件：`assetserver/migrations/001-009*.sql`，app 启动时自动执行。
已执行版本记录在 `assets.schema_migrations`。

## 生产部署前检查清单

- [ ] 设置 `JWT_ED25519_SEED`（hex 32 字节），否则每次重启全员掉线
- [ ] PostgreSQL `max_connections` ≥ 50
- [ ] 配置每日 pg_dump 备份
- [ ] app 前挂反向代理 HTTPS
- [ ] 防火墙限制 5432 端口仅 app 访问
