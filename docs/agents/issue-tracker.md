# Issue 跟踪：GitHub

本仓库的 Issue 和 PRD 以 GitHub issue 的形式存在。所有操作都使用 `gh` CLI。

## 约定

- **创建 issue**：`gh issue create --title "..." --body "..."`。多行正文使用 heredoc。
- **读取 issue**：`gh issue view <number> --comments`，用 `jq` 过滤评论，同时获取标签。
- **列出 issues**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，配合合适的 `--label` 和 `--state` 过滤条件。
- **评论 issue**：`gh issue comment <number> --body "..."`
- **添加 / 移除标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭**：`gh issue close <number> --comment "..."`

从 `git remote -v` 推断仓库——在克隆目录内运行时，`gh` 会自动完成此操作。

## 将 PR 作为分诊来源

**将 PR 作为请求来源：否。** _(若本仓库将外部 PR 视为功能请求，则设为 `yes`；`/triage` 会读取此标志。)_

当设为 `yes` 时，PR 与 issue 使用相同的标签和状态，使用 `gh pr` 对应命令：

- **读取 PR**：`gh pr view <number> --comments`，查看差异用 `gh pr diff <number>`。
- **列出用于分诊的外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，然后仅保留 `authorAssociation` 为 `CONTRIBUTOR`、`FIRST_TIME_CONTRIBUTOR` 或 `NONE` 的（丢弃 `OWNER`/`MEMBER`/`COLLABORATOR`）。
- **评论 / 打标签 / 关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

GitHub 在 issue 和 PR 之间共享同一套编号，因此裸的 `#42` 可能是其中任何一个——用 `gh pr view 42` 解析，若失败则回退到 `gh issue view 42`。

## 当技能说“发布到 issue 跟踪器”

创建一个 GitHub issue。

## 当技能说“获取相关工单”

运行 `gh issue view <number> --comments`。

## 探路操作

由 `/wayfinder` 使用。**地图**是一个单一 issue，**子** issue 作为工单。

- **地图**：一个标记为 `wayfinder:map` 的单一 issue，承载 Notes / Decisions-so-far / Fog 正文。`gh issue create --label wayfinder:map`。
- **子工单**：一个链接到地图的 issue，作为 GitHub 子 issue（`gh api` 调用于 sub-issues 端点）。当子 issue 未启用时，将子工单添加到地图正文的任务列表中，并在子工单正文顶部添加 `Part of #<map>`。标签：`wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。一旦被认领，工单即分配给主导的开发人员。
- **阻塞**：GitHub 的**原生 issue 依赖**——规范的、UI 可见的表示方式。使用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 添加一条依赖边，其中 `<blocker-db-id>` 是阻塞者的数字**数据库 id**（`gh api repos/<owner>/<repo>/issues/<n> --jq .id`，而不是 `#number` 或 `node_id`）。GitHub 会报告 `issue_dependencies_summary.blocked_by`（仅开放中的阻塞者——即实时门控）。当依赖不可用时，回退到在子工单正文顶部添加一行 `Blocked by: #<n>, #<n>`。当每个阻塞者都被关闭时，工单即解除阻塞。
- **前沿查询**：列出地图的开放子项（`gh issue list --state open`，限定在地图的子 issue / 任务列表范围内），丢弃任何存在开放阻塞者（`issue_dependencies_summary.blocked_by > 0`，或在 `Blocked by` 行中有开放 issue）或已有指派人的项；按地图顺序取第一个。
- **认领**：`gh issue edit <n> --add-assignee @me`——本会话的首次写入。
- **解决**：`gh issue comment <n> --body "<answer>"`，然后 `gh issue close <n>`，再在地图的 Decisions-so-far 中追加一个上下文指针（gist + 链接）。