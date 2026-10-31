package repository

import (
	"context"

	"ebook-server/model"

	"gorm.io/gorm"
)

// CommentRepository 公开内容（评论）数据访问。满足 service.CommentStore 接口。
type CommentRepository struct {
	db *gorm.DB
}

// NewCommentRepository 创建评论仓库实例。
func NewCommentRepository(db *gorm.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

// Create 创建评论
func (r *CommentRepository) Create(ctx context.Context, comment *model.Comment) error {
	return r.db.WithContext(ctx).Create(comment).Error
}

// FindByID 按 ID 查评论（带作者预加载）；不存在返回 model.ErrCommentNotFound。
func (r *CommentRepository) FindByID(ctx context.Context, id uint) (*model.Comment, error) {
	var comment model.Comment
	err := r.db.WithContext(ctx).Preload("User").First(&comment, id).Error
	if err != nil {
		return nil, translateNotFound(err, model.ErrCommentNotFound)
	}
	return &comment, nil
}

// FindByCommentKeys 按 M2 聚合键查评论并集（分页）。
//
// 主读路径。keys 由客户端派生且不透明，服务端只做等值匹配，不解析 `#章序号` 后缀。
// 传入单个键即精确匹配该键；排序与全局列表一致（created_at DESC）。
// 不与 book_name 组合过滤：书名在 M2 里只是展示快照，已不参与聚合。
func (r *CommentRepository) FindByCommentKeys(ctx context.Context, commentKeys []string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.WithContext(ctx).Model(&model.Comment{}).Where("comment_key IN ?", commentKeys).Preload("User")
	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindByChapterURLs 按旧聚合键（chapter_url）查评论并集（分页）。
//
// 已废弃：M2 后 chapter_url 不再是聚合键，此路径只为让未换键的历史行继续可读而保留。
// 传入单个键即等价于按该键精确匹配，因此不再另设单键方法（两者的查询体只差一个 WHERE 形式）。
// bookName 为空时不参与过滤；排序与全局列表一致（created_at DESC）。
func (r *CommentRepository) FindByChapterURLs(ctx context.Context, chapterURLs []string, bookName string, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.WithContext(ctx).Model(&model.Comment{}).Where("chapter_url IN ?", chapterURLs).Preload("User")
	if bookName != "" {
		query = query.Where("book_name = ?", bookName)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindByUserID 根据用户 ID 查找评论
func (r *CommentRepository) FindByUserID(ctx context.Context, userID uint, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.WithContext(ctx).Model(&model.Comment{}).Where("user_id = ?", userID).Preload("User")
	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// FindAllByUserID 获取某用户的全部评论（不分页，供数据导出使用）。
//
// 刻意不 Preload("User")——导出用独立的轻量结构，不需要关联用户对象。
func (r *CommentRepository) FindAllByUserID(ctx context.Context, userID uint) ([]model.Comment, error) {
	var comments []model.Comment
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

// Count 统计未删除的评论总数（后台概览用）。
func (r *CommentRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Comment{}).Count(&count).Error
	return count, err
}

// Search 按筛选条件分页查评论（后台「评论搜索」）。
//
// 零值条件不参与过滤，空条件即「全部评论」，因此不再单列一个全量方法。
// keyword 走 content 模糊匹配，LIKE 通配符已转义。
func (r *CommentRepository) Search(ctx context.Context, q model.CommentQuery, page, pageSize int) ([]model.Comment, int64, error) {
	var comments []model.Comment

	query := r.db.WithContext(ctx).Model(&model.Comment{}).Preload("User")
	if q.Keyword != "" {
		pattern := likePattern(q.Keyword)
		query = query.Where("content LIKE ? ESCAPE '\\'", pattern)
	}
	if q.BookName != "" {
		query = query.Where("book_name = ?", q.BookName)
	}

	total, err := paginateQuery(query, "created_at DESC", page, pageSize, &comments)
	if err != nil {
		return nil, 0, err
	}
	return comments, total, nil
}

// CountByUserID 统计某用户未删除的评论数（后台用户详情用）。
func (r *CommentRepository) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Comment{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// DeleteScoped 条件软删除：作者 scope 同时匹配 user_id，治理 scope 只匹配 id。
//
// RowsAffected 是判定「是否真的删了一行」的唯一依据：先 FindByID 再 Delete 会把
// 重复删除当成成功，也让条件删除的 WHERE 与归属校验分成两处。
func (r *CommentRepository) DeleteScoped(ctx context.Context, id uint, scope model.CommentScope) (bool, error) {
	q := r.db.WithContext(ctx).Where("id = ?", id)
	if uid, byUser := scope.UserID(); byUser {
		q = q.Where("user_id = ?", uid)
	}
	result := q.Delete(&model.Comment{})
	return result.RowsAffected > 0, result.Error
}

// MigrateKey 按归属范围批量改键（comment_key 列），返回受影响行数。
//
// scope 是唯一变化的东西：限定用户即公开的 migrate（合并书籍 / 改元数据修键），
// 不限用户即后台的 rehash（桶污染修复，错键上聚集了大量他人评论，逐人迁移不可能）。
// 两条路径此前各有一份只差 user_id 的 UPDATE，改键语义要修两次。
//
// 只更新 comment_key（聚合键）；chapter_name/book_name 是展示快照不随迁移更新——
// 快照的语义是「用户当时的所见」，批量改写反而伪造历史。
// GORM 自动附加 deleted_at IS NULL 条件，软删除记录不受影响。
//
// 空旧键要同时认 NULL 与空串：收养旧库时 AutoMigrate 给 comments 补的是不带默认值的
// TEXT 列，存量行拿到的是 NULL；而本服务自己写入的无键行是空串。SQLite 里拿 NULL 与
// 空串作等值比较不成真，所以只按空串收拢会恰好漏掉唯一需要它的那批数据——升级过的库上
// 「收拢我的历史评论」静默返回 migrated_count 0。
// 分两支而不是统一套 COALESCE：非空键是按索引点查的主路径，把列包进函数会让它退化成全表扫描。
func (r *CommentRepository) MigrateKey(ctx context.Context, scope model.CommentKeyScope, oldKey, newKey string) (int64, error) {
	q := r.db.WithContext(ctx).Model(&model.Comment{})
	if oldKey == "" {
		// 括号是必须的：这一串会与其他条件用 AND 相连，OR 不括起来会让归属过滤失效。
		q = q.Where("(comment_key IS NULL OR comment_key = '')")
	} else {
		q = q.Where("comment_key = ?", oldKey)
	}
	if uid, byUser := scope.UserID(); byUser {
		q = q.Where("user_id = ?", uid)
	}
	result := q.Update("comment_key", newKey)
	return result.RowsAffected, result.Error
}

// List 按过滤意图取一页评论，并在多种键形态之间定优先级。
//
// 优先级（M2 聚合键 > 已废弃的 chapter_url > 本人 > 书名 > 全局）此前写在 handler
// 的一段 switch 里——也就是住在最浅的那一层；现在只剩这一处。
// 各形态的具体 SQL 仍由上面各自的 finder 负责，本方法只决定走哪条。
//
// 「是否按本人过滤」只看 ByUserID，不看 UserID 是否为 0：model.CommentFilter 的注释
// 已经论证过拿 uid == 0 当哨兵会让一次 uid 为 0 的请求从「返回空列表」变成
// 「返回所有人的评论」。这里曾经是 `UserID != 0 || ByUserID`，前半句正是那个被
// 否决的写法——今天 CommentKeys 的优先级让它够不着，但下一个把两个条件并起来的
// 调用方就会踩中。
func (r *CommentRepository) List(ctx context.Context, filter model.CommentFilter) ([]model.Comment, int64, error) {
	switch {
	case len(filter.CommentKeys) > 0:
		return r.FindByCommentKeys(ctx, filter.CommentKeys, filter.Page, filter.PageSize)
	case len(filter.LegacyKeys) > 0:
		return r.FindByChapterURLs(ctx, filter.LegacyKeys, filter.BookName, filter.Page, filter.PageSize)
	case filter.ByUserID:
		return r.FindByUserID(ctx, filter.UserID, filter.Page, filter.PageSize)
	case filter.BookName != "":
		return r.Search(ctx, model.CommentQuery{BookName: filter.BookName}, filter.Page, filter.PageSize)
	default:
		return r.Search(ctx, model.CommentQuery{}, filter.Page, filter.PageSize)
	}
}
