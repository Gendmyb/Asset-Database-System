# Asset Database System

IT 资产管理平台 — Go + Gin + PostgreSQL 16 + Redis 7 + React 18 + Grafana

## 快速开始

```bash
# 开发环境 (需要 Docker)
docker-compose up -d

# 仅后端
cd assetserver && go run ./cmd/api-server

# 仅前端
cd web && npm run dev
```

## 项目结构

```
├── docs/               # 架构文档 + Runbook
├── assetserver/        # Go 后端 (Gin + pgx)
│   ├── cmd/api-server/ # API 入口
│   ├── internal/       # 领域/仓库/API/缓存/事件/锁/Agent
│   ├── migrations/     # PostgreSQL Schema
│   ├── deploy/         # Nginx 配置
│   └── grafana/        # 仪表盘 JSON
├── web/                # React 前端 (Vite + TS + Tailwind)
├── demo/               # Python 概念验证
└── docker-compose.yml
```

## 技术栈

| 层级 | 技术 | 说明 |
|---|---|---|
| API | Go 1.23 + Gin | REST API, EdDSA JWT |
| 数据库 | PostgreSQL 16 | SOT, ltree, 分区表 |
| 缓存 | Redis 7 + Sentinel | Cache-Aside, Pub/Sub |
| 前端 | React 18 + TypeScript | Vite, TailwindCSS, Zustand |
| 可视化 | Grafana OSS | 资产面板 |
| 部署 | Docker Compose / K8s | Patroni HA, PgBouncer |

## Phase 完成状态

| Phase | 状态 | 内容 |
|---|---|---|
| 0 | ✅ | Vault/KMS, Ed25519 密钥管理 |
| 1 | ✅ | Foundation: JWT, ltree, 中间件链 |
| 2 | ✅ | Core CRUD + 三层锁策略 |
| 3 | ✅ | Agent 采集器 + 摄入管道 |
| 4 | ✅ | 缓存三防 + 事件总线 + Webhook |
| 5 | ✅ | Dashboard + Locations + Orgs |
| 6 | ✅ | React 前端 (Vite+TS+Zustand) |
| 7 | 🔄 | Agent Polish (进行中) |
| 8 | ✅ | Grafana + Nginx + Docker Compose |
| 9 | 🔄 | Testing (进行中) |
| 10 | ✅ | Runbook + 运维文档 |
| 11 | ✅ | 混沌测试计划 |
