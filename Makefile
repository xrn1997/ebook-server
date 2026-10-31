# ebook-server 前后端 monorepo（ADR-0009）
#
# Go 代码全部在 backend/（go.mod 在 backend/）；后台前端在 frontend/（Vue3+Vite）,
# 构建时把其产物镜象进 backend/internal/admin/web（go:embed），产出单 exe。

BACKEND := backend
FRONTEND := frontend
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
OUT_BIN := $(if $(filter windows,$(GOOS)),build/ebook-server.exe,build/ebook-server)

# 发版版本号，默认 dev（本地构建）；发版时传 VERSION=v0.0.1。
VERSION ?= dev
# BuildTime 默认取 HEAD 的提交时间（UTC），同一 commit 重复构建字节稳定；
# 发版构建不应再注入当前时间，否则每次构建的 hash 都不同。可用 BUILD_TIME=... 覆盖。
BUILD_TIME ?= $(shell TZ=UTC git log -1 --format=%cd --date=format-local:%Y%m%d%H%M%S 2>/dev/null || echo unknown)
# 编译期注入构建元信息到 pkg/version（go run / 未注入时回退占位值）。
LDFLAGS := -X ebook-server/pkg/version.Version=$(VERSION) \
           -X ebook-server/pkg/version.Commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown) \
           -X ebook-server/pkg/version.BuildTime=$(BUILD_TIME)

# Windows PE VERSIONINFO 由 windres 编译；MinGW-w64 自带。
# WINRES_GOOS/GOARCH 默认跟宿主一致；交叉构建 Windows 时由 windows 目标覆盖。
WINDRES ?= windres
WINRES_SCRIPT := $(BACKEND)/winres/generate.sh
WINRES_GOOS ?= $(GOOS)
WINRES_GOARCH ?= $(GOARCH)

.PHONY: build run clean test frontend-build frontend-embed frontend-dev frontend-test all fmt deps docs linux windows \
	test-coverage test-verbose test-model test-pkg test-service test-handler test-race lint \
	test-auth test-user test-comment db-init backend-winres

# 把前端产物（若存在）镜象进 embed 目录。未执行 frontend-build 时 web/ 仅含
# .gitkeep，最终产物会是「前端资源缺失」提示页；需要完整后台界面时先构建前端。
frontend-embed:
	rm -rf $(BACKEND)/internal/admin/web/assets
	cp -rf $(FRONTEND)/dist/. $(BACKEND)/internal/admin/web/ 2>/dev/null || true

# 构建单 exe：先镜象已有的前端产物，再 go build 到 build/。
build: backend-winres frontend-embed
	mkdir -p build
	cd $(BACKEND) && go build -trimpath -ldflags "$(LDFLAGS)" -o ../$(OUT_BIN) .

# 前端构建：产物落在 frontend/dist（标准位置）。已有 node_modules 时直接构建；
# 全新环境用 npm ci，保证按 package-lock.json 安装且不改写锁文件。
frontend-build:
	cd $(FRONTEND) && (test -d node_modules || npm ci) && npm run build

# 一键：先构建前端，再产出内嵌了后台界面的单 exe 到 build/
all: frontend-build build

# 运行（从仓库根执行，config.yaml / ebook.db / logs 均在根目录解析）
run:
	mkdir -p build
	cd $(BACKEND) && go build -trimpath -ldflags "$(LDFLAGS)" -o ../$(OUT_BIN) .
	./$(OUT_BIN)

# 前端开发服务器（热更新，/admin/api 与 /api-docs 的代理见 frontend/vite.config.ts）
frontend-dev:
	npm --prefix $(FRONTEND) run dev

# 前端门禁：类型检查 + vitest。已有 node_modules 时直接跑；全新环境用 npm ci。
frontend-test:
	cd $(FRONTEND) && (test -d node_modules || npm ci) && npm run typecheck && npm test

# 重新生成 API 文档（swag init）：handler 注解变动后需重跑，产物入 backend/docs/
docs:
	@command -v swag >/dev/null 2>&1 || { echo "swag 未安装：go install github.com/swaggo/swag/cmd/swag@latest"; exit 1; }
	cd $(BACKEND) && swag init -g main.go --parseDependency --parseInternal

# 清理：删除产物与 embed 镜像，恢复仅 .gitkeep
clean:
	rm -rf build
	rm -rf logs coverage.out coverage.html
	rm -rf $(FRONTEND)/node_modules $(FRONTEND)/dist
	rm -f $(BACKEND)/rsrc_windows_*.syso $(BACKEND)/winres/version.h
	rm -f $(BACKEND)/ebook-server $(BACKEND)/ebook-server.exe
	find $(BACKEND)/internal/admin/web -mindepth 1 ! -name '.gitkeep' -delete

# 测试并发放量上限：go test 默认并行度 = CPU 核数，race/覆盖率插桩的测试二进制每个
# 实占内存可达数百 MB，多核机器上「内存 + 临时目录写盘」会一起冲顶，是整机卡死的
# 主要来源。按机器放量：TEST_JOBS=8 make test-race。
TEST_JOBS ?= 4
GO_TEST_FLAGS := -p $(TEST_JOBS) -parallel $(TEST_JOBS)

# 测试所有
test:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) ./...

# 测试详细输出
test-verbose:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v ./...

# 测试覆盖率
test-coverage:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -cover ./...
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: backend/coverage.html"

# 竞态与静态检查门禁（提交前跑；CI 未配置时由本地承担）
# GOTMPDIR 指向工作区：Windows 上部分环境会拒绝执行 %TEMP% 下的 race 测试二进制。
test-race:
	@mkdir -p .tmp
	cd $(BACKEND) && GOTMPDIR=$(CURDIR)/.tmp go test $(GO_TEST_FLAGS) -race ./...
	@rm -rf .tmp

lint:
	cd $(BACKEND) && go vet ./...

test-model:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v ./model/...

test-pkg:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v ./pkg/...

test-service:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v ./service/...

test-handler:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v ./handler/...

test-auth:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v -run TestAuth ./...

test-user:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v -run TestUser ./...

test-comment:
	cd $(BACKEND) && go test $(GO_TEST_FLAGS) -v -run TestComment ./...

# 数据库初始化说明（生产建表走启动时的版本化迁移，见 docs/development.md）
db-init:
	@echo "项目使用 SQLite；生产启动时由 backend/pkg/database 的 Migrate 执行版本化迁移"
	@echo "新增 schema 变更请在 backend/migrations/ 添加迁移文件，不要手动建表"
	@echo "如需参考 MySQL 建表脚本，请查看 sql/init.sql"

# 格式化代码
fmt:
	cd $(BACKEND) && go fmt ./...

# 获取依赖
deps:
	cd $(BACKEND) && go mod tidy

# 交叉编译 Linux
linux: frontend-embed
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o ebook-server .

# 交叉编译 Windows（在 Windows 上构建时也会嵌入 PE VERSIONINFO）。
windows: WINRES_GOOS := windows
windows: WINRES_GOARCH := amd64
windows: backend-winres frontend-embed
	cd $(BACKEND) && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o ebook-server.exe .

# ── Windows 服务端构建 ───────────────────────────────────

# 生成 Windows PE VERSIONINFO 资源对象。非 Windows 目标由脚本直接跳过；
# 文件名带 _windows_amd64，Go 链接器只会把它链进对应目标的二进制。
backend-winres:
	@VERSION="$(VERSION)" GOOS="$(WINRES_GOOS)" GOARCH="$(WINRES_GOARCH)" WINDRES="$(WINDRES)" sh "$(WINRES_SCRIPT)"
