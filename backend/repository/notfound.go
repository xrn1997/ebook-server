package repository

import (
	"errors"

	"gorm.io/gorm"
)

// translateNotFound 把 gorm 的「没查到」换成本聚合的领域哨兵，其余错误原样透出。
//
// 换的位置就在这一层，因为 repository 是唯一认识 gorm 的层：以前它把
// gorm.ErrRecordNotFound 原样往上递，service 与后台各自再判一次「是不是没查到」，
// 同一个事实在三层各翻译一遍，还要靠一个 IsRecordNotFound 助手把 gorm 类型漏过 seam
// （该助手已随之删除）。现在跨界的只有领域错误，换掉 ORM 不影响上面任何一层。
//
// sentinel 由调用方按聚合选（账号 / 评论 / 令牌），不是由流程选：
// 「查不到」在数据层是同一个事实，而它对该回哪个业务码、要不要隐藏存在性，
// 属调用方的策略（防枚举见 ADR-0006，用 errcode.Map 声明在调用点）。
func translateNotFound(err, sentinel error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sentinel
	}
	return err
}
