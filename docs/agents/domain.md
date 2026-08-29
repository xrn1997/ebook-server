# 领域文档

工程技能在探索代码库时，应如何消费本仓库的领域文档。

## 探索前，先读这些

* 仓库根目录下的 **`CONTEXT.md`**，或

* 若存在仓库根目录下的 **`CONTEXT-MAP.md`** —— 它会指向每个上下文对应的一个 `CONTEXT.md`。读取与主题相关的每一个。

* **`docs/adr/`** —— 阅读与你要处理区域相关的 ADR。在多上下文仓库中，还要检查 `src/<context>/docs/adr/` 以获取上下文特定的决策。

如果以上文件不存在，**静默跳过**。不要指出它们的缺失，也不要建议预先创建它们。`/domain-modeling` 技能（通过 `/grill-with-docs` 和 `/improve-codebase-architecture` 触发）会在术语或决策真正得到解决时惰性地创建它们。

## 文件结构

单上下文仓库（大多数仓库）：

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 0001-event-sourced-orders.md
│   └── 0002-postgres-for-write-model.md
└── src/
```

多上下文仓库（根目录存在 `CONTEXT-MAP.md`）：

```
/
├── CONTEXT-MAP.md
├── docs/adr/                          ← 系统级决策
└── src/
    ├── ordering/
    │   ├── CONTEXT.md
    │   └── docs/adr/                  ← 上下文特定决策
    └── billing/
        ├── CONTEXT.md
        └── docs/adr/
```

## 使用术语表中的词汇

当你的产出命名一个领域概念时（在 issue 标题、重构建议、假设、测试名中），请使用 `CONTEXT.md` 中定义的术语。不要漂移到术语表明确回避的同义词。

如果你需要的概念尚未收录在术语表中，那是一个信号——要么你在发明项目并未使用的语言（请重新考虑），要么确实存在空缺（记录给 `/domain-modeling`）。

## 标记 ADR 冲突

如果你的产出与现有 ADR 相矛盾，请明确指出来，而不是静默覆盖：

> _与 ADR-0007（事件溯源订单）冲突——但值得重新讨论，因为…_

