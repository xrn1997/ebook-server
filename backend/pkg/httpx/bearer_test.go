package httpx

import (
	"errors"
	"testing"
)

// TestBearerToken 逐档钉住 Bearer 形状解析：本包只认「SplitN 一刀 + 前缀」这一种形状。
//
// 空头与非法形状都回同一枚哨兵——「没带凭据」与「带了但形状不对」怎么分开回话
// 是各表面自己的策略（公开侧 A0230 两句话、后台侧统一 A0403），不在本包判定。
func TestBearerToken(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		want    string
		wantErr error
	}{
		{"合法 Bearer", "Bearer abc.def.ghi", "abc.def.ghi", nil},
		{"无 Bearer 前缀", "Token abc.def.ghi", "", ErrBearerMalformed},
		{"空头", "", "", ErrBearerMalformed},
		{"只有前缀没有凭据", "Bearer", "", ErrBearerMalformed},
		// SplitN 只切一刀：多余空格留在凭据里原样返回，与两个表面此前的解析行为一致。
		{"多空格保留凭据原样", "Bearer  spaced-token", " spaced-token", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BearerToken(tc.header)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("BearerToken(%q) err = %v, want %v", tc.header, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("BearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}
