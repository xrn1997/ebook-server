# 构建阶段
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -o ebook-server main.go

# 运行阶段
FROM alpine:latest

WORKDIR /app

# 安装 ca-certificates（用于 HTTPS）
RUN apk --no-cache add ca-certificates

# 从构建阶段复制二进制文件
COPY --from=builder /app/ebook-server .
COPY --from=builder /app/config.yaml .
COPY --from=builder /app/sql ./sql

# 暴露端口（与 config.yaml 中 server.port 一致）
EXPOSE 9090

# 运行
CMD ["./ebook-server"]
