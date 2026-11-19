# 公开 API 默认只监听回环，代理头只信任回环

Status: accepted

公开 API 的默认监听地址改为 `127.0.0.1`，Gin 只信任来自 `127.0.0.1`/`::1` 的
`X-Forwarded-For`；公网入口由同机 Nginx 终止 TLS、限流后反代到应用端口。

## 背景

在此之前的拓扑是「应用直接绑 `0.0.0.0:<server.port>`，Gin 不信任任何代理头」：
在没有反向代理时这是最小暴露面，审计与后台登录限流都取连接对端地址，伪造
`X-Forwarded-For` 无效。

接入 Nginx HTTPS 后，这组默认值变成两个问题：

* 应用仍绑 `0.0.0.0`，公网可以绕过 Nginx 直接访问 `http://<host>:9090`，TLS、
  HSTS、边缘限流与访问日志全部失效；只靠防火墙/安全组兜底，漏配一次就裸奔。
* Nginx 反代后连接对端是回环，若继续不信任任何代理头，审计与请求日志里的来源
  IP 全部退化成 `127.0.0.1`，取证价值当场消失。

## 决定

1. `server.listen_addr` 默认 `127.0.0.1`。公开 API 只监听回环；脱离 Nginx 直连
   的部署必须显式改成 `0.0.0.0` 或 `::`，并自行承担明文与限流责任。
2. 公开引擎调用 `SetTrustedProxies([]string{"127.0.0.1", "::1"})`：只有回环代理
   转来的 `X-Forwarded-For` 才被采信；外部直连的 `RemoteAddr` 不是回环，代理头
   仍被忽略，客户端无法伪造审计 IP 与日志里的来源地址。后台引擎调用
   `SetTrustedProxies(nil)`：后台没有反向代理，远程管理走 SSH 隧道，隧道连接的
   对端就是回环；若采信回环代理头，隧道用户就能轮换 `X-Forwarded-For` 绕过按 IP
   的登录锁定分桶（ADR-0010）。
3. Nginx 用 `proxy_set_header X-Forwarded-For $remote_addr` 覆写而不是追加，
   避免把客户端自带的链带进应用；应用侧与边缘侧对「真实 IP」只有一个来源。
4. 请求体在两层设上限：Nginx `client_max_body_size` 挡公网，应用
   `middleware.BodyLimit` 兜住绕过 Nginx 或边缘配置漏项；普通 JSON 256 KiB，
   头像上传单独放宽到 5 MiB + multipart 余量。

## 边界

* 应用仍不终止 TLS：证书、HSTS、OCSP、HTTP/2 都归 Nginx（见 `deploy/nginx`）。
* 管理面仍独立监听 `127.0.0.1:9091`，不经过 Nginx，继续走 SSH 隧道/VPN。
* 边缘限流是「按 IP 的兜底」，不是 DDoS 清洗；大流量仍需要云高防/WAF。
* 应用层内存限流器仍是单进程语义；多实例部署需换共享存储。

## 被否方案

* **保持 `0.0.0.0` + 只靠防火墙/安全组**：默认值本身不安全，任何一次漏配、临时
  改配置或换部署方式都会把明文端口摊到公网；默认回环让「忘记配」也仍然是安全的。
* **信任任意来源的 `X-Forwarded-For`**：Gin 默认行为，任何客户端都能伪造审计 IP
  与限流 key；在反代后面也不能接受。
* **应用自己终止 TLS**：证书续期、TLS 版本/密码套件、HTTP/2、OCSP 都要在 Go 进程
  里再实现一遍，和 Nginx 的成熟配置重复；单机部署没有理由这样做。
* **只用 Nginx 限流、应用不做请求体上限**：边缘配置是外部文件，漏一行就失效；
  应用层上限是同一语义的第二道闸，成本很低。

## 后果

* 直接运行二进制（不经 Nginx）的用户需要显式配置 `server.listen_addr: 0.0.0.0`
  或 `::`，否则只能从本机访问；这是有意的安全默认。
* 反向代理必须存在且只连回环；Nginx 必须覆写 `X-Forwarded-For`，否则应用看到的
  仍是 `127.0.0.1`。
* 审计与请求日志里的来源 IP 在 Nginx 后面恢复为真实客户端 IP；后台登录限流
  仍按连接对端（SSH 隧道即回环）分桶，不跟随 `X-Forwarded-For`（ADR-0010）。
* Nginx 终止 TLS 后应用看不到 `https`：生产必须在 `.env` 钉死
  `SERVER_PUBLIC_BASE_URL=https://<域名>`，否则头像 URL 会退化成 `http://`。
* 部署侧新增 `deploy/` 目录：Nginx 站点模板、systemd unit（应用与审计清理）、
  logrotate 配置、审计清理脚本与 `deploy/README.md`（上线验证、备份与残余风险）。

锚点：`backend/config/schema.go` 的 `defaultServerListenAddr`；`backend/router/router.go`
的 `newEngine`；`backend/middleware/bodylimit.go` 的 `BodyLimit`；
`backend/router/router_test.go`；`deploy/nginx/ebook-server.conf`。