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

# 数据库迁移
migrate:
	cd assetserver && psql $(DATABASE_URL) -f migrations/000001_init_schema.sql

# Docker 构建
docker-build:
	docker build -t asset-db-api -f Dockerfile .

# 清理
clean:
	rm -f assetserver/$(BINARY)

# 依赖
deps:
	cd assetserver && go mod tidy

# === Phase 7: Agent 交叉编译 (linux/darwin/windows × amd64/arm64) ===

AGENT_SRC := ./cmd/collection-agent
OUT_DIR := build

agent-linux:
	cd assetserver && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(OUT_DIR)/agent-linux-amd64 $(AGENT_SRC)

agent-darwin-amd64:
	cd assetserver && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(OUT_DIR)/agent-darwin-amd64 $(AGENT_SRC)

agent-darwin-arm64:
	cd assetserver && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(OUT_DIR)/agent-darwin-arm64 $(AGENT_SRC)

agent-windows:
	cd assetserver && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(OUT_DIR)/agent-windows-amd64.exe $(AGENT_SRC)

build-agent-all: agent-linux agent-darwin-amd64 agent-darwin-arm64 agent-windows
	@ls -lh assetserver/$(OUT_DIR)/

build-all: build build-agent-all
	@echo "All binaries built"
