package main

import (
	"ebook-server/config"
	"ebook-server/handler"
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/database"
	"ebook-server/pkg/logger"
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
	db.AutoMigrate(&model.User{}, &model.Comment{}, &model.OperationLog{})

	// 设置 Gin 模式
	gin.SetMode(config.AppConfig.Server.Mode)

	// 创建 Gin 引擎
	r := gin.New()

	// 注册中间件
	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		model.Success(c, gin.H{"status": "ok"})
	})

	// API 路由
	api := r.Group("/api")
	{
		// 认证相关（不需要登录）
		auth := api.Group("/auth")
		{
			authHandler := handler.NewAuthHandler()
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		// 用户相关（需要登录）
		users := api.Group("/users")
		users.Use(middleware.JWTAuth())
		{
			userHandler := handler.NewUserHandler()
			users.GET("/me", userHandler.GetMe)
			users.PUT("/me", userHandler.UpdateMe)
		}

		// 评论相关
		comments := api.Group("/comments")
		{
			commentHandler := handler.NewCommentHandler()
			comments.GET("", commentHandler.GetList)                              // 公开
			comments.POST("", middleware.JWTAuth(), commentHandler.Create)        // 需要登录
			comments.GET("/my", middleware.JWTAuth(), commentHandler.GetMyComments) // 需要登录
			comments.DELETE("/:id", middleware.JWTAuth(), commentHandler.Delete)   // 需要登录
		}

		// 日志相关（需要登录）
		logs := api.Group("/logs")
		logs.Use(middleware.JWTAuth())
		{
			logHandler := handler.NewLogHandler()
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

func init() {
	// 确保 logs 目录存在
	os.MkdirAll("logs", 0755)
}
