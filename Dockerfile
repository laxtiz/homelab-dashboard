# ---- 前端构建 ----
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ .
RUN npm run build

# ---- 后端构建 ----
FROM golang:1.26-alpine AS build
WORKDIR /src
# GOPROXY 可构建时覆盖: docker build --build-arg GOPROXY=...
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 用 web stage 构建好的前端覆盖本地 web/dist (go:embed 嵌入构建产物)
COPY --from=web /src/web/dist ./web/dist
RUN go build -o /out/dashboard ./cmd/dashboard

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
