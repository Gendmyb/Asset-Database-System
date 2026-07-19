# Asset Database System — Makefile
# 对应架构文档 §4 项目结构

.PHONY: build run dev test clean migrate

# 环境变量
export PATH := $(HOME)/go/bin:$(PATH)
export GOROOT := $(HOME)/go
export GOPATH := $(HOME)/go-workspace

BINARY := api-server
CMD_DIR := cmd/api-server

# 构建
build:
	cd assetserver && go build -o $(BINARY) ./$(CMD_DIR)

# 开发运行 (连接本机 PostgreSQL, 通过 DB_* 环境变量配置; DEMO=true 走内存模式)
run: build
	cd assetserver && ./$(BINARY)

# 开发模式 (热重载, 需要 air)
dev:
	cd assetserver && go run ./$(CMD_DIR)

# 测试
test:
	cd assetserver && go test ./...

# 数据库迁移: 启动时自动执行 (自研 runner, EXCLUSIVE 锁防多实例并发);
# 如需手动预跑, 用 DB_* 单项变量连接 (DATABASE_URL 不被识别)
migrate:
	@echo "Migrations run automatically on server startup."
	@echo "Manual (optional): export DB_HOST/DB_PORT/DB_NAME/DB_USER/DATABASE_PASSWORD,"
	@echo "  then run psql -f assetserver/migrations/NNN_*.sql in filename order."

# 清理
clean:
	rm -f assetserver/$(BINARY)

# 依赖
deps:
	cd assetserver && go mod tidy
