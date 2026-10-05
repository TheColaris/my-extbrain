package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---------- 纯函数 ----------

func TestReorderFlags(t *testing.T) {
	cases := []struct {
		in, want []string
	}{
		// 自然写法：flag 混在位置参数后 → flag 全部前置（含带值 flag 吞其值）
		{[]string{"todo", "add", "标题", "--due", "2026-01-01", "--json"}, []string{"--due", "2026-01-01", "--json", "todo", "add", "标题"}},
		// 带等号的 flag 不吞下一个参数
		{[]string{"a", "--tag=工作", "b"}, []string{"--tag=工作", "a", "b"}},
		// 布尔 flag 不吞下一个参数
		{[]string{"note", "pull", "ai/", "--stdout"}, []string{"--stdout", "note", "pull", "ai/"}},
		// 单个 - 与 -- 是位置参数（stdin/分隔符语义）
		{[]string{"note", "push", "-", "--path", "x.md"}, []string{"--path", "x.md", "note", "push", "-"}},
		// 无 flag
		{[]string{"me"}, []string{"me"}},
	}
	for _, c := range cases {
		if got := reorderFlags(c.in); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("reorderFlags(%v)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestKeyHint(t *testing.T) {
	c := &config{APIKey: "ak_live_1234567890123456789"}
	if got := c.keyHint(); got != "ak_live_12345…6789" {
		t.Fatalf("长 key hint=%q", got)
	}
	short := &config{APIKey: "ak_live_short"}
	if got := short.keyHint(); got != "ak_live_short" {
		t.Fatalf("短 key hint 应原样: %q", got)
	}
}

func TestMark(t *testing.T) {
	if mark("done") != "✔" || mark("active") != " " {
		t.Fatalf("mark 流转标记不符")
	}
}

func TestParseDue(t *testing.T) {
	cases := map[string]string{
		"2026-01-02":           "2026-01-02", // 纯日期：保持日期形态（本地 0 点）
		"2026-01-02 18:30":     "18:30",      // 日期+时间 → RFC3339 含时刻
		"2026-01-02T18:30":     "18:30",
		"2026-01-02T18:30:00Z": "18:30",
	}
	for in, want := range cases {
		if got := parseDue(in); !strings.Contains(got, want) {
			t.Fatalf("parseDue(%q)=%q 应含 %q", in, got, want)
		}
	}
}

func TestRepoQAndEscPath(t *testing.T) {
	if repoQ("") != "" {
		t.Fatalf("空仓库不应拼参数")
	}
	if repoQ("工作库") != "?repo=%E5%B7%A5%E4%BD%9C%E5%BA%93" {
		t.Fatalf("repoQ 编码不符: %q", repoQ("工作库"))
	}
	if got := escPath("ai/中文 笔记.md"); got != "ai/%E4%B8%AD%E6%96%87%20%E7%AC%94%E8%AE%B0.md" {
		t.Fatalf("escPath=%q", got)
	}
	if got := escPath("/a/b.md"); got != "a/b.md" {
		t.Fatalf("escPath 应去首尾斜杠: %q", got)
	}
}

// ---------- 配置 ----------

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, ".config", "extbrain", "config.json")
	if configPath() != cfgPath {
		t.Fatalf("configPath=%q want %q", configPath(), cfgPath)
	}
	// 未配置
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "未配置") {
		t.Fatalf("无配置应报未配置: %v", err)
	}
	// 写配置 → 读回
	os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	os.WriteFile(cfgPath, []byte(`{"server":"http://s.test","api_key":"ak_live_x"}`), 0o600)
	c, err := loadConfig()
	if err != nil || c.Server != "http://s.test" || c.APIKey != "ak_live_x" {
		t.Fatalf("loadConfig 不符: %+v err=%v", c, err)
	}
	// 配置损坏
	os.WriteFile(cfgPath, []byte("{broken"), 0o600)
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "配置损坏") {
		t.Fatalf("坏 JSON 应报配置损坏: %v", err)
	}
}

// ---------- searchCmd（三域解析与输出；覆盖 CLI 与服务端 v2 搜索契约）----------

func TestSearchCmdThreeDomains(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"todos": []map[string]any{{"id": 7, "title": "周报", "snippet": "…周五前…"}},
			"memos": []map[string]any{{"id": 3, "snippet": "…pq.StringArray…"}},
			"notes": []map[string]any{{"repo_name": "工作库", "path": "需求/a.md", "title": "a.md", "snippet": "…仓库化…"}},
		})
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, ".config", "extbrain", "config.json")
	os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	os.WriteFile(cfgPath, []byte(`{"server":"`+srv.URL+`","api_key":"ak_live_x"}`), 0o600)

	out := captureStdout(t, func() { searchCmd([]string{"关键词"}) })
	for _, want := range []string{"● 待办 #7 周报", "● 便签 #3", "● [工作库] 需求/a.md", "(3 条)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("search 输出缺 %q：\n%s", want, out)
		}
	}
	if !strings.Contains(gotPath, "/search?q=") {
		t.Fatalf("应请求 /search: %s", gotPath)
	}

	// --json：输出结构化三域
	out = captureStdout(t, func() { searchCmd([]string{"--json", "关键词"}) })
	if !strings.Contains(out, `"notes"`) || !strings.Contains(out, "工作库") {
		t.Fatalf("--json 应输出三域结构：\n%s", out)
	}
}

func TestUsage(t *testing.T) {
	out := captureStdout(t, usage)
	for _, want := range []string{"auth login", "todo add", "note push", "search"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage 缺 %q", want)
		}
	}
}

// captureStdout 捕获函数期间的 stdout（usage/searchCmd 等直接 fmt.Print 的输出）。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

// ---------- 命令函数（httptest 假服务端 + HOME 隔离 + stdout 捕获）----------

type cmdEnv struct {
	t       *testing.T
	reqs    *[]http.HandlerFunc // 按序记录：测试内用闭包断言请求
	lastReq *string
	lastM   *string
}

// newCmdEnv 起假 API 服务并写好 CLI 配置；返回服务 URL 供 login --server 用。
func newCmdEnv(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, ".config", "extbrain", "config.json")
	os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	os.WriteFile(cfgPath, []byte(`{"server":"`+srv.URL+`","api_key":"ak_live_test"}`), 0o600)
	return srv.URL
}

func TestAPIEnvelopeAndErrors(t *testing.T) {
	// 成功 + 出参解析
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ak_live_test" {
			t.Errorf("应带 Bearer Key: %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"id":9,"title":"x"}`))
	})
	var out struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	if err := api("POST", "/todos", map[string]string{"title": "x"}, &out); err != nil || out.ID != 9 {
		t.Fatalf("api 成功解析不符: %+v err=%v", out, err)
	}
	// 400 带错误信封 → 可执行文案（code）
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"bad","message":"标题必填"}}`))
	})
	err := api("POST", "/todos", map[string]string{}, nil)
	if err == nil || !strings.Contains(err.Error(), "标题必填") || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("400 信封应转可执行错误: %v", err)
	}
	// 500 无信封 → HTTP 码
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if err := api("GET", "/x", nil, nil); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("无信封 500 应回 HTTP 码: %v", err)
	}
	// out=nil（纯写操作）不解析响应体
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"ignored":true}`)) })
	if err := api("DELETE", "/todos/1", nil, nil); err != nil {
		t.Fatalf("out=nil 应成功: %v", err)
	}
}

func TestAnonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("应为 POST: %s", r.Method)
		}
		w.Write([]byte(`{"code":"AB12","auth_url":"http://x/cli-auth?code=AB12"}`))
	}))
	defer srv.Close()
	var r struct {
		Code    string `json:"code"`
		AuthURL string `json:"auth_url"`
	}
	if err := anonJSON(http.MethodPost, srv.URL, nil, &r); err != nil || r.Code != "AB12" {
		t.Fatalf("anonJSON 不符: %+v err=%v", r, err)
	}
	// 4xx
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`not found`))
	}))
	defer srv2.Close()
	if err := anonJSON(http.MethodGet, srv2.URL, nil, nil); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("4xx 应回 HTTP 码: %v", err)
	}
}

func TestTodoCmd(t *testing.T) {
	var lastPath, lastMethod, lastBody string
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		lastPath, lastMethod = r.URL.RequestURI(), r.Method
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		lastBody = string(b)
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/todos") && r.Method == http.MethodPost:
			w.Write([]byte(`{"id":5,"title":"CLI 记的待办","status":"active"}`))
		case r.Method == http.MethodGet:
			w.Write([]byte(`{"todos":[{"id":5,"title":"甲","status":"active"},{"id":6,"title":"乙","status":"done","due_time":"2026-01-02T18:30:00+08:00"}]}`))
		default:
			w.Write([]byte(`{}`))
		}
	})

	// add：body 含 title/tags/due_time；输出 ✓ #5
	out := captureStdout(t, func() {
		todoCmd([]string{"add", "CLI 记的待办", "--due", "2026-01-02", "--tag", "工作", "--tag", "旅程"})
	})
	if !strings.Contains(out, "✓ #5 CLI 记的待办") {
		t.Fatalf("todo add 输出不符:\n%s", out)
	}
	for _, want := range []string{`"title":"CLI 记的待办"`, `"tags":["工作","旅程"]`, `"due_time":"2026-01-02`} {
		if !strings.Contains(lastBody, want) {
			t.Fatalf("add body 缺 %s: %s", want, lastBody)
		}
	}
	// add --json
	out = captureStdout(t, func() { todoCmd([]string{"add", "--json", "j"}) })
	if !strings.Contains(out, `"id": 5`) {
		t.Fatalf("add --json 应结构化输出:\n%s", out)
	}
	// list：done 标 ✔、due 显示前 16 字符、计数
	out = captureStdout(t, func() { todoCmd([]string{"list"}) })
	if !strings.Contains(out, "5 [ ] 甲") || !strings.Contains(out, "6 [✔] 乙 · 2026-01-02T18:30") || !strings.Contains(out, "(2 条)") {
		t.Fatalf("todo list 输出不符:\n%s", out)
	}
	// list --status：拼 query
	captureStdout(t, func() { todoCmd([]string{"list", "--status", "done"}) })
	if !strings.Contains(lastPath, "status=done") {
		t.Fatalf("list --status 应拼 query: %s", lastPath)
	}
	// list --json
	out = captureStdout(t, func() { todoCmd([]string{"list", "--json"}) })
	if !strings.Contains(out, `"title": "甲"`) {
		t.Fatalf("list --json 应结构化:\n%s", out)
	}
	// done/undo/rm
	out = captureStdout(t, func() { todoCmd([]string{"done", "5"}) })
	if !strings.Contains(out, "✓ #5 → 已完成") || lastMethod != http.MethodPatch || !strings.Contains(lastBody, `"status":"done"`) {
		t.Fatalf("todo done 不符: %s %s %s", out, lastMethod, lastBody)
	}
	out = captureStdout(t, func() { todoCmd([]string{"undo", "5"}) })
	if !strings.Contains(out, "✓ #5 → 在途") {
		t.Fatalf("todo undo 输出不符:\n%s", out)
	}
	out = captureStdout(t, func() { todoCmd([]string{"rm", "5"}) })
	if !strings.Contains(out, "✓ 已删除 #5") || lastMethod != http.MethodDelete {
		t.Fatalf("todo rm 不符: %s %s", out, lastMethod)
	}
	// 空参数 / 未知子命令（不走网络不退出）
	out = captureStdout(t, func() { todoCmd(nil) })
	if !strings.Contains(out, "用法:") {
		t.Fatalf("todo 空参数应显示用法:\n%s", out)
	}
	out = captureStdout(t, func() { todoCmd([]string{"wat"}) })
	if !strings.Contains(out, "未知子命令: wat") {
		t.Fatalf("未知子命令应提示:\n%s", out)
	}
}

func TestMemoCmd(t *testing.T) {
	var lastMethod, lastBody string
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.Method
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		lastBody = string(b)
		switch {
		case r.Method == http.MethodPost:
			w.Write([]byte(`{"id":3,"content":"x"}`))
		case r.Method == http.MethodGet:
			w.Write([]byte(`{"memos":[{"id":1,"content":"置顶我","is_pinned":1},{"id":2,"content":"普通"}]}`))
		default:
			w.Write([]byte(`{}`))
		}
	})
	out := captureStdout(t, func() { memoCmd([]string{"add", "一句话", "--tag", "灵感"}) })
	if !strings.Contains(out, "✓ #3 已记") || !strings.Contains(lastBody, `"content":"一句话"`) || !strings.Contains(lastBody, `"tags":["灵感"]`) {
		t.Fatalf("memo add 不符: %s %s", out, lastBody)
	}
	out = captureStdout(t, func() { memoCmd([]string{"list"}) })
	if !strings.Contains(out, "1 📌 置顶我") || !strings.Contains(out, "2 普通") {
		t.Fatalf("memo list 置顶标记不符:\n%s", out)
	}
	out = captureStdout(t, func() { memoCmd([]string{"pin", "1"}) })
	if !strings.Contains(out, "✓ #1 已置顶") || !strings.Contains(lastBody, `"is_pinned":1`) {
		t.Fatalf("memo pin 不符: %s %s", out, lastBody)
	}
	out = captureStdout(t, func() { memoCmd([]string{"rm", "1"}) })
	if !strings.Contains(out, "✓ 已删除 #1") || lastMethod != http.MethodDelete {
		t.Fatalf("memo rm 不符: %s %s", out, lastMethod)
	}
	out = captureStdout(t, func() { memoCmd(nil) })
	if !strings.Contains(out, "用法:") {
		t.Fatalf("memo 空参数应显示用法:\n%s", out)
	}
}

func TestNoteCmd(t *testing.T) {
	var lastPath, lastMethod, lastBody string
	noteFile := ""
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		lastPath, lastMethod = r.URL.RequestURI(), r.Method
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		lastBody = string(b)
		switch {
		case r.Method == http.MethodPut:
			w.Write([]byte(`{"note":{"path":"cli/from-cli.md","title":"from-cli.md"}}`))
		case r.URL.Path == "/api/v1/notes" && r.Method == http.MethodGet:
			w.Write([]byte(`{"notes":[{"path":"cli/a.md","title":"a.md"}],"dirs":["cli"]}`))
		case r.Method == http.MethodGet:
			w.Write([]byte(`{"path":"cli/from-cli.md","title":"from-cli.md","content":"# CLI 笔记\n\nCLI push 的内容。"}`))
		default:
			w.Write([]byte(`{}`))
		}
	})
	home, _ := os.UserHomeDir()
	noteFile = filepath.Join(home, "n.md")
	os.WriteFile(noteFile, []byte("# CLI 笔记\n\nCLI push 的内容。"), 0o644)

	// push 显式 path+repo
	out := captureStdout(t, func() { noteCmd([]string{"push", noteFile, "--path", "cli/from-cli.md", "--repo", "旅程仓壹"}) })
	if !strings.Contains(out, "✓ cli/from-cli.md（") || !strings.Contains(lastPath, "repo=%E6%97%85%E7%A8%8B%E4%BB%93%E5%A3%B9") {
		t.Fatalf("note push 不符: %s %s", out, lastPath)
	}
	if !strings.Contains(lastBody, "# CLI 笔记") {
		t.Fatalf("push body 应为文件内容: %s", lastBody)
	}
	// push 缺省路径=文件名
	captureStdout(t, func() { noteCmd([]string{"push", noteFile}) })
	if !strings.Contains(lastPath, "/notes/n.md") {
		t.Fatalf("缺省应取文件名: %s", lastPath)
	}
	// ls：目录 + 计数
	out = captureStdout(t, func() { noteCmd([]string{"ls"}) })
	if !strings.Contains(out, "📁 cli/") || !strings.Contains(out, "cli/a.md") || !strings.Contains(out, "(1 篇 1 目录)") {
		t.Fatalf("note ls 输出不符:\n%s", out)
	}
	// cat：输出正文
	out = captureStdout(t, func() { noteCmd([]string{"cat", "cli/from-cli.md"}) })
	if !strings.Contains(out, "CLI push 的内容") {
		t.Fatalf("note cat 应输出正文:\n%s", out)
	}
	// pull --stdout：bundle 含路径注释与正文
	out = captureStdout(t, func() { noteCmd([]string{"pull", "cli", "--stdout"}) })
	if !strings.Contains(out, "<!-- cli/a.md -->") || !strings.Contains(out, "CLI push 的内容") {
		t.Fatalf("note pull --stdout bundle 不符:\n%s", out)
	}
	// rm
	out = captureStdout(t, func() { noteCmd([]string{"rm", "cli/from-cli.md"}) })
	if !strings.Contains(out, "✓ 已删除") || lastMethod != http.MethodDelete {
		t.Fatalf("note rm 不符: %s %s", out, lastMethod)
	}
	// 空参数
	out = captureStdout(t, func() { noteCmd(nil) })
	if !strings.Contains(out, "用法:") {
		t.Fatalf("note 空参数应显示用法:\n%s", out)
	}
}

func TestAuthCmdAndMe(t *testing.T) {
	// me：探活成功 → 配置有效 + 打码 key
	srvURL := newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"todos":[]}`))
	})
	out := captureStdout(t, func() { meCmd(nil) })
	if !strings.Contains(out, "✓ 配置有效") || !strings.Contains(out, "key:   ak_live_test") {
		t.Fatalf("me 输出不符:\n%s", out)
	}
	// auth show
	out = captureStdout(t, func() { authCmd([]string{"show"}) })
	if !strings.Contains(out, "server:") || !strings.Contains(out, "key:") {
		t.Fatalf("auth show 输出不符:\n%s", out)
	}
	// auth login --key：探活成功 → ✓ 配置完成
	out = captureStdout(t, func() { authCmd([]string{"login", "--server", srvURL, "--key", "ak_live_abc"}) })
	if !strings.Contains(out, "✓ 配置完成") {
		t.Fatalf("auth login --key 输出不符:\n%s", out)
	}
	// auth login --key 探活失败 → 警告但仍写配置
	newCmdEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"code":"unauthorized","message":"no"}}`))
	})
	out = captureStdout(t, func() { authCmd([]string{"login", "--server", "http://127.0.0.1:1", "--key", "ak_live_abc"}) })
	if !strings.Contains(out, "已写入配置，但验证失败") {
		t.Fatalf("探活失败应警告:\n%s", out)
	}
	// auth logout → 清配置
	out = captureStdout(t, func() { authCmd([]string{"logout"}) })
	if !strings.Contains(out, "✓ 已清除配置") {
		t.Fatalf("auth logout 输出不符:\n%s", out)
	}
}
