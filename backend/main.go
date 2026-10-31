package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"ebook-server/config"
	"ebook-server/handler"
	"ebook-server/internal/admin"
	"ebook-server/model"
	"ebook-server/pkg/code"
	"ebook-server/pkg/database"
	"ebook-server/pkg/jwt"
	"ebook-server/pkg/logger"
	"ebook-server/pkg/mail"
	"ebook-server/pkg/upload"
	"ebook-server/pkg/version"
	"ebook-server/repository"
	"ebook-server/router"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
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

// main 只做两件事：装配依赖，然后开两个监听并等退出。
//
// 路由表不在这里——它住在 router 包，有 interface 因而可被测试。AGENTS.md 说 main.go
// 是唯一装配点，指的是「把依赖递给谁」，不是「把三十行路由语句写在这里」。
func main() {
	// ── 装配依赖（ADR-0007）──────────────────────────────────────────────────
	// 配置作为一个值往下传：不再有包级全局，也没有「谁必须先加载」这条隐式前提。
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := logger.Init(cfg.Server.Mode); err != nil {
		log.Fatalf("Failed to init logger: %v", err)
	}
	defer logger.Info("Server exiting")

	// 打印构建版本信息（by ldflags 注入），便于运维核对发版产物
	fmt.Printf("ebook-server %s\n", version.String())

	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// 版本化迁移（ADR-0023）：新库执行 SQL 基线；AutoMigrate 时代的旧库先收养再标记基线。
	if err := database.Migrate(db, &model.User{}, &model.Comment{}, &model.OperationLog{}, &model.RefreshToken{}); err != nil {
		log.Fatalf("Failed to migrate schema: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)

	// 数据访问：gorm adapter，共享同一连接
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewRefreshTokenRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	logRepo := repository.NewLogRepository(db)

	// token 单元：用户端与后台端各持一把密钥，共用同一份签发/验签实现（ADR-0001/0010）
	access := jwt.New(cfg.JWT.Secret, cfg.JWT.ExpireMin)
	session := admin.NewSession(cfg.Admin)

	// 验证码存储与模块各全局唯一：注册/找回/注销共用同一个存储，
	// 而下发与校验必须同源（ADR-0008）。
	codeModule := service.NewVerificationCodes(code.NewStore(), newMailer(cfg))

	// 文件上传存储（ADR-0011）：供头像上传 handler 与用户资料更新（旧头像清理）共用
	uploadStore := upload.New(cfg.Upload.Dir, cfg.Server.PublicBaseURL)

	// 业务服务
	credentials := service.NewCredentials(userRepo, tokenRepo)
	authService := service.NewAuthService(userRepo, tokenRepo, codeModule, access, credentials)
	profileService := service.NewProfileService(userRepo, uploadStore)
	accountService := service.NewAccountService(userRepo, tokenRepo, commentRepo, codeModule, profileService)
	commentService := service.NewCommentService(commentRepo)
	logService := service.NewLogService(logRepo)

	// 公开 API（监听 0.0.0.0:<server.port>）
	engine := router.Public(router.Deps{
		Handlers: router.Handlers{
			Auth:    handler.NewAuthHandler(authService),
			User:    handler.NewUserHandler(profileService, authService),
			Account: handler.NewAccountHandler(accountService),
			Comment: handler.NewCommentHandler(commentService),
			Log:     handler.NewLogHandler(logService),
			Upload:  handler.NewUploadHandler(uploadStore),
		},
		Access:    access,
		Logs:      logService,
		AvatarDir: uploadStore.AvatarDir(),
		APIDocs:   cfg.APIDocs.Enabled,
	})

	// 后台管理系统：独立 Gin 引擎 + 独立监听地址（ADR-0009 表面分离、ADR-0010 网络隔离）。
	// 默认仅监听 127.0.0.1，公网物理上无法连接；远程管理走 SSH 隧道/VPN。
	admEngine := router.AdminConsole(router.AdminDeps{
		Session: session,
		Handler: admin.NewHandler(userRepo, commentService, logRepo, session),
	})

	// 刻意不用 r.Run()：它内部自建 http.Server 且不暴露出去，于是收不到停止信号时
	// 只能被直接终止——在途请求丢失、SQLite 只能靠自身日志文件兜底恢复。
	// 要与后台一样能被 Shutdown，就必须自己持有 Server。
	// 两个 Server 都设 ReadHeaderTimeout：公开监听绑的是 0.0.0.0:<port>，直接把
	// `:port` 挂到公网时，慢开发送请求头（Slowloris）能让连接池里的槽位无限期悬着。
	// 这里自建 Server 就是为了不把超时交给 r.Run() 的默认值。
	publicSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           engine,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	adminSrv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Admin.ListenAddr, strconv.Itoa(cfg.Admin.ListenPort)),
		Handler:           admEngine,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// 监听失败通过这条缓冲通道回报：容量 2 = 两个监听各一次，永远不会阻塞发送方。
	listenErr := make(chan error, 2)
	go serve(publicSrv, "Server", listenErr)
	go serve(adminSrv, "Admin server", listenErr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// 任一监听起不来（端口被占、地址不可绑）都要收场：另一个还在 Accept，
	// 但那是一半服务——后台活着、公开 API 没起来，客户端只会看到莫名其妙的失败。
	// 这里刻意不走 log.Fatalf：那会当场终止进程，在途请求丢失、数据库句柄没关
	// （Windows 上句柄不释放连 WAL 文件都删不掉），两种退出必须走同一条关闭路径。
	var startErr error
	select {
	case sig := <-stop:
		logger.Info("收到退出信号，开始优雅关闭", zap.String("signal", sig.String()))
	case startErr = <-listenErr:
		// stderr 无缓冲：os.Exit 之前 zap 的日志文件（带缓冲）可能来不及落盘，
		// 而这条正是运维唯一需要看到的失败原因。
		log.Printf("监听启动失败，关闭另一侧后退出: %v", startErr)
	}

	shutdown(publicSrv, adminSrv)
	if err := database.Close(db); err != nil {
		logger.Error("关闭数据库连接失败", zap.Error(err))
	}
	if startErr != nil {
		os.Exit(1)
	}
}

// shutdownGracePeriod 停止接受新连接后留给在途请求完成的时间。
// 4 秒给超时日志与关库留出余量；超过后放弃等待，避免进程无限期挂着。
const shutdownGracePeriod = 4 * time.Second

// readHeaderTimeout 读完请求头的上限。两个监听共用一份：公开监听面向网络，
// 慢请求头（Slowloris）会占住连接池；后台监听虽然默认只在回环，同一条规则
// 不必也不该分成两处写法——一处配了超时、另一处漏掉，是这类问题最常见的诞生方式。
const readHeaderTimeout = 10 * time.Second

// serve 启动一个 HTTP 监听；正常退出（Shutdown）不算错误，真实失败回报给 errs，
// 由 main 走优雅关闭后退出——它自己不做任何终止进程的动作。
func serve(srv *http.Server, label string, errs chan<- error) {
	fmt.Printf("%s starting on %s...\n", label, srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		errs <- err
	}
}

// shutdown 优雅停掉每个监听（停止接受新连接 + 等在途请求跑完）。
//
// 两个监听并行关闭并共享同一个预算：串行时第一个可能吃掉全部时间，第二个连
// Shutdown 都来不及调用。超过 shutdownGracePeriod 就放弃等待：继续挂着只会让进程管理器
// 直接终止进程，那时连这条日志都来不及落盘。数据库连接由调用方在此之后关闭。
func shutdown(servers ...*http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()

	var wg sync.WaitGroup
	for _, srv := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			if err := srv.Shutdown(ctx); err != nil {
				logger.Error("优雅关闭监听失败，可能有请求未跑完", zap.String("addr", srv.Addr), zap.Error(err))
			}
		}(srv)
	}
	wg.Wait()
}

// newMailer 按配置装配验证码邮件 adapter（ADR-0007：装配知识集中在 main.go）。
//
// SMTP 已配置 → 真实发送；未配置且非 release → 写日志（本地联调）；
// release 下未配置 SMTP → 仍装配 SMTP adapter，发送时显式报错，
// 避免在正规环境把验证码悄悄漏进日志。
func newMailer(cfg *config.Config) service.Mailer {
	if (cfg.SMTP.Host != "" && cfg.SMTP.Port != 0) || cfg.Server.Mode == config.ModeRelease {
		return mail.NewSMTPMailer(cfg.SMTP)
	}
	return mail.NewLogMailer()
}
