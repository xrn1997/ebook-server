// Package router 装配本服务的两张路由表：公开 API 与后台管理 API。
//
// 路由表是一段实现而不是一堆语句：住在 main() 里时它没有 interface，因此无法被
// 测试——handler 测试只能另抄一份「自己需要的几行路由」，真实装配是否漏挂中间件
// 没有任何用例能发现（见 ADR-0001 响应信封与认证模型、ADR-0010 后台网络隔离）。
// 本包的 interface 就是 Public 与 AdminConsole 两个函数：给依赖，拿 engine。
package router

import (
	_ "ebook-server/docs" // swag 生成的 OpenAPI spec

	"ebook-server/handler"
	"ebook-server/internal/admin"
	"ebook-server/middleware"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/upload"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Handlers 公开表面上挂的处理器集合。
type Handlers struct {
	Auth    *handler.AuthHandler
	User    *handler.UserHandler
	Account *handler.AccountHandler
	Comment *handler.CommentHandler
	Log     *handler.LogHandler
	Upload  *handler.UploadHandler
}

// Deps 装配公开 API 所需的全部依赖。
type Deps struct {
	Handlers Handlers
	// Access 用户 access token 的解析单元（*jwt.TokenBox 满足）。
	Access middleware.Parser
	// Logs 操作审计写入方；nil 表示不记录。
	Logs middleware.OperationRecorder
	// AvatarDir 头像文件的实际目录（upload.Store.AvatarDir()），经 /uploads/avatar 公开。
	// 只挂这一层而不是整个 upload.dir：静态服务会公开根目录下的每个文件，
	// 而服务端的工作目录里同时放着 config.yaml、.env 和数据库。
	AvatarDir string
	// APIDocs 是否在公开端口提供 Swagger 文档（默认关，防接口清单泄露）。
	APIDocs bool
}

// AdminDeps 装配后台 API 所需的全部依赖。
type AdminDeps struct {
	Session *admin.Session
	Handler *admin.Handler
}

// newEngine 创建两个表面共用的 Gin 引擎，并关掉代理头信任。
//
// 服务直接暴露，没有受控反向代理：Gin 默认信任任意来源的 X-Forwarded-For，
// 任何人都能把审计里的 IP 改成别的值，也能让后台登录限流按伪造 IP 分桶
// （ADR-0010 的限流因此形同虚设）。SetTrustedProxies(nil) 让 ClientIP 始终取
// 连接的对端地址。
func newEngine() *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	return r
}

// Public 组装公开 API 的 engine。
//
// 引擎级中间件固定为 RequestID → Recovery → Logger → CORS（ADR-0001）：
// request id 必须最先就位，panic 日志与响应头才带得上它。审计只挂在 /api 组上：
// 健康探针、Swagger 与头像静态文件不是「某人的一次操作」，不该经过审计中间件。
// 鉴权一律挂在**路由组**上，不逐路 inline——同一张表里两种挂法并存时，
// 新加一路很容易跟着错的那份写，且没有用例能发现它裸奔。
func Public(d Deps) *gin.Engine {
	r := newEngine()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	r.GET("/health", func(c *gin.Context) {
		errcode.Success(c, gin.H{"status": "ok"})
	})

	// API 文档：公开端口默认不提供（api_docs.enabled=false，防接口清单泄露）
	if d.APIDocs {
		registerSwagger(r)
	}

	// 头像文件（ADR-0011）：挂载点与 URL 拼接同源，都来自 upload 模块的常量，
	// 两处各写一份字面量时「上传返回的 URL 打不开」是必然结果。
	r.Static(upload.AvatarURLPrefix, d.AvatarDir)

	// 客户端业务请求统一挂在 /api 组下：审计中间件只装在这一层，
	// 上层（/health、/api-docs、/uploads/avatar）的流量天然不经过它。
	api := r.Group("/api")
	if d.Logs != nil {
		api.Use(middleware.OperationLog(d.Logs))
		// 审计必须包在 Recovery 外侧，否则 panic 时审计先落定、Recovery 之后才写
		// C0500 信封：这一行会被记成业务成功，且永远不进「只看失败」。
		// 引擎级 Recovery 仍保留给其余表面；/api 的 panic 由这一层先接住。
		api.Use(middleware.Recovery())
	}

	auth := api.Group("/auth")
	{
		auth.POST("/send-code", d.Handlers.Auth.SendCode)
		auth.POST("/register", d.Handlers.Auth.Register)
		auth.POST("/login", d.Handlers.Auth.Login)
		auth.POST("/refresh", d.Handlers.Auth.Refresh)
		auth.POST("/forgot-password/send-code", d.Handlers.Auth.ForgotPasswordSendCode)
		auth.POST("/forgot-password/reset", d.Handlers.Auth.ForgotPasswordReset)
	}

	// 需要登录的表面统一走 signedIn 这一组中间件
	signedIn := middleware.JWTAuth(d.Access)

	authed := api.Group("/auth")
	authed.Use(signedIn)
	authed.POST("/logout", d.Handlers.Auth.Logout)

	users := api.Group("/users")
	users.Use(signedIn)
	{
		users.GET("/me", d.Handlers.User.GetMe)
		users.PUT("/me", d.Handlers.User.UpdateMe)
		users.PUT("/me/password", d.Handlers.User.ChangePassword)

		users.GET("/me/data", d.Handlers.Account.ExportMyData)
		users.POST("/me/deletion/send-code", d.Handlers.Account.SendDeletionCode)
		users.POST("/me/deletion", d.Handlers.Account.DeleteAccount)
	}

	uploads := api.Group("/uploads")
	uploads.Use(signedIn)
	uploads.POST("/avatar", d.Handlers.Upload.UploadAvatar)

	// 评论：列表公开，其余需登录（同一个前缀下按鉴权分成两组）
	api.GET("/comments", d.Handlers.Comment.GetList)

	comments := api.Group("/comments")
	comments.Use(signedIn)
	{
		comments.POST("", d.Handlers.Comment.Create)
		comments.POST("/migrate", d.Handlers.Comment.MigrateKey)
		comments.GET("/my", d.Handlers.Comment.GetMyComments)
		comments.DELETE("/:id", d.Handlers.Comment.Delete)
	}

	// 只挂 /my：「读全站日志」在客户端表面不存在——审计记录里每人都有 IP 与
	// 时间线，跨用户的读取只放在后台（/admin/api/logs，ADR-0010 的隔离之后）。
	logs := api.Group("/logs")
	logs.Use(signedIn)
	logs.GET("/my", d.Handlers.Log.GetMyLogs)

	return r
}

// AdminConsole 组装后台 API 的 engine（独立进程内表面，ADR-0009/0010）。
//
// 只挂 RequestID + Recovery + Logger：不挂 CORS（前端与后台同源嵌入），也不挂
// OperationLog（后台自身流量刻意不入审计库；判定规则在 service.LogService.Record，
// 见 ADR-0018）。
func AdminConsole(d AdminDeps) *gin.Engine {
	r := newEngine()
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())

	adm := r.Group("/admin")
	{
		adm.POST("/api/login", d.Handler.Login)

		api := adm.Group("/api")
		api.Use(d.Session.AuthMiddleware())
		{
			api.GET("/stats", d.Handler.Stats)
			api.GET("/users", d.Handler.ListUsers)
			api.GET("/users/:uid", d.Handler.GetUser)
			api.GET("/comments", d.Handler.ListComments)
			api.DELETE("/comments/:id", d.Handler.DeleteComment)
			api.POST("/comments/rehash", d.Handler.RehashComments)
			api.GET("/logs", d.Handler.ListLogs)
		}

		// 内嵌前端（SPA 首页 + 静态资产 /admin/assets/*）
		adm.GET("", admin.ServeFrontend)
		adm.GET("/", admin.ServeFrontend)
		adm.GET("/assets/*filepath", admin.ServeAssets)
	}

	// 后台引擎无条件挂 Swagger：默认仅监听 127.0.0.1，无公开风险；
	// 后台前端「API 文档」页以 iframe 内嵌 /api-docs/index.html，需同端口可访问。
	registerSwagger(r)
	return r
}

// registerSwagger 在指定引擎上注册 API 文档路由（Swagger UI + OpenAPI spec）。
//
// 主引擎（公开 API）与后台引擎各挂一份：gin-swagger 对根路径（/api-docs、/api-docs/）
// 返回 404 是其固有行为，这里统一 301 到 index.html，保证浏览器直接打开 /api-docs
// 即可看到文档。
func registerSwagger(e *gin.Engine) {
	e.GET("/api-docs/*any", func(c *gin.Context) {
		if p := c.Param("any"); p == "" || p == "/" {
			c.Redirect(301, "/api-docs/index.html")
			return
		}
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	})
}
