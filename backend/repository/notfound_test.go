package repository

import (
	"errors"
	"testing"

	"ebook-server/model"

	"gorm.io/gorm"
)

// TestTranslateNotFound 边界上的替换只做一件事：认得 gorm 的哨兵，其余原样透出。
func TestTranslateNotFound(t *testing.T) {
	other := errors.New("database is locked")
	if got := translateNotFound(other, model.ErrAccountNotFound); got != other {
		t.Errorf("非「查不到」的错误被改写了: %v", got)
	}
	if got := translateNotFound(nil, model.ErrAccountNotFound); got != nil {
		t.Errorf("nil 不该被换成哨兵: %v", got)
	}
	if got := translateNotFound(gorm.ErrRecordNotFound, model.ErrCommentNotFound); !errors.Is(got, model.ErrCommentNotFound) {
		t.Errorf("查不到应换成指定哨兵, got %v", got)
	}
}

// TestNotFoundStopsAtTheSeam 账号查不到时，跨过 seam 的必须是领域哨兵，
// 而且**不是** gorm 的错误——否则 service 与后台就又得各自判一次「是不是没查到」，
// 换 ORM 也要跟着改上层（ADR-0007 当年被拒、本次采纳的那个方案）。
func TestNotFoundStopsAtTheSeam(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	_, err := NewUserRepository(testDB).FindByUID(testCtx, 999999)
	if !errors.Is(err, model.ErrAccountNotFound) {
		t.Fatalf("FindByUID 应回 model.ErrAccountNotFound, got %v", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t.Error("gorm 的哨兵漏到了 seam 之外，上层会被迫认识 gorm")
	}
}
