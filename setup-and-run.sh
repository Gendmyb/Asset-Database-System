#!/bin/bash
# Asset Database System — 本地部署脚本
# 用法: bash setup-and-run.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
echo "=== Asset DB System 本地部署 ==="
echo "工作目录: $SCRIPT_DIR"

# ---- Step 1: 安装 PostgreSQL (如需) ----
if ! command -v psql &>/dev/null; then
    echo ""
    echo "[1/4] 安装 PostgreSQL 14..."
    sudo apt-get update -qq
    sudo apt-get install -y -qq postgresql-14 postgresql-client-14
else
    echo "[1/4] PostgreSQL 已安装 ✓"
fi

# ---- Step 2: 启动 PostgreSQL 并创建数据库 ----
echo "[2/4] 配置数据库..."
sudo pg_ctlcluster 14 main start 2>/dev/null || true
sudo systemctl start postgresql 2>/dev/null || true

# 创建用户和数据库 (幂等), app_user 需要 SUPERUSER 以便迁移中创建 role 和 extension
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='app_user'" 2>/dev/null | grep -q 1 || \
    sudo -u postgres psql -c "CREATE USER app_user WITH PASSWORD 'app_pass' SUPERUSER CREATEROLE"
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='assetdb'" 2>/dev/null | grep -q 1 || \
    sudo -u postgres psql -c "CREATE DATABASE assetdb OWNER app_user"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE assetdb TO app_user"
sudo -u postgres psql -d assetdb -c "CREATE SCHEMA IF NOT EXISTS assets AUTHORIZATION app_user"

# 如果 app_user 已存在但不是 SUPERUSER，则升级
sudo -u postgres psql -c "ALTER USER app_user SUPERUSER CREATEROLE" 2>/dev/null || true

echo "  数据库就绪 ✓"

# ---- Step 3: 启动应用 ----
echo "[3/4] 启动 API Server..."

# 生成 JWT seed (如果还没有)
if [ -z "$JWT_ED25519_SEED" ]; then
    JWT_ED25519_SEED=$(openssl rand -hex 32 2>/dev/null || python3 -c "import secrets; print(secrets.token_hex(32))")
    echo "  JWT seed: $JWT_ED25519_SEED"
fi

export SERVER_HOST="0.0.0.0"
export SERVER_PORT="8080"
export DATABASE_URL="postgres://app_user:app_pass@localhost:5432/assetdb?sslmode=disable&search_path=assets"
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_NAME="assetdb"
export DB_USER="app_user"
export DATABASE_PASSWORD="app_pass"
export DEMO="false"
export JWT_ED25519_SEED

# 杀掉旧进程
kill $(pgrep -f "api-server") 2>/dev/null || true
sleep 1

# 启动应用 (后台)
nohup "$SCRIPT_DIR/assetserver/bin/api-server" > /tmp/assetdb-api.log 2>&1 &
APP_PID=$!
echo "  PID: $APP_PID"

# ---- Step 4: 等待就绪 ----
echo "[4/4] 等待服务就绪..."
for i in $(seq 1 30); do
    if curl -s http://localhost:8080/healthz >/dev/null 2>&1; then
        echo ""
        echo "=============================================="
        echo "  ✓ 部署成功！"
        echo "  前端:  http://localhost:8080"
        echo "  API:   http://localhost:8080/api/v1"
        echo "  登录:  admin / admin123"
        echo "  健康:  http://localhost:8080/healthz"
        echo "  日志:  tail -f /tmp/assetdb-api.log"
        echo "=============================================="
        echo ""
        echo "停止服务: kill $APP_PID"
        exit 0
    fi
    sleep 1
done

echo ""
echo "⚠ 服务可能启动较慢，请检查日志:"
echo "  tail -f /tmp/assetdb-api.log"
echo "  curl http://localhost:8080/healthz"

# 检查进程是否还活着
if kill -0 $APP_PID 2>/dev/null; then
    echo "  进程 $APP_PID 运行中"
else
    echo "  ✗ 进程已退出，日志如下:"
    cat /tmp/assetdb-api.log
    exit 1
fi
