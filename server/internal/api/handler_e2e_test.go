package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"extbrain-server/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 信箱 handler 测试库：TEST_DATABASE_URL；未配置则跳过。
func mailboxDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("测试库不可达: %v", err)
	}
	if err := db.AutoMigrate(&model.E2EMailbox{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	db.Exec("TRUNCATE tl_e2e_mailbox")
	t.Cleanup(func() { db.Exec("TRUNCATE tl_e2e_mailbox") })
	return db
}

func mailboxReq(t *testing.T, h *E2EHandler, method, path string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	switch method {
	case http.MethodGet:
		r.GET("/api/v1/e2e/mailbox", h.Mailbox)
	case http.MethodDelete:
		r.DELETE("/api/v1/e2e/mailbox", h.MailboxClear)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestE2EMailboxHandler(t *testing.T) {
	db := mailboxDB(t)
	h := &E2EHandler{DB: db}
	rows := []model.E2EMailbox{
		{Channel: "webhook", Target: "https://ding.test/hook", Title: "钉钉", Body: "b1", Payload: "{}"},
		{Channel: "webhook", Target: "https://feishu.test/hook", Title: "飞书", Body: "b2", Payload: "{}"},
		{Channel: "webpush", Target: "https://fcm.test/sub", Title: "推送", Body: "b3", Payload: "{}"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("种数据失败: %v", err)
	}

	// 全量：3 条，倒序（最新在前）
	code, body := mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox")
	if code != http.StatusOK || body["count"].(float64) != 3 {
		t.Fatalf("全量应 200/3 条: code=%d body=%v", code, body["count"])
	}
	entries := body["entries"].([]any)
	if first := entries[0].(map[string]any); first["channel"] != "webpush" {
		t.Fatalf("应按 id 倒序（最新 webpush 在前）: %v", first["channel"])
	}

	// channel 过滤
	_, body = mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox?channel=webhook")
	if body["count"].(float64) != 2 {
		t.Fatalf("webhook 过滤应 2 条: %v", body["count"])
	}
	// target 过滤
	_, body = mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox?target=https%3A%2F%2Ffeishu.test%2Fhook")
	if body["count"].(float64) != 1 {
		t.Fatalf("target 过滤应 1 条: %v", body["count"])
	}

	// 清一箱（webpush）→ 仅剩 webhook 2 条
	code, body = mailboxReq(t, h, http.MethodDelete, "/api/v1/e2e/mailbox?channel=webpush")
	if code != http.StatusOK || body["deleted"].(float64) != 1 {
		t.Fatalf("清 webpush 箱应删 1 行: code=%d body=%v", code, body)
	}
	_, body = mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox?channel=webpush")
	if body["count"].(float64) != 0 {
		t.Fatalf("清箱后该通道应为 0: %v", body["count"])
	}

	// 清全部 → 0
	code, body = mailboxReq(t, h, http.MethodDelete, "/api/v1/e2e/mailbox")
	if code != http.StatusOK || body["deleted"].(float64) != 2 {
		t.Fatalf("清全部应删 2 行: code=%d body=%v", code, body)
	}
	_, body = mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox")
	if body["count"].(float64) != 0 {
		t.Fatalf("全清后应为 0: %v", body["count"])
	}
}

func TestE2EMailboxHandlerDBError(t *testing.T) {
	db := mailboxDB(t)
	h := &E2EHandler{DB: db}
	db.Exec("DROP TABLE tl_e2e_mailbox") // 查询失败 → 统一 500 信封，不 panic
	code, body := mailboxReq(t, h, http.MethodGet, "/api/v1/e2e/mailbox")
	if code != http.StatusInternalServerError || body["error"] == nil {
		t.Fatalf("查询失败应 500 信封: code=%d body=%v", code, body)
	}
	code, body = mailboxReq(t, h, http.MethodDelete, "/api/v1/e2e/mailbox")
	if code != http.StatusInternalServerError || body["error"] == nil {
		t.Fatalf("清箱失败应 500 信封: code=%d body=%v", code, body)
	}
	if err := db.AutoMigrate(&model.E2EMailbox{}); err != nil { // 共享集成库：恢复表
		t.Fatalf("恢复信箱表失败: %v", err)
	}
}
