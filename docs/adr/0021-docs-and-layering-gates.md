# 文档与分层门禁随 go test 运行

Status: accepted

常驻文档的路径、链接、ADR 索引/状态/锚点与根 AGENTS 预算由 `backend/internal/docscheck` 检查；Go 包的依赖方向由 `backend/internal/archtest` 通过 `go list -json` 检查。两个包都随 `go test ./...` 运行，不另设人工门禁。

## 边界

- docscheck 不扫描 `docs/superpowers/**`，过程工件不作事实源。
- archtest 只检查生产 import，不检查测试 import。
- 前端（`frontend/`）的类型与构建门禁不在本文范围；本门禁只看 Go 包的依赖方向。

## 被否方案

- **只靠人工评审**：文档与依赖漂移不会在测试里变红；否决。
- **引入第三方 lint/检查器**：当前规则小且需要读仓库文档，标准库测试更直接；否决。
- **扫描全仓 Markdown**：会把过程工件与生成快照卷进来，误报高；否决。

锚点：`backend/internal/docscheck`；`backend/internal/archtest`；`docs/AGENTS.md`；`docs/architecture.md`。
