package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ebook-server/model"
	"ebook-server/repository"
)

// TestLogService_Record_FiltersNonClientTraffic 后台自身请求（/admin*）与
// 存活探针（/health）不入审计库。
//
// 审计的语义是「某人做了某件事」：后台 UI 的访问不是客户端行为，/health 更只是
// 部署/运维的存活探测。混进来的话，看审计的人得先从一堆探针里找出真操作。
// 规则住在模块里而不是中间件里，因此这里直接调 Record 就能钉住。
func TestLogService_Record_FiltersNonClientTraffic(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	for _, path := range []string{"/api/comments", "/api/healthz", "/admin/api/stats", "/health"} {
		if err := testLogs.Record(testCtx, model.OperationRecord{
			Method: "GET", Path: path, IP: "127.0.0.1", ResponseCode: 200,
		}); err != nil {
			t.Fatalf("Record(%s): %v", path, err)
		}
	}

	resp, err := testLogs.GetByUserID(testCtx, 0, 1, 10)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total = %d, want 2（只记 /api/* 业务请求）", resp.Total)
	}
}

// TestLogService_Record_PersistsFacts 审计事实逐字段落库，包括业务码与文案。
func TestLogService_Record_PersistsFacts(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	if err := testLogs.Record(testCtx, model.OperationRecord{
		Method: "POST", Path: "/api/comments", IP: "10.0.0.1", UserAgent: "ua",
		ResponseCode: 200, UserID: 7, Username: "u", ErrorCode: "A0303", ErrorMessage: "无权删除",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	resp, err := testLogs.GetByUserID(testCtx, 7, 1, 10)
	if err != nil || resp.Total != 1 {
		t.Fatalf("GetByUserID = %+v, %v", resp, err)
	}
	got := resp.Items[0]
	if got.Method != "POST" || got.Path != "/api/comments" || got.UserAgent != "ua" ||
		got.IP != "10.0.0.1" || got.Username != "u" || got.ErrorCode != "A0303" ||
		got.ErrorMessage != "无权删除" {
		t.Fatalf("record lost fields: %+v", got)
	}
	// 请求体刻意不落库（防登录明文密码）
	if got.RequestBody != "" {
		t.Fatalf("RequestBody must stay empty, got %q", got.RequestBody)
	}
}

// TestLogService_Record_BoundsClientControlledFields 审计行必须有界。
//
// Path 与 User-Agent 都来自客户端，HTTP 头允许到 MB 级；公开的 GET /api/comments
// 也经过审计，不截断就是一条廉价的无认证写放大通道。截断还必须保持 UTF-8 完整。
func TestLogService_Record_BoundsClientControlledFields(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	longCJK := strings.Repeat("评", 600) // 1800 字节
	if err := testLogs.Record(testCtx, model.OperationRecord{
		UserID: 9, Username: "u", Method: "GET",
		Path:         "/api/" + longCJK,
		UserAgent:    longCJK,
		ResponseCode: 200,
		ErrorMessage: longCJK,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	resp, err := testLogs.GetByUserID(testCtx, 9, 1, 10)
	if err != nil || resp.Total != 1 {
		t.Fatalf("GetByUserID = %+v, %v", resp, err)
	}
	got := resp.Items[0]
	if len(got.Path) > model.MaxOperationLogPathLength {
		t.Errorf("path length = %d, want <= %d", len(got.Path), model.MaxOperationLogPathLength)
	}
	if len(got.UserAgent) > model.MaxOperationLogUserAgentLength {
		t.Errorf("user agent length = %d, want <= %d", len(got.UserAgent), model.MaxOperationLogUserAgentLength)
	}
	if len(got.ErrorMessage) > model.MaxOperationLogErrorMessageLength {
		t.Errorf("error message length = %d, want <= %d", len(got.ErrorMessage), model.MaxOperationLogErrorMessageLength)
	}
	for name, value := range map[string]string{"path": got.Path, "user_agent": got.UserAgent, "error_message": got.ErrorMessage} {
		if !utf8.ValidString(value) {
			t.Errorf("%s was cut mid-rune: %q", name, value)
		}
	}
}

// TestLogService_Record_EnvelopeChain Record 身后全链钉住：客户端字段有界截断 →
// 业务码原样落库 → OnlyFailed 的成败判据（空码与 00000 都不算失败）。
//
// 这条链跨两段实现：截断在 Record，成败判据在 repository.FindBy 的 OnlyFailed。
// 审计缓冲把超大响应截掉之后信封解析不出来、业务码留空——空码一旦被误判成失败，
// 每次超大成功响应都会混进后台「只看失败」的列表里，且没有任何一处会报错。
func TestLogService_Record_EnvelopeChain(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logRepo := repository.NewLogRepository(testDB)
	longCJK := strings.Repeat("评", 600)

	cases := []struct {
		name      string
		record    model.OperationRecord
		truncated bool // 超长字段必须被截到上限内且 UTF-8 完整
		failed    bool // 是否被 OnlyFailed 选中
	}{
		{
			name: "超长字段截断且 UTF-8 完整",
			record: model.OperationRecord{
				UserID: 101, Username: "u", Method: "GET",
				Path:         "/api/" + longCJK,
				UserAgent:    longCJK,
				ErrorMessage: longCJK,
				ResponseCode: 200,
			},
			truncated: true,
		},
		{
			name:   "空码不算失败（超大响应被审计缓冲截断后的形态）",
			record: model.OperationRecord{UserID: 102, Method: "GET", Path: "/api/big", ResponseCode: 200},
		},
		{
			name:   "成功码不算失败",
			record: model.OperationRecord{UserID: 103, Method: "GET", Path: "/api/ok", ResponseCode: 200, ErrorCode: model.SuccessCode},
		},
		{
			name: "非空错误码算失败",
			record: model.OperationRecord{
				UserID: 104, Method: "DELETE", Path: "/api/comments/1", ResponseCode: 200,
				ErrorCode: "A0303", ErrorMessage: "无权删除",
			},
			failed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := testLogs.Record(testCtx, tc.record); err != nil {
				t.Fatalf("Record: %v", err)
			}

			// 落库事实：一行、业务码原样
			rows, total, err := logRepo.FindBy(testCtx, model.LogFilter{UserID: tc.record.UserID}, 1, 10)
			if err != nil || total != 1 {
				t.Fatalf("FindBy(user=%d) = %d 行, %v", tc.record.UserID, total, err)
			}
			got := rows[0]
			if got.ErrorCode != tc.record.ErrorCode {
				t.Errorf("落库业务码 = %q, want %q", got.ErrorCode, tc.record.ErrorCode)
			}
			if tc.truncated {
				for name, value := range map[string]string{
					"path": got.Path, "user_agent": got.UserAgent, "error_message": got.ErrorMessage,
				} {
					if !utf8.ValidString(value) {
						t.Errorf("%s 被切成了半个字符: %q", name, value)
					}
				}
				if len(got.Path) > model.MaxOperationLogPathLength ||
					len(got.UserAgent) > model.MaxOperationLogUserAgentLength ||
					len(got.ErrorMessage) > model.MaxOperationLogErrorMessageLength {
					t.Errorf("字段未被截到上限内: path=%d ua=%d msg=%d",
						len(got.Path), len(got.UserAgent), len(got.ErrorMessage))
				}
			}

			// OnlyFailed 判据
			_, failedTotal, err := logRepo.FindBy(testCtx, model.LogFilter{UserID: tc.record.UserID, OnlyFailed: true}, 1, 10)
			if err != nil {
				t.Fatalf("FindBy(OnlyFailed): %v", err)
			}
			want := int64(0)
			if tc.failed {
				want = 1
			}
			if failedTotal != want {
				t.Errorf("OnlyFailed 选中 %d 条, want %d", failedTotal, want)
			}
		})
	}
}

func TestLogService_GetByUserID(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	for i := 0; i < 8; i++ {
		if err := logService.Record(testCtx, model.OperationRecord{
			UserID: 1, Username: "user1", Method: "GET", Path: "/api/a", IP: "127.0.0.1", ResponseCode: 200,
		}); err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := logService.Record(testCtx, model.OperationRecord{
			UserID: 2, Username: "user2", Method: "POST", Path: "/api/b", IP: "127.0.0.1", ResponseCode: 200,
		}); err != nil {
			t.Fatalf("seed log: %v", err)
		}
	}

	result, err := logService.GetByUserID(testCtx, 1, 1, 10)
	if err != nil {
		t.Fatalf("Failed to get logs: %v", err)
	}
	if result.Total != 8 {
		t.Errorf("Expected total 8, got %d", result.Total)
	}

	result2, _ := logService.GetByUserID(testCtx, 2, 1, 10)
	if result2.Total != 3 {
		t.Errorf("Expected total 3, got %d", result2.Total)
	}

	result3, _ := logService.GetByUserID(testCtx, 999, 1, 10)
	if result3.Total != 0 {
		t.Errorf("Expected total 0, got %d", result3.Total)
	}
}

func TestLogService_GetByUserID_PaginationBoundaries(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	logService := testLogs
	if err := logService.Record(testCtx, model.OperationRecord{
		UserID: 1, Username: "u", Method: "GET", Path: "/api/a", IP: "127.0.0.1", ResponseCode: 200,
	}); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	// page=0 应修正为 1
	result, _ := logService.GetByUserID(testCtx, 1, 0, 10)
	if result.Page != 1 {
		t.Errorf("Expected Page 1 for page=0, got %d", result.Page)
	}

	// pageSize=0 应修正为 10
	result, _ = logService.GetByUserID(testCtx, 1, 1, 0)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=0, got %d", result.PageSize)
	}

	// pageSize=101 应修正为 10
	result, _ = logService.GetByUserID(testCtx, 1, 1, 101)
	if result.PageSize != 10 {
		t.Errorf("Expected PageSize 10 for pageSize=101, got %d", result.PageSize)
	}
}

// TestLogService_GetByUserID_PagingClamp 分页钳制与评论共用同一处实现（model.NormalizePage）。
//
// 两条列表曾经各写一份同样的钳制，改缺省或改上限要记着改两处；合并后这里钉住
// 日志这条路径跟着同一套规则走：越界页码抬到 1，超大 page_size 退回缺省 10，
// 而钳制不动 Total，也带不出别人的行。
func TestLogService_GetByUserID_PagingClamp(t *testing.T) {
	setupTestDB(t)
	defer cleanupTestDB(t)

	for i := 0; i < 12; i++ {
		if err := testLogs.Record(testCtx, model.OperationRecord{
			UserID: 1, Username: "u1", Method: "GET", Path: "/api/clamp", ResponseCode: 200,
		}); err != nil {
			t.Fatalf("seed log failed: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := testLogs.Record(testCtx, model.OperationRecord{
			UserID: 2, Username: "u2", Method: "GET", Path: "/api/clamp", ResponseCode: 200,
		}); err != nil {
			t.Fatalf("seed log failed: %v", err)
		}
	}

	resp, err := testLogs.GetByUserID(testCtx, 1, -3, 5000)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if resp.Page != 1 || resp.PageSize != model.DefaultPageSize {
		t.Errorf("paging = %d/%d, want 1/%d", resp.Page, resp.PageSize, model.DefaultPageSize)
	}
	if len(resp.Items) != model.DefaultPageSize {
		t.Errorf("items = %d, want %d", len(resp.Items), model.DefaultPageSize)
	}
	if resp.Total != 12 {
		t.Errorf("total = %d, want 12（钳制不该顺手截断总数）", resp.Total)
	}
}
