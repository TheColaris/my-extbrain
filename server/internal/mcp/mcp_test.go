package mcp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"extbrain-server/internal/migrate"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 集成测试库：TEST_DATABASE_URL；未配置则跳过。
// 自动派生独立库（<原库名>_mcp）：go test 各包并行执行，internal/service 的集成测试
// 也用 TEST_DATABASE_URL——共用一个库会互相 TRUNCATE（真机并行跑抓出），故按包隔离。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	url := isolateTestDB(t, raw)
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("测试库不可达: %v", err)
	}
	if err := migrate.Up(url); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	db.Exec("TRUNCATE tu_user, tu_api_key, sys_cache, tf_todo, tf_memo, tf_note, tf_note_content, tf_note_chunk, tp_system_config, tl_api_log, tf_repo, tf_note_perm")
	return db
}

// isolateTestDB 把 URL 里的库名换成 <db>_mcp；不存在则自动创建。
func isolateTestDB(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" || u.Path == "/" {
		return raw // 解析不出库名就退回原库（单包跑也安全）
	}
	iso := strings.TrimPrefix(u.Path, "/") + "_mcp"
	admin := *u
	admin.Path = "/postgres"
	adminDB, err := gorm.Open(postgres.Open(admin.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("测试库不可达: %v", err)
	}
	var cnt int
	adminDB.Raw(`SELECT count(*) FROM pg_database WHERE datname = ?`, iso).Scan(&cnt)
	if cnt == 0 {
		if err := adminDB.Exec(fmt.Sprintf(`CREATE DATABASE %q`, iso)).Error; err != nil {
			t.Fatalf("创建隔离测试库 %s 失败: %v", iso, err)
		}
	}
	out := *u
	out.Path = "/" + iso
	return out.String()
}

// setup 建库 → 种用户 + 指定 scope 的 Key → 构建服务与身份。
func setup(t *testing.T, scope model.KeyScope) (*env, *gorm.DB, *sdk.ClientSession) {
	t.Helper()
	db := testDB(t)
	ctx := context.Background()

	phone := "13900000001"
	u := model.User{Phone: &phone, PasswordHash: "x", NickName: "测试用户"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("种用户失败: %v", err)
	}
	keys := &service.APIKeyService{DB: db}
	k, _, err := keys.Issue(ctx, u.ID, service.IssueInput{Name: "MCP 测试"})
	if err != nil {
		t.Fatalf("签发 Key 失败: %v", err)
	}
	if scope != model.KeyScopeAll {
		if _, err := keys.Update(ctx, u.ID, k.ID, service.UpdateInput{Scope: &scope}); err != nil {
			t.Fatalf("改 scope 失败: %v", err)
		}
		k.Scope = scope
	}

	d := Deps{
		Todos:   &service.TodoService{DB: db},
		Memos:   &service.MemoService{DB: db},
		Notes:   &service.NoteService{DB: db},
		Repos:   &service.RepoService{DB: db},
		Perm:    &service.NotePermService{DB: db},
		Account: &service.AccountService{DB: db},
		Audit:   service.NewAudit(db),
		Version: "test",
	}
	id := &Identity{UserID: u.ID, KeyID: k.ID, KeyName: k.KeyName, KeyHint: k.KeyHint, Scope: k.Scope, ClientIP: "127.0.0.1"}
	srv := buildServer(d, id)

	t1, t2 := sdk.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return &env{srv: srv, d: d, id: id}, db, cs
}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("tools/call %s 协议错误: %v", name, err)
	}
	return res
}

func textOf(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func TestToolSurfaceByScope(t *testing.T) {
	// scope=all → 16 个工具
	_, _, cs := setup(t, model.KeyScopeAll)
	names := toolNames(t, cs)
	if len(names) != 16 {
		t.Fatalf("scope=all 工具数 = %d, want 16: %v", len(names), names)
	}

	// scope=todo → todo×4 + memo×4 + whoami = 9；无 note 工具
	_, _, csTodo := setup(t, model.KeyScopeTodo)
	namesTodo := toolNames(t, csTodo)
	if len(namesTodo) != 9 {
		t.Fatalf("scope=todo 工具数 = %d, want 9: %v", len(namesTodo), namesTodo)
	}
	for _, n := range namesTodo {
		if strings.HasPrefix(n, "note_") || n == "search" {
			t.Fatalf("scope=todo 不应出现 %s", n)
		}
	}
	// 越权工具调用 → 协议错误（工具不存在）
	if _, err := csTodo.CallTool(context.Background(), &sdk.CallToolParams{Name: "note_read", Arguments: map[string]any{"path": "a.md"}}); err == nil {
		t.Fatal("todo scope 调 note_read 应报错（工具未注册）")
	}

	// scope=notes → search + note×6 + whoami = 8
	_, _, csNotes := setup(t, model.KeyScopeNotes)
	namesNotes := toolNames(t, csNotes)
	if len(namesNotes) != 8 {
		t.Fatalf("scope=notes 工具数 = %d, want 8: %v", len(namesNotes), namesNotes)
	}
}

func TestSchemaRequired(t *testing.T) {
	_, _, cs := setup(t, model.KeyScopeAll)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "todo_create" {
			continue
		}
		schema, _ := tool.InputSchema.(map[string]any)
		req, _ := schema["required"].([]any)
		if len(req) != 1 || req[0] != "title" {
			t.Fatalf("todo_create required = %v, want [title]", req)
		}
		return
	}
	t.Fatal("未找到 todo_create")
}

func TestTodoMemoFlow(t *testing.T) {
	e, db, cs := setup(t, model.KeyScopeAll)
	ctx := context.Background()

	// todo_create（带 due：纯日期 → 当天 23:59）
	res := call(t, cs, "todo_create", map[string]any{"title": "写周报", "due": "2026-10-09", "tags": []string{"工作"}})
	if res.IsError || !strings.Contains(textOf(t, res), "✓ 已记待办") {
		t.Fatalf("todo_create: isErr=%v text=%s", res.IsError, textOf(t, res))
	}
	var todo model.Todo
	if err := db.Where("user_id = ?", e.id.UserID).First(&todo).Error; err != nil {
		t.Fatalf("待办未落库: %v", err)
	}
	if todo.Title != "写周报" || todo.Source != model.TodoSourceCLI {
		t.Fatalf("待办字段异常: %+v", todo)
	}
	if todo.DueTime == nil || todo.DueTime.Local().Hour() != 23 {
		t.Fatalf("纯日期 due 应为当天 23:59 本地: %v", todo.DueTime)
	}

	// todo_list
	res = call(t, cs, "todo_list", map[string]any{})
	if !strings.Contains(textOf(t, res), "写周报") {
		t.Fatalf("todo_list 未含新待办: %s", textOf(t, res))
	}

	// todo_update → done
	res = call(t, cs, "todo_update", map[string]any{"id": todo.ID, "status": "done"})
	if res.IsError || !strings.Contains(textOf(t, res), "done") {
		t.Fatalf("todo_update: %s", textOf(t, res))
	}

	// 不存在 id → isError（业务错误，AI 可见）
	res = call(t, cs, "todo_update", map[string]any{"id": int64(999999), "status": "done"})
	if !res.IsError || !strings.Contains(textOf(t, res), "不存在") {
		t.Fatalf("不存在待办应 isError 且提示: isErr=%v text=%s", res.IsError, textOf(t, res))
	}

	// memo_create + memo_update(pin)
	res = call(t, cs, "memo_create", map[string]any{"content": "GORM 写 text[] 要用 pq.StringArray"})
	if res.IsError || !strings.Contains(textOf(t, res), "✓ 已记便签") {
		t.Fatalf("memo_create: %s", textOf(t, res))
	}
	var memo model.Memo
	if err := db.Where("user_id = ?", e.id.UserID).First(&memo).Error; err != nil {
		t.Fatalf("便签未落库: %v", err)
	}
	res = call(t, cs, "memo_update", map[string]any{"id": memo.ID, "pinned": true})
	if res.IsError || !strings.Contains(textOf(t, res), "已置顶") {
		t.Fatalf("memo_update pin: %s", textOf(t, res))
	}

	// whoami
	res = call(t, cs, "whoami", map[string]any{})
	if !strings.Contains(textOf(t, res), "已连接") || !strings.Contains(textOf(t, res), "MCP 测试") {
		t.Fatalf("whoami: %s", textOf(t, res))
	}

	// 审计：等异步批量落库（1s flush），断言 action 与 REST 同构
	time.Sleep(1300 * time.Millisecond)
	var actions []string
	db.WithContext(ctx).Model(&model.APILog{}).Where("api_key_id = ?", e.id.KeyID).Order("id").Pluck("action", &actions)
	want := []string{"todo.create", "todo.read", "todo.update", "todo.update", "memo.create", "memo.update", "auth.read"}
	if len(actions) != len(want) {
		t.Fatalf("审计条数 = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("审计 action[%d] = %s, want %s（全量 %v）", i, actions[i], want[i], actions)
		}
	}
}

func TestNoteFlow(t *testing.T) {
	e, db, cs := setup(t, model.KeyScopeAll)
	ctx := context.Background()

	// note_write 新建
	res := call(t, cs, "note_write", map[string]any{"path": "ai/测试笔记.md", "content": "# 主题\n\n正文内容"})
	if res.IsError || !strings.Contains(textOf(t, res), "✓ 已保存") {
		t.Fatalf("note_write: %s", textOf(t, res))
	}

	// note_read → 含 path/hash 头
	res = call(t, cs, "note_read", map[string]any{"path": "ai/测试笔记.md"})
	text := textOf(t, res)
	if res.IsError || !strings.Contains(text, "hash: ") || !strings.Contains(text, "正文内容") {
		t.Fatalf("note_read: %s", text)
	}
	var hash1 string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "hash: ") {
			hash1 = strings.TrimPrefix(line, "hash: ")
		}
	}

	// note_list
	res = call(t, cs, "note_list", map[string]any{"prefix": "ai"})
	if !strings.Contains(textOf(t, res), "ai/测试笔记.md") {
		t.Fatalf("note_list: %s", textOf(t, res))
	}

	// 覆盖写（不带 hash）→ 新 hash
	res = call(t, cs, "note_write", map[string]any{"path": "ai/测试笔记.md", "content": "# 主题\n\n正文 v2"})
	if res.IsError {
		t.Fatalf("note_write v2: %s", textOf(t, res))
	}
	// 带过期 hash 写 → 拒绝（isError + 指引）
	res = call(t, cs, "note_write", map[string]any{"path": "ai/测试笔记.md", "content": "# 主题\n\n正文 v3", "expected_hash": hash1})
	if !res.IsError || !strings.Contains(textOf(t, res), "已被更新") {
		t.Fatalf("过期 hash 应拒绝: isErr=%v text=%s", res.IsError, textOf(t, res))
	}

	// search（三域；note 命中）
	res = call(t, cs, "search", map[string]any{"q": "正文"})
	if !strings.Contains(textOf(t, res), "ai/测试笔记.md") {
		t.Fatalf("search 未命中: %s", textOf(t, res))
	}

	// note_move
	res = call(t, cs, "note_move", map[string]any{"from": "ai/测试笔记.md", "to": "ai/改名笔记.md"})
	if res.IsError || !strings.Contains(textOf(t, res), "✓ 已移动") {
		t.Fatalf("note_move: %s", textOf(t, res))
	}

	// note_pull
	res = call(t, cs, "note_pull", map[string]any{"prefix": "ai"})
	if !strings.Contains(textOf(t, res), "改名笔记.md") || !strings.Contains(textOf(t, res), "正文 v2") {
		t.Fatalf("note_pull: %s", textOf(t, res))
	}

	// note_delete + 删除后读 → isError
	res = call(t, cs, "note_delete", map[string]any{"path": "ai/改名笔记.md"})
	if res.IsError {
		t.Fatalf("note_delete: %s", textOf(t, res))
	}
	res = call(t, cs, "note_read", map[string]any{"path": "ai/改名笔记.md"})
	if !res.IsError || !strings.Contains(textOf(t, res), "不存在") {
		t.Fatalf("删除后读取应 isError: %s", textOf(t, res))
	}

	// 越权：todo scope 的 Key 看不到 note 工具已在上方覆盖；此处校验审计 action 同构
	time.Sleep(1300 * time.Millisecond)
	var actions []string
	db.WithContext(ctx).Model(&model.APILog{}).Where("api_key_id = ?", e.id.KeyID).Order("id").Pluck("action", &actions)
	want := []string{"note.upsert", "note.read", "note.read", "note.upsert", "note.upsert", "search.query", "note.move", "note.read", "note.delete", "note.read"}
	if len(actions) != len(want) {
		t.Fatalf("审计 action = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("审计 action[%d] = %s, want %s（全量 %v）", i, actions[i], want[i], actions)
		}
	}
}

func TestParseDue(t *testing.T) {
	loc := time.Local
	got, err := parseDue("2026-10-09")
	if err != nil || got.Hour() != 23 || got.Minute() != 59 || got.Location() != loc {
		t.Fatalf("纯日期: %v %v", got, err)
	}
	got, err = parseDue("2026-10-09 18:00")
	if err != nil || got.Hour() != 18 || got.Location() != loc {
		t.Fatalf("日期时间: %v %v", got, err)
	}
	got, err = parseDue("2026-10-09T18:00:00+08:00")
	if err != nil || got.UTC().Hour() != 10 {
		t.Fatalf("RFC3339: %v %v", got, err)
	}
	if _, err := parseDue("明天下午"); err == nil {
		t.Fatal("非法格式应报错")
	}
}

// TestMCPNotePermDenied —— 目录权限：白名单外的 Key 读受限笔记 → isError 可执行文案；
// tools/list 不变（权限藏数据不藏工具）。
func TestMCPNotePermDenied(t *testing.T) {
	e, db, cs := setup(t, model.KeyScopeAll)
	ctx := context.Background()

	// 第二把 Key（同用户，不进白名单）
	keys := &service.APIKeyService{DB: db}
	k2, _, err := keys.Issue(ctx, e.id.UserID, service.IssueInput{Name: "K2"})
	if err != nil {
		t.Fatal(err)
	}

	// K1 写一篇笔记，并把根白名单设为仅 K1
	res := call(t, cs, "note_write", map[string]any{"path": "私有/机密.md", "content": "机密内容"})
	if res.IsError {
		t.Fatalf("note_write: %s", textOf(t, res))
	}
	rid, err := e.d.Repos.DefaultID(ctx, e.id.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.d.Perm.Put(ctx, e.id.UserID, service.NotePermInput{
		RepoID: rid, FolderPath: "", Mode: service.PermModeAllow, KeyIDs: []int64{e.id.KeyID},
	}); err != nil {
		t.Fatal(err)
	}

	// 第二身份构建 server（同 Deps）
	id2 := &Identity{UserID: e.id.UserID, KeyID: k2.ID, KeyName: k2.KeyName, KeyHint: k2.KeyHint, Scope: k2.Scope, ClientIP: "127.0.0.1"}
	srv2 := buildServer(e.d, id2)
	t1, t2 := sdk.NewInMemoryTransports()
	if _, err := srv2.Connect(ctx, t1, nil); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "k2", Version: "0"}, nil)
	cs2, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs2.Close()

	// tools/list 工具数不变（藏数据不藏工具）
	if names := toolNames(t, cs2); len(names) != 16 {
		t.Fatalf("K2 工具数 = %d, want 16", len(names))
	}

	// 读受限 → isError + 可执行文案
	res = call(t, cs2, "note_read", map[string]any{"path": "私有/机密.md"})
	if !res.IsError || !strings.Contains(textOf(t, res), "不可见") {
		t.Fatalf("K2 读受限应 isError+不可见: isErr=%v text=%s", res.IsError, textOf(t, res))
	}
	// 写同样拒绝
	res = call(t, cs2, "note_write", map[string]any{"path": "私有/越权.md", "content": "x"})
	if !res.IsError || !strings.Contains(textOf(t, res), "不可见") {
		t.Fatalf("K2 写受限应 isError+不可见: %s", textOf(t, res))
	}
	// 搜索不泄露
	res = call(t, cs2, "search", map[string]any{"q": "机密"})
	if strings.Contains(textOf(t, res), "机密.md") {
		t.Fatalf("K2 搜索不应命中受限笔记: %s", textOf(t, res))
	}
	// K1 自己不受影响
	res = call(t, cs, "note_read", map[string]any{"path": "私有/机密.md"})
	if res.IsError || !strings.Contains(textOf(t, res), "机密内容") {
		t.Fatalf("K1 自读应正常: %s", textOf(t, res))
	}
}
