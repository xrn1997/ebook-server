package handler

import (
	"ebook-server/middleware"
	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/service"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxKeyFilters 单次请求允许的聚合键个数上限（comment_keys 与 chapter_url 共用）。
//
// 必须设上限：每个键都会被 GORM 展开成一个绑定变量，SQLite 单语句变量数默认上限 999，
// 超出直接报错并落成 C0500；而这是无需登录的公开端点，不限长度即可被任意人打满。
// 上限来自「一条语句的绑定变量总数」而非某个键的形态，因此两个键参数共用一个值。
// 50 远大于「合并一本书涉及的章节数」这一真实场景。
const maxKeyFilters = 50

// maxChapterURLFilterLength 单个 chapter_url 的字节上限，必须与写入该列时
// model.CreateCommentRequest.ChapterURL 的 `max=2048` 一致。
//
// Gin 的 binding tag 只能写字面量、拼不了常量，所以这条一致性由
// TestChapterURLLengthLimitMatchesModelBinding 在 CI 里锁住。
const maxChapterURLFilterLength = 2048

// maxCommentKeyFilterLength 单个 comment_key 的字节上限，必须与写入该列时
// model.CreateCommentRequest.CommentKey 的 `max=200` 一致（同一列同一约束）。
//
// 200 远大于 `ck1:` + 64 位十六进制 + `#章序号` 的 69+ 字节实际长度：这是防脏数据
// 与防打满绑定变量的长度上限，**不是**对键格式的校验（服务端不解释这个 token）。
// 一致性同样由 TestCommentKeyLengthLimitMatchesModelBinding 锁住。
const maxCommentKeyFilterLength = 200

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
// @Description 创建新评论。comment_key（M2 聚合键）必填，服务端只存不解释其格式；
// @Description chapter_url / chapter_name / book_name 可选，仅作过渡期兼容与展示快照。
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
// @Description 获取评论列表。comment_keys 是 M2 聚合键过滤（逗号分隔，返回并集）；
// @Description chapter_url / book_name 已废弃，仅为让未换键的历史行继续可读而保留。
// @Tags 评论
// @Produce json
// @Param comment_keys query string false "M2 聚合键列表（逗号分隔，返回并集；最多 50 个，单键最长 200）"
// @Param chapter_url query []string false "已废弃：旧聚合键（可传多个，返回并集；最多 50 个，单键最长 2048）" collectionFormat(multi)
// @Param book_name query string false "已废弃：书名（配合 chapter_url 二次过滤，或单独过滤全书）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/comments [get]
func (h *CommentHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	commentKeys, filterErr := parseCommentKeyFilters(c)
	if filterErr != "" {
		errcode.Error(c, errcode.BadRequest, filterErr)
		return
	}
	chapterURLs, filterErr := parseChapterURLFilters(c)
	if filterErr != "" {
		errcode.Error(c, errcode.BadRequest, filterErr)
		return
	}

	bookName := c.Query("book_name")

	var result *model.CommentListResponse
	var err error
	switch {
	// M2 主路径：按不透明聚合键过滤。此时 book_name 不参与——书名已不是聚合维度。
	case len(commentKeys) > 0:
		result, err = h.commentService.GetByCommentKeys(commentKeys, page, pageSize)
	// 兼容路径：单键与多键走同一条路径：一个元素的 IN 与等值匹配等价
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

// parseCommentKeyFilters 解析并校验 comment_keys 过滤参数（M2 主读路径）。
//
// 客户端用 Retrofit 的单个 @Query 把多个键拼成逗号分隔的一个值，所以这里既按逗号切分，
// 也容忍 `?comment_keys=a&comment_keys=b` 的重复参数形态。键本身永不含逗号
// （`ck1:` + 十六进制摘要 + 可选 `#章序号`），按逗号切分没有歧义。
// 空串按「未提供该键」丢弃（`?comment_keys=` 不带值是常见形态，此时落全局最新列表）。
// 两条上限与 chapter_url 同理：单键长度不超过 maxCommentKeyFilterLength，
// 键数不超过 maxKeyFilters。第二个返回值非空即为应回给客户端的错误文案，调用方统一以 A0400 返回。
func parseCommentKeyFilters(c *gin.Context) ([]string, string) {
	raw := c.QueryArray("comment_keys")
	keys := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, key := range strings.Split(item, ",") {
			if key == "" {
				continue
			}
			if len(key) > maxCommentKeyFilterLength {
				return nil, fmt.Sprintf("comment_keys 过长（上限 %d）", maxCommentKeyFilterLength)
			}
			keys = append(keys, key)
		}
	}
	if len(keys) > maxKeyFilters {
		return nil, fmt.Sprintf("comment_keys 最多 %d 个", maxKeyFilters)
	}
	return keys, ""
}

// parseChapterURLFilters 解析并校验 chapter_url 多键过滤参数（已废弃的兼容读路径）。
//
// 空串按「未提供该键」丢弃（?chapter_url= 不带值是常见形态）。校验两条边界：
// 单键长度不超过 maxChapterURLFilterLength（比写入该列的上限还长的键永远匹配不到，
// 属无效输入而非空结果），键数不超过 maxKeyFilters。
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
	if len(keys) > maxKeyFilters {
		return nil, fmt.Sprintf("chapter_url 最多 %d 个", maxKeyFilters)
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
// @Description 将当前用户在旧聚合键（comment_key）下的评论批量迁移到新聚合键（合并书籍 / 改元数据修键场景）
// @Description new_key 必填非空——迁往空键会让评论落进任何 comment_keys 查询都命中不到的桶。
// @Description old_key 可为空串：空 = 尚未换键的历史行，因此「把本人旧评论收进正确桶」这一方向可表达；单键最长 200。
// @Description 只影响本人评论；新旧相同返回 A0305；无匹配评论返回 0（幂等，可重复调用）。
// @Tags 评论
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body model.MigrateCommentKeyRequest true "迁移请求"
// @Success 200 {object} model.Response
// @Router /api/comments/migrate [post]
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
