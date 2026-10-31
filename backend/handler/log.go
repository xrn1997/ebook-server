package handler

import (
	"ebook-server/pkg/errcode"
	"ebook-server/pkg/httpx"
	"ebook-server/service"

	"github.com/gin-gonic/gin"
)

// LogHandler 操作日志 HTTP 处理器。
type LogHandler struct {
	logService *service.LogService
}

// NewLogHandler 创建日志处理器实例。
func NewLogHandler(logService *service.LogService) *LogHandler {
	return &LogHandler{
		logService: logService,
	}
}

// GetMyLogs 获取我的操作日志
// @Summary 获取我的操作日志
// @Description 获取当前用户的操作日志
// @Tags 日志
// @Produce json
// @Security ApiKeyAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/logs/my [get]
func (h *LogHandler) GetMyLogs(c *gin.Context) {
	// 经 middleware 访问器读取上下文，不手搓 context key——
	// 当前账号的契约只应存在一处（middleware/auth.go）
	uid, signedIn := CurrentUser(c)
	if !signedIn {
		return
	}

	page, pageSize := httpx.Page(c)

	result, err := h.logService.GetByUserID(c.Request.Context(), uid, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "获取日志列表失败")
		return
	}

	errcode.Success(c, result)
}
