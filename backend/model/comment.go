package model

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Comment 评论实体（GORM 模型）。
//
// CommentKey 是评论聚合键：客户端从「书名+作者」派生的不透明 token，服务端只存
// 不解释（ADR-0014）。ChapterName/BookName 是冗余展示快照，只供列表显示，不参与
// 聚合。ChapterURL 是已废弃的旧聚合键，仅作兼容读路径：存量行无法重算成 CommentKey
// （算 ck1 需要作者，而作者从未入库），只能靠 chapter_url 参数继续可见。
type Comment struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	UserID      uint           `json:"user_id" gorm:"index;not null"` // 关联 users.uid
	User        User           `json:"user" gorm:"foreignKey:UserID"` // 所属用户（预加载）
	Content     string         `json:"content" gorm:"type:text;not null"`
	CommentKey  string         `json:"comment_key" gorm:"type:text;index"` // 评论聚合键：客户端派生的不透明 token，服务端只存不解释
	ChapterURL  string         `json:"chapter_url" gorm:"type:text;index"` // 已废弃的旧聚合键，仅作兼容读路径与展示字段
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
// comment_key 必填（required 而非 omitempty）：缺键的评论落进空桶、任何查询都读不
// 回来，宁可回 A0400 也不静默丢弃。键由客户端派生，服务端不校验格式、只限长度
// （ADR-0014）。chapter_url/chapter_name/book_name 全部可选，仅作兼容读路径与展示快照。
type CreateCommentRequest struct {
	Content     string `json:"content" binding:"required,min=1,max=1000"`
	CommentKey  string `json:"comment_key" binding:"required,max=200"`
	ChapterURL  string `json:"chapter_url" binding:"omitempty,max=2048"`
	ChapterName string `json:"chapter_name" binding:"omitempty,max=200"`
	BookName    string `json:"book_name" binding:"omitempty,max=200"`
}

// MigrateCommentKeyRequest 迁移评论聚合键请求（合并书籍 / 改元数据修键场景）。
//
// old_key/new_key 对应 comment_key 列，只迁移当前用户名下的评论。长度上限必须与
// CreateCommentRequest.CommentKey 一致（同一列同一约束），否则可以把评论迁到一个
// 自己再也提交不出、也就读不回来的键上。old_key 用 omitempty：空串对应 comment_key
// 为空的历史行，允许用户把它们收进正确的桶。new_key 用 required：迁往空键等于把评论
// 推进任何查询都命不中的黑洞，与「创建评论必须带键」是同一条理由（ADR-0014）；
// 书籍级评论由不含 # 的作品键表达，空 new_key 没有用武之地。
type MigrateCommentKeyRequest struct {
	OldKey string `json:"old_key" binding:"omitempty,max=200"`
	NewKey string `json:"new_key" binding:"required,max=200"`
}

// MigrateCommentKeyResponse 迁移评论聚合键响应。
type MigrateCommentKeyResponse struct {
	MigratedCount int64 `json:"migrated_count"`
}

// MaxCommentKeyLength 聚合键的字节上限，是 comment_key 这一列唯一的运行时约束来源，
// 改键与按键过滤两处共用。
//
// binding tag 只能写字面量、引用不了常量，tag 里的 max=200 因此由反射测试钉住：
// 本包的 TestCommentKeyBindingTagsMatchMaxCommentKeyLength 管 Create.CommentKey 与
// MigrateCommentKeyRequest 的两个键；后台 rehashRequest 在本包之外、这里的反射扫不到
// 它，由 internal/admin 同包的 TestRehashRequestBindingMatchesMaxCommentKeyLength
// 钉住。两枚测试读同一个常量、各钉各自包里的 struct，不是同一份知识抄两遍——
// 只留一枚，常量与另一侧 tag 漂移时合法的键会在那边被 A0400 拒掉。
const MaxCommentKeyLength = 200

// ParseCommentKeyFilters 拆出 comment_keys 过滤参数（主读路径）。
//
// 客户端用单个 @Query 把多个键拼成逗号分隔的一个值，所以这里按逗号切分；也容忍
// `?comment_keys=a&comment_keys=b` 的重复参数形态（逐个入参处理，不去重）。键本身
// 永不含逗号（`ck1:` + 十六进制摘要 + 可选 `#章序号`），切分无歧义。空串按「未提供
// 该键」丢弃（`?comment_keys=` 不带值是常见形态，此时落全局最新列表）。
// 长度与个数上限不在这里判定，见 CommentFilter.Normalize。
func ParseCommentKeyFilters(raw []string) []string {
	keys := make([]string, 0, len(raw))
	for _, item := range raw {
		keys = append(keys, strings.Split(item, ",")...)
	}
	return dropEmpty(keys)
}

// ParseLegacyKeyFilters 拆出 chapter_url（已废弃的旧聚合键）过滤参数。
//
// chapter_url 可以含逗号（它是 URL），因此不按逗号切分，只丢弃空值。
func ParseLegacyKeyFilters(raw []string) []string {
	return dropEmpty(raw)
}

// dropEmpty 丢弃空项：两个过滤入口共用同一份「空串不算键」的语义。
func dropEmpty(items []string) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" {
			keys = append(keys, item)
		}
	}
	return keys
}

// CommentKeyScope 一次改键作用的归属范围。
//
// 公开的 migrate 与后台的 rehash 是同一件事的两个 scope：归属做成数据而不是两条
// 路径，判重、长度、空键规则与响应形态只剩一处实现。
//
// 「是否全员」是显式一位，不拿 userID == 0 当哨兵：否则一次 uid 为 0 的请求
// （token 不指名账号、或调用方漏了校验）就会从「匹配 0 行」静默变成「改写全表」。
type CommentKeyScope struct {
	userID uint
	all    bool
}

// ScopeOfUser 只影响该用户的行。
func ScopeOfUser(userID uint) CommentKeyScope { return CommentKeyScope{userID: userID} }

// ScopeAllUsers 跨全部用户改键，只能由后台表面触达（ADR-0010）。
func ScopeAllUsers() CommentKeyScope { return CommentKeyScope{all: true} }

// UserID 返回归属过滤的用户 ID，以及「是否限定了用户」。
//
// 第二个返回值决定实现要不要挂 user_id 条件：限定用户时哪怕 ID 是 0 也照样挂上
// （匹配 0 行才是安全的结果）。
func (s CommentKeyScope) UserID() (userID uint, byUser bool) { return s.userID, !s.all }

// AllowsEmptyOldKey 报告「空旧键」在本 scope 下是否有意义。
//
// 空 comment_key 是尚未换键的历史行的标记，「把本人历史行收进正确的桶」这个迁移
// 方向必须可表达；但全员范围内的空旧键会把所有用户的历史行一次扫进同一个桶——
// 那不是修复污染，而是制造一个更大的污染桶（ADR-0014）。
func (s CommentKeyScope) AllowsEmptyOldKey() bool { return !s.all }

// CommentScope 一次评论删除操作的归属范围（ADR-0015）。
//
// 与 CommentKeyScope 同形但语义不同：改键的范围决定 WHERE 是否带 user_id；
// 删除的范围决定是否校验作者。治理删（后台）与本人删（公开）因此只差一个 scope。
type CommentScope struct {
	userID uint
	admin  bool
}

// CommentScopeOfAuthor 只允许删除该用户自己的评论。
func CommentScopeOfAuthor(userID uint) CommentScope { return CommentScope{userID: userID} }

// CommentScopeOfAdmin 治理删：只校验评论存在，不校验归属。
func CommentScopeOfAdmin() CommentScope { return CommentScope{admin: true} }

// UserID 返回作者过滤的用户 ID，以及是否限定了作者。
func (s CommentScope) UserID() (uint, bool) { return s.userID, !s.admin }

// IsAdmin 报告是否为治理删。
func (s CommentScope) IsAdmin() bool { return s.admin }

// DeleteOutcome 一次条件删除的结果归因。
//
// 条件 DELETE 只回答「删到行没有」；没删到时把原因判出来是存储实现的职责
// （ADR-0015 §2），service 只把这个值映射成错误码。RowsAffected 是「是否真的删了
// 一行」的唯一判据。
type DeleteOutcome int

const (
	// DeleteOutcomeUnknown 零值，刻意不代表任何归因：漏赋值（含未来新增的早返回）
	// 会被 service 映射成 A0304 而不是谎报成功。成功必须是显式返回的。
	DeleteOutcomeUnknown DeleteOutcome = iota
	// DeleteDeleted 删到了行。
	DeleteDeleted
	// DeleteNotFound 行不存在，或已不可见（并发删除留下的软删行对调用方同样不在）。
	DeleteNotFound
	// DeleteNotOwner 行存在且可见，但作者不是 scope 指定的用户。
	DeleteNotOwner
)

// String 让断言的失败信息自解释：读到的是归因名，而不是要回头数 iota 的 `outcome = 0`。
func (o DeleteOutcome) String() string {
	switch o {
	case DeleteDeleted:
		return "DeleteDeleted"
	case DeleteNotFound:
		return "DeleteNotFound"
	case DeleteNotOwner:
		return "DeleteNotOwner"
	default:
		return "DeleteOutcomeUnknown"
	}
}

// MaxChapterURLLength 单个 chapter_url（已废弃的旧聚合键）的字节上限。
//
// 创建请求的 binding max 按字符计，这个常量按字节计；CommentService.Create 会按它
// 再判一次，保证写进去的历史行仍能被 chapter_url 过滤条件读出来。
const MaxChapterURLLength = 2048

// MaxCommentFilterKeys 一次查询允许的键个数上限（comment_keys 与 chapter_url 共用）。
//
// 必须设上限：每个键都会被 GORM 展开成一个绑定变量，SQLite 单语句变量数默认上限 999，
// 超出直接报错并落成 C0500；而评论列表是无需登录的公开端点，不限长度即可被任意人打满。
// 上限来自「一条语句的绑定变量总数」而非某个键的形态，因此两个键参数共用一个值；
// 50 远大于「合并一本书涉及的章节数」这一真实场景。
const MaxCommentFilterKeys = 50

// DefaultPageSize 全站列表的缺省每页数量——公开 API 与后台是同一个值。
//
// 缺省属于分页语义而不是某个表面，只留一处，「省略 page_size 会拿到多少条」才有唯一答案。
const DefaultPageSize = 10

// MaxPageSize 单次请求可取回的最大行数。
//
// 必须有上限：page_size 是客户端可控参数，不设限等于允许任何人用一条请求把整张表
// 读走并序列化成响应。100 足够覆盖「一次拉满一屏 + 前端预取」的实际用法。
const MaxPageSize = 100

// NormalizePage 归一化分页参数：页码最小 1，每页数量缺省 DefaultPageSize、上限 MaxPageSize。
//
// 分页钳制收敛在一处（评论与操作日志共用同一个约定）。缺省、非数字与越界都归到
// 缺省值，调用方可以直接把未经检查的原始整数交进来。
func NormalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}
	return page, pageSize
}

// CommentFilter 一次评论列表查询的完整意图。
//
// 读路径有「聚合键 / 已废弃的 chapter_url / 书名 / 本人 / 全局」五种形态，键的优先级
// 与边界都是查询语义，收成一个值交给 module 的一条 List，而不是散在 handler 的 switch 里。
type CommentFilter struct {
	// CommentKeys 是评论聚合键过滤（主读路径）：单个即精确匹配，多个取并集
	// （跨书源合并同一作品时客户端会把该作品的多个键一并传来）。
	CommentKeys []string
	// LegacyKeys 是已废弃的旧聚合键（chapter_url），仅为让未换键的历史行继续可读而保留。
	LegacyKeys []string
	// BookName 书名快照过滤：配合 LegacyKeys 二次过滤，或单独收窄到一本书。
	// 书名不是聚合维度，因此主读路径（CommentKeys 非空）下刻意不参与过滤。
	BookName string
	// ByUserID 为真即只看该用户的评论（「我的评论」路径）。
	//
	// 与 UserID 分成两半是刻意的：拿 uid == 0 当「不按用户过滤」的哨兵，会让一次
	// uid 为 0 的请求从「返回空列表」变成「返回所有人的评论」。
	ByUserID bool
	UserID   uint
	Page     int
	PageSize int
}

// Normalize 钳制分页并校验键过滤的边界，就地修正 f。
//
// 不合规返回映射为 A0400 的领域错误（见 pkg/errcode 表）：这些上限是查询语义的
// 一部分，由 module 判定，调用方不必各抄一份。
func (f *CommentFilter) Normalize() error {
	f.Page, f.PageSize = NormalizePage(f.Page, f.PageSize)

	if len(f.CommentKeys) > MaxCommentFilterKeys {
		return fmt.Errorf("%w：comment_keys 最多 %d 个", ErrCommentFilterInvalid, MaxCommentFilterKeys)
	}
	for _, key := range f.CommentKeys {
		// 空键是尚未换键的历史行的标记，不是可查询的桶：放进去就会命中那些
		// 「任何 comment_key 查询都命中不到」的行（ADR-0014）。
		if key == "" {
			return fmt.Errorf("%w：comment_keys 不能包含空键", ErrCommentFilterInvalid)
		}
		if len(key) > MaxCommentKeyLength {
			return fmt.Errorf("%w：comment_keys 单个最长 %d 字节", ErrCommentFilterInvalid, MaxCommentKeyLength)
		}
	}

	if len(f.LegacyKeys) > MaxCommentFilterKeys {
		return fmt.Errorf("%w：chapter_url 最多 %d 个", ErrCommentFilterInvalid, MaxCommentFilterKeys)
	}
	for _, key := range f.LegacyKeys {
		if key == "" {
			return fmt.Errorf("%w：chapter_url 不能包含空键", ErrCommentFilterInvalid)
		}
		if len(key) > MaxChapterURLLength {
			return fmt.Errorf("%w：chapter_url 单个最长 %d 字节", ErrCommentFilterInvalid, MaxChapterURLLength)
		}
	}
	return nil
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
// comment_key 为空的历史行序列化为空串（客户端按「无键」处理）。chapter_url 刻意
// 继续返回：旧客户端仍靠它展示与再查询，新客户端忽略。
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

// AdminCommentView 后台评论列表的固定视图（ADR-0015）。
//
// 后台不直接序列化 model.Comment：实体加字段不该自动变成对外契约，user 也只含
// 展示字段（email 在用户详情里看）。created_at 保持 Go time 的 RFC3339 形态，
// 与后台前端现有字段一致。
type AdminCommentView struct {
	ID          uint            `json:"id"`
	UserID      uint            `json:"user_id"`
	User        CommentUserView `json:"user"`
	Content     string          `json:"content"`
	CommentKey  string          `json:"comment_key"`
	ChapterURL  string          `json:"chapter_url"`
	ChapterName string          `json:"chapter_name"`
	BookName    string          `json:"book_name"`
	CreatedAt   time.Time       `json:"created_at"`
}

// AdminCommentListResponse 后台评论列表信封（list + total，不含分页回显）。
type AdminCommentListResponse struct {
	List  []AdminCommentView `json:"list"`
	Total int64              `json:"total"`
}

// NewAdminCommentView 把评论实体转换为后台视图。
func NewAdminCommentView(c *Comment) AdminCommentView {
	return AdminCommentView{
		ID:          c.ID,
		UserID:      c.UserID,
		User:        newCommentUserView(&c.User),
		Content:     c.Content,
		CommentKey:  c.CommentKey,
		ChapterURL:  c.ChapterURL,
		ChapterName: c.ChapterName,
		BookName:    c.BookName,
		CreatedAt:   c.CreatedAt,
	}
}

// NewAdminCommentListResponse 转换一页后台评论。
func NewAdminCommentListResponse(comments []Comment, total int64) *AdminCommentListResponse {
	list := make([]AdminCommentView, 0, len(comments))
	for i := range comments {
		list = append(list, NewAdminCommentView(&comments[i]))
	}
	return &AdminCommentListResponse{List: list, Total: total}
}
