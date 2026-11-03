package admin

import (
	"reflect"
	"strconv"
	"testing"

	"ebook-server/model"
)

// TestRehashRequestBindingMatchesMaxCommentKeyLength 钉住后台改键请求的 tag 与
// model.MaxCommentKeyLength 同源。
//
// 为什么 model 包那条钉不够：它只反射 model 里的请求类型，扫不到本包的 rehashRequest，
// 而 struct tag 只能写字面量、引用不了常量。少了这一枚，把常量从 200 抬到 300 时后台
// 仍按 200 拒绝——合法的键在后台 HTTP 边界被 A0400，公开 migrate 却收得下，同一列在
// 两个表面给出两个答案。运行时 MigrateKeys 的字节检查只在放行之后才判，救不了被 tag
// 提前拒掉请求的情况。
//
// 整串比对而不是只挑 max：required 同样是对外契约（空 old_key 必须被拒，见
// TestAdminRehashComments_RejectsEmptyOldKey）。
func TestRehashRequestBindingMatchesMaxCommentKeyLength(t *testing.T) {
	want := "required,max=" + strconv.Itoa(model.MaxCommentKeyLength)

	typ := reflect.TypeOf(rehashRequest{})
	for _, name := range []string{"OldKey", "NewKey"} {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("rehashRequest has no field %s", name)
		}
		if got := field.Tag.Get("binding"); got != want {
			t.Errorf("rehashRequest.%s binding = %q, want %q（与 comment_key 列同源）", name, got, want)
		}
	}
}
