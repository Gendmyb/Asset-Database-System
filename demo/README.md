# Asset Database System — Demo

> 概念验证 Demo，使用 Python/FastAPI/SQLite 验证架构设计
> 生产环境对应: Go/Gin/PostgreSQL 16/Redis 7

## 快速启动

```bash
pip3 install fastapi uvicorn
cd demo
python3 main.py
# 访问 http://localhost:8080/docs (Swagger UI)
# 健康检查: http://localhost:8080/healthz
```

## 已实现功能

| 功能 | 端点 | 说明 |
|---|---|---|
| 健康检查 | GET /healthz | 存活探针 |
| 就绪检查 | GET /readyz | 数据库可达性 |
| 资产列表 | GET /api/v1/assets | 游标分页 + 搜索 + 过滤 |
| 资产创建 | POST /api/v1/assets | JSON Schema 校验 |
| 资产详情 | GET /api/v1/assets/{id} | 含 version 号 |
| 资产更新 | PUT /api/v1/assets/{id} | 乐观锁 If-Match |
| 软删除 | DELETE /api/v1/assets/{id} | deleted_at |
| 领用 | POST /api/v1/assets/{id}/assign | 悲观锁 |
| 归还 | POST /api/v1/assets/{id}/release | — |
| 审计日志 | GET /api/v1/assets/{id}/history | 不可变 |

## 架构模式对应

| 生产 (Go) | Demo (Python) |
|---|---|
| Gin Router | FastAPI Router |
| pgx/v5 | sqlite3 (标准库) |
| Repository 层 | Repository 类 |
| Service 层 | Service 类 |
| 中间件链 (RequestID→Recovery→Logging→Auth) | FastAPI Middleware |
| 游标分页 | Base64 cursor |
| 乐观锁 (version + If-Match) | 同逻辑 |
| 软删除 (deleted_at) | 同逻辑 |
