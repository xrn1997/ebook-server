# Agent 文档分层与根 AGENTS 预算

Status: accepted

根 `AGENTS.md` 只保留每次会话必须看到的读序、硬约束、必过门与环境坑，目标 ≤8,000 字符；其余按事实之家分层：用户事实进 README，词汇进 CONTEXT，结构与归属进 `docs/architecture.md`，操作进 `docs/development.md`，决策进 ADR，文档规则进 `docs/AGENTS.md`。

此前根 AGENTS 同时承载架构、命令、配置、测试、提交规范与已知问题，重复内容会随代码漂移，agent 读到两个答案时无法判断哪个是现状。分层后每条事实只有一个家，过程工件留在 `docs/superpowers/`，不作事实源。

## 边界

- 本文不规定具体构建命令、API 端点或配置字段；它们各有主人。
- 本文不把 ADR 当状态清单；当前结构仍以 `docs/architecture.md` 为准。
- 文档门与分层门在里程碑 B 另立决策，不在本文承诺。

## 被否方案

- **单一大型 AGENTS**：入口短、维护省事，但每次会话都要携带全部细节，重复内容必然漂移；否决。
- **自动生成全部文档**：能消除手写漂移，但架构判断、被否方案与边界无法从代码生成；否决。
- **按子系统各写一份规格**：会造出第二份契约，与“一条事实一个家”冲突；否决。

锚点：`AGENTS.md`；`docs/AGENTS.md`；`docs/architecture.md`；`docs/adr/ADR-FORMAT.md`；`backend/internal/docscheck` 的 `Check` 与 `maxAgentsChars`（钉住根 AGENTS 预算）。
