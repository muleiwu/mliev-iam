FROM node:22-slim AS builder-web

ENV TZ=Asia/Shanghai
# 非交互构建：关闭 corepack 下载确认提示，避免卡住
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0

# 用最新 corepack（旧版可能缺 pnpm 新版本的签名密钥）并启用，
# 之后 pnpm 会按各项目 package.json 的 packageManager 字段自动选版本：
#   - admin-web      → pnpm@10.14.0
#   - frontend-oidc  → pnpm@10.27.0
RUN npm i -g corepack@latest && corepack enable

WORKDIR /app

# copy package.json and pnpm-lock.yaml to workspace
COPY . /app

# 安装 pnpm 并构建前端项目
#   - admin-web (monorepo)：管理后台 web-antd
#   - frontend-oidc (独立 repo)：OIDC 登录/同意 UI，Vite outDir 直指 ../static/auth
RUN pnpm config set registry https://mirrors.cloud.tencent.com/npm/ && \
    cd /app/admin-web && \
    pnpm install && \
    pnpm build:antd && \
    cd /app/frontend-oidc && \
    pnpm install && \
    pnpm build && \
    ls /app && ls /app/static && ls /app/static/auth


# 构建阶段
FROM golang:1.26-alpine AS builder-go

# 设置工作目录
WORKDIR /app

# 安装必要的包
RUN apk --no-cache add ca-certificates git tzdata

# 设置时区
RUN cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 设置Go环境变量
#ENV GOPROXY=https://goproxy.cn,direct
ENV GO111MODULE=on
ENV CGO_ENABLED=0

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

RUN rm -rf /app/admin-web && ls /app/static

# 下载依赖
# 复制构建产物到 nginx 的默认静态文件目录
COPY --from=builder-web /app/static /app/static

# 下载依赖
RUN ls /app/static

# 构建应用
RUN go build -ldflags="-s -w" -o mliev-iam main.go

# 运行阶段
FROM alpine:latest

# 安装ca证书和时区数据
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
RUN cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder-go /app/mliev-iam .

# 创建日志目录
RUN mkdir -p logs && \
    mkdir -p config


# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/simple || exit 1

# 启动应用
CMD ["./mliev-iam", "start"]