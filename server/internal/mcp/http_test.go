package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

// headerRT 给测试客户端请求带上 Bearer（模拟 api 层鉴权前的请求形态）。
type headerRT struct{ token string }

func (h *headerRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+h.token)
	return http.DefaultTransport.RoundTrip(r)
}

// testAuth 测试用鉴权中间件：与 api 层同语义（校验 Key → 注入 Identity）。
func testAuth(next http.Handler, keys *service.APIKeyService, db *gorm.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cred := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		k, err := keys.VerifyByRawKey(r.Context(), cred)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"API Key 无效"}}`))
			return
		}
		id := &Identity{UserID: k.UserID, KeyID: k.ID, KeyName: k.KeyName, KeyHint: k.KeyHint, Scope: k.Scope, ClientIP: "127.0.0.1"}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// TestHTTPHandlerEndToEnd —— /mcp 的 Streamable HTTP 全链路（无状态 + JSONResponse + 按 Key 隔离）。
func TestHTTPHandlerEndToEnd(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	phone := "13900000002"
	u := model.User{Phone: &phone, PasswordHash: "x", NickName: "HTTP 测试"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("种用户失败: %v", err)
	}
	keys := &service.APIKeyService{DB: db}
	_, rawAll, err := keys.Issue(ctx, u.ID, service.IssueInput{Name: "all"})
	if err != nil {
		t.Fatal(err)
	}
	scopeTodo := model.KeyScopeTodo
	kTodo, rawTodo, err := keys.Issue(ctx, u.ID, service.IssueInput{Name: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Update(ctx, u.ID, kTodo.ID, service.UpdateInput{Scope: &scopeTodo}); err != nil {
		t.Fatal(err)
	}

	d := Deps{
		Todos:   &service.TodoService{DB: db},
		Memos:   &service.MemoService{DB: db},
		Notes:   &service.NoteService{DB: db},
		Account: &service.AccountService{DB: db},
		Audit:   service.NewAudit(db),
		Version: "test",
	}
	ts := httptest.NewServer(testAuth(Handler(d), keys, db))
	defer ts.Close()

	connect := func(token string) *sdk.ClientSession {
		t.Helper()
		client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil)
		cs, err := client.Connect(ctx, &sdk.StreamableClientTransport{
			Endpoint:             ts.URL,
			HTTPClient:           &http.Client{Transport: &headerRT{token: token}},
			DisableStandaloneSSE: true,
		}, nil)
		if err != nil {
			t.Fatalf("connect(%s): %v", token, err)
		}
		t.Cleanup(func() { _ = cs.Close() })
		return cs
	}

	// 有效 Key：全量工具 + 调用
	csAll := connect(rawAll)
	res, err := csAll.ListTools(ctx, nil)
	if err != nil || len(res.Tools) != 16 {
		t.Fatalf("all key tools = %d (err=%v), want 16", len(res.Tools), err)
	}
	callRes, err := csAll.CallTool(ctx, &sdk.CallToolParams{Name: "todo_create", Arguments: map[string]any{"title": "HTTP 建的待办"}})
	if err != nil || callRes.IsError {
		t.Fatalf("HTTP todo_create: err=%v res=%+v", err, callRes)
	}
	var n int64
	db.Model(&model.Todo{}).Where("user_id = ? AND title = ?", u.ID, "HTTP 建的待办").Count(&n)
	if n != 1 {
		t.Fatalf("待办未落库 n=%d", n)
	}

	// 同端点、不同 Key → 工具集按 scope 隔离（证明 getServer 每请求按身份构建）
	csTodo := connect(rawTodo)
	resTodo, err := csTodo.ListTools(ctx, nil)
	if err != nil || len(resTodo.Tools) != 9 {
		t.Fatalf("todo key tools = %d (err=%v), want 9", len(resTodo.Tools), err)
	}

	// 无效 Key → 401（连接失败）
	bad := sdk.NewClient(&sdk.Implementation{Name: "bad", Version: "0"}, nil)
	if _, err := bad.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             ts.URL,
		HTTPClient:           &http.Client{Transport: &headerRT{token: "ak_live_wrong"}},
		DisableStandaloneSSE: true,
	}, nil); err == nil {
		t.Fatal("无效 Key 应连接失败（401）")
	}
}
