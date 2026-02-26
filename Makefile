# OctopusClaw - 快速启动、编译、构建镜像
# 用法: make [target]

.PHONY: help run run-all run-ui build build-backend build-frontend docker-build clean test

# 默认目标
help:
	@echo "OctopusClaw Makefile"
	@echo ""
	@echo "  快速启动:"
	@echo "    make run           - 构建后端并启动 Gateway（端口 18789，托管前端）"
	@echo "    make run-all       - 一键启动前后端：构建 + 启动 Gateway + 启动前端开发服务器（5173）"
	@echo "    make run-ui        - 仅启动前端开发服务器（端口 5173）"
	@echo ""
	@echo "  编译:"
	@echo "    make build         - 编译后端 + 构建前端（产出: src/openclaw, dist/control-ui）"
	@echo "    make build-backend - 仅编译 Go 后端"
	@echo "    make build-frontend- 仅构建 Control UI 前端"
	@echo ""
	@echo "  镜像:"
	@echo "    make docker-build  - 构建包含前后端的 Docker 镜像"
	@echo "    make docker-build VERSION=x.y.z  - 指定镜像版本（默认从 git describe）"
	@echo ""
	@echo "  其他:"
	@echo "    make test          - 运行后端与前端测试"
	@echo "    make clean         - 清理构建产物"

# 编译后端（产出到 src/openclaw）
build-backend:
	cd src && go build -o openclaw ./cmd/openclaw

# 构建前端（产出到 dist/control-ui）
build-frontend:
	cd ui && pnpm install && pnpm build

# 同时编译后端与前端
build: build-backend build-frontend

# 启动 Gateway：先 build，再运行（托管 dist/control-ui）
run: build-backend
	cd src && ./openclaw gateway run

# 一键启动前后端：构建 + 启动 Gateway + 启动前端开发服务器
run-all: build
	@echo ">>> 启动 Gateway（端口 18789）..."
	@(cd src && ./openclaw gateway run) & B=$$!; \
	(cd ui && pnpm dev); \
	kill $$B 2>/dev/null || true

# 仅启动前端开发服务器（需后端 Gateway 另行运行）
run-ui:
	cd ui && pnpm dev

# 构建 Docker 镜像（前后端一体）；VERSION 默认从 git tag 或 0.0.0-dev
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "0.0.0-dev")
DOCKER_IMAGE ?= openocta/openocta
docker-build: build
	docker build -f deploy/Dockerfile -t $(DOCKER_IMAGE):$(VERSION) -t $(DOCKER_IMAGE):latest .

# 运行测试
test:
	cd src && go test ./...
	cd ui && pnpm test

# 清理构建产物
clean:
	rm -f src/openclaw
	rm -rf dist/control-ui
	rm -rf ui/node_modules ui/dist
