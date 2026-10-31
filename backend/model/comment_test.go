package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestCommentFilter_Normalize 分页钳制与键边界只在这一个函数里判定。
//
// 这些上限存在的理由是「一条语句的绑定变量总数」（SQLite 默认 999），而评论列表是
// 公开端点：不限长度就能被任意人打满并静默落 C0500。它们此前抄在 handler 与后台两处。
func TestCommentFilter_Normalize(t *testing.T) {
	long := strings.Repeat("k", MaxCommentKeyLength+1)
	longURL := strings.Repeat("u", MaxChapterURLLength+1)
	many := make([]string, MaxCommentFilterKeys+1)
	for i := range many {
		many[i] = "ck1:x"
	}

	cases := []struct {
		name    string
		filter  CommentFilter
		page    int
		size    int
		wantErr error
	}{
		{"缺省分页归一到 1/10", CommentFilter{}, 1, 10, nil},
		{"负页码抬到 1", CommentFilter{Page: -3, PageSize: 5}, 1, 5, nil},
		{"超过 100 的每页数量退回 10", CommentFilter{Page: 2, PageSize: 101}, 2, 10, nil},
		{"0 页大小退回 10", CommentFilter{Page: 2, PageSize: 0}, 2, 10, nil},
		{"正常主读路径放行", CommentFilter{CommentKeys: []string{"ck1:a#1"}, Page: 1, PageSize: 20}, 1, 20, nil},
		{"聚合键超数被拒", CommentFilter{CommentKeys: many, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"单个聚合键超长被拒", CommentFilter{CommentKeys: []string{long}, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"空聚合键被拒（不是可查询的桶）", CommentFilter{CommentKeys: []string{""}, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"旧键超数被拒", CommentFilter{LegacyKeys: many, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"单个旧键超长被拒", CommentFilter{LegacyKeys: []string{longURL}, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"空旧键被拒", CommentFilter{LegacyKeys: []string{""}, Page: 1, PageSize: 10}, 1, 10, ErrCommentFilterInvalid},
		{"两族键各自守自己的上限", CommentFilter{CommentKeys: []string{"ck1:a"}, LegacyKeys: []string{"https://src.example.com/1.html"}, Page: 1, PageSize: 10}, 1, 10, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := tc.filter
			err := filter.Normalize()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Normalize() err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil {
				if filter.Page != tc.page || filter.PageSize != tc.size {
					t.Errorf("paging = %d/%d, want %d/%d", filter.Page, filter.PageSize, tc.page, tc.size)
				}
			}
		})
	}
}

// TestCommentKeyScope_UserZeroIsStillScoped 归属限定与「是不是 0」是两件事。
//
// 若拿 uid == 0 当「不限用户」的哨兵，一次 uid 为 0 的迁移就会从「匹配 0 行」
// 变成「改写所有用户的行」——那是跨用户写，只应由后台表面发起。
func TestCommentKeyScope_UserZeroIsStillScoped(t *testing.T) {
	uid, byUser := ScopeOfUser(0).UserID()
	if uid != 0 || !byUser {
		t.Errorf("ScopeOfUser(0).UserID() = (%d, %v), want (0, true)", uid, byUser)
	}

	uid, byUser = ScopeOfUser(42).UserID()
	if uid != 42 || !byUser {
		t.Errorf("ScopeOfUser(42).UserID() = (%d, %v), want (42, true)", uid, byUser)
	}

	uid, byUser = ScopeAllUsers().UserID()
	if uid != 0 || byUser {
		t.Errorf("ScopeAllUsers().UserID() = (%d, %v), want (0, false)", uid, byUser)
	}
}

// TestCommentKeyScope_AllowsEmptyOldKey 空旧键只在本人范围内有意义。
func TestCommentKeyScope_AllowsEmptyOldKey(t *testing.T) {
	if !ScopeOfUser(7).AllowsEmptyOldKey() {
		t.Error("本人范围内空旧键是合法迁移方向（收拢未换键的历史行）")
	}
	if !ScopeOfUser(0).AllowsEmptyOldKey() {
		t.Error("限定用户这一事实不能因为 uid 是 0 就退化成全员范围")
	}
	if ScopeAllUsers().AllowsEmptyOldKey() {
		t.Error("全员范围内的空旧键会把所有人的历史行扫进同一个桶，必须禁止")
	}
}

// TestCommentScope_AuthorVsAdmin 作者删与治理删的区别只在 scope 这一处判定。
//
// 与 CommentKeyScope 同理：uid 为 0 仍然是一次「限定作者」的删除（匹配 0 行），
// 不能退化成「任意作者的评论都能删」。
func TestCommentScope_AuthorVsAdmin(t *testing.T) {
	uid, byAuthor := CommentScopeOfAuthor(0).UserID()
	if uid != 0 || !byAuthor {
		t.Errorf("CommentScopeOfAuthor(0).UserID() = (%d, %v), want (0, true)", uid, byAuthor)
	}
	if CommentScopeOfAuthor(0).IsAdmin() {
		t.Error("作者删不是治理删")
	}

	uid, byAuthor = CommentScopeOfAuthor(42).UserID()
	if uid != 42 || !byAuthor {
		t.Errorf("CommentScopeOfAuthor(42).UserID() = (%d, %v), want (42, true)", uid, byAuthor)
	}

	uid, byAuthor = CommentScopeOfAdmin().UserID()
	if uid != 0 || byAuthor {
		t.Errorf("CommentScopeOfAdmin().UserID() = (%d, %v), want (0, false)", uid, byAuthor)
	}
	if !CommentScopeOfAdmin().IsAdmin() {
		t.Error("治理删必须报告 IsAdmin")
	}
}

// TestNewAdminCommentView_CarriesNoEmail 后台视图是固定契约：
// 它带作者展示字段，但不带 email——email 只在用户详情里看。
func TestNewAdminCommentView_CarriesNoEmail(t *testing.T) {
	created := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c := &Comment{
		ID:          7,
		UserID:      42,
		User:        User{UID: 42, Email: "secret@example.com", Username: "reader", Nickname: "读友", Avatar: "/uploads/avatar/a.png"},
		Content:     "好看",
		CommentKey:  "ck1:abc#3",
		ChapterURL:  "https://src.example.com/1.html",
		ChapterName: "第三章",
		BookName:    "某书",
		CreatedAt:   created,
	}

	view := NewAdminCommentView(c)
	if view.ID != c.ID || view.UserID != c.UserID || view.Content != c.Content ||
		view.CommentKey != c.CommentKey || view.ChapterURL != c.ChapterURL ||
		view.ChapterName != c.ChapterName || view.BookName != c.BookName {
		t.Errorf("NewAdminCommentView() 丢了字段: %+v", view)
	}
	if !view.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", view.CreatedAt, created)
	}
	if view.User.UID != 42 || view.User.Username != "reader" || view.User.Nickname != "读友" || view.User.Avatar != "/uploads/avatar/a.png" {
		t.Errorf("User 视图 = %+v", view.User)
	}
}

// TestNewAdminCommentListResponse_EmptyListIsArray 空页返回 []，不是 null。
func TestNewAdminCommentListResponse_EmptyListIsArray(t *testing.T) {
	resp := NewAdminCommentListResponse(nil, 0)
	if resp.List == nil {
		t.Error("空页的 list 必须是空数组，null 会让前端渲染出「无数据」与「加载失败」两种含义")
	}
	if len(resp.List) != 0 || resp.Total != 0 {
		t.Errorf("resp = %+v, want empty list and total 0", resp)
	}

	resp = NewAdminCommentListResponse([]Comment{
		{ID: 1, Content: "a"},
		{ID: 2, Content: "b"},
	}, 9)
	if len(resp.List) != 2 || resp.List[0].ID != 1 || resp.List[1].ID != 2 || resp.Total != 9 {
		t.Errorf("resp = %+v, want ordered list and total 9", resp)
	}
}

// TestNewCommentUserView_NilIsZero 未预加载用户时回零值而不是 panic。
func TestNewCommentUserView_NilIsZero(t *testing.T) {
	if got := newCommentUserView(nil); got != (CommentUserView{}) {
		t.Errorf("newCommentUserView(nil) = %+v, want zero value", got)
	}
}
