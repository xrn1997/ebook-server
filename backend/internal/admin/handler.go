package admin

import (
	"context"
	"sync"
	"time"

	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/httpx"
	"ebook-server/pkg/ratelimit"

	"github.com/gin-gonic/gin"
)

// UserStore 管理面对账号的访问能力（consumer-defined，ADR-0007）。
//
// 只列后台实际需要的方法，由 repository.UserRepository 满足。
// 全量列表不单列方法：Search 传空关键字即不参与过滤（两份方法只差一个 WHERE 形式）。
type UserStore interface {
	Count(ctx context.Context) (int64, error)
	Search(ctx context.Context, keyword string, page, pageSize int) ([]model.User, int64, error)
	FindByUID(ctx context.Context, uid uint) (*model.User, error)
}

// CommentModule 管理面对评论域的能力（consumer-defined，ADR-0007）。
//
// 由 service.CommentService 满足：列表视图、分页、治理删、统计与全局改键都在
// 模块内，后台只声明自己需要的能力。两个表面共用同一个模块，但引擎与鉴权仍分离
// （ADR-0010）。列表不再序列化实体：返回的是固定视图（无 email）。
type CommentModule interface {
	Count(ctx context.Context) (int64, error)
	CountByUserID(ctx context.Context, userID uint) (int64, error)
	ListForAdmin(ctx context.Context, query model.CommentQuery, page, pageSize int) (*model.AdminCommentListResponse, error)
	Delete(ctx context.Context, commentID uint, scope model.CommentScope) error
	MigrateKeys(ctx context.Context, scope model.CommentKeyScope, oldKey, newKey string) (int64, error)
}

// LogStore 管理面对操作日志的访问能力（consumer-defined，ADR-0007）。
//
// 由 repository.LogRepository 满足。全量列表同样是 FindBy 的零值条件特例。
type LogStore interface {
	FindBy(ctx context.Context, filter model.LogFilter, page, pageSize int) ([]model.OperationLog, int64, error)
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
	comments     CommentModule
	logs         LogStore
	session      *Session
	loginLimiter *ratelimit.Limiter
	// loginMu 让「Peek → 校验口令 → Allow」成为一次原子操作。
	//
	// 分开调用时，一批并发请求可以全部通过 Peek（此时还没人记失败），于是限流
	// 写着的「每分钟 5 次」实际变成「一次并发爆发里任意多次」——口令比对本身很快，
	// 这正是爆破想要的形态。临界区里只有一次摘要比较，串行化不影响正常登录。
	loginMu sync.Mutex
}

// NewHandler 创建后台处理器。
//
// comments 与公开表面共用同一个评论模块（读视图、治理删、统计与改键规则都只有
// 一份实现）；session 是后台的凭据与 token 生命周期，与路由中间件共用同一实例。
func NewHandler(users UserStore, comments CommentModule, logs LogStore, session *Session) *Handler {
	return &Handler{
		users:        users,
		comments:     comments,
		logs:         logs,
		session:      session,
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
// 频繁登录被误锁；超限返回 A0241，防暴力破解。检查与记账在 loginMu 内完成，
// 并发请求按到达顺序分享同一份配额，而不是各自先看到「未锁定」。
//
// @Summary 后台登录
// @Tags 管理后台
// @Accept json
// @Produce json
// @Success 200 {object} model.Response
// @Router /admin/api/login [post]
func (h *Handler) Login(c *gin.Context) {
	ip := c.ClientIP()
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "后台登录")
		return
	}

	// 绑定失败不占配额；从「是否已锁定」到「记录这次失败」必须在同一临界区里，
	// 否则并发爆发能绕过限流（见 loginMu）。
	h.loginMu.Lock()
	defer h.loginMu.Unlock()
	if h.loginLimiter.Peek(ip) {
		errcode.Error(c, errcode.AttemptTooMany, loginLockMessage)
		return
	}
	token, err := h.session.Login(req.Username, req.Password)
	if err != nil {
		// 仅失败尝试计数（防爆破）
		h.loginLimiter.Allow(ip)
		errcode.Error(c, adminAuthFailureCode, ErrCredInvalid.Error())
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
	userCount, err := h.users.Count(c.Request.Context())
	if err != nil {
		errcode.Respond(c, err, "查询统计失败")
		return
	}
	commentCount, err := h.comments.Count(c.Request.Context())
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
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /admin/api/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	page, pageSize := httpx.Page(c)
	users, total, err := h.users.Search(c.Request.Context(), c.Query("keyword"), page, pageSize)
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
	uid, err := httpx.PathUint(c, "uid")
	if err != nil {
		errcode.RespondBindError(c, err, "查询用户")
		return
	}

	user, err := h.users.FindByUID(c.Request.Context(), uid)
	if err != nil {
		// 账号不存在 → A0201 由 errcode 表给出（repository 已翻成领域哨兵），
		// 其余错误没有业务码，落兜底文案。
		errcode.Respond(c, err, "查询用户失败")
		return
	}

	commentCount, err := h.comments.CountByUserID(c.Request.Context(), uid)
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
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /admin/api/comments [get]
func (h *Handler) ListComments(c *gin.Context) {
	page, pageSize := httpx.Page(c)
	result, err := h.comments.ListForAdmin(c.Request.Context(), model.CommentQuery{
		Keyword:  c.Query("keyword"),
		BookName: c.Query("book_name"),
	}, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询评论失败")
		return
	}
	errcode.Success(c, result)
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
	id, err := httpx.PathUint(c, "id")
	if err != nil {
		errcode.RespondBindError(c, err, "删除评论")
		return
	}

	if err := h.comments.Delete(c.Request.Context(), id, model.CommentScopeOfAdmin()); err != nil {
		// 评论不存在 → A0304，与公开端点同一枚业务码，客户端无需区分来源
		errcode.Respond(c, err, "删除评论失败")
		return
	}
	errcode.SuccessMsg(c, "删除成功", nil)
}

// rehashRequest 后台全局改键请求体。
//
// binding 上留 required + max=200 只为把契约写进对外文档（Swagger 会照着 tag 标出
// 「必填」与 maxLength）；真正的判定在评论模块 MigrateKeys 一处，两处的数值由
// TestRehashRequestBindingMatchesColumnLimit 钉住一致——此前后台这份 200 没有任何
// 用例盯着，可以静默漂离写这一列的约束。
type rehashRequest struct {
	OldKey string `json:"old_key" binding:"required,max=200"`
	NewKey string `json:"new_key" binding:"required,max=200"`
}

// RehashComments 后台全局改键：把所有用户挂在旧聚合键下的评论迁到新键。
//
// 这是 M2 客户端派生键的配套运维能力：某个键算错了（归一化规则变更、书名作者
// 解析出错）会让一堆评论聚错桶，而公开端点的 migrate 只动本人行，救不了别人的评论。
// 放在后台引擎而非公开 API（ADR-0010）：它按定义要跨用户写数据，
// 挂到公网端口等于把「任意改全员评论归属」暴露给未认证流量。
// 新旧相同返回 A0305；无匹配行返回 0（幂等，可重复调用）——两条规则来自
// 评论模块 MigrateKeys，与公开 migrate 同源。
//
// @Summary 后台全局改评论聚合键（跨用户）
// @Tags 管理后台
// @Accept json
// @Produce json
// @Param request body rehashRequest true "新旧聚合键"
// @Success 200 {object} model.Response
// @Router /admin/api/comments/rehash [post]
func (h *Handler) RehashComments(c *gin.Context) {
	var req rehashRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "后台全局改键")
		return
	}

	count, err := h.comments.MigrateKeys(c.Request.Context(), model.ScopeAllUsers(), req.OldKey, req.NewKey)
	if err != nil {
		errcode.Respond(c, err, "改键失败")
		return
	}
	errcode.Success(c, model.MigrateCommentKeyResponse{MigratedCount: count})
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
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /admin/api/logs [get]
func (h *Handler) ListLogs(c *gin.Context) {
	page, pageSize := httpx.Page(c)

	filter, filterErr := logFilterFrom(c)
	if filterErr != nil {
		errcode.RespondBindError(c, filterErr, "查询操作日志")
		return
	}

	logs, total, err := h.logs.FindBy(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "查询操作日志失败")
		return
	}
	errcode.Success(c, gin.H{"list": logs, "total": total})
}

// logFilterFrom 解析日志筛选查询参数；返回的 error 由调用方交给 RespondBindError。
func logFilterFrom(c *gin.Context) (model.LogFilter, error) {
	filter := model.LogFilter{
		Method:     c.Query("method"),
		Path:       c.Query("path"),
		ErrorCode:  c.Query("error_code"),
		OnlyFailed: c.Query("failed") == "1" || c.Query("failed") == "true",
	}
	uid, err := httpx.QueryUint(c, "user_id")
	if err != nil {
		return model.LogFilter{}, err
	}
	filter.UserID = uid
	return filter, nil
}
