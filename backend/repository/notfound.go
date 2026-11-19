package repository

import (
	"errors"

	"gorm.io/gorm"
)

// translateNotFound 把 gorm 的「没查到」收口换成本聚合的领域哨兵，其余错误原样透出。
//
// 在这一层收口，因为 repository 是唯一认识 gorm 的层：跨界只有领域错误，
// 换掉 ORM 不影响上面任何一层。
//
// sentinel 由调用方按聚合选（账号 / 评论 / 令牌），不按流程选：「查不到」在数据层
// 是同一个事实，而它对该回哪个业务码、要不要隐藏存在性，属调用方的策略
// （防枚举见 ADR-0006，用 errcode.Map 声明在调用点）。
func translateNotFound(err, sentinel error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sentinel
	}
	return err
}
