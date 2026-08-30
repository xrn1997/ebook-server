package admin

import (
	"embed"
	"io/fs"
	"net/http"

	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// webFS 内嵌的前端静态资源（ADR-0009）。
//
// 构建时由 Makefile 把 frontend/dist/* 灌入本目录（web/），
// `go build` 时经 //go:embed 打进单一二进制。gorm/embed 不允许跨目录 `..`，
// 故产物必须先入本目录再编译。
//
//go:embed all:web
var webFS embed.FS

// webSub 以 web/ 为根的静态文件系统（供 StaticFS 使用）。
var webSub, _ = fs.Sub(webFS, "web")

// ServeFrontend 返回 /admin 前端首页（内嵌 SPA 的 index.html）。
func ServeFrontend(c *gin.Context) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		errcode.Error(c, errcode.ServerError, "前端资源缺失，请先执行 make frontend-build")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// StaticFiles 返回可挂到路由的静态文件系统（资产 /assets 等）。
func StaticFiles() http.FileSystem {
	return http.FS(webSub)
}

// indexExists 判断当前构建是否已灌入前端产物（占位 index.html 也算）。
func indexExists() bool {
	_, err := webFS.ReadFile("web/index.html")
	return err == nil
}
