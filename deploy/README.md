# 生产部署：Nginx HTTPS + 单机 API

这套文件对应单机拓扑：

```
Android APP
   │  HTTPS 443
   ▼
Nginx（TLS 终止、边缘限流、访问日志、安全响应头）
   │  HTTP 127.0.0.1:9090
   ▼
ebook-server（公开 API；后台 127.0.0.1:9091 只走 SSH 隧道/VPN）
```

应用**不监听公网**：`server.listen_addr` 与 `admin.listen_addr` 默认都是 `127.0.0.1`。
公网只暴露 Nginx 的 80/443；9090/9091 由云安全组与主机防火墙双重拒绝。

本文只覆盖**本项目**的部署面（配置、Nginx 站点、systemd、日志、审计清理、备份）。
主机本身的安全（SSH 加固、防火墙规则、fail2ban、账号治理）属于运维基线，
不在本仓维护。

## 0. 前置条件

干净主机先装齐工具，本手册后续命令都假定它们存在：

```bash
sudo apt-get update
sudo apt-get install -y nginx logrotate sqlite3
```

- **Nginx ≥ 1.25.1**：站点配置用了 `http2 on;`（1.25.1 起的写法）。更老的发行版
  要改回 `listen 443 ssl http2;`，否则 `nginx -t` 直接失败。
- **二进制来自构建**：`build/` 不入库。发布机上先 `make all`（产出
  `build/ebook-server`），再把二进制连同 `config.yaml` 一起带到部署目录；
  `.env` 永远不随仓库分发，按 §2 在目标机上现场创建。
- `sqlite3` 供 §5 审计清理与 §8 在线备份使用；`logrotate` 包提供 `/etc/cron.daily` 钩子。

## 1. 目录与账号

```bash
sudo useradd --system --home /opt/ebook-server --shell /usr/sbin/nologin ebook
sudo install -d -o ebook -g ebook -m 750 /opt/ebook-server
sudo install -d -o ebook -g ebook -m 700 /opt/ebook-server/logs /opt/ebook-server/uploads

sudo install -o root -g ebook -m 640 config.yaml /opt/ebook-server/config.yaml
sudo install -o ebook -g ebook -m 600 .env /opt/ebook-server/.env
sudo install -o ebook -g ebook -m 755 build/ebook-server /opt/ebook-server/ebook-server
```

`config.yaml` 是公开配置；`.env` 里是 JWT/后台口令等密钥，权限必须是 `600`。
数据库 `ebook.db`、WAL/SHM、`logs/`、`uploads/` 都由进程在部署目录写入，systemd
的 `UMask=0077` 会保证新文件默认仅属主可读。

**真实密钥只放 `/opt/ebook-server/.env`**。不要把生产 `.env` 留在开发工作区：那里的
副本会被备份、依赖安装脚本和 coding agent 读到；工作区里放占位值即可。若工作区曾经
被不可信代码执行过，上线前轮换 `JWT_SECRET`、`ADMIN_JWT_SECRET` 与 `ADMIN_PASSWORD`。

## 2. 生产配置

`config.yaml` 至少确认：

```yaml
server:
  port: 9090
  listen_addr: 127.0.0.1   # 不要改成 0.0.0.0
  mode: debug              # 由 .env 的 SERVER_MODE=release 覆盖
  public_base_url: ""      # 由 .env 的 SERVER_PUBLIC_BASE_URL 覆盖

api_docs:
  enabled: false           # 公开面不要开 Swagger

admin:
  listen_addr: 127.0.0.1   # 后台只走 SSH 隧道/VPN
  listen_port: 9091
```

生产 `.env` 示例（密钥必须替换为真实值；带默认值的键也建议显式钉死）：

```dotenv
SERVER_MODE=release
SERVER_LISTEN_ADDR=127.0.0.1
SERVER_PUBLIC_BASE_URL=https://<你的域名>
JWT_SECRET=<至少 32 字节随机串>
ADMIN_JWT_SECRET=<至少 32 字节随机串>
ADMIN_PASSWORD=<强口令>
SMTP_PASSWORD=<邮箱服务商授权码>
DATABASE_PATH=ebook.db
```

`release` 模式会拒绝仓库内置默认值与 `.env.example` 里的占位串。`SERVER_PUBLIC_BASE_URL`
必须钉死为 HTTPS origin：应用在 Nginx 后面看不到 TLS（`c.Request.TLS == nil`），留空会让
头像 URL 拼成 `http://`，同时给伪造 Host 留入口。

SMTP 也要一起确认：`release` 模式不会拒绝未配置的 SMTP，但注册/找回密码的验证码
会发不出去。先用真实发信账号覆盖 `SMTP_HOST`/`SMTP_PORT`/`SMTP_USERNAME`/
`SMTP_PASSWORD`/`SMTP_FROM`，再验证一次收信；不发信的功能也应在 `.env` 里显式写明，
不要留着 `smtp.example.com` 让运行时才发现。

## 3. Nginx

站点模板是 `deploy/nginx/ebook-server.conf`：先把里面的 `book.example.com` 换成你的
域名、证书路径换成实际部署的文件，再安装：

```bash
sudo install -o root -g root -m 600 deploy/nginx/ebook-server.conf \
  /etc/nginx/sites-available/ebook-server.conf
sudo ln -sfn ../sites-available/ebook-server.conf /etc/nginx/sites-enabled/ebook-server.conf
sudo nginx -t
sudo systemctl reload nginx
```

配置做了这些事：

- HTTP 301 到 HTTPS apex，`www` 也 301 到 apex；
- TLS 1.2/1.3、HSTS、OCSP stapling、session cache、关闭 session tickets；
- `client_max_body_size` 默认 256 KiB，头像上传单独 7 MiB；
- 登录/注册/重置 10 次/分钟/IP、发码 3 次/分钟/IP、头像上传 30 次/分钟/IP、
  其余 API 20 次/秒/IP，按 IP 限制并发连接；
- 429/413 仍返回统一信封（HTTP 200 + `A0241`/`A0400`）；429 带 `X-Rate-Limited` 响应头
  与访问日志的 `rate_limited=` 字段留可观测信号，413 是体积超限不是频控、不打该头；
- 不代理 `/admin`、`/api-docs`、`/config.yaml`、`/ebook.db`、`/logs`、点文件；
- 反向代理只发 `X-Forwarded-For $remote_addr`，不追加客户端自带值；公开引擎只信任
  回环代理，后台引擎不信任任何代理头（SSH 隧道对端即回环）。

Nginx 主配置建议同时调高连接上限（`/etc/nginx/nginx.conf`）：

```nginx
worker_rlimit_nofile 65535;
events {
    worker_connections 8192;
    multi_accept on;
}
```

先 `sudo nginx -t` 再 reload。证书按你选用的 CA/签发方式部署；到期前必须续期并
reload，把到期日放进日历提前 30 天处理。

## 4. systemd

```bash
sudo install -o root -g root -m 644 deploy/systemd/ebook-server.service \
  /etc/systemd/system/ebook-server.service
sudo systemctl daemon-reload
sudo systemctl enable --now ebook-server
sudo systemctl status ebook-server --no-pager
```

unit 以 `ebook` 用户运行，`ProtectSystem=strict`、`ProtectHome=true`、`NoNewPrivileges=true`，
只允许写 `/opt/ebook-server`，并设 `UMask=0077`。

应用日志（`logs/app.log`、`logs/error.log`）由 zap 持续写入，没有内置轮转；安装
logrotate 配置按天轮转（或超 50M 即轮转）、保留 14 天。仓库内文件带 `.conf` 后缀
以躲开根 `.gitignore` 的 `ebook-server` 二进制忽略规则，装到系统时去掉：

```bash
sudo install -o root -g root -m 644 deploy/logrotate/ebook-server.conf /etc/logrotate.d/ebook-server
sudo logrotate -d /etc/logrotate.d/ebook-server   # 先 dry-run 检查
```

## 5. 审计保留（防磁盘写满）

每次 `/api` 请求都会写一条 `operation_logs`。持续流量或攻击下，这张表会先于磁盘
写满；用定时器每天清理超过 30 天的记录：

```bash
sudo install -o root -g ebook -m 750 deploy/scripts/prune-audit.sh /opt/ebook-server/prune-audit.sh
sudo install -o root -g root -m 644 deploy/systemd/ebook-audit-prune.service /etc/systemd/system/
sudo install -o root -g root -m 644 deploy/systemd/ebook-audit-prune.timer /etc/systemd/system/
sudo systemctl daemon-reload
# 先启动应用、让 ebook.db 建出来，再启用清理定时器；否则 timer 会因库不存在而失败。
sudo systemctl enable --now ebook-audit-prune.timer
```

保留天数用 `AUDIT_RETENTION_DAYS` 覆盖（systemd unit 的 `Environment=` 或手工执行）。
SQLite 删除后空间会被 WAL 复用，不需要每天 `VACUUM`；磁盘紧张时再手工执行一次。

## 6. 网络暴露面

- 云安全组与主机防火墙只放行 SSH/80/443；**9090/9091 不对公网开放**。应用默认只绑
  回环，防火墙是配置写错、临时改 `listen_addr` 时的第二道防线。
- 后台管理不经过 Nginx，走 SSH 隧道访问：
  `ssh -L 9091:127.0.0.1:9091 <你的主机>`，然后浏览器开 `http://127.0.0.1:9091/admin`。
  后台引擎不采信 `X-Forwarded-For`，隧道用户的代理头不会改变登录限流的分桶。
- SSH 与主机账号的加固（密钥治理、sshd 收紧、暴力破解封禁）属于主机运维基线，
  按你的组织规范执行，不在本仓提供。

## 7. 上线验证

```bash
# 应用只在回环；从公网应连接失败
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9090/health
curl -sS -o /dev/null -w '%{http_code}\n' https://<你的域名>/health

# 业务信封与 TLS
curl -sS https://<你的域名>/api/comments?page=1
curl -sSI https://<你的域名>/ | grep -Ei 'strict-transport|x-content-type|x-frame|server'

# 限流：连续打登录，观察 429 被改写成 200 + A0241 + X-Rate-Limited
for i in $(seq 1 12); do
  curl -sS -D- -o /dev/null https://<你的域名>/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"email":"nobody@example.com","password":"bad-password"}'
done

# 确认 9090/9091 没有公网监听（应按防火墙/绑定地址失败）
ss -lntp | grep -E ':(9090|9091)\b'
```

预期：`/health` 返回 `{"code":"00000",...}`（成功码是五位 `00000`，不是 `A0000`）；
`/` 与 `/admin` 返回 404；`/.env`、`/config.yaml` 返回 404；快速连打登录时前 6 次左右
返回凭据类业务错误，之后触发边缘限流（10r/m + burst=5），响应变成 `A0241` 且带
`X-Rate-Limited: 1`。

## 8. 备份与恢复

审计清理防的是磁盘写满，防不了“库坏了 / 被删了 / 主机被勒索”。备份必须成对：
`ebook.db` 里存着头像 URL，丢了 `uploads/` 文件，恢复后所有头像 404。SQLite 在
WAL 模式下直接 `cp ebook.db` 可能拿到不一致快照，用 `sqlite3` 的在线备份；
`.backup` 以进程 umask 落盘（实测 umask 022 下产物是 0644），**必须显式收紧权限**。

```bash
sudo install -d -o ebook -g ebook -m 700 /opt/ebook-server/backups
# umask 077 让本次会话创建的一切都是 600/700
( umask 077
  sudo -u ebook sqlite3 /opt/ebook-server/ebook.db \
    ".backup '/opt/ebook-server/backups/ebook-$(date +%F).db'"
  sudo -u ebook tar -C /opt/ebook-server -czf \
    "/opt/ebook-server/backups/uploads-$(date +%F).tar.gz" uploads
)
# 兜底：既有副本权限统一收紧（备份目录本身 700，但副本会被 rsync 带走）
sudo -u ebook chmod 600 /opt/ebook-server/backups/*
# 本地保留 14 天；异地副本用 rsync/rclone，别和主机放在同一个故障域
sudo -u ebook find /opt/ebook-server/backups -name 'ebook-*.db' -mtime +14 -delete
sudo -u ebook find /opt/ebook-server/backups -name 'uploads-*.tar.gz' -mtime +14 -delete
```

备份里有邮箱、评论与审计记录，传异地前先加密（`age`/`gpg`）。
恢复时先停应用，避免残留 WAL 把旧页写回；`.env` 是**现场密钥，刻意不进备份**——
恢复后按 §2 重新创建，密钥若怀疑随旧主机泄露则轮换：

```bash
sudo systemctl stop ebook-server
sudo -u ebook cp /opt/ebook-server/backups/ebook-YYYY-MM-DD.db /opt/ebook-server/ebook.db
sudo -u ebook rm -f /opt/ebook-server/ebook.db-wal /opt/ebook-server/ebook.db-shm
sudo -u ebook tar -C /opt/ebook-server -xzf /opt/ebook-server/backups/uploads-YYYY-MM-DD.tar.gz
sudo systemctl start ebook-server
```

监控最小集：

- 外部探活 `GET https://<你的域名>/health`（云监控/Uptime Kuma），异常短信/邮件；
- 磁盘与 `ebook.db` 大小告警（审计表在攻击流量下增长最快）；
- 证书到期日进日历，提前 30 天续期。

## 9. 残余风险

- **单机 Nginx 挡不住大流量 DDoS**：按 IP 限流对代理池/僵尸网络无效。需要云厂商
  DDoS 高防或 WAF 做流量清洗与 L7 规则；本机只负责应用层兜底。
- **应用层内存状态**：验证码、限流器、后台登录锁定都是单进程内存；多实例部署要换 Redis。
- **审计表仍会增长**：定时器按 30 天清理，但攻击流量足够大时仍可能在两次清理之间
  写满磁盘；需要监控 `ebook.db` 大小与磁盘余量，并考虑把审计写入异步化/外置。
- **access token 无法主动作废**：改密/注销只撤 refresh token，旧 access token 活到过期。
- **管理端 token 在浏览器 `localStorage`**：后台只应经 SSH 隧道/VPN 访问；后台页面
  没有 CSP，管理 UI 一旦出现 XSS，token 可被直接读走。
- **Web 层不按 IP 封禁**：只有 Nginx 与应用自己的限流；扫描器会持续打 `/`、`/.env`、
  `/api/*`。要封源 IP 需要额外的 nginx jail 或云 WAF。
- **没有异地备份与告警**：单机磁盘损坏、误删或勒索会直接丢数据；最小方案见 §8。
- **证书是手工部署**：没有 certbot/acme 定时任务时，必须监控到期日并人工续期。
