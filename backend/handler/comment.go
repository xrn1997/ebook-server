package handler

import (
	"strings"

	"ebook-server/model"
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/httpx"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

// 评论列表的边界上限（键数、单键长度、分页钳制）不在这里：它们属查询语义，
// 由 model.CommentFilter.Normalize 一处判定，映射 A0400。历史上这几个数值抄在
// handler 与后台两处、其中一处没有测试钉住。

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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	var req model.CreateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "创建评论")
		return
	}

	comment, err := h.commentService.Create(c.Request.Context(), userID, &req)
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
	page, pageSize := httpx.Page(c)

	// handler 只做协议转换：把查询参数拆成键列表。哪个键赢、上限多少，属查询语义。
	filter := model.CommentFilter{
		CommentKeys: parseCommentKeyFilters(c),
		LegacyKeys:  parseLegacyKeyFilters(c),
		BookName:    c.Query("book_name"),
		Page:        page,
		PageSize:    pageSize,
	}

	result, err := h.commentService.List(c.Request.Context(), filter)
	if err != nil {
		errcode.Respond(c, err, "获取评论列表失败")
		return
	}

	errcode.Success(c, result)
}

// parseCommentKeyFilters 拆出 comment_keys 过滤参数（M2 主读路径）。
//
// 客户端用 Retrofit 的单个 @Query 把多个键拼成逗号分隔的一个值，所以这里既按逗号切分，
// 也容忍 `?comment_keys=a&comment_keys=b` 的重复参数形态。键本身永不含逗号
// （`ck1:` + 十六进制摘要 + 可选 `#章序号`），按逗号切分没有歧义。
// 空串按「未提供该键」丢弃（`?comment_keys=` 不带值是常见形态，此时落全局最新列表）。
// 长度与个数上限不在这里判定，见 model.CommentFilter.Normalize。
func parseCommentKeyFilters(c *gin.Context) []string {
	return splitKeyFilters(c.QueryArray("comment_keys"), true)
}

// parseLegacyKeyFilters 拆出 chapter_url（已废弃的旧聚合键）过滤参数。
//
// chapter_url 可以含逗号（它是 URL），因此不按逗号切分，只丢弃空值。
func parseLegacyKeyFilters(c *gin.Context) []string {
	return splitKeyFilters(c.QueryArray("chapter_url"), false)
}

// splitKeyFilters 丢弃空值、按需按逗号切分，得到过滤用的键列表。
func splitKeyFilters(raw []string, splitOnComma bool) []string {
	keys := make([]string, 0, len(raw))
	for _, item := range raw {
		if item == "" {
			continue
		}
		if splitOnComma {
			for _, key := range strings.Split(item, ",") {
				if key != "" {
					keys = append(keys, key)
				}
			}
			continue
		}
		keys = append(keys, item)
	}
	return keys
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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	page, pageSize := httpx.Page(c)

	result, err := h.commentService.List(c.Request.Context(), model.CommentFilter{
		UserID:   userID,
		ByUserID: true,
		Page:     page,
		PageSize: pageSize,
	})
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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	id, err := httpx.PathUint(c, "id")
	if err != nil {
		errcode.RespondBindError(c, err, "删除评论")
		return
	}

	if err := h.commentService.Delete(c.Request.Context(), id, model.CommentScopeOfAuthor(userID)); err != nil {
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
	userID, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	var req model.MigrateCommentKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.RespondBindError(c, err, "迁移聚合键")
		return
	}

	count, err := h.commentService.MigrateKey(c.Request.Context(), userID, req.OldKey, req.NewKey)
	if err != nil {
		errcode.Respond(c, err, "迁移评论聚合键失败")
		return
	}

	errcode.Success(c, model.MigrateCommentKeyResponse{MigratedCount: count})
}
