package handler

import (
	"ebook-server/middleware"
	"ebook-server/pkg/errcode"
	"ebook-server/service"
	"strconv"

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

// GetList 获取操作日志列表
// @Summary 获取操作日志列表
// @Description 获取所有操作日志列表
// @Tags 日志
// @Produce json
// @Security ApiKeyAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} model.Response
// @Router /api/logs [get]
func (h *LogHandler) GetList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.logService.GetAll(page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "获取日志列表失败")
		return
	}

	errcode.Success(c, result)
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
	uid, exists := middleware.GetCurrentUserID(c)
	if !exists {
		errcode.Error(c, errcode.LoginExpired, "未登录")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.logService.GetByUserID(uid, page, pageSize)
	if err != nil {
		errcode.Respond(c, err, "获取日志列表失败")
		return
	}

	errcode.Success(c, result)
}
