# ebook-server 桌面管理应用设计

## 概述

将现有 Go 后端服务封装为 Electron 桌面应用程序，提供全功能管理界面，支持 Windows/macOS/Linux 全平台。用户无需手动编辑配置文件或命令行操作，所有管理功能在桌面应用内完成。

## 核心需求

1. **服务管理**：启动/停止/重启 Go 后端服务
2. **配置管理**：通过 GUI 配置 SMTP、JWT 密钥、管理员账号等敏感参数，以及端口、模式、路径等基础参数
3. **运行监控**：实时查看服务状态、日志输出、数据库统计
4. **现有功能**：用户管理、评论管理、操作日志查看等现有后台功能

## 技术选型

| 组件 | 技术 | 理由 |
|------|------|------|
| 桌面框架 | Electron | 全平台支持成熟，生态丰富 |
| 前端 | Vue 3 + Vite | 与现有 admin 前端技术栈一致 |
| 后端 | Go（现有代码） | 零改动，sidecar 模式复用 |
| 打包 | electron-builder | 全平台安装包生成 |

## 架构设计

```
┌─────────────────────────────────────────────────┐
│                 Electron App                     │
│                                                  │
│  ┌──────────────┐    ┌───────────────────────┐  │
│  │  Main Process │    │  Renderer Process     │  │
│  │              │    │                       │  │
│  │  - 服务管理   │    │  管理界面 (Vue 3)      │  │
│  │  - 配置读写   │    │  - 概览/服务控制       │  │
│  │  - 日志收集   │    │  - 配置管理           │  │
│  │  - 系统托盘   │    │  - 用户/评论/日志     │  │
│  └──────┬───────┘    └───────────┬───────────┘  │
│         │                        │               │
│    spawn/kill               HTTP + IPC           │
│         │                        │               │
│  ┌──────▼───────┐                │               │
│  │ Go Sidecar   │◄───────────────┘               │
│  │ (子进程)      │  localhost:9090/9091           │
│  └──────────────┘                                │
└─────────────────────────────────────────────────┘
```

### 组件职责

**Electron Main Process**
- Go 子进程生命周期管理（spawn/kill/restart）
- 配置文件读写（config.yaml + .env）
- 系统托盘图标与菜单
- 日志收集与转发
- IPC 通道处理

**Electron Renderer Process**
- Vue 3 SPA 管理界面
- 通过 HTTP 调用 Go 后端 API（业务功能）
- 通过 IPC 调用 Main Process（服务控制、配置读写）

**Go Sidecar**
- 与现有 `ebook-server.exe` 完全相同，零改动
- 提供公开 API（:9090）和管理 API（:9091）
- 处理所有业务逻辑

## 界面设计

### 整体布局

侧边栏导航 + 内容区的经典桌面布局，功能按职责分组为独立页面：

```
┌──────────────────────────────────────────────────┐
│  ● ebook-server              [─] [□] [×]        │
├────────┬─────────────────────────────────────────┤
│        │                                         │
│  📊 概览 │   服务状态：运行中 ●                    │
│  ⚙ 配置 │   公开API：http://localhost:9090        │
│  👥 用户 │   管理后台：http://localhost:9091        │
│  💬 评论 │   数据库：ebook.db (12.3 MB)           │
│  📋 日志 │   运行时间：2h 34m                     │
│  🔧 服务 │                                       │
│        │   [ 重启服务 ]  [ 停止服务 ]              │
├────────┤                                         │
│        │                                         │
└────────┴─────────────────────────────────────────┘
```

### 页面清单

| 页面 | 职责 | 来源 |
|------|------|------|
| **概览** | 服务状态、端口、数据库大小、运行时间、快捷操作 | 新增 |
| **配置** | SMTP / JWT / 管理员账号等敏感配置 + 基础配置，分 Tab 展示 | 新增 |
| **用户** | 用户列表、搜索、详情 | 复用现有 Vue 代码 |
| **评论** | 评论列表、搜索、删除 | 复用现有 Vue 代码 |
| **日志** | 操作日志列表、筛选 | 复用现有 Vue 代码 |
| **服务** | 启动/停止/重启、实时日志流输出、端口检测 | 新增 |

### 配置页面结构

配置页面内部分 Tab：

- **邮件服务**：SMTP 主机/端口/账号/密码/发件人
- **安全密钥**：JWT Secret、Admin JWT Secret
- **管理员**：管理员用户名/密码
- **服务设置**：端口、运行模式、数据库路径、上传目录、API 文档开关

配置修改后点「保存」→ Electron Main 写入 config.yaml / .env → 提示「需要重启服务生效」→ 用户确认 → 重启 Go 进程。

## Go 服务生命周期管理

### 启动流程

1. Electron 启动 → 检测打包目录内的 Go 二进制路径（`process.resourcesPath + /backend/ebook-server`）
2. 以子进程方式 spawn Go 二进制，工作目录设为用户数据目录
3. 轮询 `http://localhost:9090/health` 直到返回 200，标记服务为「运行中」
4. 如果 10 秒内未就绪，标记为「启动失败」并展示错误日志

### 停止流程

1. 向 Go 子进程发送 SIGTERM（Windows 上用 `taskkill`）
2. 等待 5 秒优雅退出；超时则 SIGKILL 强杀
3. 释放端口，标记为「已停止」

### 重启

停止 → 启动，中间检查配置是否需要重载。

### 异常处理

- Go 进程意外崩溃 → Electron 检测到退出 → 自动尝试重启（最多 3 次，间隔递增）
- 端口冲突 → 从健康检查失败中识别 → 在界面上提示「端口被占用」

### 首次运行

- 如果用户数据目录下没有 config.yaml，从内置模板复制一份默认配置
- 如果 .env 不存在，创建空的 .env 文件

## 配置管理

### 文件策略

- `config.yaml`：基础配置，Electron Main 直接读写（用 `js-yaml` 库解析/序列化）
- `.env`：敏感配置（SMTP 密码、JWT Secret 等），Electron Main 以 `KEY=VALUE` 格式读写
- 两份文件都存放在用户数据目录（Electron 的 `app.getPath('userData')`），而非代码目录

### 用户数据目录

- Windows: `%APPDATA%/ebook-server/`
- macOS: `~/Library/Application Support/ebook-server/`
- Linux: `~/.config/ebook-server/`

### 配置读写流程

1. 用户打开配置页 → Electron Main 读取 config.yaml + .env → 通过 IPC 返回给 Renderer
2. 用户修改后点「保存」→ Renderer 通过 IPC 发送新配置 → Electron Main 分别写入两个文件
3. 界面提示「配置已保存，需要重启服务生效」
4. 用户点「重启」→ 走重启流程

### 配置校验

- 端口号范围检查（1-65535）
- SMTP 密码为空时提示「邮件功能将不可用」
- JWT Secret 为空时阻止保存并提示

## 目录结构

```
desktop/                    ← Electron 项目根目录
├── package.json
├── electron-builder.yml    ← 打包配置
├── src/
│   ├── main/               ← Electron Main Process
│   │   ├── index.ts        ← 入口：窗口创建、托盘、服务管理
│   │   ├── sidecar.ts      ← Go 子进程生命周期管理
│   │   ├── config.ts       ← config.yaml / .env 读写
│   │   └── ipc.ts          ← IPC 通道定义与处理
│   ├── renderer/           ← Vue 3 前端（扩展现有 frontend/）
│   │   ├── src/
│   │   │   ├── views/      ← 概览、配置、服务（新增）+ 用户/评论/日志（复用）
│   │   │   ├── api.js      ← 适配 Electron 环境（IPC + HTTP 混合）
│   │   │   └── ...
│   │   └── vite.config.ts
│   └── preload/            ← preload 脚本（暴露 IPC API 给 Renderer）
│       └── index.ts
└── resources/              ← Go 二进制打包目录
    └── backend/
        └── ebook-server    ← 编译好的 Go 二进制（按平台放置）
```

## 构建与打包

### 构建流程

1. `make desktop-build-backend` → 交叉编译 Go 二进制到 `desktop/resources/backend/`
2. `make desktop-build-frontend` → `npm run build` 编译 Vue 前端
3. `make desktop-package` → `electron-builder` 打包为各平台安装包

### 安装包格式

- Windows: `.exe` 安装包（NSIS）
- macOS: `.dmg`
- Linux: `.AppImage` + `.deb`

### 打包产物

最终安装包包含：
- Electron 运行时（~60-80MB）
- Go 后端二进制（~40MB）
- Vue 前端资源（~2-3MB）

总安装包大小约 100-120MB。

## 关键决策

| 决策 | 选择 | 理由 |
|------|------|------|
| Go 后端改动 | 零改动 | sidecar 模式，现有代码完全复用 |
| 前端框架 | Vue 3（扩展现有代码） | 与现有 admin 前端技术栈一致 |
| 配置文件位置 | 用户数据目录 | 不因应用更新丢失，各平台惯例路径 |
| 通信方式 | HTTP + IPC 混合 | HTTP 走业务 API，IPC 走服务控制/配置 |
| 打包工具 | electron-builder | 生态成熟，全平台支持好 |

## 测试策略

- **单元测试**：配置读写、服务管理逻辑
- **集成测试**：Electron + Go 通信、服务生命周期
- **E2E 测试**：关键用户流程（启动服务、修改配置、查看数据）

## 后续扩展

- 系统通知：服务异常时推送桌面通知
- 自动更新：检测新版本并提示下载
- 多语言支持：界面国际化
- 插件系统：扩展管理功能
