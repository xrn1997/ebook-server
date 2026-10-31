package errcode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ebook-server/model"
)

// bindViaGin 用真实绑定链路产出错误：validator 给出的字段名与标签就是要翻译的输入，
// 手工复刻一份 ValidationErrors 只会与真库漂移（也测不到 tag/param 的实际取值）。
func bindViaGin(t *testing.T, target any, body string) error {
	t.Helper()
	c, _ := setupErrcodeContext()
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	err := c.ShouldBindJSON(target)
	if err == nil {
		t.Fatal("预期绑定失败却成功了，用例失去意义")
	}
	return err
}

// envelopeOf 读出响应信封。
func envelopeOf(t *testing.T, w *httptest.ResponseRecorder) model.Response {
	t.Helper()
	var env model.Response
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是合法信封: %v / %s", err, w.Body.String())
	}
	return env
}

// TestRespondBindError_SpeaksHuman 校验失败回一句人话，不回校验器原文。
//
// 客户端把信封里的 error 字段原样 Toast 给最终用户，所以「Key: 'X.Content' Error:Field
// validation for 'Content' failed on the 'max' tag」这类内部字符串一旦出现就是用户可见的
// 故障（AGENTS.md「错误处理」条）。
func TestRespondBindError_SpeaksHuman(t *testing.T) {
	tooLong := `{"comment_key":"ck1:a#0","content":"` + strings.Repeat("字", 1001) + `"}`

	cases := []struct {
		name   string
		target any
		body   string
		want   string
	}{
		{"缺必填", &model.LoginRequest{}, `{"password":"x"}`, "请填写邮箱"},
		{"超上限", &model.CreateCommentRequest{}, tooLong, "评论内容长度不能超过 1000 字"},
		{"低于下限", &model.RegisterRequest{}, `{"email":"a@b.com","code":"123456","password":"123"}`, "密码长度不能少于 6 字"},
		{"格式不对", &model.LoginRequest{}, `{"email":"not-an-email","password":"secret123"}`, "邮箱格式不正确"},
		{"定长不符", &model.RegisterRequest{}, `{"email":"a@b.com","code":"12345","password":"secret123"}`, "验证码长度必须为 6 字"},
		{"链接非法", &model.UpdateUserRequest{}, `{"avatar":"not-a-url"}`, "头像地址不是合法的链接"},
		// 同一条 min 标签在字符串上量长度、在数值上量取值：把数值也说成「字」，
		// 客户端读到的就是一句看不懂的话。
		{"数值下限不说成字", &struct {
			Page int `json:"page" binding:"min=1"`
		}{}, `{"page":0}`, "该字段不能小于 1"},
		{"数值上限不说成字", &struct {
			Size int `json:"size" binding:"max=100"`
		}{}, `{"size":101}`, "该字段不能大于 100"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := bindViaGin(t, tc.target, tc.body)
			c, w := setupErrcodeContext()

			RespondBindError(c, err, "test")

			env := envelopeOf(t, w)
			if env.Code != BadRequest {
				t.Errorf("业务码应为 A0400，got %s", env.Code)
			}
			if env.Error != tc.want {
				t.Errorf("文案应为 %q，got %q", tc.want, env.Error)
			}
			for _, leak := range []string{"Key:", "Field validation", "Error:", "tag"} {
				if strings.Contains(env.Error, leak) {
					t.Errorf("响应文案泄漏校验器内部字符串 %q: %s", leak, env.Error)
				}
			}
		})
	}
}

// TestRespondBindError_UnknownFieldStillReadsWell 未登记进字段表的字段仍要给一句能读的话。
//
// 兜底分支不是装饰：请求结构体以后新加字段而忘了登记标签时，用户看到的应该是
// 「该字段……」而不是英文标签名，功能也不该因此回成 C0500。
func TestRespondBindError_UnknownFieldStillReadsWell(t *testing.T) {
	type unlabelledRequest struct {
		SomeBrandNewField string `binding:"required"`
	}
	err := bindViaGin(t, &unlabelledRequest{}, `{}`)
	c, w := setupErrcodeContext()

	RespondBindError(c, err, "test")

	if got := envelopeOf(t, w).Error; got != "请填写该字段" {
		t.Errorf("未登记字段应落到兜底说法，got %q", got)
	}
}

// TestRespondBindError_UsesDomainVocabulary 字段标签必须与 CONTEXT 的规范词一致；
// 同名 Username 要按动作区分「后台登录名」与「展示用名」。
func TestRespondBindError_UsesDomainVocabulary(t *testing.T) {
	adminLogin := struct {
		Username string `binding:"required"`
	}{}

	cases := []struct {
		name   string
		target any
		body   string
		action string
		want   string
	}{
		{"评论聚合键", &model.CreateCommentRequest{}, `{"content":"x"}`, "创建评论", "请填写评论聚合键"},
		{"展示用名", &model.UpdateUserRequest{}, `{"username":"ab"}`, "更新资料", "展示用名长度不能少于 3 字"},
		{"后台登录名", &adminLogin, `{}`, "后台登录", "请填写后台登录名"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := bindViaGin(t, tc.target, tc.body)
			c, w := setupErrcodeContext()

			RespondBindError(c, err, tc.action)

			if got := envelopeOf(t, w).Error; got != tc.want {
				t.Errorf("文案应为 %q，got %q", tc.want, got)
			}
		})
	}
}

// TestRespondBindError_SyntaxErrorLeavesNoParserDetail 非校验类的解析错误同样不回原文。
func TestRespondBindError_SyntaxErrorLeavesNoParserDetail(t *testing.T) {
	err := bindViaGin(t, &model.LoginRequest{}, `{"email":`)
	c, w := setupErrcodeContext()

	RespondBindError(c, err, "test")

	env := envelopeOf(t, w)
	if env.Code != BadRequest {
		t.Errorf("业务码应为 A0400，got %s", env.Code)
	}
	if env.Error != "请求格式不正确" {
		t.Errorf("got %q，期望不含 JSON 解析器细节的兜底文案", env.Error)
	}
	if strings.Contains(env.Error, "unexpected end") || strings.Contains(env.Error, "invalid character") {
		t.Errorf("把 JSON 解析器的英文原文回给了客户端: %s", env.Error)
	}
}
