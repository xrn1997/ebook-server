package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"ebook-server/model"

	"github.com/gin-gonic/gin"
)

func contextWith(query string, params gin.Params) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodGet, "/any?"+query, nil)
	c.Params = params
	return c
}

func TestPage(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantPage int
		wantSize int
	}{
		{"缺省参数取缺省页", "", 1, model.DefaultPageSize},
		{"正常取值", "page=3&page_size=25", 3, 25},
		{"页码为 0 抬到 1", "page=0&page_size=25", 1, 25},
		{"页码为负抬到 1", "page=-7&page_size=25", 1, 25},
		{"超出上限退回缺省", "page=1&page_size=99999", 1, model.DefaultPageSize},
		{"0 条视为未提供", "page=1&page_size=0", 1, model.DefaultPageSize},
		{"非数字视为未提供", "page=abc&page_size=xyz", 1, model.DefaultPageSize},
		{"只给 page", "page=4", 4, model.DefaultPageSize},
		{"空串按未提供", "page=&page_size=", 1, model.DefaultPageSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, pageSize := Page(contextWith(tc.query, nil))
			if page != tc.wantPage || pageSize != tc.wantSize {
				t.Errorf("Page(%q) = %d/%d, want %d/%d", tc.query, page, pageSize, tc.wantPage, tc.wantSize)
			}
		})
	}
}

// TestPage_UsesSharedPagingPolicy 边界值用 model 的常量表达，不在本包抄一份数字。
//
// 断言写成「MaxPageSize 这一档可用、MaxPageSize+1 退回缺省」而不是「100 可用、101 退回」：
// 改了 model 的常量这里跟着一起动，本包若偷偷另写一份数值就会红，规则因此始终只有一份。
func TestPage_UsesSharedPagingPolicy(t *testing.T) {
	upper := strconv.Itoa(model.MaxPageSize)
	if _, size := Page(contextWith("page_size="+upper, nil)); size != model.MaxPageSize {
		t.Errorf("page_size=%d 被改写为 %d, want 原样生效", model.MaxPageSize, size)
	}
	if _, size := Page(contextWith("page_size="+strconv.Itoa(model.MaxPageSize+1), nil)); size != model.DefaultPageSize {
		t.Errorf("超出上限时 size = %d, want model.DefaultPageSize(%d)", size, model.DefaultPageSize)
	}
	if _, size := Page(contextWith("page_size=1", nil)); size != 1 {
		t.Errorf("下限 1 被当成缺省: %d", size)
	}
}

func TestPathUint(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		want    uint
		wantErr bool
	}{
		{"正常 ID", "42", 42, false},
		{"零非法", "0", 0, true},
		{"负数非法", "-3", 0, true},
		{"非数字非法", "abc", 0, true},
		{"小数非法", "1.5", 0, true},
		{"空参数非法", "", 0, true},
		{"超出 32 位非法", "4294967296", 0, true},
		{"32 位上界可用", "4294967295", 4294967295, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := contextWith("", gin.Params{{Key: "id", Value: tc.value}})
			got, err := PathUint(c, "id")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("PathUint(%q) 应报错，实际返回 %d", tc.value, got)
				}
				if got != 0 {
					t.Errorf("报错时的值应为 0，实际 %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("PathUint(%q) 意外报错: %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("PathUint(%q) = %d, want %d", tc.value, got, tc.want)
			}
		})
	}
}

// TestPathUint_MessageNamesParam 报错文案必须指名是哪个参数。
//
// 后台一次要校验 uid、id、user_id 三个 ID，客户端只看到「必须是正整数」时无法定位；
// 而文案直接进响应信封，所以这是契约的一部分而不是内部细节。
func TestPathUint_MessageNamesParam(t *testing.T) {
	c := contextWith("", gin.Params{{Key: "uid", Value: "x"}})
	_, err := PathUint(c, "uid")
	if err == nil || err.Error() != "uid 必须是正整数" {
		t.Errorf("err = %v, want \"uid 必须是正整数\"", err)
	}
	if !errors.Is(err, errNotPositive) {
		t.Error("包装链断了：调用方无法按哨兵判定，只能比字符串")
	}
}

func TestQueryUint(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		want    uint
		wantErr bool
	}{
		{"正常过滤值", "user_id=7", 7, false},
		{"未提供即无过滤", "", 0, false},
		{"空串即无过滤", "user_id=", 0, false},
		{"提供了但非法要报错", "user_id=abc", 0, true},
		{"0 非法", "user_id=0", 0, true},
		{"负数非法", "user_id=-1", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := QueryUint(contextWith(tc.query, nil), "user_id")
			if tc.wantErr && err == nil {
				t.Fatalf("QueryUint(%q) 应报错", tc.query)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("QueryUint(%q) 意外报错: %v", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("QueryUint(%q) = %d, want %d", tc.query, got, tc.want)
			}
		})
	}
}

// TestQueryUint_IgnoresUnrelatedParams 只认自己那个参数名，别的路径参数不参与。
func TestQueryUint_IgnoresUnrelatedParams(t *testing.T) {
	c := contextWith("page=2", gin.Params{{Key: "user_id", Value: "9"}})
	got, err := QueryUint(c, "user_id")
	if err != nil || got != 0 {
		t.Errorf("QueryUint 应从 query 取值而非 path 参数，got %d err %v", got, err)
	}
}
