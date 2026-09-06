package repository

import (
	"testing"

	"ebook-server/model"
)

// query_test.go 覆盖共用的查询原语：LIKE 转义与分页窗口。
// 建立在其上的仓储查询测试按被测对象拆在 user/comment/log 各自的 _test.go。

func TestLikePatternEscapesWildcards(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"普通关键字", "abc", "%abc%"},
		{"百分号必须当普通字符", "100%", `%100\%%`},
		{"下划线必须当普通字符", "a_b", `%a\_b%`},
		{"转义符自身要先转义", `a\b`, `%a\\b%`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := likePattern(c.input); got != c.want {
				t.Errorf("likePattern(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestPaginateQueryReturnsPageWindow 分页助手必须同时给对总数与当页条数。
func TestPaginateQueryReturnsPageWindow(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	uid := seedUser(t, "page@example.com", "page", "page")
	repo := NewCommentRepository(testDB)
	for _, key := range []string{"k1", "k2", "k3", "k4", "k5"} {
		repo.Create(&model.Comment{UserID: uid, Content: "c", ChapterURL: key})
	}

	comments, total, err := repo.FindByChapterURLs([]string{"k1", "k2", "k3"}, "", 2, 2)
	if err != nil {
		t.Fatalf("FindByChapterURLs failed: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (总数不受分页影响)", total)
	}
	if len(comments) != 1 {
		t.Errorf("page 2 of size 2 should hold 1 row, got %d", len(comments))
	}
}
