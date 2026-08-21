.PHONY: help build test lint clean fmt vet tidy run

# 项目信息
BINARY_NAME := keeper
VERSION := 0.1.0-dev
GO := /usr/local/go/bin/go
CGO_ENABLED := 0

# 构建标志
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

help: ## 显示帮助信息
	@echo "keeper - AI Agent 轻量级 Linux 运行时环境"
	@echo ""
	@echo "可用目标:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## 构建二进制文件
	@echo "构建 $(BINARY_NAME) $(VERSION)..."
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/keeper
	@echo "构建完成: bin/$(BINARY_NAME)"
	@ls -lh bin/$(BINARY_NAME)

build-static: ## 构建静态二进制文件
	@echo "构建静态二进制..."
	@CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-static ./cmd/keeper
	@echo "构建完成: bin/$(BINARY_NAME)-static"

build-multiarch: ## 多架构构建
	@echo "多架构构建..."
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/keeper
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/keeper
	@echo "多架构构建完成:"
	@ls -lh bin/$(BINARY_NAME)-linux-*

test: ## 运行单元测试
	@echo "运行测试..."
	@$(GO) test -v -race -coverprofile=coverage.out ./...
	@$(GO) tool cover -func=coverage.out | tail -1

test-short: ## 运行短测试（跳过慢速测试）
	@echo "运行短测试..."
	@$(GO) test -short ./...

lint: ## 运行代码检查
	@echo "运行 golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m; \
	else \
		echo "golangci-lint 未安装，跳过"; \
	fi

fmt: ## 格式化代码
	@echo "格式化代码..."
	@$(GO) fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	fi

vet: ## 运行 go vet
	@echo "运行 go vet..."
	@$(GO) vet ./...

tidy: ## 整理依赖
	@echo "整理依赖..."
	@$(GO) mod tidy

deps: ## 下载依赖
	@echo "下载依赖..."
	@$(GO) mod download

run: build ## 构建并运行
	@echo "运行 $(BINARY_NAME)..."
	@./bin/$(BINARY_NAME) $(ARGS)

clean: ## 清理构建产物
	@echo "清理..."
	@rm -rf bin/
	@rm -f coverage.out
	@rm -f *.prof
	@echo "清理完成"

proto: ## 生成协议代码（如有需要）
	@echo "生成协议代码..."
	@echo "无协议定义，跳过"

docker-build: ## 构建 Docker 镜像
	@echo "构建 Docker 镜像..."
	@docker build -t $(BINARY_NAME):$(VERSION) -f deploy/Dockerfile .
	@docker build -t $(BINARY_NAME):latest -f deploy/Dockerfile .

docker-test: ## 在 Docker 中运行测试（模拟 CI 环境）
	@echo "在 Docker 中运行测试..."
	@docker run --rm -v $(PWD):/workspace -w /workspace $(BINARY_NAME):$(VERSION) sh -c "make test lint vet"

ci-local: tidy lint vet test ## 本地 CI 完整测试流程
	@echo "本地 CI 测试完成"

ci-test: tidy lint vet test ## CI 完整测试流程
	@echo "CI 测试完成"

# 默认目标
.DEFAULT_GOAL := help
