// Package httpx 负责「从 HTTP 请求里把原始字符串取成 Go 值」，是全项目唯一的取值入口。
//
// 单独成包的原因：handler 与 internal/admin 都要读分页参数和路径/查询里的数字 ID，
// 共享工具没有合法落点时两边各写一份，「省略 page_size 给多少条」「什么样的 ID 算
// 非法」就有了两个答案。
//
// 边界：本包只管取值，不管规矩——缺省与上限交给 model.NormalizePage，业务码与
// 响应信封交给 pkg/errcode。它不认识任何具体业务参数。
package httpx

import (
	"errors"
	"fmt"
	"strconv"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

// errNotPositive 参数不是正整数。文案带参数名，便于日志定位；响应由调用方
// 统一交给 errcode.RespondBindError 收敛成 A0400 的人话。
var errNotPositive = errors.New("必须是正整数")

// Page 读取 page / page_size 并钳制，返回值一定满足 page≥1、1≤pageSize≤model.MaxPageSize。
//
// 缺省与非数字一视同仁（都按「未提供」处理）：`?page_size=abc` 与省略 `page_size`
// 拿到同样大小的合法一页。分页参数写错不值得回 400——它不改变查询意图，
// 也不像 ID 那样有「拿错值去查别人」的风险。
//
// 归一化在这里做过一次，service 侧还会再做一次（见 model.NormalizePage）：边界必须
// 在 module 一侧，不是所有调用方都从 HTTP 进来。over-cap 的结果是**回落到缺省的
// DefaultPageSize**，不是截到 MaxPageSize：超限的 page_size 与省略它同样大小。
func Page(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return model.NormalizePage(page, pageSize)
}

// PathUint 读取路径参数并解析为正整数 ID，如 /api/comments/:id 里的 12。
//
// 0 也算非法：报错让客户端立刻发现拼接错误，胜过一次必然落空的查询。解析宽度固定
// 32 位（不按平台 uint 宽度）：自增主键远达不到这个量级，超过只可能是拼错的 URL，
// 固定宽度让 32 位与 64 位平台得到同一个判定。
//
// 失败返回的 error 文案带参数名（如 "id 必须是正整数"），调用方交给
// errcode.RespondBindError 回 A0400；原文只进日志。
func PathUint(c *gin.Context, name string) (uint, error) {
	id, err := positiveUint(c.Param(name))
	if err != nil {
		return 0, fmt.Errorf("%s %w", name, err)
	}
	return id, nil
}

// QueryUint 读取查询参数并解析为正整数 ID，如 /admin/api/logs?user_id=7。
//
// 参数缺省或为空串返回 (0, nil)：「没有这个过滤条件」与「提供了但值非法」必须分开，
// 否则少传一个筛选项就会收到报错。提供了但非法时返回带参数名的 error，
// 同 PathUint 交给 errcode.RespondBindError。
func QueryUint(c *gin.Context, name string) (uint, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, nil
	}
	id, err := positiveUint(raw)
	if err != nil {
		return 0, fmt.Errorf("%s %w", name, err)
	}
	return id, nil
}

// positiveUint 解析未提供之外的取值：非数字、溢出、0 都归到 errNotPositive。
func positiveUint(raw string) (uint, error) {
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || v == 0 {
		return 0, errNotPositive
	}
	return uint(v), nil
}
