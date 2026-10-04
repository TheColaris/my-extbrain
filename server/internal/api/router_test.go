package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"extbrain-server/internal/config"

	"github.com/gin-gonic/gin"
)

func TestHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := Router(Deps{Cfg: config.Load(), Audit: nil}) // db/audit 为 nil（单元测试不起库；healthz 容忍）
	if r == nil {
		t.Fatal("router nil")
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz = %d, want 200", w.Code)
	}
	if !jsonContains(w.Body.String(), `"status":"ok"`) {
		t.Fatalf("healthz 响应异常: %s", w.Body.String())
	}
}

func jsonContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestInstallShInjectsBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := Router(Deps{Cfg: config.Load()})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/install.sh", nil)
	req.Host = "example.com"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("install.sh = %d", w.Code)
	}
	body := w.Body.String()
	if !jsonContains(body, "EXTBRAIN_BASE:-http://example.com") {
		t.Fatalf("BASE 未注入当前 host: %s", body[:120])
	}
}
