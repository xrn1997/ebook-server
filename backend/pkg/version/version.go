// Package version 集中承载编译期注入的构建元信息（版本号/提交/构建时间）。
//
// 这些值在构建时通过 ldflags -X 注入（见 Makefile 的 LDFLAGS），
// 直接读源码跑（go run / 未注入）时回退为默认占位值，便于本地开发。
package version

import "fmt"

// 以下三个变量由构建工具通过 -ldflags "-X <pkg>.Version=..." 注入。
// 命名采用大写，与外层 go build -X 注入的公共导出变量约定一致。
var (
	// Version 语义化版本号，如 v0.0.1；未注入时为 dev。
	Version = "dev"
	// Commit 构建时的 git 短提交哈希；未注入时为 unknown。
	Commit = "unknown"
	// BuildTime UTC 时间的 YYYYMMDDHHMMSS；未注入时为 unknown。
	BuildTime = "unknown"
)

// String 返回一行人类可读的版本摘要，常在启动日志中打印以便运维核对产物。
func String() string {
	return fmt.Sprintf("version=%s commit=%s buildTime=%s", Version, Commit, BuildTime)
}