package handler

import (
	"net/http"

	"ebook-server/pkg/errcode"
	"ebook-server/pkg/logger"
	"ebook-server/pkg/upload"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// UploadHandler 文件上传 HTTP 处理器（ADR-0011）。
type UploadHandler struct {
	store *upload.Store
}

// NewUploadHandler 创建上传处理器实例。
func NewUploadHandler(store *upload.Store) *UploadHandler {
	return &UploadHandler{
		store: store,
	}
}

// UploadAvatar 上传头像
// @Summary 上传头像
// @Description 上传头像图片（multipart/form-data，文件字段名 avatar），返回可访问的图片 URL
// @Tags 用户
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param avatar formData file true "头像图片（jpg/png/webp，≤5MB）"
// @Success 200 {object} model.Response
// @Router /api/uploads/avatar [post]
func (h *UploadHandler) UploadAvatar(c *gin.Context) {
	// 登录态由路由组 JWTAuth 保证（main.go 装配，handler 内不再重复校验）

	// 限制请求体大小：超限时 FormFile 会返回 "request body too large"，
	// 防止恶意超大 multipart 占用内存/磁盘（5MB 上限 + 1MB multipart 开销余量）
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, upload.MaxAvatarBytes+(1<<20))

	file, err := c.FormFile("avatar")
	if err != nil {
		errcode.Error(c, errcode.BadRequest, "请选择头像文件（jpg/png/webp，≤5MB）")
		return
	}

	name, err := h.store.SaveAvatar(file)
	if err != nil {
		if err == upload.ErrInvalidImage {
			errcode.Error(c, errcode.BadRequest, err.Error())
			return
		}
		// 记录具体错误（目录缺失/权限/磁盘等），否则用户侧只能看到兜底文案，难以定位
		logger.Error("保存头像失败", zap.Error(err))
		errcode.Error(c, errcode.ServerError, "头像保存失败")
		return
	}

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	errcode.Success(c, gin.H{"url": h.store.PublicURL(scheme, c.Request.Host, name)})
}
