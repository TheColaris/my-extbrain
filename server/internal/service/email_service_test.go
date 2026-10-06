package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"extbrain-server/internal/auth"
)

// mockResend 假 Resend 端点：记录 Authorization 与请求体，按 status 返回。
func mockResend(t *testing.T, status int) (*httptest.Server, *map[string]any, *string) {
	t.Helper()
	body := map[string]any{}
	authHeader := ""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
		if status >= 300 {
			_, _ = w.Write([]byte(`{"message":"mock reject"}`))
		} else {
			_, _ = w.Write([]byte(`{"id":"mock-id"}`))
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &body, &authHeader
}

func emailTestCfg() *SysConfig {
	return &SysConfig{vals: map[string]string{
		CfgEmailEnabled:  "true",
		CfgEmailAPIKey:   "re_test_key",
		CfgEmailFromAddr: "noreply@test.dev",
		CfgEmailFromName: "TestBrain",
	}}
}

func TestEmailServiceSend(t *testing.T) {
	ts, body, authHeader := mockResend(t, 200)
	svc := &EmailService{Cfg: emailTestCfg(), HTTP: ts.Client(), Endpoint: ts.URL}
	if !svc.Ready() {
		t.Fatal("配置齐备 Ready 应为 true")
	}
	if err := svc.SendCode(context.Background(), "u@test.dev", "483921", 10); err != nil {
		t.Fatalf("SendCode 失败: %v", err)
	}
	if *authHeader != "Bearer re_test_key" {
		t.Fatalf("Authorization 头异常: %q", *authHeader)
	}
	if (*body)["from"] != "TestBrain <noreply@test.dev>" {
		t.Fatalf("from 异常: %v", (*body)["from"])
	}
	html, _ := (*body)["html"].(string)
	if !strings.Contains(html, "483921") || !strings.Contains(html, "10 分钟") {
		t.Fatal("HTML 模板未含验证码/有效期变量替换结果")
	}
	text, _ := (*body)["text"].(string)
	if !strings.Contains(text, "483921") {
		t.Fatal("纯文本未含验证码")
	}
}

func TestEmailServiceNotReady(t *testing.T) {
	svc := &EmailService{Cfg: &SysConfig{vals: map[string]string{}}, HTTP: http.DefaultClient}
	if svc.Ready() {
		t.Fatal("未配置 Ready 应为 false")
	}
	if err := svc.Send(context.Background(), "u@test.dev", "s", "t", "<p></p>"); err == nil {
		t.Fatal("未配置发送应报错")
	}
}

func TestEmailServiceUpstreamReject(t *testing.T) {
	ts, _, _ := mockResend(t, 401)
	svc := &EmailService{Cfg: emailTestCfg(), HTTP: ts.Client(), Endpoint: ts.URL}
	err := svc.Send(context.Background(), "u@test.dev", "s", "t", "<p></p>")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("上游拒绝应带状态码: %v", err)
	}
}

// TestSendEmailCodeFlow 发码全流程（集成）：发码入缓存 → 冷却 → 用码注册 → 码消费。
func TestSendEmailCodeFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ts, _, _ := mockResend(t, 200)
	cs := &CacheService{DB: db}
	svc := &AuthService{
		DB: db, Cache: cs, JWT: auth.NewManager("test-secret"),
		Email: &EmailService{DB: db, Cfg: emailTestCfg(), HTTP: ts.Client(), Endpoint: ts.URL},
	}
	if err := svc.SendEmailCode(ctx, "New@Test.DEV"); err != nil {
		t.Fatalf("发码失败: %v", err)
	}
	saved, err := cs.Get(ctx, emailCodeKey("new@test.dev"))
	if err != nil || len(saved) != 6 {
		t.Fatalf("验证码未入缓存或非 6 位: %q %v", saved, err)
	}
	// 60 秒冷却
	if err := svc.SendEmailCode(ctx, "new@test.dev"); err == nil {
		t.Fatal("冷却期内重发应被拒")
	}
	// 错码注册失败、正确码注册成功
	if _, err := svc.Register(ctx, RegisterInput{Account: "new@test.dev", Password: "password123", EmailCode: "000000"}); err == nil {
		t.Fatal("错码不应注册成功")
	}
	if _, err := svc.Register(ctx, RegisterInput{Account: "new@test.dev", Password: "password123", EmailCode: saved}); err != nil {
		t.Fatalf("正确码注册失败: %v", err)
	}
	if _, err := cs.Get(ctx, emailCodeKey("new@test.dev")); err == nil {
		t.Fatal("注册后验证码应已被消费")
	}
	// 邮件服务未配置 → 明确报错（不 panic）
	svc2 := &AuthService{DB: db, Cache: cs, JWT: auth.NewManager("test-secret")}
	if err := svc2.SendEmailCode(ctx, "x@test.dev"); err == nil {
		t.Fatal("邮件服务未配置应报错")
	}
}
