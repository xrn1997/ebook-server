package admin

import (
	"strconv"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/ratelimit"
	"ebook-server/repository"

	"github.com/gin-gonic/gin"
)

// UserStore 管理面对账号的访问能力（consumer-defined，ADR-0007）。
//
// 只列后台实际需要的方法，由 repository.UserRepository 满足。
// 全量列表不单列方法：Search 传空关键字即不参与过滤（两份方法只差一个 WHERE 形式）。
type UserStore interface {
	Count() (int64, error)
	Search(keyword string, page, pageSize int) ([]model.User, int64, error)
	FindByUID(uid uint) (*model.User, error)
}

// CommentStore 管理面对评论的访问与治理能力（consumer-defined，ADR-0007）。
//
// 由 repository.CommentRepository 满足。含 Delete：后台的评论治理（删违规内容）
// 不校验归属，这正是它与公开端点的区别，故接口不再叫 Reader。
type CommentStore interface {
	Count() (int64, error)
	Search(q model.CommentQuery, page, pageSize int) ([]model.Comment, int64, error)
	FindByID(id uint) (*model.Comment, error)
	CountByUserID(userID uint) (int64, error)
	Delete(id uint) error
}

// LogStore 管理面对操作日志的访问能力（consumer-defined，ADR-0007）。
//
// 由 repository.LogRepository 满足。全量列表同样是 FindBy 的零值条件特例。
type LogStore interface {
	FindBy(filter model.LogFilter, page, pageSize int) ([]model.OperationLog, int64, error)
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
	users        UserStore
	comments     CommentStore
	logs         LogStore
	loginLimiter *ratelimit.Limiter
}

// NewHandler 创建后台处理器。
func NewHandler(users UserStore, comments CommentStore, logs LogStore) *Handler {
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
//
// @Summary 后台登录
// @Tags 管理后台
// @Accept json
// @Produce json
// @Success 200 {object} model.Response
// @Router /admin/api/login [post]
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
//
// @Summary 后台统计概览
// @Tags 管理后台
// @Produce json
// @Success 200 {object} model.Response
// @Router /admin/api/stats [get]
func (h *Handler) Stats(c *gin.Context) {
	userCount, err := h.users.Count()
	if err != nil {
		errcode.Respond(c, err, "查询统计失败")
		return
	}
	commentCount, err := h.comments.Count()
	if err != nil {
		errcode.Respond(c, err, "查询统计失败")
		return
	}
	errcode.Success(c, gin.H{"users": userCount, "comments": commentCount})
}

// ListUsers 后台用户列表（分页 + 关键字搜索）。
//
// keyword 模糊匹配邮箱/用户名/昵称，纯数字时兼命中 UID；缺省即全量。
//
// @Summary 后台用户列表
// @Tags 管理后台
// @Produce json
// @Param keyword query string false "关键字（模糊匹配邮箱/用户名/昵称，纯数字兼命中 UID）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	page, pageSize := paginate(c)
	users, total, err := h.users.Search(c.Query("keyword"), page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询用户失败")
		return
	}
	errcode.Success(c, gin.H{"list": users, "total": total})
}

// GetUser 后台用户详情（账号资料 + 该用户评论数）。
//
// 后台 User 序列化含 email（管理员本就能看到），Password/LockedUntil 仍由
// model 上的 `json:"-"` 挡住，不经此端点出网。
//
// @Summary 后台用户详情
// @Tags 管理后台
// @Produce json
// @Param uid path int true "用户 UID"
// @Success 200 {object} model.Response
// @Router /admin/api/users/{uid} [get]
func (h *Handler) GetUser(c *gin.Context) {
	uid, paramErr := pathUint(c, "uid")
	if paramErr != "" {
		errcode.Error(c, errcode.BadRequest, paramErr)
		return
	}

	user, err := h.users.FindByUID(uid)
	if err != nil {
		if repository.IsRecordNotFound(err) {
			errcode.Error(c, errcode.AccountNotFound, "账号不存在")
			return
		}
		errcode.Respond(c, err, "查询用户失败")
		return
	}

	commentCount, err := h.comments.CountByUserID(uid)
	if err != nil {
		errcode.Respond(c, err, "查询用户评论数失败")
		return
	}
	errcode.Success(c, gin.H{"user": user, "comment_count": commentCount})
}

// ListComments 后台评论列表（分页 + 内容关键字 + 书名过滤）。
//
// @Summary 后台评论列表
// @Tags 管理后台
// @Produce json
// @Param keyword query string false "内容关键字（模糊匹配）"
// @Param book_name query string false "书名过滤"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/comments [get]
func (h *Handler) ListComments(c *gin.Context) {
	page, pageSize := paginate(c)
	query := model.CommentQuery{
		Keyword:  c.Query("keyword"),
		BookName: c.Query("book_name"),
	}
	comments, total, err := h.comments.Search(query, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询评论失败")
		return
	}
	errcode.Success(c, gin.H{"list": comments, "total": total})
}

// DeleteComment 后台删除评论（治理违规内容，软删除）。
//
// 与公开 DELETE /api/comments/:id 的区别是不做归属校验——管理员代用户删正是
// 治理的定义。评论不存在返回 A0304（与公开端点同一业务码，客户端无需区分来源）。
//
// @Summary 后台删除评论（治理）
// @Tags 管理后台
// @Produce json
// @Param id path int true "评论 ID"
// @Success 200 {object} model.Response
// @Router /admin/api/comments/{id} [delete]
func (h *Handler) DeleteComment(c *gin.Context) {
	id, paramErr := pathUint(c, "id")
	if paramErr != "" {
		errcode.Error(c, errcode.BadRequest, paramErr)
		return
	}

	if _, err := h.comments.FindByID(id); err != nil {
		if repository.IsRecordNotFound(err) {
			errcode.Error(c, errcode.CommentNotFoundErr, "评论不存在")
			return
		}
		errcode.Respond(c, err, "查询评论失败")
		return
	}
	if err := h.comments.Delete(id); err != nil {
		errcode.Respond(c, err, "删除评论失败")
		return
	}
	errcode.SuccessMsg(c, "删除成功", nil)
}

// ListLogs 后台操作日志（请求审计）列表：分页 + 方法/路径/账号/业务码筛选。
//
// @Summary 后台操作日志列表
// @Tags 管理后台
// @Produce json
// @Param method query string false "HTTP 方法（如 GET/POST）"
// @Param path query string false "路径片段（模糊匹配）"
// @Param user_id query int false "按用户 UID 过滤"
// @Param error_code query string false "业务码（如 A0303）"
// @Param failed query string false "只看失败请求：1 或 true"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} model.Response
// @Router /admin/api/logs [get]
func (h *Handler) ListLogs(c *gin.Context) {
	page, pageSize := paginate(c)

	filter, paramErr := logFilterFrom(c)
	if paramErr != "" {
		errcode.Error(c, errcode.BadRequest, paramErr)
		return
	}

	logs, total, err := h.logs.FindBy(filter, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询操作日志失败")
		return
	}
	errcode.Success(c, gin.H{"list": logs, "total": total})
}

// logFilterFrom 解析日志筛选查询参数；第二个返回值非空即为 A0400 的文案。
func logFilterFrom(c *gin.Context) (model.LogFilter, string) {
	filter := model.LogFilter{
		Method:     c.Query("method"),
		Path:       c.Query("path"),
		ErrorCode:  c.Query("error_code"),
		OnlyFailed: c.Query("failed") == "1" || c.Query("failed") == "true",
	}
	if raw := c.Query("user_id"); raw != "" {
		uid, paramErr := parseUint(raw)
		if paramErr != "" {
			return model.LogFilter{}, "user_id " + paramErr
		}
		filter.UserID = uid
	}
	return filter, ""
}

// pathUint 解析路径参数里的数字 ID；第二个返回值非空即为 A0400 的文案。
func pathUint(c *gin.Context, name string) (uint, string) {
	id, paramErr := parseUint(c.Param(name))
	if paramErr != "" {
		return 0, name + " " + paramErr
	}
	return id, ""
}

// parseUint 把字符串解析为正整数 ID（0 与非法输入都算非法）。
func parseUint(raw string) (uint, string) {
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		return 0, "必须是正整数"
	}
	return uint(id), ""
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
