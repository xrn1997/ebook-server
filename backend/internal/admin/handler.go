package admin

import (
	"strconv"

	"ebook-server/model"
	"ebook-server/pkg/errcode"

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

// Handler 后台 API 处理器。薄薄一层：解析请求、调用能力、统一信封返回。
type Handler struct {
	users    UserReader
	comments CommentReader
	logs     LogReader
}

// NewHandler 创建后台处理器。
func NewHandler(users UserReader, comments CommentReader, logs LogReader) *Handler {
	return &Handler{users: users, comments: comments, logs: logs}
}

// loginRequest 后台登录请求体。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login 后台登录：校验管理端账号密码，返回管理端 token。
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "参数错误")
		return
	}
	token, err := Login(req.Username, req.Password)
	if err != nil {
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
