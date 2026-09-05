package main

import (
	"ebook-server/config"
	"ebook-server/handler"
	"ebook-server/internal/admin"
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/database"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/logger"
	"ebook-server/pkg/mail"
	"ebook-server/pkg/upload"
	"ebook-server/pkg/version"
	"ebook-server/repository"
	"ebook-server/service"
	"fmt"
	"log"
	"net/http"
	"os"

	// 引入 swag 生成的文档（swag init 产物），供 gin-swagger 提供 OpenAPI 3.0 spec。
	_ "ebook-server/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
)

// 以下为 swag 通用的 API 文档元信息（与各 handler 上的 Swagger 注释搭配，
// push swag init 扫描生成 docs/）。
//
// @title ebook-server API
// @version 1.0.0
// @description 电子书阅读器后端 API（账号/认证/评论/操作日志）。业务响应用统一信封 {code,error,data}，HTTP 恒为 200；需认证的接口用 Bearer access token。
// @BasePath /
//
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

func main() {
	// 加载配置
	if err := config.LoadConfig("config.yaml"); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 初始化日志
	if err := logger.Init(config.AppConfig.Server.Mode); err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Info("Server exiting")

	// 打印构建版本信息（by ldflags 注入），便于运维核对发版产物
	fmt.Printf("ebook-server %s\n", version.String())

	// 初始化数据库
	if err := database.Init(); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	// 自动迁移数据库表
	db := database.GetDB()
	db.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{})

	// 设置 Gin 模式
	gin.SetMode(config.AppConfig.Server.Mode)

	// ── 装配依赖（ADR-0007：main.go 是唯一装配点）───────────────────────────
	// 数据访问：gorm adapter，共享同一连接
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewRefreshTokenRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	logRepo := repository.NewLogRepository(db)

	// 验证码存储全局唯一（注册/找回/注销共用）；邮件按配置选择 adapter
	codeStore := code.NewStore()
	mailer := newMailer(config.AppConfig)

	// 验证码发送模块全局唯一，注入认证/账号两模块（ADR-0008）
	sender := service.NewVerificationCodeSender(codeStore, mailer)

	// 文件上传存储（ADR-0011）：供头像上传 handler 与用户资料更新（旧头像清理）共用
	uploadStore := upload.New(config.AppConfig.Upload.Dir)

	// 业务服务
	authService := service.NewAuthService(userRepo, tokenRepo, codeStore, sender)
	userService := service.NewUserService(userRepo, uploadStore)
	accountService := service.NewAccountService(userRepo, tokenRepo, commentRepo, codeStore, sender)
	commentService := service.NewCommentService(commentRepo)
	logService := service.NewLogService(logRepo)

	// 创建 Gin 引擎
	r := gin.New()

	// 注册中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	// 操作审计：把每次请求写入 operation_logs（供后台查看，排除 RequestBody 明文密码）
	r.Use(middleware.OperationLog(logRepo))
	r.Use(middleware.CORS())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		errcode.Success(c, gin.H{"status": "ok"})
	})

	// API 文档（Swagger UI + OpenAPI spec）：公开端口默认不提供（api_docs.enabled=false，
	// 防接口清单泄露，见 config.APIDocsConfig）。联调/内网开放时改配置为 true。
	if config.AppConfig.APIDocs.Enabled {
		registerSwagger(r)
	}

	// 文件上传（ADR-0011）：uploads/ 目录经 /uploads/* 公开访问（仅头像等公开资源）
	r.Static("/uploads", config.AppConfig.Upload.Dir)

	// API 路由
	api := r.Group("/api")
	{
		// 认证相关（不需要登录）
		auth := api.Group("/auth")
		{
			authHandler := handler.NewAuthHandler(authService)
			auth.POST("/send-code", authHandler.SendCode)
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/forgot-password/send-code", authHandler.ForgotPasswordSendCode)
			auth.POST("/forgot-password/reset", authHandler.ForgotPasswordReset)
			auth.POST("/logout", middleware.JWTAuth(), authHandler.Logout)
		}

		// 用户相关（需要登录）
		users := api.Group("/users")
		users.Use(middleware.JWTAuth())
		{
			userHandler := handler.NewUserHandler(userService, authService)
			users.GET("/me", userHandler.GetMe)
			users.PUT("/me", userHandler.UpdateMe)
			users.PUT("/me/password", userHandler.ChangePassword)

			accountHandler := handler.NewAccountHandler(accountService)
			users.GET("/me/data", accountHandler.ExportMyData)
			users.POST("/me/deletion/send-code", accountHandler.SendDeletionCode)
			users.POST("/me/deletion", accountHandler.DeleteAccount)
		}

		// 文件上传（ADR-0011）：头像两步提交的第一步「传文件拿 URL」
		uploadHandler := handler.NewUploadHandler(uploadStore)
		api.POST("/uploads/avatar", middleware.JWTAuth(), uploadHandler.UploadAvatar)

		// 评论相关
		comments := api.Group("/comments")
		{
			commentHandler := handler.NewCommentHandler(commentService)
			comments.GET("", commentHandler.GetList)                                // 公开
			comments.POST("", middleware.JWTAuth(), commentHandler.Create)          // 需要登录
			comments.POST("/migrate-key", middleware.JWTAuth(), commentHandler.MigrateKey) // 需要登录
			comments.GET("/my", middleware.JWTAuth(), commentHandler.GetMyComments) // 需要登录
			comments.DELETE("/:id", middleware.JWTAuth(), commentHandler.Delete)    // 需要登录
		}

		// 日志相关（需要登录）
		logs := api.Group("/logs")
		logs.Use(middleware.JWTAuth())
		{
			logHandler := handler.NewLogHandler(logService)
			logs.GET("", logHandler.GetList)
			logs.GET("/my", logHandler.GetMyLogs)
		}
	}

	// ── 后台管理系统（ADR-0009：独立表面，独立鉴权）───────────────────────
	// 后台与公开 API 拆成两个 Gin 引擎、两个监听地址，实现网络层隔离：
	// 公开 API 监听 0.0.0.0:<server.port>，后台默认仅监听 127.0.0.1:<admin.listen_port>，
	// 公网物理上无法连接后台。远程管理请走 SSH 隧道/VPN，不要直接开放后台端口。
	adminHandler := admin.NewHandler(userRepo, commentRepo, logRepo)

	admEngine := gin.New()
	// 后台引擎：只挂 Recovery + Logger。不挂 CORS（前端与后台同源嵌入，无需跨域）；
	// 不挂 OperationLog（后台自身流量刻意不入审计库，见 middleware/operationlog.go）。
	admEngine.Use(middleware.Recovery())
	admEngine.Use(middleware.Logger())

	adm := admEngine.Group("/admin")
	{
		// 后台 API
		adm.POST("/api/login", adminHandler.Login)

		api := adm.Group("/api")
		api.Use(admin.AuthMiddleware())
		{
			api.GET("/stats", adminHandler.Stats)
			api.GET("/users", adminHandler.ListUsers)
			api.GET("/comments", adminHandler.ListComments)
			api.GET("/logs", adminHandler.ListLogs)
		}

		// 内嵌前端（SPA 首页 + 静态资产 /admin/assets/*）
		adm.GET("", admin.ServeFrontend)
		adm.GET("/", admin.ServeFrontend)
		adm.GET("/assets/*filepath", admin.ServeAssets)
	}

	// 后台引擎始终挂 Swagger 文档：默认仅监听 127.0.0.1（本机），无公开风险；
	// 前端「API 文档」页以 iframe 内嵌 /api-docs/index.html，需同端口可访问。
	// 公开端口的文档由 api_docs.enabled 控制（默认关闭），两个表面互不影响。
	registerSwagger(admEngine)

	// 启动后台服务器（独立监听地址；goroutine 内运行，主协程继续启动公开 API）
	adminAddr := fmt.Sprintf("%s:%d", config.AppConfig.Admin.ListenAddr, config.AppConfig.Admin.ListenPort)
	adminSrv := &http.Server{Addr: adminAddr, Handler: admEngine}
	go func() {
		fmt.Printf("Admin server starting on %s...\n", adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start admin server: %v", err)
		}
	}()

	// 启动公开 API 服务器
	port := config.AppConfig.Server.Port
	fmt.Printf("Server starting on port %d...\n", port)
	if err := r.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// registerSwagger 在指定引擎上注册 API 文档路由（Swagger UI + OpenAPI spec）。
//
// 主引擎（公开 API）与后台引擎各挂一份：gin-swagger 对根路径（/api-docs、/api-docs/）
// 返回 404 是其固有行为，这里统一 301 到 index.html，保证浏览器直接打开 /api-docs
// 即可看到文档；后台前端「API 文档」页以 iframe 内嵌 /api-docs/index.html，同端口
// 注册才能加载（ADR-0009 端口分离后的适配）。
func registerSwagger(e *gin.Engine) {
	e.GET("/api-docs/*any", func(c *gin.Context) {
		if p := c.Param("any"); p == "" || p == "/" {
			c.Redirect(http.StatusMovedPermanently, "/api-docs/index.html")
			return
		}
		ginSwagger.WrapHandler(swaggerFiles.Handler)(c)
	})
}

// newMailer 按配置装配验证码邮件 adapter（ADR-0007：装配知识集中在 main.go）。
//
// SMTP 已配置 → 真实发送；未配置且非 release → 写日志（本地联调）；
// release 下未配置 SMTP → 仍装配 SMTP adapter，发送时显式报错，
// 避免在正规环境把验证码悄悄漏进日志。
func newMailer(cfg *config.Config) service.Mailer {
	if (cfg.SMTP.Host != "" && cfg.SMTP.Port != 0) || cfg.Server.Mode == "release" {
		return mail.NewSMTPMailer(cfg.SMTP)
	}
	return mail.NewLogMailer()
}

func init() {
	// 确保 logs 目录存在
	os.MkdirAll("logs", 0755)
}
