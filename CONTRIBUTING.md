# 贡献指南

本文件只回答“怎么提交、怎么开 PR、提交前跑什么”。构建与运行命令见 `docs/development.md`；架构与归属见 `docs/architecture.md`；文档放置规则见 `docs/AGENTS.md`。

## 提交规范

本仓库遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>(<scope>): <description>

[body]

[footer]
```

### Type（标准类型，不得自定义）

| Type | 说明 |
| --- | --- |
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `build` | 构建系统 / 依赖变更（Go Modules、Makefile 等） |
| `refactor` | 重构（不改变外部行为） |
| `docs` | 文档变更 |
| `test` | 测试变更 |
| `perf` | 性能优化 |
| `chore` | 杂务（CI 配置、脚本、工具链等） |
| `revert` | 回退提交 |

### Scope

Scope 直接使用模块目录名：

- **业务模块**：`handler` / `service` / `repository` / `model`
- **基础设施**：`middleware` / `pkg` / `config`
- **横切关注点**：`docs` / `sql`
- **可省略**：改动仅涉及根目录文件（如 `.gitignore`、`AGENTS.md`）或与 scope 无关的杂务时，可省略 scope。

### Description

- 使用中文，不超过 72 个字符。
- **动词前置**：以动词开头（新增…、修复…、重构…、升级…），禁止名词性短语（如“关于…的修改”“xxx 的变更”）。
- **英文缩写保持原样**：JWT、GORM、Gin、API 等专有名词保持原始大小写，不强行翻译。
- **禁止模糊表述**：不允许“更新了 xxx”“优化了一下”等无信息量描述。
- **禁止嵌入 issue 编号**：编号放在 footer（`Closes #123`），不要写在 description 里。
- 不加句号。

### Body

- **何时写**：单行 description 不足以解释动机或影响时，用 body 补充 why / how。
- **格式**：与 header 之间空一行；**正文优先用列表分条目**（每个条目一个改动点，条目按行换行、续行缩进两空格），每行不超过 100 字符，禁止一整段超长单行；条目组之间空一行。
- **语言**：与 description 一致（中文）。

### Footer

- **Breaking Change**：在 footer 中声明 `BREAKING CHANGE: <描述>`，或在 type 后加 `!`（如 `feat!: ...`）。
- **关联 Issue**：`Closes #123` / `Refs #456`。

### 语义版本映射

| type / footer | 版本 bump | 示例 |
| --- | --- | --- |
| `fix` | PATCH (0.0.x) | 修复崩溃 |
| `feat` | MINOR (0.x.0) | 新增功能 |
| `BREAKING CHANGE` / `!` | MAJOR (x.0.0) | API 不兼容变更 |
| `build` / `chore` / `docs` / `test` / `refactor` / `perf`（无 BREAKING CHANGE） | 不 bump | 版本号不变 |

任何 type 都可以携带 `BREAKING CHANGE`（不限于 `feat`/`fix`），如 `refactor!: 删除废弃 API` 触发 MAJOR。

### Revert 格式

Revert 有特殊格式约定，`description` 为被回退提交的完整 header，body 中必须包含 `This reverts commit <hash>`：

```
revert: feat(handler): 新增书签收藏接口

This reverts commit abc1234.
原因: 数据库迁移脚本有误，待修复后重新合入。
```

### 示例

常规提交：

```
feat(handler): 新增书签收藏接口               → MINOR bump (0.x.0)

引入 /api/bookmarks 路由，支持登录用户
对章节进行收藏与取消收藏操作。

Closes #42
```

```
fix(service): 修复评论删除未校验用户归属       → PATCH bump (0.0.x)

删除评论时增加 ownership 校验，
防止用户删除他人评论。
```

```
build: 升级 Gin 到 v1.10.1                    → 不 bump
```

Breaking change 提交（两种写法等价）：

```
refactor!: 重构 JWT token 过期时间配置         → MAJOR bump (x.0.0)

将 token 过期时间从硬编码改为 config.yaml 配置，
旧配置文件需新增 expire_min 字段。

BREAKING CHANGE: config.yaml 必须包含 jwt.expire_min 字段
```

## Pull Request

- 描述变更内容与动机，并关联对应 Issue。
- 列出已运行的测试（如 `go test -v ./service/...`）。
- 每个 PR 聚焦单一变更，避免夹带无关修改。
- 涉及 API 变更时同步更新 `README.md`。
- 涉及 Swagger 注解时重新生成 `backend/docs/`。

## 提交前验证

```bash
cd backend
gofmt -l <本次改过的 Go 文件>   # 期望无输出
go vet ./...
go test ./...
go test -race ./...
go mod tidy
cd ..

make frontend-test         # 涉及 frontend/ 时；vue-tsc + vitest
make docs                  # 涉及 Swagger 注解时
```

只对本次编辑过的 Go 文件执行 `gofmt -w <files>`，不要做目录级格式化。

## 构建与代码约定

- 遵循 Go 标准项目布局：服务入口是 `backend/main.go`。
- 依赖管理使用 Go Modules（`go.mod` / `go.sum`），不要手动修改；新增依赖用 `go get <package>@latest`，提交前 `go mod tidy`。
- 导出函数、类型和重要逻辑分支必须有注释，说明“是什么、为什么”。
- 文档与代码必须保持一致；改动在提交前同步更新相关文档。
- `config.yaml` 不含真实密钥（仅占位符），可安全提交。
- `.env` 已在 `.gitignore` 中排除，禁止提交真实密钥。
- 数据库文件 `*.db` 已在 `.gitignore` 中排除，禁止提交。
- 提交代码必须通过 `go vet`（当前的静态检查门禁）；仓库未配置 golangci-lint。
- 发布用 `VERSION=vX.Y.Z make all`（或 `make linux` / `make windows`），详见 `docs/development.md`。
- 不引入包级全局；配置、JWT 密钥、数据库连接以值注入，装配只在 `backend/main.go`。

## 文档归属

- 用户可见事实：`README.md`。
- 领域词汇与语义：`CONTEXT.md`。
- 当前架构、依赖方向与唯一主人：`docs/architecture.md`。
- 构建/运行/测试/发版：`docs/development.md`。
- 决策理由：`docs/adr/`（索引见 `docs/adr/README.md`）。
- 文档放置与写作规则：`docs/AGENTS.md`。
- 测试策略与读数：`TESTING.md`。

一条事实只住一个家；不要在本文件复制架构、API 或配置细节，只链接。
