package service

import (
	"context"
	"fmt"

	"ebook-server/model"
)

// CommentService 评论业务服务。
type CommentService struct {
	comments CommentStore
}

// NewCommentService 创建评论服务实例。
func NewCommentService(comments CommentStore) *CommentService {
	return &CommentService{
		comments: comments,
	}
}

// Create 创建评论
func (s *CommentService) Create(ctx context.Context, userID uint, req *model.CreateCommentRequest) (*model.CommentResponse, error) {
	// 缺键必须在模块边界挡住，不能只靠 handler 的 binding=required：空 comment_key
	// 是「换轨前历史行」的标记，写进去的评论没有任何 comment_keys 查询能命中，
	// 也没有迁移路径能把它读回来（ADR-0014）。内部调用方不经过 binding。
	if req.CommentKey == "" {
		return nil, fmt.Errorf("%w：评论聚合键不能为空", model.ErrCommentKeyInvalid)
	}
	// binding 的 max 按字符（validator 语义）计数，而列约束与查询上限按字节描述；
	// 多字节键可能通过 handler 校验却超过模块边界。写入前按字节再判一次，
	// 保证「建得进去」与「迁得出去」是同一个上限。
	if len(req.CommentKey) > model.MaxCommentKeyLength {
		return nil, fmt.Errorf("%w：聚合键最长 %d 字节", model.ErrCommentKeyInvalid, model.MaxCommentKeyLength)
	}
	// chapter_url 是已废弃的旧聚合键，但过滤路径仍按字节上限读它：写入时若不拦，
	// 一个多字节 URL 可以建得进去、却永远查不回来（列表参数会回 A0400）。
	if len(req.ChapterURL) > model.MaxChapterURLLength {
		return nil, fmt.Errorf("%w：章节地址最长 %d 字节", model.ErrCommentKeyInvalid, model.MaxChapterURLLength)
	}

	comment := &model.Comment{
		UserID:      userID,
		Content:     req.Content,
		CommentKey:  req.CommentKey,
		ChapterURL:  req.ChapterURL,
		ChapterName: req.ChapterName,
		BookName:    req.BookName,
	}

	if err := s.comments.Create(ctx, comment); err != nil {
		return nil, err
	}

	// 重新查询以加载关联的用户信息
	created, err := s.comments.FindByID(ctx, comment.ID)
	if err != nil {
		return nil, err
	}
	resp := model.NewCommentResponse(created)
	return &resp, nil
}

// List 按过滤意图取一页评论——评论读路径的唯一入口。
//
// 键的优先级（M2 聚合键 > 已废弃的 chapter_url > 本人 > 书名 > 全局）与边界上限
// 都是查询语义，交给 module 判定：调用方只递意图，不再各自决定谁赢。
// 分页钳制同样在这里（Normalize），五个读路径共用一条。
func (s *CommentService) List(ctx context.Context, filter model.CommentFilter) (*model.CommentListResponse, error) {
	if err := filter.Normalize(); err != nil {
		return nil, err
	}

	comments, total, err := s.comments.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	return toListResponse(comments, total, filter.Page, filter.PageSize), nil
}

// toListResponse 把实体列表转换为响应视图列表（ADR-0011 契约）。
func toListResponse(comments []model.Comment, total int64, page, pageSize int) *model.CommentListResponse {
	items := make([]model.CommentResponse, 0, len(comments))
	for i := range comments {
		items = append(items, model.NewCommentResponse(&comments[i]))
	}
	return &model.CommentListResponse{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}
}

// Delete 删除评论：本人删校验归属，治理删（后台）不做归属校验。
//
// 非本人返回 model.ErrCommentNotOwner（映射 A0303）——评论域的「非本人」是专门语义，
// 客户端可据此给出「无权删除他人评论」的明确提示，而不是通用「无权限」。
// 删除本身是条件 DELETE：0 行才回落查存在性，重复删除因此得到 A0304 而非假成功。
func (s *CommentService) Delete(ctx context.Context, commentID uint, scope model.CommentScope) error {
	deleted, err := s.comments.DeleteScoped(ctx, commentID, scope)
	if err != nil {
		return err
	}
	if deleted {
		return nil
	}

	comment, err := s.comments.FindByID(ctx, commentID)
	if err != nil {
		return err // 评论不存在 → model.ErrCommentNotFound（A0304）
	}
	if !scope.IsAdmin() {
		uid, _ := scope.UserID()
		if comment.UserID != uid {
			return model.ErrCommentNotOwner
		}
	}
	// 存在但没删掉且归属无误：只可能是并发删除，按「已经不在了」处理。
	return model.ErrCommentNotFound
}

// ListForAdmin 后台评论列表：查询语义、分页钳制与固定视图都在模块内。
func (s *CommentService) ListForAdmin(ctx context.Context, query model.CommentQuery, page, pageSize int) (*model.AdminCommentListResponse, error) {
	page, pageSize = model.NormalizePage(page, pageSize)
	comments, total, err := s.comments.Search(ctx, query, page, pageSize)
	if err != nil {
		return nil, err
	}
	return model.NewAdminCommentListResponse(comments, total), nil
}

// Count 评论总数（后台概览）。
func (s *CommentService) Count(ctx context.Context) (int64, error) { return s.comments.Count(ctx) }

// CountByUserID 某用户的评论数（后台用户详情）。
func (s *CommentService) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	return s.comments.CountByUserID(ctx, userID)
}

// MigrateKeys 在给定归属范围内把 oldKey 下的评论改到新键——评论聚合键迁移的唯一实现。
//
// 公开的 migrate 与后台的 rehash 只差一个 scope，因此规则全部收敛在这里：
//   - 目标键为空 → model.ErrCommentKeyInvalid（A0400）：空键是任何 comment_keys
//     查询都命中不到的黑洞，与「创建评论必须带键」是同一条理由
//   - 全员范围内旧键为空 → 同 A0400：那会把所有用户尚未换键的历史行一次扫进
//     同一个桶，制造一个更大的污染桶；限定用户时空旧键是合法的迁移方向
//   - 任一键超出 model.MaxCommentKeyLength → 同 A0400：这一列装不下的键写进去
//     就再也提交不出、也就永远读不回来
//   - 新旧相同 → model.ErrCommentKeySame（A0305）
//
// 「键本身不合规」判定先于「新旧相同」：空/空这种退化输入是缺参数，不是一次
// 恰好没动的迁移——回 A0305 会把调用方引向「换个不同的键试试」而不是「把键传全」。
//
// 无匹配行返回 0 而非错误（幂等语义，客户端可重复调用）。
func (s *CommentService) MigrateKeys(ctx context.Context, scope model.CommentKeyScope, oldKey, newKey string) (int64, error) {
	if newKey == "" {
		return 0, fmt.Errorf("%w：目标聚合键不能为空", model.ErrCommentKeyInvalid)
	}
	if oldKey == "" && !scope.AllowsEmptyOldKey() {
		return 0, fmt.Errorf("%w：全局改键必须指定旧键", model.ErrCommentKeyInvalid)
	}
	if len(oldKey) > model.MaxCommentKeyLength || len(newKey) > model.MaxCommentKeyLength {
		return 0, fmt.Errorf("%w：聚合键最长 %d 字节", model.ErrCommentKeyInvalid, model.MaxCommentKeyLength)
	}
	if oldKey == newKey {
		return 0, model.ErrCommentKeySame
	}
	return s.comments.MigrateKey(ctx, scope, oldKey, newKey)
}

// MigrateKey 迁移当前用户在旧聚合键下的评论到新聚合键（合并书籍 / 改元数据修键）。
//
// 作用列是 comment_key。oldKey 传空串 = 把本人尚未换键的历史行一次性收进正确的桶。
// 全部规则见 MigrateKeys（同一实现，仅归属范围不同）。
func (s *CommentService) MigrateKey(ctx context.Context, userID uint, oldKey, newKey string) (int64, error) {
	return s.MigrateKeys(ctx, model.ScopeOfUser(userID), oldKey, newKey)
}
