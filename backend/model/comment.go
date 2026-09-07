package model

import (
	"time"

	"gorm.io/gorm"
)

// Comment 评论实体（GORM 模型）。
//
// 每条评论属于一个用户，支持软删除。聚合键是 CommentKey：由客户端从「书名+作者」
// 派生的不透明 token（ck1:sha256[...]，章评再追加 #章序号），服务端不校验格式、
// 不解释含义，只按它过滤与改键——正因没有人分配，各客户端才能无协调地算出同一个桶。
//
// ChapterName/BookName 为**冗余展示快照**：只供列表显示，不再参与聚合。
// ChapterURL 是 M2 之前的旧聚合键（书源章节 URL），已降级为过渡期兼容读路径：
// 服务端无法把它重算成 CommentKey（算 ck1 需要作者，而作者从未入库），
// 因此旧行只能靠 chapter_url 参数继续可见。见 docs/adr/0014。
type Comment struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"index;not null"` // 关联 users.uid
	User        User           `json:"user" gorm:"foreignKey:UserID"` // 所属用户（预加载）
	Content     string         `json:"content" gorm:"type:text;not null"`
	CommentKey  string         `json:"comment_key" gorm:"type:text;index"` // 评论聚合键（M2）：客户端派生的不透明 token，服务端只存不解释
	ChapterURL  string         `json:"chapter_url" gorm:"type:text;index"` // 已废弃：旧聚合键，降级为过渡期兼容读路径与展示字段
	ChapterName string         `json:"chapter_name" gorm:"size:200"`       // 章节名快照
	BookName    string         `json:"book_name" gorm:"size:200"`          // 书名快照
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// TableName 指定表名
func (Comment) TableName() string {
	return "comments"
}

// CreateCommentRequest 创建评论请求。
//
// comment_key **必填**（M2）：它是评论唯一的聚合键，缺了键的评论会落进空桶、
// 再没有任何查询路径能把它读回来。刻意用 required 而非 omitempty——静默丢弃这个
// 字段正是本次要对齐掉的 bug，宁可回 A0400 让客户端当场发现。
// 键由客户端派生，服务端**不校验格式**（只限长度，理由同 ADR-0011 不校验 URL 格式）：
// 后端无书源与作者数据，既无法判断键是否正确，也不该把算法版本写死进服务端。
//
// chapter_url / chapter_name / book_name 全部可选，仅作过渡期兼容与展示快照。
type CreateCommentRequest struct {
	Content     string `json:"content" binding:"required,min=1,max=1000"`
	CommentKey  string `json:"comment_key" binding:"required,max=200"`
	ChapterURL  string `json:"chapter_url" binding:"omitempty,max=2048"`
	ChapterName string `json:"chapter_name" binding:"omitempty,max=200"`
	BookName    string `json:"book_name" binding:"omitempty,max=200"`
}

// MigrateCommentKeyRequest 迁移评论聚合键请求（合并书籍 / 改元数据修键场景）。
//
// old_key/new_key 对应 comment 的 **comment_key** 字段（M2 前指向 chapter_url，
// 随聚合键换轨一并改指）；只迁移当前用户名下的评论。
// 长度上限必须与 CreateCommentRequest.CommentKey 一致（同一列同一约束）：否则可以把
// 评论迁到一个自己再也提交不出、也就永远读不回来的键上。
//
// **old_key 用 omitempty**：空串对应尚未换键的历史行（chapter_url 时代入库、没有
// comment_key 的记录），允许用户把自己的旧评论一次性收进正确的桶——这是换轨时必需的迁移方向。
// **new_key 用 required**：迁往空键等于把评论推进没有任何 `comment_keys` 查询能命中的
// 黑洞，与「创建评论必须带键」是同一条理由，不能从迁移侧开后门。
// 旧的空键语义（「书籍级评论」）在 M2 已由「不含 # 的作品键」表达，因此空 new_key 不再有用武之地。
type MigrateCommentKeyRequest struct {
	OldKey string `json:"old_key" binding:"omitempty,max=200"`
	NewKey string `json:"new_key" binding:"required,max=200"`
}

// MigrateCommentKeyResponse 迁移评论聚合键响应。
type MigrateCommentKeyResponse struct {
	MigratedCount int64 `json:"migrated_count"`
}

// CommentQuery 后台评论列表的筛选条件。
//
// 零值字段一律不参与过滤，因此空调 = 全量列表（后台「评论搜索」用关键字，
// 按书名收窄是同一件事的另一个维度，不必再开一个方法）。
type CommentQuery struct {
	Keyword  string // 评论内容模糊匹配
	BookName string // 书名精确匹配
}

// CommentUserView 评论内嵌的用户视图（ADR-0011 契约：仅 uid/username/nickname/avatar）。
//
// 与 User 实体分离：不序列化 email 等账号字段，避免评论列表泄露用户隐私。
type CommentUserView struct {
	UID      uint   `json:"uid"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// CommentResponse 评论响应视图。
//
// 与实体分离的独立契约结构（ADR-0011）：user 只含四个展示字段；
// add_time 固定 Asia/Shanghai 时区 + "yyyy-MM-dd HH:mm:ss" 格式，不依赖服务器时区。
// comment_key 是 M2 的聚合键；未换键的历史行序列化为空串（客户端按「无键」处理）。
// chapter_url 刻意继续返回：旧客户端仍靠它展示与再查询，新客户端忽略。
type CommentResponse struct {
	ID          uint            `json:"id"`
	User        CommentUserView `json:"user"`
	CommentKey  string          `json:"comment_key"`
	Content     string          `json:"content"`
	ChapterURL  string          `json:"chapter_url"`
	ChapterName string          `json:"chapter_name"`
	BookName    string          `json:"book_name"`
	AddTime     string          `json:"add_time"`
}

// NewCommentResponse 把评论实体转换为响应视图。
func NewCommentResponse(c *Comment) CommentResponse {
	return CommentResponse{
		ID:          c.ID,
		User:        newCommentUserView(&c.User),
		CommentKey:  c.CommentKey,
		Content:     c.Content,
		ChapterURL:  c.ChapterURL,
		ChapterName: c.ChapterName,
		BookName:    c.BookName,
		AddTime:     FormatShanghaiTime(c.CreatedAt),
	}
}

// newCommentUserView 从 User 实体提取评论视图（空用户回零值）。
func newCommentUserView(u *User) CommentUserView {
	if u == nil {
		return CommentUserView{}
	}
	return CommentUserView{
		UID:      u.UID,
		Username: u.Username,
		Nickname: u.Nickname,
		Avatar:   u.Avatar,
	}
}

// CommentListResponse 评论列表响应
type CommentListResponse struct {
	Items    []CommentResponse `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}
