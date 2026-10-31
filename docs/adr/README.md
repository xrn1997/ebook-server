# ADR 索引

每篇 ADR 在本表恰好出现一次。状态与取代规则见 `docs/adr/ADR-FORMAT.md`。

| 编号 | 标题 | 状态 | 一句话范围 |
| --- | --- | --- | --- |
| 0001 | 响应信封重构 + 认证模型升级 | accepted；账号体系/密码管理部分被 0002 取代 | 统一响应信封；access/refresh 双 token 认证模型 |
| 0002 | 邮箱即主键的注册与账号模型 | accepted | 邮箱作为唯一登录标识，uid 为主键，资料与凭据分离 |
| 0003 | refresh 仅凭证、登录才建身份 | accepted | refresh 仅作续期凭据，登录成功才建立身份 |
| 0004 | 登录主标识不可变 | accepted | 登录主标识注册后不可变更 |
| 0005 | 账号注销用匿名化而非删除 | accepted | 注销采用匿名化，保留公开内容 |
| 0006 | 统一防账号枚举 | accepted | 注册/找回/登录统一防账号枚举 |
| 0007 | repository 接口由消费方定义，测试替身用 SQLite :memory: | accepted | repository 接口由消费方定义，测试替身用 SQLite :memory: |
| 0008 | 发码编排收拢进验证码发送纵深模块 | accepted | 验证码的发送与校验收拢进纵深模块 |
| 0009 | 前后端 monorepo 重组 | accepted；部分被 0024 取代（desktop 成员撤销） | backend/frontend 单仓分层 |
| 0010 | 后台管理系统网络隔离与登录限流 | accepted | 后台独立引擎与 JWT，默认仅回环监听并限流 |
| 0011 | 评论章节归属与头像文件上传 | accepted；§1 聚合键与空键语义被 0014 取代 | 章节评论归属与头像文件生命周期 |
| 0012 | 桌面管理应用：Go 后端 sidecar 复用 | superseded by 0024 | Electron 以 sidecar 复用 Go 后端，主进程持有管理凭据 |
| 0013 | 评论聚合键迁移、多键并集查询与后台管理面扩展 | accepted；§1、§2 被 0014 取代 | 评论聚合键迁移、多键并集查询与后台管理面 |
| 0014 | 评论聚合键换轨为客户端派生的 comment_key | accepted | comment_key 改为客户端派生，服务端只存不解释 |
| 0015 | 评论删除范围与后台固定视图 | accepted | 作者删与治理删共用 scope，后台提供固定视图 |
| 0016 | 配置契约由 Go 单一来源生成 | accepted；部分被 0024 取代（生成物与桌面执行器） | 配置字段、默认值与校验规则由 Go 单一来源维护 |
| 0017 | 账号资料与凭据的生命周期收进模块 | accepted | 账号资料与凭据的生命周期收进模块 |
| 0018 | 操作审计的接入边界与有界捕获 | accepted | 审计只挂 /api，有界捕获响应信封 |
| 0019 | sidecar 的停止/重启以 Promise 表达 | superseded by 0024 | sidecar 停止/重启以 Promise 表达，新增 restarting |
| 0020 | Agent 文档分层与根 AGENTS 预算 | accepted | 根 AGENTS 只留会话规则，事实按层分家 |
| 0021 | 文档与分层门禁随 go test 运行 | accepted | docscheck 与 archtest 随 go test 运行 |
| 0022 | context 贯穿 service 与 repository | accepted | service/repository 请求作用域方法贯穿 ctx |
| 0023 | 版本化迁移采用嵌入 SQL + 基线收养 | accepted | 嵌入 SQL 的版本化迁移与基线收养 |
| 0024 | 后台收敛为内嵌 web 唯一表面 | accepted | 删除桌面端与 sidecar，web 后台补位数据管理 |
