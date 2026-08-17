# syntax=docker/dockerfile:1

# ---- 前端构建 ----
# 用宿主机架构原生执行, 避免多架构构建时在 QEMU 模拟下跑 npm
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
# 复用 npm 缓存, 依赖不变时秒装
RUN --mount=type=cache,target=/root/.npm npm ci || npm install
COPY web/ .
RUN npm run build

# ---- 后端构建 ----
# 同样原生执行, 用交叉编译产出目标架构二进制
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
# GOPROXY 可构建时覆盖: docker build --build-arg GOPROXY=...
ARG GOPROXY=https://goproxy.cn,direct
# TARGETOS/TARGETARCH 由 BuildKit 根据目标平台自动注入
ARG TARGETOS TARGETARCH
ENV GOPROXY=${GOPROXY} CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH}
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# 用 web stage 构建好的前端覆盖本地 web/dist (go:embed 嵌入构建产物)
COPY --from=web /src/web/dist ./web/dist
# 复用 Go 编译缓存, 源码小幅改动时只重编受影响包
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/dashboard ./cmd/dashboard

# ---- 运行时 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/dashboard /app/dashboard
EXPOSE 8080
# 配置目录: 程序按顺序解析配置, 找不到时会在工作目录生成样板
# (工作目录即挂载卷, 生成的 dashboard.yaml 宿主直接可见)
WORKDIR /config
VOLUME ["/config"]
ENTRYPOINT ["/app/dashboard"]
CMD []
