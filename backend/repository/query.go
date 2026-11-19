// query.go 收敛各列表查询共用的两件小事：分页形状与 LIKE 模式转义。
//
// 同一逻辑只留一处，不散在各仓库里逐份复制、逐份漂移。
package repository

import (
	"strings"

	"gorm.io/gorm"
)

// paginateQuery 执行「先数总数、再取一页」的统一分页查询。
//
// 调用方传入已带好过滤条件的 query（含 Preload/Model），本函数只负责排序与取页。
// order 为空时不排序（调用方自己保证不依赖顺序时才可留空）。
func paginateQuery[T any](query *gorm.DB, order string, page, pageSize int, dest *[]T) (int64, error) {
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, err
	}

	if order != "" {
		query = query.Order(order)
	}
	if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(dest).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// likePattern 把用户输入包成 LIKE 模式，并把 LIKE 的通配符转义成普通字符。
//
// 不转义的话，搜索框里一个 % 就等于「匹配全部」——那是错误结果而不是宽松结果。
// 配合 SQL 里的 `ESCAPE '\'` 使用（SQLite/MySQL 均认）。
func likePattern(keyword string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyword)
	return "%" + escaped + "%"
}
