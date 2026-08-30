# 构建阶段（ADR-0009：Go 代码在 backend/）
FROM golang:1.22-alpine AS builder

WORKDIR /src

# 复制配置与 SQL 参考（位于仓库根）
COPY config.yaml ./config.yaml
COPY sql ./sql

# 复制依赖文件
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# 复制后端源代码（含内嵌前端的 web/ 产物或占位页）
COPY backend/ .

# 构建（CGO_ENABLED=0 产出静态单二进制）
RUN CGO_ENABLED=0 GOOS=linux go build -o ebook-server .

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 安装 ca-certificates（用于 HTTPS）
RUN apk --no-cache add ca-certificates

# 从构建阶段复制二进制文件
COPY --from=builder /src/ebook-server .
COPY --from=builder /src/config.yaml .
COPY --from=builder /src/sql ./sql

# 暴露端口（与 config.yaml 中 server.port 一致）
EXPOSE 9090

# 运行
CMD ["./ebook-server"]