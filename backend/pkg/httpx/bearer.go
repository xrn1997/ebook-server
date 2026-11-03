package httpx

import (
	"errors"
	"strings"
)

// ErrBearerMalformed Authorization 头不是合法的 Bearer 形状。
//
// 本包只裁决形状：空头、uid 判定、业务码与文案都属于各表面自己的策略——
// 公开侧（middleware.JWTAuth）与后台侧（admin.Session.AuthMiddleware）失败码不同
// （A0230 / A0403），把它们收进来只会让取值包认识两张信封。
var ErrBearerMalformed = errors.New("authorization 头不是 Bearer 形状")

// BearerToken 从 Authorization 头的值取出 Bearer 凭据。
//
// 只做 SplitN(header, " ", 2) + 前缀判定：返回凭据（不含 "Bearer " 前缀）。
// SplitN 只切一刀，"Bearer  token" 的多余空格留在凭据里原样返回——两个表面
// 此前都是这个形状，改判「多空格非法」是对外行为变化，不在本包擅自发生。
func BearerToken(header string) (string, error) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", ErrBearerMalformed
	}
	return parts[1], nil
}
