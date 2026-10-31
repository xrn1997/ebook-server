package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"ebook-server/model"
)

// LogService 操作日志业务服务。
type LogService struct {
	logs LogStore
}

// NewLogService 创建日志服务实例。
func NewLogService(logs LogStore) *LogService {
	return &LogService{
		logs: logs,
	}
}

// Record 记录一次客户端操作。
//
// 「什么算一次操作」是操作日志的域规则（CONTEXT.md）：存活探针（/health）
// 不是任何人做的某件事；后台自身流量（/admin*）是管理界面在看数据，也不是
// 客户端行为。两者都不写库，否则审计读者得先从探针与后台刷新里找出真操作。
//
// 规则放在模块里而不是中间件里：接入方式（挂在哪张表、哪个路由组）会变，
// 规则不该跟着复制一遍。
func (s *LogService) Record(ctx context.Context, record model.OperationRecord) error {
	if strings.HasPrefix(record.Path, "/admin") || record.Path == "/health" {
		return nil
	}
	// 边界在写入方而不是中间件：客户端可控的请求行与 UA 都可能接近 HTTP 头的
	// 1 MB 上限，直接落库等于给匿名流量一个廉价写放大通道。
	record.Path = truncateUTF8(record.Path, model.MaxOperationLogPathLength)
	record.UserAgent = truncateUTF8(record.UserAgent, model.MaxOperationLogUserAgentLength)
	record.ErrorMessage = truncateUTF8(record.ErrorMessage, model.MaxOperationLogErrorMessageLength)
	return s.logs.Create(ctx, &model.OperationLog{
		OperationRecord: record,
	})
}

// truncateUTF8 按字节上限截断，同时避免把多字节字符切成半个。
//
// 输入不保证是合法 UTF-8：User-Agent 与请求行由客户端控制，能塞进任意字节。
// 边界回退到 0 说明整段前缀都是续接字节（没有合法的字符边界可退），此时
// 保留原始字节而不是把整个字段吞成空串——审计记录里「字段为空」与
// 「字段内容全是不可打印字节」是两件要能分辨的事。
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if cut == 0 {
		return s[:maxBytes]
	}
	return s[:cut]
}

// GetByUserID 根据用户 ID 获取日志
func (s *LogService) GetByUserID(ctx context.Context, userID uint, page, pageSize int) (*model.LogListResponse, error) {
	page, pageSize = model.NormalizePage(page, pageSize)

	logs, total, err := s.logs.FindByUserID(ctx, userID, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &model.LogListResponse{
		Items:    logs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}
