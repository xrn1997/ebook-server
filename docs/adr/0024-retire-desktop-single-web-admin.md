# 后台收敛为内嵌 web 唯一表面

Status: accepted

桌面端曾以 Electron sidecar 复用同一 Go 后端。为了把后端维持在“单二进制 + Web 后台”的轻量形态，后台管理面收敛为内嵌 web 唯一表面：删除 `desktop/`（已删）与 sidecar 生命周期，把用户/评论/日志管理能力移植进 `frontend/`；配置与服务控制不进入浏览器。

## 背景

桌面端为“零命令行管理”引入了 sidecar 进程生命周期、跨语言配置契约、Electron 打包与发版脚本，以及 Go 测试对桌面源码的漂移校验。后台管理 API 与内嵌 web 前端已经能独立完成数据管理，桌面端的增量价值不足以抵消这些维护面。

## 决定

- `desktop/`（已删）整体移除；后台只有内嵌 web 一个管理表面。
- 用户/评论/日志的搜索、筛选、分页、详情、删除由 `frontend/` 承担，调用同源 `/admin/api`。
- 管理 token 存浏览器 `localStorage`，鉴权失败清除并回登录页；管理员凭据不进入前端。
- 配置编辑、服务启停、运行监控不迁移；配置仍由部署者通过 `config.yaml`/`.env` 维护，进程由 systemd 或直接启动托管。
- 后端只保留 SIGINT/SIGTERM 优雅退出；stdin `shutdown` 协议随 sidecar 移除。
- Windows 独立服务端构建保留 PE 版本信息；桌面安装包、签名与发版校验脚本不再提供。

## 边界

- `/admin/api` 与公开 API 的契约不变；管理面的独立 Gin 引擎、独立 JWT 与回环监听不变（ADR-0010）。
- Docker 部署的移除是另一项范围决定，由 `README.md` 的部署一节记录当前形态，不由此篇 ADR 追溯。
- 历史 ADR 保留原文；设计说明与实施计划等过程工件只留在工作区、整目录不入库。两者都不是当前事实源。

## 后果

- 失去 GUI 配置编辑与 sidecar 服务控制；服务端定位下由部署工具承担。
- 管理 token 进入浏览器存储，安全性弱于“令牌不出主进程”；后台回环监听与短期令牌是配套缓解。
- 后台前端新增 TypeScript + Vitest 门禁；桌面端的真后端集成与 E2E 不再存在，真实联调改为手工烟测。
- 后台前端与 Go 测试互不包含，两边都要绿。

## 被否方案

- 保留 web 与桌面两套后台：同一管理 API 两份 UI、两份测试，长期必然漂移。
- 桌面端退化为 web 壳：登录、路由与配置页都要重做，收益不明显。
- 把配置编辑与服务控制搬进浏览器：把密钥与进程管理暴露到更弱的信任边界，超出“数据管理”范围。

锚点：`frontend/src/api.ts`；`frontend/src/useAdminList.ts`；`backend/router/router.go`；`docs/architecture.md` 的「系统形态」。
