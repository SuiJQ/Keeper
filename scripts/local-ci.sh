#!/bin/bash
set -euo pipefail

echo "=========================================="
echo "Keeper 本地 CI 测试"
echo "=========================================="
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 设置 Go 路径
export PATH=/usr/local/go/bin:/root/go/bin:$PATH

FAILED=0

# 1. 环境检查
echo -e "${YELLOW}[1/6] 环境检查${NC}"
go version
if command -v golangci-lint &> /dev/null; then
    golangci-lint version
else
    echo -e "${RED}警告: golangci-lint 未安装${NC}"
fi
if command -v staticcheck &> /dev/null; then
    staticcheck --version
else
    echo -e "${RED}警告: staticcheck 未安装${NC}"
fi
echo ""

# 2. 依赖整理
echo -e "${YELLOW}[2/6] 整理依赖${NC}"
if ! go mod tidy; then
    echo -e "${RED}依赖整理失败${NC}"
    FAILED=1
fi
echo ""

# 3. 代码格式化检查
echo -e "${YELLOW}[3/6] 代码格式化检查${NC}"
UNFORMATTED=$(gofmt -d .)
if [ -n "$UNFORMATTED" ]; then
    echo -e "${RED}代码格式不符合规范:${NC}"
    echo "$UNFORMATTED"
    FAILED=1
else
    echo -e "${GREEN}代码格式检查通过${NC}"
fi
echo ""

# 4. Vet
echo -e "${YELLOW}[4/6] 运行 go vet${NC}"
if ! go vet ./...; then
    echo -e "${RED}go vet 失败${NC}"
    FAILED=1
else
    echo -e "${GREEN}go vet 通过${NC}"
fi
echo ""

# 5. Lint
echo -e "${YELLOW}[5/6] 运行 Linter${NC}"
if command -v staticcheck &> /dev/null; then
    if ! staticcheck ./...; then
        echo -e "${RED}staticcheck 失败${NC}"
        FAILED=1
    else
        echo -e "${GREEN}staticcheck 通过${NC}"
    fi
fi

if command -v golangci-lint &> /dev/null; then
    if ! golangci-lint run --timeout 5m; then
        echo -e "${RED}golangci-lint 失败${NC}"
        FAILED=1
    else
        echo -e "${GREEN}golangci-lint 通过${NC}"
    fi
fi
echo ""

# 6. 测试
echo -e "${YELLOW}[6/6] 运行测试${NC}"
if ! go test -v -race ./...; then
    echo -e "${RED}测试失败${NC}"
    FAILED=1
else
    echo -e "${GREEN}测试通过${NC}"
fi
echo ""

# 总结
echo "=========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ 本地 CI 全部通过${NC}"
    echo "=========================================="
    exit 0
else
    echo -e "${RED}✗ 本地 CI 失败，请检查上述错误${NC}"
    echo "=========================================="
    exit 1
fi
