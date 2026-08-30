package admin

import (
	"strconv"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/ratelimit"

	"github.com/gin-gonic/gin"
)

// UserReader 管理面账号只读访问（consumer-defined，ADR-0007）。
//
// 只列后台实际需要的方法，由 repository.UserRepository 满足。
type UserReader interface {
	Count() (int64, error)
	FindAll(page, pageSize int) ([]model.User, int64, error)
}

// CommentReader 管理面评论只读访问（consumer-defined，ADR-0007）。
//
// 由 repository.CommentRepository 满足。
type CommentReader interface {
	FindAll(page, pageSize int) ([]model.Comment, int64, error)
}

// LogReader 管理面操作日志只读访问（consumer-defined，ADR-0007）。
//
// 由 repository.LogRepository 满足。
type LogReader interface {
	FindAll(page, pageSize int) ([]model.OperationLog, int64, error)
}

// 后台登录限流参数（防暴力破解的应用层兜底）。
//
// 后台登录是暴力破解的主要攻击面（密码门若被撞开，用户隐私数据全量泄露），
// 必须做应用层兜底——即使后台已通过独立监听地址做了网络隔离，也要防
// 内网/本机侧的爆破。限流 key 用来源 IP，单进程内存实现（pkg/ratelimit）。
const (
	loginLimiterLimit  = 5           // 每个窗口最多 5 次尝试
	loginLimiterWindow = time.Minute // 固定窗口 1 分钟
	loginLockMessage   = "登录尝试过于频繁，请稍后再试"
)

// Handler 后台 API 处理器。薄薄一层：解析请求、调用能力、统一信封返回。
type Handler struct {
	users        UserReader
	comments     CommentReader
	logs         LogReader
	loginLimiter *ratelimit.Limiter
}

// NewHandler 创建后台处理器。
func NewHandler(users UserReader, comments CommentReader, logs LogReader) *Handler {
	return &Handler{
		users:        users,
		comments:     comments,
		logs:         logs,
		loginLimiter: ratelimit.New(loginLimiterLimit, loginLimiterWindow),
	}
}

// loginRequest 后台登录请求体。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 后台登录：校验管理端账号密码，返回管理端 token。
//
// 限流（ADR-0010）：按来源 IP 在固定窗口内限制**失败尝试**次数——请求到达先 Peek
// 是否已锁定（不计数），仅密码校验失败时记录。成功登录不消耗配额，避免合法管理员
// 频繁登录被误锁；超限返回 A0241，防暴力破解。
func (h *Handler) Login(c *gin.Context) {
	ip := c.ClientIP()
	if h.loginLimiter.Peek(ip) {
		errcode.Error(c, errcode.AttemptTooMany, loginLockMessage)
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "参数错误")
		return
	}
	token, err := Login(req.Username, req.Password)
	if err != nil {
		// 仅失败尝试计数（防爆破）
		h.loginLimiter.Allow(ip)
		errcode.Error(c, errcode.Forbidden, ErrCredInvalid.Error())
		return
	}
	errcode.Success(c, gin.H{"token": token})
}

// Stats 后台统计概览（用户数 + 评论数）。
func (h *Handler) Stats(c *gin.Context) {
	userCount, err := h.users.Count()
	if err != nil {
		errcode.Respond(c, err, "查询统计失败")
		return
	}
	_, commentTotal, err := h.comments.FindAll(1, 1)
	if err != nil {
		errcode.Respond(c, err, "查询统计失败")
		return
	}
	errcode.Success(c, gin.H{"users": userCount, "comments": commentTotal})
}

// ListUsers 后台用户列表（分页）。
func (h *Handler) ListUsers(c *gin.Context) {
	page, pageSize := paginate(c)
	users, total, err := h.users.FindAll(page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询用户失败")
		return
	}
	errcode.Success(c, gin.H{"list": users, "total": total})
}

// ListComments 后台评论列表（分页）。
func (h *Handler) ListComments(c *gin.Context) {
	page, pageSize := paginate(c)
	comments, total, err := h.comments.FindAll(page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询评论失败")
		return
	}
	errcode.Success(c, gin.H{"list": comments, "total": total})
}

// ListLogs 后台操作日志（请求审计）列表（分页）。
func (h *Handler) ListLogs(c *gin.Context) {
	page, pageSize := paginate(c)
	logs, total, err := h.logs.FindAll(page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询操作日志失败")
		return
	}
	errcode.Success(c, gin.H{"list": logs, "total": total})
}

// paginate 解析并规整分页参数（page≥1，pageSize 1..100，缺省 1/20）。
func paginate(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return page, pageSize
}
