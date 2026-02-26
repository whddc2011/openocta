# OctopusClaw 前后端一体镜像
# 多阶段：1) 构建 Control UI  2) 构建 Go 二进制  3) 最终镜像

# ------------------------------------------------------------------------------
# Stage 1: 构建 Control UI（前端）
# ------------------------------------------------------------------------------
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui

COPY ui/package.json ui/pnpm-lock.yaml* ./
RUN corepack enable pnpm && pnpm install --frozen-lockfile || pnpm install

COPY ui/ ./
RUN pnpm build

# ------------------------------------------------------------------------------
# Stage 2: 构建 Go 二进制（后端）
# ------------------------------------------------------------------------------
FROM golang:1.24-alpine AS go-builder
WORKDIR /app/src

RUN apk add --no-cache git ca-certificates

COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/ ./
ARG VERSION=0.0.1-dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X github.com/openclaw/openclaw/pkg/version.Version=${VERSION}" \
    -o /openocta ./cmd/openocta

# ------------------------------------------------------------------------------
# Stage 3: 最终运行镜像（二进制 + 静态资源）
# ------------------------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 从 Stage 1 复制 Control UI 构建产物（vite 输出到 ../dist/control-ui 即 /app/dist/control-ui）
COPY --from=ui-builder /app/dist/control-ui /app/control-ui
# 从 Stage 2 复制二进制
COPY --from=go-builder /openclaw /usr/local/bin/openclaw

# 状态目录（可挂载卷）
ENV OPENCLAW_STATE_DIR=/var/lib/openocta
# 可选：若 Gateway 支持 OPENCLAW_WEB_ROOT，可设为 /app/control-ui
ENV OPENCLAW_WEB_ROOT=/app/control-ui

EXPOSE 18789

# 默认启动 Gateway；可通过 CMD 覆盖
ENTRYPOINT ["openocta"]
CMD ["gateway", "run", "--port", "18789"]
