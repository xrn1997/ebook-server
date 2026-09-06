package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"ebook-server/model"
	"ebook-server/repository"
)

// handler_test.go 覆盖后台管理面新增的端点：用户搜索/详情、评论搜索/删除、日志筛选。
// 走真实 repository（:memory: 库），不写 mock——与仓库测试取向一致（ADR-0007）。

func TestAdminSearchUsers(t *testing.T) {
	r, db := setup(t)
	tok := adminLogin(t, r)

	users := repository.NewUserRepository(db)
	users.Create(&model.User{Email: "alice@example.com", Password: "hp", Username: "alice", Nickname: "小艾"})
	users.Create(&model.User{Email: "bob@example.com", Password: "hp", Username: "bob", Nickname: "阿鲍"})

	cases := []struct {
		name    string
		keyword string
		want    float64
	}{
		{"按邮箱", "alice@example", 1},
		{"按昵称", "小艾", 1},
		{"空关键字全量", "", 2},
		{"通配符当普通字符", "%", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := perform(r, http.MethodGet, "/admin/api/users?keyword="+url.QueryEscape(c.keyword), "", tok)
			if resp["code"] != "00000" {
				t.Fatalf("search failed: %v", resp)
			}
			if got := resp["data"].(map[string]interface{})["total"].(float64); got != c.want {
				t.Errorf("keyword=%q total = %v, want %v", c.keyword, got, c.want)
			}
		})
	}
}

// TestAdminGetUser 详情含账号资料与评论数；不存在的账号返回 A0201，非法 UID 返回 A0400。
func TestAdminGetUser(t *testing.T) {
	r, db := setup(t)
	tok := adminLogin(t, r)

	users := repository.NewUserRepository(db)
	comments := repository.NewCommentRepository(db)
	user := &model.User{Email: "detail@example.com", Password: "hp", Username: "detail", Nickname: "详"}
	users.Create(user)
	comments.Create(&model.Comment{UserID: user.UID, Content: "c1"})
	comments.Create(&model.Comment{UserID: user.UID, Content: "c2"})

	resp := perform(r, http.MethodGet, "/admin/api/users/"+strconv.FormatUint(uint64(user.UID), 10), "", tok)
	if resp["code"] != "00000" {
		t.Fatalf("get user failed: %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	got := data["user"].(map[string]interface{})
	if got["email"] != "detail@example.com" {
		t.Errorf("detail should expose email to admin, got %v", got["email"])
	}
	if got["password"] != nil {
		t.Error("password must never serialize out of the admin API")
	}
	if data["comment_count"].(float64) != 2 {
		t.Errorf("comment_count = %v, want 2", data["comment_count"])
	}

	assertAdminCode(t, perform(r, http.MethodGet, "/admin/api/users/999999", "", tok), "A0201")
	assertAdminCode(t, perform(r, http.MethodGet, "/admin/api/users/not-a-number", "", tok), "A0400")
}

// TestAdminSearchComments 内容关键字 + 书名筛选。
func TestAdminSearchComments(t *testing.T) {
	r, db := setup(t)
	tok := adminLogin(t, r)

	users := repository.NewUserRepository(db)
	user := &model.User{Email: "c@example.com", Password: "hp", Username: "c", Nickname: "c"}
	users.Create(user)
	comments := repository.NewCommentRepository(db)
	comments.Create(&model.Comment{UserID: user.UID, Content: "很精彩", BookName: "书A"})
	comments.Create(&model.Comment{UserID: user.UID, Content: "一般", BookName: "书A"})
	comments.Create(&model.Comment{UserID: user.UID, Content: "精彩绝伦", BookName: "书B"})

	cases := []struct {
		name  string
		query string
		total float64
	}{
		{"关键字", "?keyword=精彩", 2},
		{"书名", "?book_name=书A", 2},
		{"关键字+书名", "?keyword=精彩&book_name=书A", 1},
		{"空条件全量", "", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := perform(r, http.MethodGet, "/admin/api/comments"+c.query, "", tok)
			if resp["code"] != "00000" {
				t.Fatalf("search failed: %v", resp)
			}
			if got := resp["data"].(map[string]interface{})["total"].(float64); got != c.total {
				t.Errorf("query=%q total = %v, want %v", c.query, got, c.total)
			}
		})
	}
}

// TestAdminDeleteComment 后台删除不做归属校验；删后从列表消失，重复删返回 A0304。
func TestAdminDeleteComment(t *testing.T) {
	r, db := setup(t)
	tok := adminLogin(t, r)

	users := repository.NewUserRepository(db)
	user := &model.User{Email: "del@example.com", Password: "hp", Username: "del", Nickname: "del"}
	users.Create(user)
	comments := repository.NewCommentRepository(db)
	comment := &model.Comment{UserID: user.UID, Content: "target", BookName: "书A"}
	comments.Create(comment)

	path := fmt.Sprintf("/admin/api/comments/%d", comment.ID)
	assertAdminCode(t, perform(r, http.MethodDelete, path, "", tok), "00000")

	// 软删除后列表里不再出现
	resp := perform(r, http.MethodGet, "/admin/api/comments", "", tok)
	if got := resp["data"].(map[string]interface{})["total"].(float64); got != 0 {
		t.Errorf("deleted comment must leave the list, total = %v", got)
	}

	assertAdminCode(t, perform(r, http.MethodDelete, path, "", tok), "A0304")
	assertAdminCode(t, perform(r, http.MethodDelete, "/admin/api/comments/abc", "", tok), "A0400")
}

// TestAdminFilterLogs 方法/路径/账号/只看失败筛选都走查询参数。
func TestAdminFilterLogs(t *testing.T) {
	r, db := setup(t)
	tok := adminLogin(t, r)

	users := repository.NewUserRepository(db)
	user := &model.User{Email: "l@example.com", Password: "hp", Username: "l", Nickname: "l"}
	users.Create(user)
	logs := repository.NewLogRepository(db)
	logs.Create(&model.OperationLog{UserID: user.UID, Method: "GET", Path: "/api/comments", ErrorCode: "00000"})
	logs.Create(&model.OperationLog{UserID: user.UID, Method: "POST", Path: "/api/comments", ErrorCode: "A0303"})
	// 非信封流量无业务码，「只看失败」不得把它算进去
	logs.Create(&model.OperationLog{Method: "GET", Path: "/missing", ResponseCode: 404})

	cases := []struct {
		name  string
		query string
		total float64
	}{
		{"按方法", "?method=POST", 1},
		{"按路径片段", "?path=/api/comments", 2},
		{"只看失败", "?failed=true", 1},
		{"按业务码", "?error_code=A0303", 1},
		{"空调全量", "", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := perform(r, http.MethodGet, "/admin/api/logs"+c.query, "", tok)
			data, ok := resp["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected data, got %v", resp)
			}
			if got := data["total"].(float64); got != c.total {
				t.Errorf("query=%q total = %v, want %v", c.query, got, c.total)
			}
		})
	}

	// 非数字 user_id 是参数错误（A0400），而不是静默忽略过滤条件
	assertAdminCode(t, perform(r, http.MethodGet, "/admin/api/logs?user_id=abc", "", tok), "A0400")
}

// assertAdminCode 断言后台响应的业务码。
func assertAdminCode(t *testing.T, resp map[string]interface{}, want string) {
	t.Helper()
	if resp["code"] != want {
		t.Errorf("Expected code %s, got %v (error=%v)", want, resp["code"], resp["error"])
	}
}
