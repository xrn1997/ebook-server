# Review Remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落实 2026-09-06 对未提交改动的代码评审结论：Go 侧 swagger 注解/测试归位/文档同步，桌面主进程安全加固与健壮性修复，敏感值掩码，以及四个 spec 缺口补齐。

**Architecture:** 三段推进。Phase 0 处理 Go 后端（注解 → 错误码文档 → 测试归位 → 测试稳定性）；Phase 1 加固 Electron 主进程（安全 → 退出时序 → sidecar 输出解码 → 死代码清理 → 配置校验 → 构建卫生 → 端点派生）；Phase 2 处理 renderer 与文档（敏感值掩码 → 分页组件 → 缺失配置项 → 端口检测 → E2E → spec 同步）。每完成一个任务跑对应门禁，最后全量验证。

**Tech Stack:** Go 1.22 + Gin + GORM + swag（`make docs` 重新生成）；Electron 33 + TypeScript 5 + Vue 3 + Vitest 2 + esbuild（preload 单文件打包）+ Playwright（`_electron.launch`，依赖本机 electron 二进制）。

**硬约束（每个任务都必须遵守）**

- **本计划没有任何 `git add` / `git commit` 步骤**：工作区承载着用户大量未提交改动，绝不提交、绝不暂存。
- `.env`、`*.db` 永不入库；敏感值（`jwt.secret`、`admin.password`、`admin.jwt_secret`、`smtp.password`）不得进入 renderer（ADR-0012 §3）。
- `shutdown` 字面量在 `backend/main.go` 与 `desktop/src/main/sidecar.ts` 各有一份；本计划不改任一侧，若确需改动必须两处同步。
- **preload 产物必须保持单文件**（Task 6 Step 0 用 esbuild 打包）：沙箱化 preload 的 `require` 只认 `electron`/`events`/`timers`/`url` 白名单，相对路径一律 `module not found`（Electron 33 源码 `lib/sandboxed_renderer/init.ts` 的 `preloadRequire`，实测确认）。改动 preload 时不得让 tsc 多文件产物直接生效。
- 桌面任务完成后跑 `cd desktop && npm run typecheck && npx vitest run`；Go 任务后跑 `cd backend && go test ./...`。对应命令必须全绿才算任务完成。
- 路径一律用正斜杠；所有命令在仓库根 `D:/develop/GitHub/ebook-server` 执行（另注明者除外）。

## 文件结构总览

| 区域 | 文件 | 责任 |
|------|------|------|
| Go 注解 | `backend/handler/comment.go`、`backend/internal/admin/handler.go`、`backend/docs/*`（重新生成） | swagger 描述与实现一致 |
| Go 文档/测试 | `README.md`、`backend/pkg/errcode/errcode_test.go`、`backend/repository/{query,user,comment,log,test_helper}_test.go` | 错误码文档与码表同步；测试按被测对象归位 |
| Electron 主进程 | `desktop/src/main/index.ts`、`sidecar.ts`、`ipc.ts`、`config.ts`、`tsconfig.build.json`（新）、`src/renderer/vite.config.ts`、`scripts/preload-smoke.js`（新） | preload 单文件打包、安全加固、退出时序、输出解码、敏感值掩码 |
| 共享契约 | `desktop/src/shared/channels.ts`、`desktop/src/shared/types.ts`、`desktop/src/preload/index.ts` | 新通道 CHECK_PORT、新端点字段、PortCheckResult |
| Renderer | `desktop/src/renderer/src/`（Overview/Config/Service/Users/Comments/Logs、electron.ts、index.html、`components/Pager.vue` 新）、`tests/{Overview,Config,api,AdminViews}.test.ts` | UI 修正、缺口与既有测试同步 |
| 测试/E2E/文档 | `desktop/src/main/__tests__/*`、`desktop/e2e/app.spec.ts`（新）、`desktop/vitest.config.ts`（已有，加 exclude）、`desktop/playwright.config.ts`（新）、`Makefile`、`desktop/package.json`、`TESTING.md`、`AGENTS.md`、`docs/superpowers/specs/2026-09-05-desktop-app-design.md` | 门禁与文档同步 |

---

## Phase 0：Go 后端

### Task 1: 评论列表接口 chapter_url 参数补 collectionFormat

**Files:**
- Modify: `backend/handler/comment.go:77`
- Regenerate: `backend/docs/docs.go`、`backend/docs/swagger.json`、`backend/docs/swagger.yaml`

背景：`@Param chapter_url query []string` 在 swag 生成 OpenAPI 时默认按单值处理，而实现用 `c.QueryArray` 读多值；缺 `collectionFormat(multi)` 时 swagger 文档与真实行为不符。

- [ ] **Step 1: 修改注解**

把 `backend/handler/comment.go:77` 的

```go
// @Param chapter_url query []string false "书源章节聚合键（可传多个，返回并集；最多 50 个，单键最长 2048）"
```

改为

```go
// @Param chapter_url query []string false "书源章节聚合键（可传多个，返回并集；最多 50 个，单键最长 2048）" collectionFormat(multi)
```

- [ ] **Step 2: 重新生成 swagger 文档**

Run: `make docs`
Expected: 命令成功退出，`backend/docs/` 下三个文件有更新。

- [ ] **Step 3: 验证生成结果**

Run: `grep -c '"collectionFormat": "multi"' backend/docs/swagger.json`
Expected: 输出 `>= 1`

### Task 2: 后台管理端 7 个 handler 补 swagger 注解（初版漏计 Login，已补）

**Files:**
- Modify: `backend/internal/admin/handler.go`（7 个 handler 的注释块）
- Modify: `README.md`、`AGENTS.md`（api_docs 开启后果说明）
- Regenerate: `backend/docs/*`

背景：admin 包的 7 个导出 gin handler（Stats/ListUsers/GetUser/ListComments/DeleteComment/ListLogs + **Login**，`internal/admin/handler.go:84`，路由 `backend/main.go:196`）都没有 swagger 注解，生成的文档缺失后台 API。注解不写 `@Security`（后台 JWT 由独立引擎校验，不在公开文档里声明鉴权方式）。

- [ ] **Step 1: 为 7 个 handler 的注释块追加注解**

每个 handler 的现有说明注释保持原样，在其后追加注解行（注解行必须紧贴 `func` 声明上方的连续注释块内）。逐个如下：

`Stats`（现有 `// Stats 后台统计概览（用户数 + 评论数）。` 之后）：

```go
// @Summary 后台统计概览
// @Tags 管理后台
// @Produce json
// @Success 200 {object} model.Response
// @Router /admin/api/stats [get]
```

`ListUsers`（现有三行说明注释之后）：

```go
// @Summary 后台用户列表
// @Tags 管理后台
// @Produce json
// @Param keyword query string false "关键字（模糊匹配邮箱/用户名/昵称，纯数字兼命中 UID）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/users [get]
```

`GetUser`（现有说明注释块末尾，`:133-136` 共 4 行含空 `//` 行）：

```go
// @Summary 后台用户详情
// @Tags 管理后台
// @Produce json
// @Param uid path int true "用户 UID"
// @Success 200 {object} model.Response
// @Router /admin/api/users/{uid} [get]
```

`ListComments`（现有说明注释之后）：

```go
// @Summary 后台评论列表
// @Tags 管理后台
// @Produce json
// @Param keyword query string false "内容关键字（模糊匹配）"
// @Param book_name query string false "书名过滤"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/comments [get]
```

`DeleteComment`（现有说明注释块末尾，`:177-180` 共 4 行含空 `//` 行）：

```go
// @Summary 后台删除评论（治理）
// @Tags 管理后台
// @Produce json
// @Param id path int true "评论 ID"
// @Success 200 {object} model.Response
// @Router /admin/api/comments/{id} [delete]
```

`ListLogs`（现有说明注释之后）：

```go
// @Summary 后台操作日志列表
// @Tags 管理后台
// @Produce json
// @Param method query string false "HTTP 方法（如 GET/POST）"
// @Param path query string false "路径片段（模糊匹配）"
// @Param user_id query int false "按用户 UID 过滤"
// @Param error_code query string false "业务码（如 A0303）"
// @Param failed query string false "只看失败请求：1 或 true"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/logs [get]
```

`Login`（现有 5 行说明注释之后，`internal/admin/handler.go:79-83`）：

```go
// @Summary 后台登录
// @Tags 管理后台
// @Accept json
// @Produce json
// @Success 200 {object} model.Response
// @Router /admin/api/login [post]
```

- [ ] **Step 2: 重新生成并验证**

Run: `make docs && grep -c '/admin/api/' backend/docs/swagger.json`
Expected: 命令成功，计数 `>= 12`（7 路径 × paths+tags 引用至少出现一次；重点是 grep 非零且无报错）。

Run: `cd backend && go build ./...`
Expected: 成功（注解不影响编译，此步防手误）。

- [ ] **Step 3: 注明 api_docs 的公开后果**

`make docs` 的命令带 `--parseInternal`（`Makefile:49-51`），admin 注解会进生成文档；公开引擎在 `api_docs.enabled=true` 时挂同一份 spec（`backend/main.go:116-120`），届时**后台端点清单一并对外**——AGENTS.md 写「默认关防接口清单泄露」正是为此。把 `README.md:368` 的注释改为：

```yaml
  enabled: false      # 公开端口是否提供 Swagger 文档（默认关，防接口清单泄露；开启后会连后台管理端点清单一并展示，仅限受控网络临时开启）
```

`AGENTS.md` 路由表 `/api-docs` 一行末尾追加：「；开启后会连后台端点清单一并展示」。

### Task 3: README 评论域错误码补 A0305 + errcode 码表测试补齐

**Files:**
- Modify: `README.md:138`
- Modify: `backend/pkg/errcode/errcode_test.go`（TestError_Codes 的码表）

背景：`CommentKeySame`（A0305）已随评论迁移功能上线，但 README 的错误码说明还停在「A0301/A0302 预留」；`errcode_test.go` 的码表也漏了 `EmailImmutable`（A0113）与全部评论域错误码，码表与 `errcode.go` 常量失同步。

- [ ] **Step 1: 更新 README**

把 `README.md` 中的

```markdown
> 评论域错误码：`A0303` 无权删除、`A0304` 评论不存在（`A0301`/`A0302` 预留）。
```

改为

```markdown
> 评论域错误码：`A0303` 无权删除、`A0304` 评论不存在、`A0305` 聚合键与旧键相同（`A0301`/`A0302` 预留）。
```

- [ ] **Step 2: 补齐 TestError_Codes 码表**

在 `backend/pkg/errcode/errcode_test.go` 的 `codes := map[string]string{...}` 中：

`EmailExists: "A0112",` 一行之后插入：

```go
		EmailImmutable: "A0113",
```

`MailSendFailed: "C0503",` 一行之后插入：

```go
		CommentContentEmpty:   "A0301",
		CommentChapterInvalid: "A0302",
		CommentNotOwner:       "A0303",
		CommentNotFoundErr:    "A0304",
		CommentKeySame:        "A0305",
```

- [ ] **Step 3: 格式化并验证**

Run: `cd backend && gofmt -w pkg/errcode/errcode_test.go && go test ./pkg/errcode/...`
Expected: gofmt 会对 map 字面量重新对齐（属预期），测试 PASS。

### Task 4: query_test.go 按被测对象拆分归位

**Files:**
- Modify: `backend/repository/query_test.go`（只留 LIKE 转义 + 分页两个测试）
- Modify: `backend/repository/test_helper_test.go`（收编 seedUser）
- Modify: `backend/repository/user_test.go`（收 TestUserRepository_Search）
- Modify: `backend/repository/comment_test.go`（收 3 个评论测试）
- Modify: `backend/repository/log_test.go`（收 TestLogRepository_FindBy）

背景：AGENTS.md 约定「测试文件与被测文件同目录、命名 `<subject>_test.go`」。`TestUserRepository_Search` 却在 `query_test.go`，读者按文件名找不到它。`seedUser` 是公共种子，应上移到 test_helper。

- [ ] **Step 1: seedUser 移入 test_helper_test.go**

`backend/repository/test_helper_test.go` 最终全文如下（import 增加 `ebook-server/model`，文件尾部追加 seedUser）：

```go
package repository

import (
	"testing"

	"ebook-server/model"
	"ebook-server/pkg/testdb"

	"gorm.io/gorm"
)

// testDB 当前测试的独立 :memory: 数据库。
//
// 每次 setupTestDB 都创建全新实例，测试之间互不共享状态；
// go test 同包默认串行执行，包内变量在此是安全的（ADR-0007）。
var testDB *gorm.DB

// setupTestDB 创建本测试专用的 :memory: 数据库（连接由 testdb 在测试结束时关闭）
func setupTestDB(t *testing.T) {
	t.Helper()
	testDB = testdb.Open(t)
}

// cleanupTestDB 释放当前测试的数据库引用（连接关闭由 testdb 的 t.Cleanup 负责）
func cleanupTestDB(t *testing.T) {
	t.Helper()
	testDB = nil
}

// seedUser 造一个账号，返回其 UID。
func seedUser(t *testing.T, email, username, nickname string) uint {
	t.Helper()
	user := &model.User{Email: email, Password: "hp", Username: username, Nickname: nickname}
	if err := NewUserRepository(testDB).Create(user); err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return user.UID
}
```

- [ ] **Step 2: TestUserRepository_Search 移入 user_test.go**

在 `backend/repository/user_test.go` 的 import 块中加入 `"strconv"`（与 `"testing"`/`"time"` 同组，gofmt 会排序），然后在文件末尾追加（函数体从 query_test.go 原样剪切）：

```go
// TestUserRepository_Search 关键字命中邮箱/用户名/昵称，纯数字时兼命中 UID。
func TestUserRepository_Search(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	repo := NewUserRepository(testDB)
	uid := seedUser(t, "alice@example.com", "alice", "小艾")
	seedUser(t, "bob@example.com", "bob", "阿鲍")

	cases := []struct {
		name    string
		keyword string
		want    int64
	}{
		{"按邮箱", "alice@example", 1},
		{"按昵称", "小艾", 1},
		{"按 UID 数字", strconv.FormatUint(uint64(uid), 10), 1},
		{"空关键字即全量", "", 2},
		{"通配符不被当模式", "%", 0},
		{"无匹配", "zzz", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.Search(c.keyword, 1, 10)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if total != c.want {
				t.Errorf("Search(%q) total = %d, want %d", c.keyword, total, c.want)
			}
		})
	}
}
```

- [ ] **Step 3: 三个评论测试移入 comment_test.go**

在 `backend/repository/comment_test.go` 文件末尾追加（import 不变，无需新增）：

```go
// TestCommentRepository_Search 内容关键字 + 书名筛选，零值条件不过滤。
func TestCommentRepository_Search(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cs@example.com", "cs", "cs")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "这一章很精彩", BookName: "书A"})
	repo.Create(&model.Comment{UserID: uid, Content: "排版有问题", BookName: "书A"})
	repo.Create(&model.Comment{UserID: uid, Content: "很精彩的序言", BookName: "书B"})

	cases := []struct {
		name  string
		query model.CommentQuery
		want  int64
	}{
		{"关键字", model.CommentQuery{Keyword: "精彩"}, 2},
		{"书名", model.CommentQuery{BookName: "书A"}, 2},
		{"关键字+书名", model.CommentQuery{Keyword: "精彩", BookName: "书A"}, 1},
		{"空调=全量", model.CommentQuery{}, 3},
		{"通配符不被当模式", model.CommentQuery{Keyword: "%"}, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.Search(c.query, 1, 10)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}
			if total != c.want {
				t.Errorf("Search(%+v) total = %d, want %d", c.query, total, c.want)
			}
		})
	}
}

// TestCommentRepository_Count 只数未删除评论（后台概览用真实计数）。
func TestCommentRepository_Count(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "cnt@example.com", "cnt", "cnt")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "keep"})
	repo.Create(&model.Comment{UserID: uid, Content: "drop"})
	repo.Delete(2)

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Count = %d, want 1 (软删除不得计入)", count)
	}

	perUser, err := repo.CountByUserID(uid)
	if err != nil {
		t.Fatalf("CountByUserID failed: %v", err)
	}
	if perUser != 1 {
		t.Errorf("CountByUserID = %d, want 1", perUser)
	}
}

// TestCommentRepository_MigrateKey_BookLevel 旧键为空 = 迁移书籍级评论。
//
// 空串是 chapter_url 的合法值（ADR-0011），迁移必须能覆盖这个方向，
// 否则「合并书籍后把书籍级评论归到章节」做不到。
func TestCommentRepository_MigrateKey_BookLevel(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "migbl@example.com", "migbl", "migbl")
	repo := NewCommentRepository(testDB)
	repo.Create(&model.Comment{UserID: uid, Content: "book-1", ChapterURL: ""})
	repo.Create(&model.Comment{UserID: uid, Content: "book-2", ChapterURL: ""})
	repo.Create(&model.Comment{UserID: uid, Content: "chapter-1", ChapterURL: "key-a"})

	migrated, err := repo.MigrateKey(uid, "", "ch-1")
	if err != nil {
		t.Fatalf("MigrateKey failed: %v", err)
	}
	if migrated != 2 {
		t.Errorf("migrated = %d, want 2", migrated)
	}

	if _, total, _ := repo.FindByChapterURLs([]string{"ch-1"}, "", 1, 10); total != 2 {
		t.Errorf("comments under ch-1 = %d, want 2", total)
	}
	if _, total, _ := repo.FindByChapterURLs([]string{"key-a"}, "", 1, 10); total != 1 {
		t.Errorf("unrelated chapter should be untouched, got %d", total)
	}
}
```

- [ ] **Step 4: TestLogRepository_FindBy 移入 log_test.go**

在 `backend/repository/log_test.go` 文件末尾追加（import 不变，无需新增）：

```go
// TestLogRepository_FindBy 后台日志筛选：方法/路径/账号/业务码/只看失败。
func TestLogRepository_FindBy(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "log@example.com", "log", "log")
	repo := NewLogRepository(testDB)
	repo.Create(&model.OperationLog{UserID: uid, Method: "GET", Path: "/api/comments", ErrorCode: "00000"})
	repo.Create(&model.OperationLog{UserID: uid, Method: "POST", Path: "/api/comments", ErrorCode: "A0303"})
	repo.Create(&model.OperationLog{UserID: uid, Method: "GET", Path: "/api/users/me", ErrorCode: "00000"})
	// 非信封流量（404 等）没有业务码，「只看失败」不该把它们捞出来
	repo.Create(&model.OperationLog{Method: "GET", Path: "/missing", ResponseCode: 404})

	cases := []struct {
		name   string
		filter model.LogFilter
		want   int64
	}{
		{"空调=全量", model.LogFilter{}, 4},
		{"按方法", model.LogFilter{Method: "POST"}, 1},
		{"按路径片段", model.LogFilter{Path: "/api/comments"}, 2},
		{"按账号", model.LogFilter{UserID: uid}, 3},
		{"按业务码", model.LogFilter{ErrorCode: "A0303"}, 1},
		{"只看失败", model.LogFilter{OnlyFailed: true}, 1},
		{"组合条件", model.LogFilter{Method: "GET", Path: "/api"}, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, total, err := repo.FindBy(c.filter, 1, 10)
			if err != nil {
				t.Fatalf("FindBy failed: %v", err)
			}
			if total != c.want {
				t.Errorf("FindBy(%+v) total = %d, want %d", c.filter, total, c.want)
			}
		})
	}
}
```

- [ ] **Step 5: query_test.go 瘦身为共用查询原语**

`backend/repository/query_test.go` 最终全文（strconv import 删除，只留两个测试）：

```go
package repository

import (
	"testing"

	"ebook-server/model"
)

// query_test.go 覆盖共用的查询原语：LIKE 转义与分页窗口。
// 建立在其上的仓储查询测试按被测对象拆在 user/comment/log 各自的 _test.go。

func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"普通关键字", "abc", "%abc%"},
		{"百分号必须当普通字符", "100%", `%100\%%`},
		{"下划线必须当普通字符", "a_b", `%a\_b%`},
		{"转义符自身要先转义", `a\b`, `%a\\b%`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := likePattern(c.input); got != c.want {
				t.Errorf("likePattern(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestPaginateQueryReturnsPageWindow 分页助手必须同时给对总数与当页条数。
func TestPaginateQueryReturnsPageWindow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "page@example.com", "page", "page")
	repo := NewCommentRepository(testDB)
	for _, key := range []string{"k1", "k2", "k3", "k4", "k5"} {
		repo.Create(&model.Comment{UserID: uid, Content: "c", ChapterURL: key})
	}

	comments, total, err := repo.FindByChapterURLs([]string{"k1", "k2", "k3"}, "", 2, 2)
	if err != nil {
		t.Fatalf("FindByChapterURLs failed: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (总数不受分页影响)", total)
	}
	if len(comments) != 1 {
		t.Errorf("page 2 of size 2 should hold 1 row, got %d", len(comments))
	}
}
```

- [ ] **Step 6: 验证**

Run: `cd backend && go test ./repository/... && gofmt -l .`
Expected: 测试 PASS，`gofmt -l` 无输出。

### Task 5: DeleteComment 测试改用真实 ID + pathUint 局部变量改名

**Files:**
- Modify: `backend/internal/admin/handler_test.go`（TestAdminDeleteComment）
- Modify: `backend/internal/admin/handler.go:240-246`（pathUint）

背景：TestAdminDeleteComment 用写死的 `/admin/api/comments/1`——依赖「首条评论 ID 恰为 1」的自增巧合，换种子顺序即脆断。另外 pathUint 的局部变量叫 `uid` 但它解析任意 ID 参数，名不副实。

- [ ] **Step 1: 测试用 Create 返回的真实 ID**

在 `backend/internal/admin/handler_test.go` 的 TestAdminDeleteComment 中，把

```go
	comments.Create(&model.Comment{UserID: user.UID, Content: "target", BookName: "书A"})
	path := fmt.Sprintf("/admin/api/comments/%d", 1)
```

改为

```go
	comment := &model.Comment{UserID: user.UID, Content: "target", BookName: "书A"}
	comments.Create(comment)
	path := fmt.Sprintf("/admin/api/comments/%d", comment.ID)
```

（若该函数内其余断言引用了字面量 `1` 的位置，同步改用 `comment.ID`。）

- [ ] **Step 2: pathUint 局部变量改名**

把 `backend/internal/admin/handler.go` 的 pathUint 从

```go
// pathUint 解析路径参数里的数字 ID；第二个返回值非空即为 A0400 的文案。
func pathUint(c *gin.Context, name string) (uint, string) {
	uid, paramErr := parseUint(c.Param(name))
	if paramErr != "" {
		return 0, name + " " + paramErr
	}
	return uid, ""
}
```

改为

```go
// pathUint 解析路径参数里的数字 ID；第二个返回值非空即为 A0400 的文案。
func pathUint(c *gin.Context, name string) (uint, string) {
	id, paramErr := parseUint(c.Param(name))
	if paramErr != "" {
		return 0, name + " " + paramErr
	}
	return id, ""
}
```

- [ ] **Step 3: 验证**

Run: `cd backend && go test ./internal/admin/...`
Expected: PASS

Run: `cd backend && go vet ./internal/admin/...`
Expected: 无输出

---

## Phase 1 — 桌面主进程（Electron / sidecar）

> 每个任务的验证门禁：`cd desktop && npm run typecheck && npx vitest run`，期望 typecheck 无输出、vitest 全部通过。
> **全程禁止 `git add` / `git commit`**：工作区里的改动是用户的未提交工作，本计划只改文件。

### Task 6: Electron 窗口加固（preload 打包 / sandbox / CSP / 弹窗与导航拦截 / 单实例锁）

**Files:**
- Create: `desktop/scripts/preload-smoke.js`
- Modify: `desktop/package.json`（build:main / build:preload / test:preload 脚本）
- Modify: `desktop/src/main/index.ts`（webPreferences 约 L59-63、loadURL if/else 约 L66-70、bootSidecar 与 app.whenReady 之间）
- Modify: `desktop/src/renderer/index.html`（viewport meta 之后）

**关键事实（源码级 + 实测 2026-09-06）**：沙箱化 preload 的 `require` 只认 `electron`/`events`/`timers`/`url` 白名单，相对路径一律 `module not found`（Electron 33 源码 `lib/sandboxed_renderer/init.ts` 的 `preloadRequire`）；而 Electron 20+ 渲染进程**默认就是沙箱**（`web_contents_preferences.cc` 的 `IsSandboxed()`：`nodeIntegration:false` 且未显式声明即沙箱）。本应用 preload 运行时 `require('../shared/channels')`（编译产物 `dist/preload/index.js:4`）——**实测探针确认：现状应用在真实 shell 里 preload 已经加载失败、`window.electronAPI` 为 undefined**。单元测试全靠 happy-dom 打桩，所以一直没暴露。Step 0 是修复现存缺陷，不是预防性工作。

- [ ] **Step 0: preload 单文件打包 + 冒烟脚本**

① 安装打包器：

Run: `cd desktop && npm install -D esbuild`
Expected: 安装成功

② `desktop/package.json` scripts（以现有值为准做最小修改，`build:main` 由 Task 11 继续改为 tsconfig.build.json）：

```json
    "build:preload": "esbuild src/preload/index.ts --bundle --format=cjs --platform=node --external:electron --outfile=dist/preload/index.js",
    "test:preload": "electron scripts/preload-smoke.js",
```

并把 `build:main` 追加打包步：`"build:main": "tsc -p tsconfig.node.json && npm run build:preload"`。

（tsc 仍负责 preload 的类型检查；esbuild 产物覆盖 `dist/preload/index.js`，`--external:electron` 让 `require("electron")` 原样保留——它在白名单里。）

③ 新建 `desktop/scripts/preload-smoke.js`：

```js
// 用真实 dist/preload 验证沙箱兼容性：preload 加载失败（module not found 等）时退出码 1
const { app, BrowserWindow } = require('electron')
const path = require('node:path')

app.whenReady().then(async () => {
  const win = new BrowserWindow({
    show: false,
    webPreferences: {
      preload: path.join(__dirname, '..', 'dist', 'preload', 'index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })
  let preloadError = null
  win.webContents.on('preload-error', (_e, _p, err) => { preloadError = String(err) })
  await win.loadURL('data:text/html,<h1>probe</h1>')
  const api = await win.webContents.executeJavaScript('typeof window.electronAPI')
  win.destroy()
  if (preloadError || api !== 'object') {
    console.error(`[preload-smoke] FAIL preloadError=${preloadError} electronAPI=${api}`)
    app.exit(1)
    return
  }
  console.log('[preload-smoke] OK: window.electronAPI 已暴露')
  app.exit(0)
})
```

④ 前置：本机 electron 二进制缺失（`node_modules/electron/dist` 不存在——2026-09-06 实测本机缺失且安装缓存里的 zip 解压中断，`dist/` 只有 LICENSES 文件）时补齐：

Run: `cd desktop && node node_modules/electron/install.js`（网络受限加 `ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/`；若解压仍不完整，用 `unzip -oq "$LOCALAPPDATA/electron/Cache/electron-v33.4.11-win32-x64.zip" -d node_modules/electron/dist && printf 'electron.exe' > node_modules/electron/path.txt` 手动补齐）

⑤ 验证：

Run: `cd desktop && npm run build && npm run test:preload`
Expected: 输出 `[preload-smoke] OK: window.electronAPI 已暴露`，退出码 0

- [ ] **Step 1: webPreferences 显式开启 sandbox**

在 `desktop/src/main/index.ts` 的 `webPreferences` 对象（现有 preload / nodeIntegration / contextIsolation 字段处）加一行：

```ts
        sandbox: true,
```

（Electron 33 本就默认沙箱，显式声明是把隐式行为固化成契约；preload 已在 Step 0 打包成单文件，白名单 require 约束满足。）

- [ ] **Step 2: 拦截 window.open 与跨页导航**

在 loadURL 的 if/else 之后、`ready-to-show` 监听之前插入：

```ts
    // 渲染层被注入脚本时，window.open / 任意导航是把内容送出应用的通道：
    // 一律拒绝，只允许加载本应用自己的页面
    mainWindow.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
    mainWindow.webContents.on('will-navigate', (event, url) => {
      const allowed = isDev
        ? url.startsWith('http://localhost:5173')
        : url.startsWith('file://')
      if (!allowed) event.preventDefault()
    })
```

（缩进对齐现有 `mainWindow.webContents.*` 调用；`isDev` 变量已存在于该文件。）

- [ ] **Step 3: 单实例锁**

在 `bootSidecar` 定义之后、`app.whenReady()` 之前（模块顶层）加：

```ts
// 单实例锁：第二个实例直接退出，避免两份进程抢同一个 sidecar 端口与数据库
const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  app.quit()
}

app.on('second-instance', () => {
  if (mainWindow) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.focus()
  }
})
```

并在 `app.whenReady()` 回调的第一行加 `if (!gotLock) return`。

（与 Task 7 的交互：第二实例 `app.quit()` 会触发 before-quit——`quitting` 置位后二次事件直接放行，不会死循环；未拿到锁时 `whenReady` 首行 return，不会 createWindow/bootSidecar。）

- [ ] **Step 4: CSP meta**

`desktop/src/renderer/index.html` 的 viewport meta 之后加：

```html
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: http:; font-src 'self'; connect-src 'self' http: ws:; base-uri 'none'; form-action 'none'; frame-src 'none'; object-src 'none'" />
```

（`img-src http:` 是头像走后端绝对地址；`connect-src http:/ws:` 覆盖本机 API 与 vite HMR。）

- [ ] **Step 5: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run && npm run test:preload`
Expected: typecheck 无输出，vitest 全部通过，preload-smoke OK

Run: `grep -c "sandbox: true" desktop/src/main/index.ts && grep -c "Content-Security-Policy" desktop/src/renderer/index.html`
Expected: 两行各输出 `1`

（CSP meta 与退出时序的真实行为由 Task 17 的 E2E 断言覆盖；window.open 拦截与单实例锁无自动化断言，本任务完成后 `npm run dev` 人工冒烟一次。）

### Task 7: before-quit 等待 sidecar 优雅退出

**Files:**
- Modify: `desktop/src/main/index.ts`（文件末尾的 `app.on('before-quit', ...)`）

现状是同步回调里 `sidecar?.stop(); unregisterIpcHandlers()` 后直接返回——Electron 会立刻继续收尾，把正在优雅退出中的子进程强杀。

- [ ] **Step 1: 整体替换 before-quit 处理器**

找到（现有代码）：

```ts
app.on('before-quit', () => {
  sidecar?.stop()
  unregisterIpcHandlers()
})
```

替换为：

```ts
// 退出时先等 sidecar 落定再真正退出：before-quit 的处理是同步的，此刻直接返回会让
// Electron 立刻收尾、把正在优雅退出中的子进程强杀；preventDefault + 轮询状态、
// 到期再 app.quit() 才能让「写 stdin shutdown → 5 秒宽限」的停止流程走完。
let quitting = false
app.on('before-quit', (event) => {
  if (quitting) return
  quitting = true
  event.preventDefault()
  unregisterIpcHandlers()
  sidecar?.stop()
  const deadline = Date.now() + 6_000
  const tick = (): void => {
    const stillStopping = sidecar !== undefined && sidecar.getStatus() === 'stopping'
    if (stillStopping && Date.now() < deadline) {
      setTimeout(tick, 100)
      return
    }
    app.quit()
  }
  tick()
})
```

（6 秒 > sidecar 5 秒宽限；真实退出路径与耗时上界由 Task 17 的「退出时序」E2E 断言覆盖。）

- [ ] **Step 2: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 8: sidecar 输出按行缓冲与 taskkill 加固

**Files:**
- Modify: `desktop/src/main/sidecar.ts`
- Test: `desktop/src/main/__tests__/sidecar.test.ts`

问题：现有 `emitOutput` 直接按 chunk 转发——多字节 UTF-8 字符被 chunk 劈开时产生替换符，半行输出（如端口冲突报错被劈在中间）会被提前转发、漏定性；`runTaskkill` 的 try/catch 罩不住 spawn 的异步 ENOENT。

- [ ] **Step 1: 修正两个端口占用测试的 emit 缺尾换行**

`desktop/src/main/__tests__/sidecar.test.ts` 中「端口被别的进程占用」相关两处 emit 末尾补 `\n`：

- stderr 处：`mockProc.stderr.emit('data', Buffer.from('listen tcp :9090: bind: address already in use'))` → 内容末尾加 `\n`
- stdout 处：`mockProc.stdout.emit('data', Buffer.from('listen tcp 0.0.0.0:9090: bind: Only one usage of each socket address'))` → 内容末尾加 `\n`

按行缓冲后，没有尾换行的内容会先被缓存、不转发也不定性，这两个测试会因此挂掉——先补齐。

- [ ] **Step 2: 写失败测试**

「端口被别的进程占用」测试组之后加前三个；「宽限期后强杀」测试之后加第四个（文件已有 `import { EventEmitter } from 'node:events'`，无需新增导入）：

```ts
  it('多字节 UTF-8 字符跨 chunk 到达时按行缓冲、不产生替换符', () => {
    createManager()
    manager.start()

    const line = '[GIN] | 200 | 中文评论内容'
    const bytes = Buffer.from(line + '\n', 'utf8')
    // 把「中」字（3 字节）从中间劈开：half 落在其首字节之后
    const half = bytes.indexOf(Buffer.from('中', 'utf8')[0]) + 1
    mockProc.stdout.emit('data', bytes.subarray(0, half))
    mockProc.stdout.emit('data', bytes.subarray(half))

    expect(onLog).toHaveBeenCalledWith(`[go:stdout] ${line}`)
  })

  it('半行输出先缓存，凑齐整行才转发与定性', () => {
    createManager()
    manager.start()

    mockProc.stdout.emit('data', Buffer.from('listen tcp :9090: bind: address'))
    expect(manager.getStatus()).toBe('starting')
    expect(onError).not.toHaveBeenCalled()

    mockProc.stdout.emit('data', Buffer.from(' already in use\n'))
    expect(manager.getStatus()).toBe('error')
    expect(onError).toHaveBeenCalledWith('port-conflict', expect.stringContaining('被占用'))
  })

  it('进程退出时兜底吐出未换行的残留输出', () => {
    createManager()
    manager.start()

    mockProc.stdout.emit('data', Buffer.from('panic: runtime error')) // 半行，无换行
    mockProc.emit('exit', 1, null)

    expect(onLog).toHaveBeenCalledWith('[go:stdout] panic: runtime error')
  })

  it('taskkill spawn 异步报错（ENOENT）时兜底走 proc.kill', () => {
    createManager(undefined, { platform: 'win32' })
    manager.start()
    const pid = mockProc.pid

    // spawn 同步返回成功，ENOENT 走异步 error 事件：现有 try/catch 罩不住
    const killer = new EventEmitter()
    vi.mocked(spawn).mockImplementationOnce(() => killer as any)
    manager.stop()
    vi.advanceTimersByTime(5_000)

    expect(spawnCallArgs(1)[0]).toBe('taskkill')
    killer.emit('error', new Error('spawn taskkill ENOENT'))
    expect(mockProc.kill).toHaveBeenCalledWith('SIGKILL')
  })
```

- [ ] **Step 3: 确认测试失败**

Run: `cd desktop && npx vitest run src/main/__tests__/sidecar.test.ts`
Expected: 前三个新测试 FAIL（替换符 / 半行被提前转发定性 / 残留丢失），第四个 FAIL（`proc.kill` 未被调用）；现有测试 PASS

- [ ] **Step 4: 实现行缓冲（①~⑥）**

① `desktop/src/main/sidecar.ts` 顶部导入加：

```ts
import { StringDecoder } from 'node:string_decoder'
```

② `activePort` 字段声明之后加两个私有字段：

```ts
  /** 行解码器：多字节 UTF-8 字符跨 chunk 到达时不会产生替换符 */
  private lineDecoders: { stdout: StringDecoder; stderr: StringDecoder } = {
    stdout: new StringDecoder('utf8'),
    stderr: new StringDecoder('utf8'),
  }
  /** 未凑齐整行的半行缓存，等下一次 data 或进程退出时输出 */
  private pendingLines: { stdout: string; stderr: string } = { stdout: '', stderr: '' }
```

③ `start()` 里 `this.activePort = this.opts.port()` 之后重置两者：

```ts
    this.lineDecoders = { stdout: new StringDecoder('utf8'), stderr: new StringDecoder('utf8') }
    this.pendingLines = { stdout: '', stderr: '' }
```

④ stdout / stderr 的 `data` 回调从 `emitOutput(...)` 改为（两个回调分别用 `'go:stdout', 'stdout'` 与 `'go:stderr', 'stderr'`）：

```ts
      (chunk: Buffer) => this.emitLines('go:stdout', 'stdout', chunk)
```

⑤ `emitOutput` 方法整体替换为：

```ts
  /** 按行转发子进程输出：StringDecoder 处理跨 chunk 的多字节字符，半行先缓存等凑齐 */
  private emitLines(prefix: string, stream: 'stdout' | 'stderr', chunk: Buffer): void {
    const text = this.lineDecoders[stream].write(chunk)
    const buffered = this.pendingLines[stream] + text
    const lines = buffered.split(/\r?\n/)
    this.pendingLines[stream] = lines.pop() ?? ''
    for (const line of lines.filter(Boolean)) {
      this.opts.onLog(`[${prefix}] ${line}`)
      if (!this.portConflictSeen && PORT_IN_USE_PATTERN.test(line)) {
        this.reportPortConflict()
      }
    }
  }
```

（`PORT_IN_USE_PATTERN` / `reportPortConflict` 沿用现有实现；若 `emitOutput` 还被其他位置引用，随本步一并改掉。）

⑥ exit 处理器里 `if (this.portConflictSeen) return` 之后、`if (!this.intentionalStop)` 之前插入：

```ts
    // 退出时兜底吐出残留半行：end() 的返回值没有换行符，不能走 emitLines（会再次被缓存而静默丢失）
    for (const [stream, prefix] of [
      ['stdout', 'go:stdout'],
      ['stderr', 'go:stderr'],
    ] as const) {
      const residual = (this.pendingLines[stream] + this.lineDecoders[stream].end()).trim()
      if (residual) this.opts.onLog(`[${prefix}] ${residual}`)
    }
    this.pendingLines = { stdout: '', stderr: '' }
```

- [ ] **Step 5: 确认行缓冲测试通过**

Run: `cd desktop && npx vitest run src/main/__tests__/sidecar.test.ts`
Expected: Step 2 新增的前三个测试 PASS，现有测试 PASS（含 Step 1 补过尾换行的两处端口占用用例）

- [ ] **Step 6: 实现 taskkill 加固（⑦）**

⑦ `runTaskkill` 整体替换（现有实现基础上：force 参数加 `/T` 杀进程树；spawn 返回的 ChildProcess 挂异步 error 监听，ENOENT 等异步失败时兜底 `proc.kill`；原 try/catch 兜底保留）：

```ts
  private runTaskkill(proc: ChildProcess, force: boolean): void {
    if (proc.pid === undefined) return
    const args = force ? ['/F', '/T', '/PID', String(proc.pid)] : ['/PID', String(proc.pid)]
    let killer: ChildProcess
    try {
      killer = spawn('taskkill', args, { stdio: 'ignore' })
    } catch {
      // taskkill 不可用时退回 Node 自带信号，至少不留下孤儿进程
      try { proc.kill(force ? 'SIGKILL' : 'SIGTERM') } catch { /* already dead */ }
      return
    }
    killer.once('error', () => {
      // spawn 的 ENOENT 走异步 error 事件，同步 try/catch 罩不住
      try { proc.kill(force ? 'SIGKILL' : 'SIGTERM') } catch { /* already dead */ }
    })
  }
```

- [ ] **Step 7: 更新强杀测试**

「宽限期（5 秒）耗尽仍存活才强杀（taskkill /F）」测试（`sidecar.test.ts:291-302`）：标题加「/T」，`:301` 的断言从：

```ts
expect(spawnCallArgs(1)[1]).toEqual(['/F', '/PID', String(pid)])
```

改为：

```ts
expect(spawnCallArgs(1)[1]).toEqual(['/F', '/T', '/PID', String(pid)])
```

（`:300` 的 `expect(spawnCallArgs(1)[0]).toBe('taskkill')` 不动。注意 `spawnCallArgs(n)` 返回整个 `mock.calls[n]`——命令与参数数组分属下标 0/1，计划初版引用的无下标写法在文件里不存在。）

- [ ] **Step 8: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 9: 清理 port-conflict 死分支

**Files:**
- Modify: `desktop/src/main/sidecar.ts`

死代码判定（评审结论）：`reportPortConflict` 内部会清掉 startTimeout 且状态置 error，所以 startTimeout 回调里「portConflictSeen 为 true」的分支永远走不到；exit handler 的 `if (this.portConflictSeen) return` 是 load-bearing（端口冲突时进程自杀，不该走崩溃重启），保留。

- [ ] **Step 1: startTimeout 回调去掉死分支**

真实回调在 `desktop/src/main/sidecar.ts:159-177`：守卫是 `if (this.status === 'starting') {`，超时是字面量 `10_000`（**没有** `START_TIMEOUT_MS` 常量）。**只把 `:162-166` 的 `if (this.portConflictSeen) {...}` 分支整体删除、else 体原样提升（去掉 else 包裹），其余一字不动**——尤其不要动 `startup-timeout` 的消息文案，`sidecar.test.ts:228` 断言它 `stringContaining('9090')`。改完形如：

```ts
    this.startTimeout = setTimeout(() => {
      if (this.status === 'starting') {
        this.opts.onLog('[sidecar] Health check timeout (10s), marking as error')
        this.setStatus('error')
        this.opts.onError?.('startup-timeout', `后端在 10 秒内未通过健康检查（http://${this.healthHost}:${this.activePort}/health）`)
        // Kill the process to prevent zombie state
        if (this.process) {
          this.terminate(this.process)
        }
      }
    }, 10_000)
```

（死代码判定依据：`reportPortConflict`（`:196-208`）经 `cleanup()` 清掉 startTimeout 并置 error，`:162` 分支不可达。）

- [ ] **Step 2: handleCrash 删除端口冲突假设**

`handleCrash`（`desktop/src/main/sidecar.ts:359-382`）开头（`:360-363`）把 `portConflictSeen` 三元映射为 `port-conflict` 上报。把 `reason` 固定为 `'crashed'`、`message` 固定为 `'后端进程意外退出。'`（删掉两个三元的 port-conflict 分支），其余崩溃重启逻辑不动：

```ts
  private handleCrash(): void {
    // 走到这里必然不是端口冲突：exit handler 对 portConflictSeen 已提前 return（:150）
    const reason: ServiceErrorReason = 'crashed'
    const message = '后端进程意外退出。'
    // …以下 restartCount/maxRestarts 逻辑保持原样
```

（不可达依据：exit handler `:149-150` 对 `portConflictSeen` 提前 return，且 `reportPortConflict` 已置 `intentionalStop = true`，双保险。`'port-conflict'` 仍保留在 `ServiceErrorReason` 联合类型里——`reportPortConflict` 还在用。）

- [ ] **Step 3: 门禁**

Run: `cd desktop && npx vitest run src/main/__tests__/sidecar.test.ts`
Expected: 端口占用 2 例 + Task 8 新增 3 例、startup-timeout（消息含 9090）、崩溃重启、优雅退出等全部 PASS

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 10: admin.listen_addr 主机名校验（isIP）

**Files:**
- Modify: `desktop/src/main/config.ts`
- Test: `desktop/src/main/__tests__/config.test.ts`

ADR-0010 靠 `listen_addr` 与公网隔离。现在只挡通配地址：填 `my-host.local` 这类主机名会被放行，后端启动时 DNS 失败或解析到公网地址，隔离假设悄悄失效。

- [ ] **Step 1: 写失败测试**

`desktop/src/main/__tests__/config.test.ts` 中「通配地址被拒绝」测试之后（紧邻内网 IP 通过用例）加：

```ts
  it('后台监听地址为主机名时拒绝（isIP 校验），localhost 与留空除外', () => {
    const errors = validateConfig({
      ...validConfig(),
      admin: { ...validConfig().admin, listen_addr: 'my-host.local' },
    })
    expect(errors.join(' ')).toContain('listen_addr')
    expect(
      validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: 'localhost' } }),
    ).toEqual([])
    expect(
      validateConfig({ ...validConfig(), admin: { ...validConfig().admin, listen_addr: '' } }),
    ).toEqual([])
  })
```

- [ ] **Step 2: 确认测试失败**

Run: `cd desktop && npx vitest run src/main/__tests__/config.test.ts`
Expected: 新测试 FAIL（主机名当前被放行，errors 为空数组）

- [ ] **Step 3: 实现**

① `desktop/src/main/config.ts` 顶部导入加：

```ts
import { isIP } from 'node:net'
```

② `validateConfig` 的通配地址检查（`errors.push('admin.listen_addr 不允许使用通配地址…')` 所在 if 块）之后加：

```ts
  // 主机名一律拒绝：isIP 把「填错成域名」挡在保存前，否则后端启动时 DNS 解析失败
  // 或解析到公网地址，都会让 ADR-0010 的隔离假设悄悄失效
  if (
    !WILDCARD_LISTEN_ADDRS.includes(listenAddr) &&
    listenAddr !== '' &&
    listenAddr !== 'localhost' &&
    isIP(listenAddr) === 0
  ) {
    errors.push('admin.listen_addr 必须是 IP 地址或 localhost，不能填主机名')
  }
```

（`listenAddr` 是该函数里已有的 trimmed 局部变量名；若实际名不同，以现有代码为准对应替换。）

- [ ] **Step 4: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 11: 构建产物卫生（tsconfig.build.json / emptyOutDir）

**Files:**
- Create: `desktop/tsconfig.build.json`
- Modify: `desktop/package.json`（`build:main` 脚本）、`desktop/src/renderer/vite.config.ts`（`emptyDir` 键——注意它在 `src/renderer/` 下，不在 desktop/ 根；`build:renderer` 以 `src/renderer` 为 root 时自动加载它）

两个问题：① `build:main` 直接跑 `tsc`（tsconfig.node.json 无 exclude），测试文件会进 `dist/main/__tests__/`；② vite.config 里的 `emptyDir: true` 不是 Vite 合法键（正确键名是 `emptyOutDir`），清空输出从未生效。

- [ ] **Step 1: 新建 desktop/tsconfig.build.json**

```json
{
  "extends": "./tsconfig.node.json",
  "include": ["src/main/**/*.ts", "src/preload/**/*.ts", "src/shared/**/*.ts"],
  "exclude": ["src/main/__tests__"]
}
```

（include 必须重写：`extends` 不会合并数组字段，只写 exclude 会继承不到 include。）

- [ ] **Step 2: package.json 的 build:main 改走 build 配置**

```json
    "build:main": "tsc -p tsconfig.build.json && npm run build:preload",
```

（以 Task 6 Step 0 改过的 `build:main` 为准做最小替换，**保留 `&& npm run build:preload`**，只把 tsc 换成 build 配置。）

- [ ] **Step 3: src/renderer/vite.config.ts 修正键名**

`:10` 的 `emptyDir: true` 改为：

```ts
      emptyOutDir: true,
```

- [ ] **Step 4: 验证**

Run: `rm -rf desktop/dist && cd desktop && npm run typecheck && npm run build`
Expected: typecheck 无输出，构建成功

Run: `ls desktop/dist/main`
Expected: 无 `__tests__` 目录

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 12: ServiceEndpoints 增补 apiBaseUrl / adminOrigin（主进程是端点唯一来源）

**Files:**
- Modify: `desktop/src/shared/types.ts`、`desktop/src/main/config.ts`、`desktop/src/main/ipc.ts`
- Test: `desktop/src/main/__tests__/config.test.ts`、`desktop/src/main/__tests__/ipc.test.ts`
- Modify: `desktop/src/renderer/src/views/Overview.vue`
- Modify: `desktop/src/renderer/tests/Overview.test.ts`、`desktop/src/renderer/tests/api.test.ts`、`desktop/src/renderer/tests/AdminViews.test.ts`（fixture 补新字段，缺了 typecheck 直接红）

ADR-0012：端口/端点由主进程解析后下发给界面，renderer 不得自行猜。现在 Overview.vue:49-52 在渲染层用 `adminBaseUrl.replace(/\/admin\/api$/, '')` 派生「后台页面地址」；公开 API 地址虽已渲染（`:90`），但 `http://localhost:` 前缀写死在模板里、由 renderer 拼 apiPort——把这两个派生值挪进主进程。

- [ ] **Step 1: 写失败测试**

`desktop/src/main/__tests__/config.test.ts` 的 deriveEndpoints 测试，在 `dbPath` 断言之后加：

```ts
    expect(endpoints.apiBaseUrl).toBe('http://127.0.0.1:8080')
    expect(endpoints.adminOrigin).toBe('http://127.0.0.1:9191')
```

`desktop/src/main/__tests__/ipc.test.ts` 的「getRuntimeInfo 在配置缺失时」测试断言改为：

```ts
      expect(info.endpoints).toEqual({
        apiPort: 0,
        apiBaseUrl: '',
        adminBaseUrl: '',
        adminOrigin: '',
        dbPath: '',
      })
```

三个 renderer 测试的 fixture 同步补两键（`ServiceEndpoints` 新增字段是必填）：

`desktop/src/renderer/tests/Overview.test.ts:12-16`：

```ts
  endpoints: {
    apiPort: 8080,
    apiBaseUrl: 'http://127.0.0.1:8080',
    adminBaseUrl: 'http://192.168.1.10:9191/admin/api',
    adminOrigin: 'http://192.168.1.10:9191',
    dbPath: 'C:\\Users\\me\\ebook.db',
  },
```

并把 `:47-48` 的断言改为（模板改用主进程下发的字段后，`http://localhost:8080` 不再出现）：

```ts
    expect(text).toContain('http://127.0.0.1:8080')
    expect(text).toContain('http://192.168.1.10:9191')
```

`desktop/src/renderer/tests/api.test.ts:11` 与 `desktop/src/renderer/tests/AdminViews.test.ts:19` 的单行 fixture 同样补 `apiBaseUrl: 'http://127.0.0.1:8080'` 与 `adminOrigin: 'http://127.0.0.1:9191'` 两键。

- [ ] **Step 2: 确认测试失败**

Run: `cd desktop && npx vitest run src/main/__tests__/config.test.ts src/main/__tests__/ipc.test.ts`
Expected: 两处 FAIL（apiBaseUrl / adminOrigin 为 undefined）

- [ ] **Step 3: 类型与实现**

① `desktop/src/shared/types.ts` 的 `ServiceEndpoints`：`apiPort` 之后加：

```ts
  /** 公开 API 基址（公开端口绑 0.0.0.0，回环必可达） */
  apiBaseUrl: string
```

`adminBaseUrl` 之后加：

```ts
  /** 后台页面基址（不含 /admin/api，供概览页展示） */
  adminOrigin: string
```

② `desktop/src/main/config.ts` 的 `deriveEndpoints` 返回对象加两个字段：

```ts
      apiBaseUrl: `http://127.0.0.1:${config.server.port}`,
      adminOrigin: `http://${resolveConnectHost(config.admin.listen_addr)}:${config.admin.listen_port}`,
```

③ `desktop/src/main/ipc.ts` 的 `buildRuntimeInfo` fallback 字面量同步为五键：

```ts
      : { apiPort: 0, apiBaseUrl: '', adminBaseUrl: '', adminOrigin: '', dbPath: '' }
```

④ `desktop/src/renderer/src/views/Overview.vue`：删除 `adminOrigin` computed（`computed` 导入保留，概览页还有别的 computed 在用）；展示处 `{{ adminOrigin }}` 改为 `{{ endpoints?.adminOrigin ?? '-' }}`，公开 API 地址处改为 `{{ endpoints?.apiBaseUrl ?? '-' }}`。

- [ ] **Step 4: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

---

## Phase 2 — 渲染层收尾、CHECK_PORT、E2E 与文档

### Task 13: 敏感值掩码（读路径 mask / 写路径 fill）——密钥不出主进程

**Files:**
- Modify: `desktop/src/main/config.ts`、`desktop/src/main/ipc.ts`
- Test: `desktop/src/main/__tests__/config.test.ts`、`desktop/src/main/__tests__/ipc.test.ts`、`desktop/src/renderer/tests/Config.test.ts`
- Modify: `desktop/src/renderer/src/views/Config.vue`

问题：GET_CONFIG 把 config.yaml + .env 的明文密钥（JWT_SECRET、SMTP_PASSWORD、ADMIN_PASSWORD、ADMIN_JWT_SECRET）整个下发给 renderer，违反 ADR-0012 §3「凭据不出主进程」。方案（用户裁决）：读路径掩码为空串；SAVE_CONFIG 把空串视为「保持磁盘现值」，回填必须在业务校验之前。

- [ ] **Step 1: config.test.ts 写失败测试**

导入加 `maskSensitiveFields, fillMaskedSecrets`（与现有 `writeFullConfig, type AppConfig` 同一 import 语句或相邻行）；文件末尾追加：

```ts
describe('敏感值掩码（读路径 mask / 写路径 fill）', () => {
  it('maskSensitiveFields 把四个敏感字段替换为空串，其余字段原样', () => {
    const masked = maskSensitiveFields(validConfig())
    expect(masked.jwt.secret).toBe('')
    expect(masked.smtp.password).toBe('')
    expect(masked.admin.password).toBe('')
    expect(masked.admin.jwt_secret).toBe('')
    expect(masked.jwt.expire_min).toBe(120)
    expect(masked.admin.listen_addr).toBe('127.0.0.1')
  })

  it('maskSensitiveFields 返回深拷贝，不改入参', () => {
    const source = validConfig()
    maskSensitiveFields(source)
    expect(source.jwt.secret).toBe('my-jwt-secret')
  })

  it('fillMaskedSecrets：空串回填磁盘现值，非空新值保留', () => {
    const masked = maskSensitiveFields(validConfig())
    masked.jwt.secret = 'brand-new'
    const filled = fillMaskedSecrets(masked, validConfig())
    expect(filled.jwt.secret).toBe('brand-new')
    expect(filled.smtp.password).toBe('p')
    expect(filled.admin.password).toBe('p')
    expect(filled.admin.jwt_secret).toBe('j')
  })

  it('磁盘上本就没有该敏感值时，掩码空串原样保留（不虚构默认值）', () => {
    const onDisk = validConfig()
    onDisk.smtp.password = null as unknown as string
    const filled = fillMaskedSecrets(maskSensitiveFields(onDisk), onDisk)
    expect(filled.smtp.password).toBe('')
    expect(filled.jwt.secret).toBe('my-jwt-secret')
  })
})
```

（validConfig 的字段值见该文件头部：jwt.secret='my-jwt-secret'、smtp.password='p'、admin.password='p'、admin.jwt_secret='j'。若该文件已有顶层 describe 包裹，去掉外层 describe 直接加四个 it。）

- [ ] **Step 2: ipc.test.ts 写失败测试**

`import { writeFullConfig, type AppConfig } from '../config'` 加上 `maskSensitiveFields`；broken-yaml GET_CONFIG 测试之后加三个（GET_CONFIG 返回 `{ config }` 包装——与 `ipc.ts:106` 一致；磁盘值来自 beforeEach 写入的 `validConfig()`：jwt='jwt-s'、smtp='smtp-s'、admin.password='adm-p'、admin.jwt_secret='adm-j'）：

```ts
  it('GET_CONFIG 返回掩码后的配置：敏感字段为空串、其余原样', () => {
    const result = invoke<{ config?: AppConfig }>(Channels.GET_CONFIG)
    expect(result.config?.jwt.secret).toBe('')
    expect(result.config?.smtp.password).toBe('')
    expect(result.config?.admin.password).toBe('')
    expect(result.config?.admin.jwt_secret).toBe('')
    expect(result.config?.server.port).toBe(9090)
  })

  it('SAVE_CONFIG 收到掩码配置时空串回填磁盘现值再落盘', () => {
    invoke(Channels.SAVE_CONFIG, maskSensitiveFields(validConfig()))
    expect(deps.persistConfig).toHaveBeenCalledTimes(1)
    const saved = deps.persistConfig.mock.calls[0][0] as AppConfig
    expect(saved.jwt.secret).toBe('jwt-s')
    expect(saved.smtp.password).toBe('smtp-s')
    expect(saved.admin.password).toBe('adm-p')
    expect(saved.admin.jwt_secret).toBe('adm-j')
  })

  it('SAVE_CONFIG 收到非空新值时按新值落盘', () => {
    invoke(Channels.SAVE_CONFIG, { ...validConfig(), jwt: { ...validConfig().jwt, secret: 'brand-new' } })
    const saved = deps.persistConfig.mock.calls[0][0] as AppConfig
    expect(saved.jwt.secret).toBe('brand-new')
  })
```

- [ ] **Step 3: 确认测试失败**

Run: `cd desktop && npx vitest run src/main/__tests__/config.test.ts src/main/__tests__/ipc.test.ts`
Expected: 新增 7 例 FAIL（函数不存在 / GET_CONFIG 返回明文 / 空串被原样落盘）

- [ ] **Step 4: config.ts 实现两个纯函数**

`SENSITIVE_FIELDS` 常量之后追加（按 section/key 索引访问，配合 Record 收窄）：

```ts
/** 读路径掩码：密钥不出主进程（ADR-0012 §3），界面只见空串 */
export function maskSensitiveFields(config: AppConfig): AppConfig {
  const copy = JSON.parse(JSON.stringify(config)) as Record<string, Record<string, unknown>>
  for (const { section, key } of SENSITIVE_FIELDS) {
    copy[section][key] = ''
  }
  return copy as unknown as AppConfig
}

/** 写路径回填：掩码留下的空串视为「保持磁盘现值」 */
export function fillMaskedSecrets(incoming: AppConfig, onDisk: AppConfig): AppConfig {
  const copy = JSON.parse(JSON.stringify(incoming)) as Record<string, Record<string, unknown>>
  const disk = onDisk as unknown as Record<string, Record<string, unknown>>
  for (const { section, key } of SENSITIVE_FIELDS) {
    if (copy[section][key] === '' && disk[section][key] !== undefined && disk[section][key] !== null) {
      copy[section][key] = disk[section][key]
    }
  }
  return copy as unknown as AppConfig
}
```

- [ ] **Step 5: ipc.ts 接线**

导入加 `fillMaskedSecrets, maskSensitiveFields`（与现有 deriveEndpoints 等同一 import 语句）。

GET_CONFIG handler 的 `return { config }` 改为：

```ts
      // 密钥不出主进程（ADR-0012 §3）：界面只见掩码空串
      return { config: maskSensitiveFields(config) }
```

SAVE_CONFIG 里 `const config = sanitized.config` 替换为：

```ts
      // 掩码回填必须在业务校验之前：GET_CONFIG 把密钥掩成空串，空串在此表示「保持磁盘现值」
      const onDisk = readManagedConfig(deps.configPath, deps.envPath)
      const config = onDisk ? fillMaskedSecrets(sanitized.config, onDisk) : sanitized.config
```

（`readManagedConfig` 与 `deps.configPath` / `deps.envPath` 已在该文件使用；若实际依赖名不同，以现有代码为准。`readManagedConfig` 返回 null 时（config.yaml 缺失/损坏）掩码空串无法回填，会撞上 `validateConfig` 的 jwt.secret/admin.jwt_secret 非空校验（`config.ts:216-221`）——该路径经 UI 不可达（GET_CONFIG 在配置缺失时已先返回 error，`ipc.ts:104`），故不另写分支，此推理记录备查。）

- [ ] **Step 6: Config.vue 适配掩码 + 同步 Config.test.ts**

- 删除 `smtpWarning` computed（及模板中 `<div v-if="smtpWarning" class="alert alert-warning">{{ smtpWarning }}</div>` 一行）；`import { ref, onMounted, computed } from 'vue'` 去掉 `computed`
- 四个敏感输入（smtp.password / jwt.secret / admin.jwt_secret / admin.password）的 `placeholder` 统一改为 `"留空保持现值"`
- smtp 密码下的提示整句改为：`界面不回显已保存的值：留空保存 = 保持现值；输入新值 = 覆盖。敏感键写入用户数据目录的 .env（SMTP_PASSWORD），其余密钥写入 config.yaml，改完需重启服务生效`
- 管理员 tab 提示中删去「密码留空时这些视图无法加载」半句，保留其余说明

`desktop/src/renderer/tests/Config.test.ts` 两处会因本任务失败，同步改写：

① `:52-59`「首屏回填…」改为（placeholder 变了、敏感值被掩码）：

```ts
  it('首屏敏感字段掩码显示：空串 + 留空保持现值提示', async () => {
    const wrapper = await mountConfig({ config: baseConfig() })

    expect((wrapper.find('input[placeholder="smtp.qq.com"]').element as HTMLInputElement).value)
      .toBe('smtp.qq.com')
    expect((wrapper.find('input[placeholder="留空保持现值"]').element as HTMLInputElement).value)
      .toBe('')
    expect(wrapper.text()).toContain('留空保存 = 保持现值')
  })
```

② `:100-106`「SMTP 密码留空时提示邮件功能不可用」整条删除——`smtpWarning` computed 已随本任务移除，spec 里对应的校验条目在 Task 18 同步。

- [ ] **Step 7: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 14: 分页组件去重（Pager.vue）

**Files:**
- Create: `desktop/src/renderer/src/components/Pager.vue`
- Modify: `desktop/src/renderer/src/views/Users.vue`、`Comments.vue`、`Logs.vue`
- Regression: `desktop/src/renderer/tests/AdminViews.test.ts`（`:94-101` 断言分页文案与禁用态；Pager 保留同结构同文案，应保持通过——列为本任务回归面）

三个视图各写了一份同样的「上一页 / 第 x / y 页，共 N 个单位 / 下一页」分页 div，只有文案单位不同。分页**状态与方法**本就共享自 `src/renderer/src/useAdminList.ts`（page/pageCount/total/hasPrev/hasNext/prevPage/nextPage），重复的只是模板块——Pager 只抽模板，不动逻辑。

- [ ] **Step 1: 新建 Pager.vue**

```vue
<script setup lang="ts">
defineProps<{
  page: number
  pageCount: number
  total: number
  unit: string
  hasPrev: boolean
  hasNext: boolean
}>()

defineEmits<{ prev: []; next: [] }>()
</script>

<template>
  <div style="display: flex; gap: 12px; align-items: center; margin-top: 12px;">
    <button class="btn" :disabled="!hasPrev" @click="$emit('prev')">上一页</button>
    <span class="sub">第 {{ page }} / {{ pageCount }} 页，共 {{ total }} {{ unit }}</span>
    <button class="btn" :disabled="!hasNext" @click="$emit('next')">下一页</button>
  </div>
</template>
```

- [ ] **Step 2: 三个视图替换**

各视图 `<script setup>` 加 `import Pager from '../components/Pager.vue'`，把各自的分页 div（Users.vue 末尾「个账号」、Comments.vue「条评论」、Logs.vue「条记录」，结构相同）替换为：

```vue
    <Pager
      :page="page"
      :page-count="pageCount"
      :total="total"
      unit="个账号"
      :has-prev="hasPrev"
      :has-next="hasNext"
      @prev="prevPage"
      @next="nextPage"
    />
```

（`unit` 分别为 `个账号` / `条评论` / `条记录`；prop 与事件名以各视图现有的分页状态变量与翻页方法为准，逐个对照替换，不改任何分页逻辑。）

- [ ] **Step 3: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 15: 配置页补 admin.expire_min 字段

**Files:**
- Modify: `desktop/src/renderer/src/views/Config.vue`

`AppConfig.admin.expire_min`（管理端令牌有效期）存在于类型与 config.yaml，但配置页管理员 tab 没有输入框，用户无法修改。

- [ ] **Step 1: 管理员 tab 密码输入之后加字段**

在「管理员密码」form-group 之后（该 tab 提示文案之前）加：

```html
      <div class="form-group">
        <label>管理端令牌有效期（分钟）</label>
        <input v-model.number="config.admin.expire_min" type="number" min="1" />
        <p class="sub">管理端登录态的有效时长，改完需重启服务</p>
      </div>
```

（该页提示统一用 `sub`（`style.css:74`）；`hint` class 全库不存在，不要新建。）

- [ ] **Step 2: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 16: CHECK_PORT——服务页启动前端口占用检测

**Files:**
- Modify: `desktop/src/shared/channels.ts`、`desktop/src/shared/types.ts`、`desktop/src/main/ipc.ts`、`desktop/src/preload/index.ts`、`desktop/src/renderer/src/electron.ts`、`desktop/src/renderer/src/views/Service.vue`
- Test: `desktop/src/main/__tests__/ipc.test.ts`

spec 缺口：用户只能「启动 → 看报错」才知道端口被占。端口语义由主进程唯一裁决（ADR-0012），这里加一个只读探测通道，renderer 只做展示。

- [ ] **Step 1: 契约先行**

`desktop/src/shared/channels.ts` 的 `GET_RUNTIME_INFO` 之后加：

```ts
  CHECK_PORT: 'check-port',
```

`desktop/src/shared/types.ts` 的 `ServiceEndpoints` 接口之后加：

```ts
/** check-port 的响应：主进程对真实端口做一次 connect 探测 */
export interface PortCheckResult {
  port: number
  inUse: boolean
}
```

`ElectronAPI` 的 `getRuntimeInfo` 之后加：

```ts
  checkPort: () => Promise<PortCheckResult | null>
```

- [ ] **Step 2: ipc.test.ts 写失败测试**

顶部加 `import net from 'node:net'`，类型导入加 `PortCheckResult`；「getRuntimeInfo」相关测试之后加两个：

```ts
  it('CHECK_PORT 探测被占端口返回 inUse: true', async () => {
    const server = net.createServer()
    await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve))
    const port = (server.address() as { port: number }).port
    deps.getRuntimeConfig = () => ({ ...validConfig(), server: { port, mode: 'release' } })
    try {
      const payload = await invoke<PortCheckResult>(Channels.CHECK_PORT)
      expect(payload).toEqual({ port, inUse: true })
    } finally {
      server.close()
    }
  })

  it('CHECK_PORT 配置缺失时返回 port 0 与 inUse: false', async () => {
    deps.getRuntimeConfig = () => null
    const payload = await invoke<PortCheckResult>(Channels.CHECK_PORT)
    expect(payload).toEqual({ port: 0, inUse: false })
  })
```

- [ ] **Step 3: 确认测试失败**

Run: `cd desktop && npx vitest run src/main/__tests__/ipc.test.ts`
Expected: 两个新测试 FAIL（`handler check-port 未注册`）

- [ ] **Step 4: ipc.ts 实现**

顶部加 `import net from 'node:net'`；`GET_RUNTIME_INFO` handler 之后（同一个注册 handler 的函数作用域内，与现有 handler 并列）加：

```ts
  /** 对回环做一次真实 connect：连得上说明端口被占（无论被谁占） */
  async function probePort(port: number): Promise<boolean> {
    return new Promise((resolve) => {
      let settled = false
      const socket = net.connect({ port, host: '127.0.0.1' })
      const settle = (inUse: boolean): void => {
        if (settled) return
        settled = true
        socket.destroy()
        resolve(inUse)
      }
      socket.setTimeout(500)
      socket.on('connect', () => settle(true))
      socket.on('timeout', () => settle(false))
      socket.on('error', () => settle(false))
    })
  }

  ipcMain.handle(Channels.CHECK_PORT, async () => {
    const config = deps.getRuntimeConfig()
    if (!config) {
      return { port: 0, inUse: false }
    }
    return { port: config.server.port, inUse: await probePort(config.server.port) }
  })
```

- [ ] **Step 5: preload 与 renderer 封装**

`desktop/src/preload/index.ts` 的 `getRuntimeInfo` 之后加：

```ts
  checkPort: () => ipcRenderer.invoke(Channels.CHECK_PORT),
```

`desktop/src/renderer/src/electron.ts` 的 `getRuntimeInfo` 之后加（文件顶部 type 导入加 `PortCheckResult`）：

```ts
export async function checkPort(): Promise<PortCheckResult | null> {
  const api = getAPI()
  if (!api) return null
  return api.checkPort()
}
```

- [ ] **Step 6: Service.vue 加检测入口**

`<script setup>`：vue 导入加 `computed`（现有导入无）；加 `import type { PortCheckResult } from '../../../shared/types'`；状态与方法（`serviceStatus` 来自现有 `../stores/service` 导入；若实际变量名不同，以现有代码为准）：

```ts
const portCheck = ref<PortCheckResult | null>(null)
const checkingPort = ref(false)

async function checkPortNow() {
  checkingPort.value = true
  try {
    portCheck.value = await ipc.checkPort()
  } finally {
    checkingPort.value = false
  }
}

const portCheckText = computed(() => {
  const c = portCheck.value
  if (!c || c.port === 0) return ''
  if (c.inUse && serviceStatus.value === 'running') return `端口 ${c.port} 正由本服务使用`
  if (c.inUse) return `端口 ${c.port} 已被其他进程占用，启动会失败——请在配置页改端口`
  return `端口 ${c.port} 空闲，可以启动`
})
```

模板：启停按钮的 div 之后加：

```html
      <div style="margin-top: 8px; display: flex; gap: 8px; align-items: center;">
        <button class="btn" @click="checkPortNow" :disabled="checkingPort">
          {{ checkingPort ? '检测中...' : '检测端口' }}
        </button>
        <span v-if="portCheckText" class="sub">{{ portCheckText }}</span>
      </div>
```

- [ ] **Step 7: 门禁**

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck 无输出，vitest 全部通过

### Task 17: E2E 冒烟测试（Playwright _electron）

**Files:**
- Modify: `desktop/src/main/index.ts`、`desktop/vitest.config.ts`、`desktop/package.json`、`Makefile`、`TESTING.md`、`AGENTS.md`
- Create: `desktop/playwright.config.ts`、`desktop/tsconfig.e2e.json`、`desktop/e2e/app.spec.ts`

约束：`e2e/` 在 tsconfig.node.json 的 `rootDir: "src"` 之外，直接加进主 typecheck 会 TS6059，所以单独给 `tsconfig.e2e.json`；E2E 需要把 userData 指到临时目录、必要时指定 renderer 文件，走环境变量钩子。Playwright `_electron.launch` 不需要下载浏览器二进制，但用的就是 `node_modules/electron` 的运行时——二进制缺失时按 Task 6 Step 0 ④补齐。

- [ ] **Step 1: index.ts 加环境变量钩子**

① app 导入语句之后加：

```ts
// E2E 把 userData 指到临时目录，避免污染真实用户数据
const userDataOverride = process.env.EBOOK_SERVER_USER_DATA
if (userDataOverride) {
  app.setPath('userData', userDataOverride)
}
```

② loadURL 的 `if (isDev)` 改为：

```ts
    if (isDev && !process.env.EBOOK_SERVER_RENDERER_FILE) {
```

（E2E 直接 loadFile 打包后的 renderer，绕开 vite dev server。）

- [ ] **Step 2: 三个新配置文件**

`desktop/vitest.config.ts` 的 `test` 配置加 exclude（vitest 不吃 e2e；已有 exclude 数组则合并，没有则新增）：

```ts
      exclude: ['e2e/**', 'node_modules/**', 'dist/**'],
```

`desktop/playwright.config.ts` 新建：

```ts
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 120_000,
  expect: { timeout: 15_000 },
  workers: 1,
  forbidOnly: !!process.env.CI,
})
```

`desktop/tsconfig.e2e.json` 新建：

```json
{
  "compilerOptions": {
    "noEmit": true,
    "module": "Node16",
    "moduleResolution": "Node16",
    "target": "ES2022",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "types": ["node"]
  },
  "include": ["e2e/**/*.ts", "playwright.config.ts"]
}
```

（不并入主 typecheck 的原因：tsconfig.node.json 有 `rootDir: "src"`，e2e 在其外会 TS6059。`skipLibCheck` 免受 @playwright/test 类型噪声拖累。）

- [ ] **Step 3: 依赖与脚本**

前置检查：`_electron.launch` 依赖本机 electron 二进制，`node_modules/electron/dist/electron.exe` 缺失时先按 Task 6 Step 0 ④补齐（2026-09-06 实测本机就缺）。

Run: `cd desktop && npm install -D @playwright/test`
Expected: 安装成功（无需 `npx playwright install`，`_electron` 不下载浏览器）

`desktop/package.json` scripts 加：

```json
    "test:e2e": "playwright test",
```

typecheck 脚本末尾追加 ` && tsc -p tsconfig.e2e.json --noEmit`（以现有脚本内容为准做最小追加）。

- [ ] **Step 4: e2e/app.spec.ts**

```ts
import { test, expect, _electron, type ElectronApplication, type Page } from '@playwright/test'
import { mkdtempSync, existsSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const projectRoot = join(__dirname, '..', '..')
// 本仓库桌面测试只在 Windows 跑：Go 二进制名带 .exe（与 Makefile 的 GOOS 判断一致），非 Windows 上整段 skip
const sidecarBinary = join(projectRoot, 'desktop', 'resources', 'backend', 'ebook-server.exe')
const mainEntry = join(projectRoot, 'desktop', 'dist', 'main', 'index.js')
const rendererFile = join(projectRoot, 'desktop', 'dist', 'renderer', 'index.html')

test.skip(!existsSync(sidecarBinary) || !existsSync(mainEntry), '需要先 make desktop-build-backend 与构建 renderer 产物')

let app: ElectronApplication
let window: Page
let userData: string
let closed = false

/** 幂等关闭并返回耗时（ms），供退出时序断言与 afterEach 复用 */
async function closeApp(): Promise<number> {
  if (closed) return 0
  closed = true
  const start = Date.now()
  await app.close()
  return Date.now() - start
}

test.beforeEach(async () => {
  userData = mkdtempSync(join(tmpdir(), 'ebook-e2e-'))
  closed = false
  app = await _electron.launch({
    args: [mainEntry],
    env: {
      ...process.env,
      EBOOK_SERVER_USER_DATA: userData,
      EBOOK_SERVER_RENDERER_FILE: rendererFile,
    },
  })
  window = await app.firstWindow()
})

test.afterEach(async () => {
  await closeApp()
  rmSync(userData, { recursive: true, force: true })
})

test('启动后概览可见、sidecar 进入运行中', async () => {
  await expect(window.locator('h2')).toContainText('概览')
  await expect(window.getByText('服务状态：运行中')).toBeVisible({ timeout: 30_000 })
  await expect(window.getByText(/ebook\.db/)).toBeVisible()
})

test('渲染页带 CSP meta（Task 6 的真实行为）', async () => {
  const hasCsp = await window.evaluate(
    () => !!document.querySelector('meta[http-equiv="Content-Security-Policy"]'),
  )
  expect(hasCsp).toBe(true)
})

test('退出时等待 sidecar 落定而不是立即收尾（Task 7 的真实行为）', async () => {
  await expect(window.getByText('服务状态：运行中')).toBeVisible({ timeout: 30_000 })
  const elapsed = await closeApp()
  // Task 7 落地前是同步 stop 后立即收尾（耗时≈0）；落地后至少经一轮轮询，且不得烧满 6 秒 deadline
  expect(elapsed).toBeLessThan(6_000)
})
```

（断言中的中文文案以 Overview.vue 实际渲染为准；标题或状态文案若不同，对应改断言。）

- [ ] **Step 5: Makefile 目标**

`.PHONY` 行加 `desktop-e2e`；`desktop-test` 目标附近加：

```makefile
desktop-e2e: desktop-build-backend desktop-build-frontend
	cd desktop && npm run build:main && npm run test:preload && npx playwright test
```

- [ ] **Step 6: 验证**

Run: `make desktop-e2e`
Expected: 产物齐全时黄金路径 PASS；产物缺失时测试 skip 且 make 不因 skip 失败

Run: `cd desktop && npm run typecheck && npx vitest run`
Expected: typecheck（含 e2e 项目）无输出，vitest 全部通过（e2e 已被 exclude）

- [ ] **Step 7: 文档同步**

TESTING.md 与 AGENTS.md 的桌面测试小节各加一段：E2E 运行方式 `make desktop-e2e`、前置条件（sidecar 二进制与 dist 产物齐备）、产物缺失时整段 skip 的语义（与现有「裸 npm test 会让集成用例整段跳过」的门禁说明呼应）。

### Task 18: spec 文档同步与「明确不做」

**Files:**
- Modify: `docs/superpowers/specs/2026-09-05-desktop-app-design.md`

- [ ] **Step 1: 停止流程改为实现现状**

「### 停止流程」一节的现有三条（SIGTERM / taskkill 描述）替换为：

```markdown
### 停止流程

1. 向 Go 子进程 stdin 写入 `shutdown` 行（Windows 无窗口控制台收不到 SIGTERM，stdin 是父子双方都拿得到的通道）
2. 等待 5 秒优雅退出（Go 侧 signal.Notify + http.Server.Shutdown 覆盖两个监听并关库）；超时则 Windows `taskkill /F /T` 强杀
3. 释放端口，标记为「已停止」

> `shutdown` 字面量在 `backend/main.go` 与 `desktop/src/main/sidecar.ts` 各有一份，改动任一侧必须同步另一侧。
```

- [ ] **Step 2: spec 补记本轮新增能力**

`docs/superpowers/specs/2026-09-05-desktop-app-design.md`（按对应小节现文改写）：

① 「### 配置读写流程」：第 1 步改为「Main 读取 config.yaml + .env → **掩码敏感字段（掩码值 = 空串）** → 通过 IPC 返回」，第 2 步补「掩码空串在业务校验前回填磁盘现值」。
② 「### 配置校验」：删除「SMTP 密码为空时提示『邮件功能将不可用』」（`smtpWarning` 随掩码方案移除）；补「后台监听地址必须是 IP 或 localhost（isIP 校验）」。
③ 「### 页面清单」服务页「端口检测」：对应 CHECK_PORT 只读探测通道（端口语义由主进程裁决，renderer 只展示）。
④ 「### 配置页面结构」管理员 tab 补 `expire_min` 输入框；「### 关键决策」表补一行：preload 单文件打包（esbuild）——沙箱 preload 的 require 白名单约束。

- [ ] **Step 3: 文末追加评审裁决记录**

```markdown
## 评审明确不做（2026-09-06）

- 空 `chapter_url` 的评论聚合键不对称：用户裁决不处理
- Overview / Service 启停按钮去重：YAGNI
- 端口冲突文案抽公共常量：YAGNI（Task 9 清理死分支后已无重复面）
- sidecar exit handler 的 `if (this.portConflictSeen) return`：load-bearing（保住 error 状态不被改回 stopped），保留

## 评审裁决记录（2026-09-06 计划复审后补）

- preload 单文件打包（esbuild）：采纳。沙箱 preload 的 require 只认白名单，`require('../shared/channels')` 在真实 shell 必挂（Electron 33 源码 `lib/sandboxed_renderer/init.ts` + 探针实测）；且 Electron 20+ 默认沙箱，这是修复现存缺陷而非新约束
- admin `Login` 端点补 swagger 注解（第 7 个 handler，初版计划漏计）
- api_docs 开启会连后台端点清单一并对外：README/AGENTS 已注明，开关语义不变
- 掩码的语义损失：空串 = 保持现值，界面从此无法「清空」敏感值——接受；确需清空时直接编辑用户数据目录的 config.yaml / .env
- 掩码回填在 onDisk 为 null 时会撞非空校验：经 UI 不可达（GET_CONFIG 此时已先报错），不写分支
```

- [ ] **Step 4: 全量验证**

Run: `cd backend && go test ./...`
Expected: PASS

Run: `cd backend && gofmt -l . && go vet ./...`
Expected: 无输出

Run: `make desktop-test`
Expected: 通过

Run: `make desktop-e2e`
Expected: PASS（产物齐全时）

Run: `cd backend && golangci-lint run`
Expected: 无警告（若本机安装了 golangci-lint）
