# ebook-server 前后端 monorepo（ADR-0009）
#
# Go 代码全部在 backend/（go.mod 在 backend/）；后台前端在 frontend/（Vue3+Vite）,
# 构建时把其产物镜象进 backend/internal/admin/web（go:embed），产出单 exe。

BACKEND := backend
FRONTEND := frontend
GOOS := $(shell go env GOOS)
OUT_BIN := $(if $(filter windows,$(GOOS)),build/ebook-server.exe,build/ebook-server)

# 发版版本号，默认 dev（本地构建）；发版时传 VERSION=v0.0.1。
VERSION ?= dev
# 编译期注入构建元信息到 pkg/version（go run / 未注入时回退占位值）。
LDFLAGS := -X ebook-server/pkg/version.Version=$(VERSION) \
           -X ebook-server/pkg/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
           -X ebook-server/pkg/version.BuildTime=$(shell date -u +%Y%m%d%H%M%S)

.PHONY: build run clean test frontend-build frontend-dev all fmt deps docs linux windows \
	test-coverage test-verbose test-model test-pkg test-service test-handler \
	test-auth test-user test-comment db-init docker docker-run

# 构建单 exe：把前端产物（若存在）镜象进 embed 目录，再 go build 到 build/。
# 未执行 frontend-build 时 web/ 仅含 .gitkeep，产物会是「前端资源缺失」提示页，
# 运行各自可构建。
build:
	mkdir -p build
	rm -rf $(BACKEND)/internal/admin/web/assets
	cp -rf $(FRONTEND)/dist/. $(BACKEND)/internal/admin/web/ 2>/dev/null || true
	cd $(BACKEND) && go build -ldflags "$(LDFLAGS)" -o ../$(OUT_BIN) .

# 前端构建：产物落在 frontend/dist（标准位置）
frontend-build:
	npm --prefix $(FRONTEND) install
	npm --prefix $(FRONTEND) run build

# 一键：先构建前端，再产出内嵌了后台界面的单 exe 到 build/
all: frontend-build build

# 运行（从仓库根执行，config.yaml / ebook.db / logs 均在根目录解析）
run:
	cd $(BACKEND) && go build -ldflags "$(LDFLAGS)" -o ../$(OUT_BIN) .
	./$(OUT_BIN)

# 前端开发服务器（热更新，连后端需自行配置 /admin/api 代理）
frontend-dev:
	npm --prefix $(FRONTEND) run dev

# 重新生成 API 文档（swag init）：handler 注解变动后需重跑，产物入 backend/docs/
docs:
	cd $(BACKEND) && swag init -g main.go --parseDependency --parseInternal

# 清理：删除产物与 embed 镜像，恢复仅 .gitkeep
clean:
	rm -rf build
	rm -rf logs coverage.out coverage.html
	rm -rf $(FRONTEND)/node_modules $(FRONTEND)/dist
	find $(BACKEND)/internal/admin/web -mindepth 1 ! -name '.gitkeep' -delete

# 测试所有
test:
	cd $(BACKEND) && go test ./...

# 测试详细输出
test-verbose:
	cd $(BACKEND) && go test -v ./...

# 测试覆盖率
test-coverage:
	cd $(BACKEND) && go test -cover ./...
	cd $(BACKEND) && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: backend/coverage.html"

test-model:
	cd $(BACKEND) && go test -v ./model/...

test-pkg:
	cd $(BACKEND) && go test -v ./pkg/...

test-service:
	cd $(BACKEND) && go test -v ./service/...

test-handler:
	cd $(BACKEND) && go test -v ./handler/...

test-auth:
	cd $(BACKEND) && go test -v -run TestAuth ./...

test-user:
	cd $(BACKEND) && go test -v -run TestUser ./...

test-comment:
	cd $(BACKEND) && go test -v -run TestComment ./...

# 数据库初始化（项目使用 SQLite + GORM AutoMigrate，无需手动建表）
db-init:
	@echo "项目使用 SQLite，表结构由 GORM AutoMigrate 自动管理"
	@echo "如需参考 MySQL 建表脚本，请查看 sql/init.sql"

# Docker 构建
docker:
	docker build -t ebook-server .

docker-run:
	docker run -p 9090:9090 --env-file .env ebook-server

# 格式化代码
fmt:
	cd $(BACKEND) && go fmt ./...

# 获取依赖
deps:
	cd $(BACKEND) && go mod tidy

# 交叉编译 Linux
linux:
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ebook-server .

# 交叉编译 Windows
windows:
	cd $(BACKEND) && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ebook-server.exe .