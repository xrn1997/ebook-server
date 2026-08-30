package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"ebook-server/pkg/errcode"

	"github.com/gin-gonic/gin"
)

// webFS 内嵌的前端静态资源（ADR-0009）。
//
// 构建时由 Makefile 把 frontend/dist/* 镜象进本目录（web/），`go build` 时经
// //go:embed 打进单一二进制。gorm/embed 不允许跨目录 `..`，故产物必须先入本目录再编译。
// 未构建前端时 web/ 仅含 .gitkeep，/admin 会返回"前端资源缺失"提示。
//
//go:embed all:web
var webFS embed.FS

// serveFS 以 web/ 为根的文件系统（供静态子路径访问用；web 根本身用于 index）。
var serveFS, _ = fs.Sub(webFS, "web")

// ServeFrontend 返回 /admin 前端首页（内嵌 SPA 的 index.html）。
func ServeFrontend(c *gin.Context) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		errcode.Error(c, errcode.ServerError, "前端资源缺失，请先执行 make frontend-build 后重新构建")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

// ServeAssets 处理 /admin/assets/*：从内嵌的 web/assets 目录取文件。
//
// SPA 的 index.html 以 base=/admin/ 引用 /admin/assets/*；这里把剩余路径定位到
// web/assets 下的同名文件，避免 StaticFS 在剥前缀后找不到 assets/ 层级的问题。
func ServeAssets(c *gin.Context) {
	reqPath := c.Param("filepath")
	if reqPath == "" || strings.Contains(reqPath, "..") {
		c.Status(http.StatusNotFound)
		return
	}
	// path.Join 会清理掉多余的 / 与 .；assets/ 为固定前缀
	name := path.Join("assets", reqPath)
	// 防止逃逸出 assets 目录（最终再对 ../../ 做一次防御）
	if name != path.Clean("assets/"+reqPath) || strings.HasPrefix(name, "../") {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := fs.ReadFile(serveFS, name)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Data(http.StatusOK, mimeType(name, data), data)
}

// mimeType 返回静态资源的 Content-Type。
//
// 不能只用 http.DetectContentType：它对 .js 只能识别出 text/plain，而 ES module 要求
// JS 必须是 text/javascript，否则浏览器会拒绝执行脚本（导致 SPA 白屏）。故对常见
// 扩展名用固定映射，其余回退到 DetectContentType。
func mimeType(name string, data []byte) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".woff", ".woff2":
		return "font/woff2"
	case ".map":
		return "application/json"
	default:
		return http.DetectContentType(data)
	}
}
