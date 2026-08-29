package handler

import (
	"ebook-server/model"
	"ebook-server/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type LogHandler struct {
	logService *service.LogService
}

func NewLogHandler() *LogHandler {
	return &LogHandler{
		logService: service.NewLogService(),
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
		model.InternalError(c, "获取日志列表失败")
		return
	}

	model.Success(c, result)
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
	userID, exists := c.Get("user_id")
	if !exists {
		model.Unauthorized(c, "未登录")
		return
	}

	uid, ok := userID.(uint)
	if !ok {
		model.InternalError(c, "用户ID类型错误")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	result, err := h.logService.GetByUserID(uid, page, pageSize)
	if err != nil {
		model.InternalError(c, "获取日志列表失败")
		return
	}

	model.Success(c, result)
}
