package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// maxChapterURLFilters 单次请求允许的 chapter_url 聚合键个数上限。
//
// 必须设上限：每个键都会被 GORM 展开成一个绑定变量，SQLite 单语句变量数默认上限 999，
// 超出直接报错并落成 C0500；而这是无需登录的公开端点，不限长度即可被任意人打满。
// 50 远大于「合并一本书涉及的章节数」这一真实场景。
const maxChapterURLFilters = 50

// maxChapterURLFilterLength 单个聚合键的字节上限，必须与写入该列时
// model.CreateCommentRequest.ChapterURL 的 `max=2048` 一致。
//
// Gin 的 binding tag 只能写字面量、拼不了常量，所以这条一致性由
// TestChapterURLLengthLimitMatchesModelBinding 在 CI 里锁住。
const maxChapterURLFilterLength = 2048

// CommentHandler 评论 HTTP 处理器。
type CommentHandler struct {
	commentService *service.CommentService
}

// NewCommentHandler 创建评论处理器实例。
func NewCommentHandler(commentService *service.CommentService) *CommentHandler {
	return &CommentHandler{
		commentService: commentService,
	}
}

// Create 创建评论
// @Summary 创建评论
// @Description 创建新评论
// @Tags 评论
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.CreateCommentRequest true "评论内容"
// @Success 200 {object} model.Response
// @Router /api/comments [post]
func (h *CommentHandler) Create(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	comment, err := h.commentService.Create(userID, &req)
	if err != nil {
		errcode.Respond(c, err, "创建评论失败")
		return
	}

	errcode.Success(c, comment)
}

// GetList 获取评论列表
// @Summary 获取评论列表
// @Description 获取评论列表，可按章节过滤（chapter_url 支持多个，返回并集；book_name 可单独或配合过滤）
// @Tags 评论
// @Produce json
// @Param chapter_url query []string false "书源章节聚合键（可传多个，返回并集；最多 50 个，单键最长 2048）" collectionFormat(multi)
// @Param book_name query string false "书名（配合 chapter_url 二次过滤，或单独过滤全书）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments [get]
func (h *CommentHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	chapterURLs, filterErr := parseChapterURLFilters(c)
	if filterErr != "" {
		errcode.Error(c, errcode.BadRequest, filterErr)
		return
	}

	bookName := c.Query("book_name")

	var result *model.CommentListResponse
	var err error
	switch {
	// 单键与多键走同一条路径：一个元素的 IN 与等值匹配等价
	case len(chapterURLs) > 0:
		result, err = h.commentService.GetByChapterURLs(chapterURLs, bookName, page, pageSize)
	case bookName != "":
		result, err = h.commentService.GetByBook(bookName, page, pageSize)
	default:
		result, err = h.commentService.GetAll(page, pageSize)
	}
	if err != nil {
		errcode.Respond(c, err, "获取评论列表失败")
		return
	}

	errcode.Success(c, result)
}

// parseChapterURLFilters 解析并校验公开的 chapter_url 多键过滤参数。
//
// 空串按「未提供该键」丢弃（?chapter_url= 不带值是常见形态）。校验两条边界：
// 单键长度不超过 maxChapterURLFilterLength（比写入该列的上限还长的键永远匹配不到，
// 属无效输入而非空结果），键数不超过 maxChapterURLFilters。
// 第二个返回值非空即为应回给客户端的错误文案，调用方统一以 A0400 返回。
func parseChapterURLFilters(c *gin.Context) ([]string, string) {
	raw := c.QueryArray("chapter_url")
	keys := make([]string, 0, len(raw))
	for _, u := range raw {
		if u == "" {
			continue
		}
		if len(u) > maxChapterURLFilterLength {
			return nil, fmt.Sprintf("chapter_url 过长（上限 %d）", maxChapterURLFilterLength)
		}
		keys = append(keys, u)
	}
	if len(keys) > maxChapterURLFilters {
		return nil, fmt.Sprintf("chapter_url 最多 %d 个", maxChapterURLFilters)
	}
	return keys, ""
}

// GetMyComments 获取我的评论列表
// @Summary 获取我的评论列表
// @Description 获取当前用户的评论列表
// @Tags 评论
// @Produce json
// @Security ApiKeyAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments/my [get]
func (h *CommentHandler) GetMyComments(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.commentService.GetByUserID(userID, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "获取评论列表失败")
		return
	}

	errcode.Success(c, result)
}

// Delete 删除评论
// @Summary 删除评论
// @Description 删除指定评论
// @Tags 评论
// @Produce json
// @Security ApiKeyAuth
// @Param id path int true "评论ID"
// @Success 200 {object} model.Response
// @Router /api/comments/{id} [delete]
func (h *CommentHandler) Delete(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		errcode.Error(c, errcode.BadRequest, "无效的评论ID")
		return
	}

	if err := h.commentService.Delete(uint(id), userID); err != nil {
		errcode.Respond(c, err, "删除评论失败")
		return
	}

	errcode.SuccessMsg(c, "删除成功", nil)
}

// MigrateKey 迁移评论聚合键
// @Summary 迁移评论聚合键
// @Description 将当前用户在旧聚合键下的评论批量迁移到新聚合键（合并书籍场景）
// @Description 两键均可为空串，空 = 书籍级评论（因此「书籍级↔章节」两个方向都支持）；单键最长 2048。
// @Description 只影响本人评论；新旧相同返回 A0305；无匹配评论返回 0（幂等，可重复调用）。
// @Tags 评论
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.MigrateCommentKeyRequest true "迁移请求"
// @Success 200 {object} model.Response
// @Router /api/comments/migrate-key [post]
func (h *CommentHandler) MigrateKey(c *gin.Context) {
	userID, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	var req model.MigrateCommentKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Error(c, errcode.BadRequest, "请求参数错误: "+err.Error())
		return
	}

	count, err := h.commentService.MigrateKey(userID, req.OldKey, req.NewKey)
	if err != nil {
		errcode.Respond(c, err, "迁移评论聚合键失败")
		return
	}

	errcode.Success(c, model.MigrateCommentKeyResponse{MigratedCount: count})
}
