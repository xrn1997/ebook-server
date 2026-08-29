.PHONY: build run clean test test-coverage test-verbose docker

# 构建
build:
	go build -o ebook-server main.go

# 运行
run:
	go run main.go

# 清理
clean:
	rm -f ebook-server
	rm -rf logs
	rm -f coverage.out coverage.html

# 测试所有
test:
	go test ./...

# 测试详细输出
test-verbose:
	go test -v ./...

# 测试覆盖率
test-coverage:
	go test -cover ./...
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 运行指定包的测试
test-model:
	go test -v ./model/...

test-pkg:
	go test -v ./pkg/...

test-service:
	go test -v ./service/...

test-handler:
	go test -v ./handler/...

# 运行指定测试
test-auth:
	go test -v -run TestAuth ./...

test-user:
	go test -v -run TestUser ./...

test-comment:
	go test -v -run TestComment ./...

# 数据库初始化（项目使用 SQLite + GORM AutoMigrate，无需手动建表）
# 以下命令仅供 MySQL 迁移参考，实际开发直接 go run main.go 即可
db-init:
	@echo "项目使用 SQLite，表结构由 GORM AutoMigrate 自动管理"
	@echo "如需参考 MySQL 建表脚本，请查看 sql/init.sql"

# Docker 构建
docker:
	docker build -t ebook-server .

# Docker 运行
docker-run:
	docker run -p 9090:9090 --env-file .env ebook-server

# 格式化代码
fmt:
	go fmt ./...

# 获取依赖
deps:
	go mod tidy

# 交叉编译 Linux
linux:
	GOOS=linux GOARCH=amd64 go build -o ebook-server main.go

# 交叉编译 Windows
windows:
	GOOS=windows GOARCH=amd64 go build -o ebook-server.exe main.go
