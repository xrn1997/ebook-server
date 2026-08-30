package main

import (
	"ebook-server/config"
	"ebook-server/handler"
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/database"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/logger"
	"ebook-server/pkg/mail"
	"ebook-server/repository"
	"ebook-server/service"
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

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

	// 业务服务
	authService := service.NewAuthService(userRepo, tokenRepo, codeStore, sender)
	userService := service.NewUserService(userRepo)
	accountService := service.NewAccountService(userRepo, tokenRepo, commentRepo, codeStore, sender)
	commentService := service.NewCommentService(commentRepo)
	logService := service.NewLogService(logRepo)

	// 创建 Gin 引擎
	r := gin.New()

	// 注册中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		errcode.Success(c, gin.H{"status": "ok"})
	})

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

		// 评论相关
		comments := api.Group("/comments")
		{
			commentHandler := handler.NewCommentHandler(commentService)
			comments.GET("", commentHandler.GetList)                                // 公开
			comments.POST("", middleware.JWTAuth(), commentHandler.Create)          // 需要登录
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

	// 启动服务器
	port := config.AppConfig.Server.Port
	fmt.Printf("Server starting on port %d...\n", port)
	if err := r.Run(fmt.Sprintf(":%d", port)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
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
