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

# 开发运行 (使用 SQLite 内存模式)
run: build
	cd assetserver && ./$(BINARY)

# 开发模式 (热重载, 需要 air)
dev:
	cd assetserver && go run ./$(CMD_DIR)

# 测试
test:
	cd assetserver && go test ./...

# 数据库迁移 (占位; 实际运行需指定 DATABASE_URL)
migrate:
	@echo "Run manually: psql \$$DATABASE_URL -f assetserver/migrations/001_init.sql"
	@echo "                 psql \$$DATABASE_URL -f assetserver/migrations/002_settings_sequences.sql"

# 清理
clean:
	rm -f assetserver/$(BINARY)

# 依赖
deps:
	cd assetserver && go mod tidy
