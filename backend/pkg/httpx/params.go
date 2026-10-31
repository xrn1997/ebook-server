// Package httpx 负责「从 HTTP 请求里把原始字符串取成 Go 值」，是全项目唯一的取值入口。
//
// 为什么需要单独一个包：handler 与 internal/admin 是兄弟包，两边都要读分页参数和
// 路径/查询里的数字 ID。共享工具没有一个合法落点时，各自就会各写一份，于是
// 「省略 page_size 给多少条」「什么样的 ID 算非法」这类问题出现两个答案。
//
// 边界同样清楚：本包只管取值，不管规矩——缺省与上限交给 model.NormalizePage，
// 业务码与响应信封交给 pkg/errcode。它不认识任何具体业务参数。
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
// 拿到同样大小的合法一页。分页参数写错不值得回 400——它既不改变查询意图，
// 也不像 ID 那样存在「拿错值去查别人」的风险。
//
// 归一化在这里做过一次，service 侧还会再做一次（见 model.NormalizePage）：这不是冗余，
// 而是边界必须在 module 一侧——不是所有调用方都从 HTTP 进来，测试与内部调用会直接递整数。
// 注意 over-cap 的结果是**回落到缺省的 DefaultPageSize**，不是截到 MaxPageSize：
// 递一个超限的 page_size 与省略它得到的是同样大小的一页。
func Page(c *gin.Context) (page, pageSize int) {
	page, _ = strconv.Atoi(c.Query("page"))
	pageSize, _ = strconv.Atoi(c.Query("page_size"))
	return model.NormalizePage(page, pageSize)
}

// PathUint 读取路径参数并解析为正整数 ID，如 /api/comments/:id 里的 12。
//
// 0 也算非法：ID 是主键，第 0 行不存在，放过去只是把「写错的 URL」变成一次必然落空的
// 查询，而报错能让客户端立刻发现自己的拼接有问题。解析宽度固定为 32 位（不按平台的
// uint 宽度走）：自增主键远达不到这个量级，超过它的数字只可能是拼错的 URL，而固定
// 宽度让 32 位与 64 位平台得到同一个判定。
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
// 参数缺省或为空串返回 (0, nil)：那是「没有这个过滤条件」，与「提供了但值非法」必须分开，
// 否则调用方少传一个筛选项就会收到报错。提供了但非法时返回带参数名的 error，
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
