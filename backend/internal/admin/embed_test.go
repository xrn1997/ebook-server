package admin

import "testing"

// TestMimeType ES module 必须返回 JS MIME，否则浏览器拒绝执行导致白屏（回归保护）。
//
// 留在包内测试：mimeType 是内嵌资产服务的局部规则，与路由表无关，外部测试包碰不到它。
// 后台的路由与鉴权用例都在外部包 admin_test 里站真路由表（见 admin_test.go 文件头）。
func TestMimeType(t *testing.T) {
	cases := []struct{ name, want string }{
		{"assets/index-abc.js", "text/javascript; charset=utf-8"},
		{"assets/app-abc.css", "text/css; charset=utf-8"},
		{"assets/app-abc.json", "application/json; charset=utf-8"},
		{"assets/logo.svg", "image/svg+xml"},
	}
	for _, c := range cases {
		if got := mimeType(c.name, nil); got != c.want {
			t.Errorf("mimeType(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
