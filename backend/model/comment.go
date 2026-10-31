package model

import (
	"fmt"
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

// MaxCommentKeyLength 聚合键的字节上限，是 comment_key 这一列的唯一运行时约束来源。
//
// 改键、按键过滤两处都要靠它挡脏数据与「一条语句打满 SQLite 绑定变量」，
// 历史上同一个 200 抄了四份、其中后台那份没有测试钉住。绑定 tag 只能写字面量，
// 所以 tag 里的 max=200 由 handler 与 internal/admin 各自的反射用例钉住一致性
// （TestCommentKeyLengthLimitMatchesModelBinding、
// TestRehashRequestBindingMatchesColumnLimit）。
const MaxCommentKeyLength = 200

// CommentKeyScope 一次改键作用的归属范围。
//
// 公开的 migrate 与后台的 rehash 是同一件事的两个 scope——此前两条路径各写一遍
// 判重、长度、空键规则与响应形态，差一点就要在两处修。把归属做成数据而不是两条路径，
// 规则就只剩一处实现。
//
// 「是否全员」是显式一位，不是拿 userID == 0 当哨兵：那样一次 uid 为 0 的请求
// （token 不指名账号、或调用方漏了校验）就会从「匹配 0 行」静默变成「改写全表」。
type CommentKeyScope struct {
	userID uint
	all    bool
}

// ScopeOfUser 只影响该用户的行。
func ScopeOfUser(userID uint) CommentKeyScope { return CommentKeyScope{userID: userID} }

// ScopeAllUsers 跨全部用户改键，只能由后台表面触达（ADR-0010 的网络隔离之后）。
func ScopeAllUsers() CommentKeyScope { return CommentKeyScope{all: true} }

// UserID 返回归属过滤的用户 ID，以及「本次是否限定了用户」。
//
// 第二个返回值决定实现要不要挂 user_id 条件：限定用户时哪怕 ID 是 0 也照样挂上
// （匹配 0 行才是安全的结果）。
func (s CommentKeyScope) UserID() (userID uint, byUser bool) { return s.userID, !s.all }

// AllowsEmptyOldKey 报告「空旧键」在本 scope 下是否有意义。
//
// 空 comment_key 是换轨前历史行的标记，所以「把本人尚未换键的评论收进正确的桶」
// 这个迁移方向必须可表达；但全员范围内的空旧键意味着把所有用户的历史行一次扫进
// 同一个桶——那不是修复污染，而是制造一个更大的污染桶。
func (s CommentKeyScope) AllowsEmptyOldKey() bool { return !s.all }

// CommentScope 一次评论删除操作的归属范围。
//
// 与 CommentKeyScope 同形但语义不同：改键的范围决定 WHERE 是否带 user_id；
// 删除的范围决定是否校验作者。治理删（后台）与本人删（公开）因此只差一个 scope，
// 不再各写一份「查存在 → 删」。
type CommentScope struct {
	userID uint
	admin  bool
}

// CommentScopeOfAuthor 只允许删除该用户自己的评论。
func CommentScopeOfAuthor(userID uint) CommentScope { return CommentScope{userID: userID} }

// CommentScopeOfAdmin 治理删：只校验评论存在，不校验归属。
func CommentScopeOfAdmin() CommentScope { return CommentScope{admin: true} }

// UserID 返回作者过滤的用户 ID，以及本次是否限定作者。
func (s CommentScope) UserID() (uint, bool) { return s.userID, !s.admin }

// IsAdmin 报告本次是否为治理删。
func (s CommentScope) IsAdmin() bool { return s.admin }

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
// 曾经后台另有一份 20：两处各写各的，于是「省略 page_size 会拿到多少条」这个问题
// 没有唯一答案。缺省属于分页语义而不是属于某个表面，所以只留一处（后台的真实调用方
// 本就显式传 20，收敛缺省不改变它看到的行为）。
const DefaultPageSize = 10

// MaxPageSize 单次请求可取回的最大行数。
//
// 必须有上限：page_size 是客户端可控参数，不设限等于允许任何人用一条请求把整张表
// 读走并序列化成响应。100 足够覆盖「一次拉满一屏 + 前端预取」的实际用法。
const MaxPageSize = 100

// NormalizePage 归一化分页参数：页码最小 1，每页数量缺省 DefaultPageSize、上限 MaxPageSize。
//
// 分页语义是全站列表接口的统一约定，收敛在一处：评论与操作日志此前各有一份
// 同样的钳制，改上限或改缺省就得记着改两处。非数字、缺省、越界都归到缺省值，
// 因此调用方可以直接把未经检查的原始整数交进来。
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
// 聚合键换轨后读路径有「M2 键 / 已废弃的 chapter_url / 书名 / 本人 / 全局」五种形态。
// 此前它们在 interface 上是五个方法（各只差一个 WHERE），在 handler 里是一段决定
// 优先级的 switch——键的优先级与边界都是查询语义，却住在最浅的那一层。
// 收成一个值之后：调用方只递意图，module 只有一条 List。
type CommentFilter struct {
	// CommentKeys 是 M2 聚合键（主读路径）：单个即精确匹配，多个取并集
	// （跨书源合并同一作品时客户端会把该作品的多个键一并传来）。
	CommentKeys []string
	// LegacyKeys 是已废弃的旧聚合键（chapter_url），仅为让未换键的历史行继续可读而保留。
	LegacyKeys []string
	// BookName 书名快照过滤：配合 LegacyKeys 二次过滤，或单独收窄到一本书。
	// 书名在 M2 后不再是聚合维度，因此主读路径（CommentKeys 非空）下刻意不参与过滤。
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
// 不合规返回映射为 A0400 的领域错误（见 pkg/errcode 表）：这些上限是查询语义的一部分，
// 由 module 判定比让每个调用方各抄一份更有把握——历史上同一个上限抄在 handler 与
// 后台两处，其中一处没有测试钉住。
func (f *CommentFilter) Normalize() error {
	f.Page, f.PageSize = NormalizePage(f.Page, f.PageSize)

	if len(f.CommentKeys) > MaxCommentFilterKeys {
		return fmt.Errorf("%w：comment_keys 最多 %d 个", ErrCommentFilterInvalid, MaxCommentFilterKeys)
	}
	for _, key := range f.CommentKeys {
		// 空键是换轨前历史行的标记，不是可查询的桶：放进去就会命中那些
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

// AdminCommentView 后台评论列表的固定视图。
//
// 后台不再直接序列化 model.Comment：实体加字段不该自动变成对外契约，user 也只含
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
