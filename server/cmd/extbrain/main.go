// extbrain —— my-extbrain CLI（纯标准库，零第三方依赖）。
//
// 配置：~/.config/extbrain/config.json（server + api_key），auth login 写入。
// 输出：默认纯文本（AI 可直接引用）；--json 供程序化消费。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var version = "0.3.2-dev"

const configHelp = "先执行: extbrain auth login --server https://<实例>（浏览器点授权即可，无需密钥）"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "--version":
		fmt.Println("extbrain " + version)
	case "auth":
		authCmd(args)
	case "todo":
		todoCmd(args)
	case "memo":
		memoCmd(args)
	case "note":
		noteCmd(args)
	case "search":
		searchCmd(args)
	case "me":
		meCmd(args)
	default:
		fmt.Fprintln(os.Stderr, "未知命令:", cmd)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`extbrain —— 我的外脑 CLI

用法: extbrain <命令> [参数]

  auth login --server URL                     配置（浏览器点授权，写入 ~/.config/extbrain/config.json；备选 --key ak_live_...）
  auth logout / auth show
  todo add "标题" [--due 2026-10-09] [--tag 工作]     记待办
  todo list [--status active|done] [--json]
  todo done <id> / todo undo <id> / todo rm <id>      完成 / 恢复在途 / 删除
  memo add "内容" [--tag 想法]                        记便签（≤2000 字）
  memo list [--json] / memo pin <id> / memo rm <id>
  note push <本地.md> [--path ai/名字.md] [--repo 仓库名]  存知识（缺省取文件名；缺省仓库=默认仓库）
  note ls [prefix] / note cat <path> / note rm <path>     （均可 --repo 指定仓库）
  note pull [prefix] [--stdout] [--repo 仓库名]           拉整个目录拼成 MD bundle（给 AI 塞上下文）
  search <关键词> [--json]                            全文检索
  me                                                  验证配置与身份
`)
}

/* ============ flag 骨架 ============ */

type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(s string) error { *l = append(*l, s); return nil }

type flags struct {
	*flag.FlagSet
	asJSON bool
}

func newFlags(name string) *flags {
	f := &flags{FlagSet: flag.NewFlagSet(name, flag.ExitOnError)}
	f.BoolVar(&f.asJSON, "json", false, "JSON 输出")
	return f
}

func (f *flags) out(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "✗", err)
	os.Exit(1)
}

// reorderFlags 把 flag 参数挪到位置参数前（Go flag 遇位置参数即停，
// 允许 `todo add "标题" --due x` 这类自然写法）
func reorderFlags(args []string) []string {
	boolFlags := map[string]bool{"--json": true, "--stdout": true}
	var flagsPart, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" && a != "--" {
			flagsPart = append(flagsPart, a)
			if !boolFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flagsPart = append(flagsPart, args[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flagsPart, pos...)
}

func parseID(f *flags, usage string) int64 {
	if f.NArg() == 0 {
		die(fmt.Errorf("%s", usage))
	}
	var id int64
	fmt.Sscanf(f.Arg(0), "%d", &id)
	if id <= 0 {
		die(fmt.Errorf("id 必须是正整数"))
	}
	return id
}

/* ============ 配置与 HTTP ============ */

type config struct {
	Server string `json:"server"`
	APIKey string `json:"api_key"`
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "extbrain", "config.json")
}

func loadConfig() (*config, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("未配置。%s", configHelp)
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("配置损坏: %v", err)
	}
	return &c, nil
}

func (c *config) keyHint() string {
	if len(c.APIKey) > 17 {
		return c.APIKey[:13] + "…" + c.APIKey[len(c.APIKey)-4:]
	}
	return c.APIKey
}

func api(method, path string, body any, out any) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(cfg.Server, "/")+"/api/v1"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("网络错误: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var e struct {
			Error struct {
				Code, Message string
			}
		}
		_ = json.Unmarshal(b, &e)
		if e.Error.Message != "" {
			return fmt.Errorf("%s（%s）", e.Error.Message, e.Error.Code)
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

/* ============ 匿名请求（设备授权 start/poll）============ */

func anonJSON(method, url string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("网络错误: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}

/* ============ auth ============ */

func authCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: extbrain auth login|logout|show")
		return
	}
	switch args[0] {
	case "login":
		f := newFlags("login")
		server := f.String("server", "", "服务器地址")
		key := f.String("key", "", "API Key")
		f.Parse(args[1:])
		if *server == "" {
			die(fmt.Errorf("--server 必填"))
		}
		if *key == "" {
			deviceLogin(*server) // 浏览器授权流程
			return
		}
		if !strings.HasPrefix(*key, "ak_live_") {
			die(fmt.Errorf("key 应以 ak_live_ 开头"))
		}
		if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
			die(err)
		}
		b, _ := json.MarshalIndent(config{Server: *server, APIKey: *key}, "", "  ")
		if err := os.WriteFile(configPath(), b, 0o600); err != nil {
			die(err)
		}
		var probe struct{}
		if err := api("GET", "/todos?limit=1", nil, &probe); err != nil {
			fmt.Println("已写入配置，但验证失败:", err)
			return
		}
		fmt.Println("✓ 配置完成", configPath())
	case "logout":
		_ = os.Remove(configPath())
		fmt.Println("✓ 已清除配置")
	case "show":
		c, err := loadConfig()
		if err != nil {
			die(err)
		}
		fmt.Println("server:", c.Server)
		fmt.Println("key:  ", c.keyHint())
	}
}

// deviceLogin 浏览器设备授权（gh auth login 同款）：
// start 取码 → 开浏览器 → 轮询 → 一次性取走密钥并写入配置
func deviceLogin(server string) {
	base := strings.TrimRight(server, "/")
	var st struct {
		Code    string `json:"code"`
		AuthURL string `json:"auth_url"`
	}
	if err := anonJSON("POST", base+"/api/v1/cli-auth/start", nil, &st); err != nil {
		die(fmt.Errorf("发起授权失败: %v", err))
	}
	fmt.Println("请在浏览器完成授权（已自动打开）：")
	fmt.Println("  " + st.AuthURL)
	fmt.Println("  授权码: " + st.Code)
	openBrowser(st.AuthURL)

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		var p struct {
			Status string `json:"status"`
			Key    string `json:"key"`
		}
		if err := anonJSON("GET", base+"/api/v1/cli-auth/poll?code="+st.Code, nil, &p); err != nil {
			continue // 网络抖动继续轮询
		}
		switch p.Status {
		case "approved":
			if err := os.MkdirAll(filepath.Dir(configPath()), 0o755); err != nil {
				die(err)
			}
			b, _ := json.MarshalIndent(config{Server: server, APIKey: p.Key}, "", "  ")
			if err := os.WriteFile(configPath(), b, 0o600); err != nil {
				die(err)
			}
			var probe struct{}
			if err := api("GET", "/todos?limit=1", nil, &probe); err != nil {
				fmt.Println("已写入配置，但验证失败:", err)
				return
			}
			fmt.Println("✓ 授权成功，配置已写入", configPath())
			return
		case "expired":
			die(fmt.Errorf("授权码已过期，请重新执行 extbrain auth login"))
		}
	}
	die(fmt.Errorf("等待授权超时（5 分钟），请重新执行"))
}

/* ============ todo ============ */

type todo struct {
	ID     int64    `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Due    *string  `json:"due_time"`
	Tags   []string `json:"tags"`
	Source string   `json:"source"`
}

func todoCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: extbrain todo add|list|done|undo|rm")
		return
	}
	sub, rest := args[0], args[1:]
	f := newFlags("todo " + sub)
	switch sub {
	case "add":
		var due string
		var tags stringList
		f.StringVar(&due, "due", "", "截止时间")
		f.Var(&tags, "tag", "标签（可多次）")
		f.Parse(reorderFlags(rest))
		if f.NArg() == 0 {
			die(fmt.Errorf("标题必填"))
		}
		body := map[string]any{"title": strings.Join(f.Args(), " ")}
		if len(tags) > 0 {
			body["tags"] = []string(tags)
		}
		if due != "" {
			body["due_time"] = parseDue(due)
		}
		var t todo
		if err := api("POST", "/todos", body, &t); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(t)
		} else {
			fmt.Printf("✓ #%d %s\n", t.ID, t.Title)
		}
	case "list":
		var status string
		f.StringVar(&status, "status", "", "状态筛选 active|done")
		f.Parse(reorderFlags(rest))
		q := ""
		if status != "" {
			q = "?status=" + status
		}
		var r struct {
			Todos []todo `json:"todos"`
		}
		if err := api("GET", "/todos"+q, nil, &r); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(r.Todos)
			return
		}
		for _, t := range r.Todos {
			d := ""
			if t.Due != nil && len(*t.Due) >= 16 {
				d = " · " + (*t.Due)[:16]
			}
			fmt.Printf("%4d [%s] %s%s\n", t.ID, mark(t.Status), t.Title, d)
		}
		fmt.Printf("(%d 条)\n", len(r.Todos))
	case "done", "undo", "rm":
		f.Parse(reorderFlags(rest))
		id := parseID(f, fmt.Sprintf("用法: extbrain todo %s <id>", sub))
		switch sub {
		case "done":
			if err := api("PATCH", fmt.Sprintf("/todos/%d", id), map[string]string{"status": "done"}, nil); err != nil {
				die(err)
			}
			fmt.Printf("✓ #%d → 已完成\n", id)
		case "undo":
			if err := api("PATCH", fmt.Sprintf("/todos/%d", id), map[string]string{"status": "active"}, nil); err != nil {
				die(err)
			}
			fmt.Printf("✓ #%d → 在途\n", id)
		default:
			if err := api("DELETE", fmt.Sprintf("/todos/%d", id), nil, nil); err != nil {
				die(err)
			}
			fmt.Printf("✓ 已删除 #%d\n", id)
		}
	default:
		fmt.Println("未知子命令:", sub)
	}
}

func mark(status string) string {
	if status == "done" {
		return "✔"
	}
	return " "
}

func parseDue(s string) string {
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02T15:00:00Z07:00")
	}
	die(fmt.Errorf("时间格式不识别: %s（支持 2006-01-02 / 2006-01-02 15:04 / RFC3339）", s))
	return ""
}

/* ============ memo ============ */

type memo struct {
	ID       int64  `json:"id"`
	Content  string `json:"content"`
	IsPinned int16  `json:"is_pinned"`
}

func memoCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: extbrain memo add|list|pin|rm")
		return
	}
	sub, rest := args[0], args[1:]
	f := newFlags("memo " + sub)
	switch sub {
	case "add":
		var tags stringList
		f.Var(&tags, "tag", "标签（可多次）")
		f.Parse(reorderFlags(rest))
		if f.NArg() == 0 {
			die(fmt.Errorf("内容必填"))
		}
		body := map[string]any{"content": strings.Join(f.Args(), " ")}
		if len(tags) > 0 {
			body["tags"] = []string(tags)
		}
		var m memo
		if err := api("POST", "/memos", body, &m); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(m)
		} else {
			fmt.Printf("✓ #%d 已记\n", m.ID)
		}
	case "list":
		f.Parse(reorderFlags(rest))
		var r struct {
			Memos []memo `json:"memos"`
		}
		if err := api("GET", "/memos", nil, &r); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(r.Memos)
			return
		}
		for _, m := range r.Memos {
			pin := ""
			if m.IsPinned == 1 {
				pin = "📌 "
			}
			fmt.Printf("%4d %s%s\n", m.ID, pin, m.Content)
		}
	case "pin", "rm":
		f.Parse(reorderFlags(rest))
		id := parseID(f, fmt.Sprintf("用法: extbrain memo %s <id>", sub))
		if sub == "pin" {
			if err := api("PATCH", fmt.Sprintf("/memos/%d", id), map[string]int{"is_pinned": 1}, nil); err != nil {
				die(err)
			}
			fmt.Printf("✓ #%d 已置顶\n", id)
		} else {
			if err := api("DELETE", fmt.Sprintf("/memos/%d", id), nil, nil); err != nil {
				die(err)
			}
			fmt.Printf("✓ 已删除 #%d\n", id)
		}
	default:
		fmt.Println("未知子命令:", sub)
	}
}

/* ============ note ============ */

type noteMeta struct {
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Tags   []string `json:"tags"`
	Update string   `json:"update_time"`
}

type noteFull struct {
	noteMeta
	Content string `json:"content"`
}

func noteCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: extbrain note push|ls|cat|pull|rm")
		return
	}
	sub, rest := args[0], args[1:]
	f := newFlags("note " + sub)
	switch sub {
	case "push":
		var path, repo string
		f.StringVar(&path, "path", "", "远端路径（缺省取文件名）")
		f.StringVar(&repo, "repo", "", "仓库名（缺省=默认仓库）")
		f.Parse(reorderFlags(rest))
		if f.NArg() == 0 {
			die(fmt.Errorf("本地文件必填"))
		}
		b, err := os.ReadFile(f.Arg(0))
		if err != nil {
			die(err)
		}
		if path == "" {
			path = filepath.Base(f.Arg(0))
		}
		var r struct {
			Note noteMeta `json:"note"`
		}
		if err := api("PUT", "/notes/"+escPath(path)+repoQ(repo), map[string]any{"content": string(b)}, &r); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(r.Note)
		} else {
			fmt.Printf("✓ %s（%d 字节）\n", r.Note.Path, len(b))
		}
	case "ls":
		var prefix, repo string
		f.StringVar(&prefix, "prefix", "", "目录前缀")
		f.StringVar(&repo, "repo", "", "仓库名（缺省=默认仓库）")
		f.Parse(reorderFlags(rest))
		if prefix == "" && f.NArg() > 0 {
			prefix = f.Arg(0)
		}
		q := repoQ(repo)
		if prefix != "" {
			sep := "&"
			if q == "" {
				sep = "?"
			}
			q += sep + "prefix=" + url.QueryEscape(prefix)
		}
		var r struct {
			Notes []noteMeta `json:"notes"`
			Dirs  []string   `json:"dirs"`
		}
		if err := api("GET", "/notes"+q, nil, &r); err != nil {
			die(err)
		}
		if f.asJSON {
			f.out(r)
			return
		}
		for _, d := range r.Dirs {
			fmt.Println("📁", d+"/")
		}
		for _, n := range r.Notes {
			fmt.Println("  ", n.Path)
		}
		fmt.Printf("(%d 篇 %d 目录)\n", len(r.Notes), len(r.Dirs))
	case "cat":
		var repo string
		f.StringVar(&repo, "repo", "", "仓库名（缺省=默认仓库）")
		f.Parse(reorderFlags(rest))
		if f.NArg() == 0 {
			die(fmt.Errorf("路径必填"))
		}
		var n noteFull
		if err := api("GET", "/notes/"+escPath(f.Arg(0))+repoQ(repo), nil, &n); err != nil {
			die(err)
		}
		fmt.Print(n.Content)
	case "pull":
		var toStdout bool
		var prefix, repo string
		f.StringVar(&prefix, "prefix", "", "目录前缀")
		f.StringVar(&repo, "repo", "", "仓库名（缺省=默认仓库）")
		f.BoolVar(&toStdout, "stdout", false, "输出到 stdout（MD bundle）")
		f.Parse(reorderFlags(rest))
		if prefix == "" && f.NArg() > 0 {
			prefix = f.Arg(0)
		}
		q := repoQ(repo)
		if prefix != "" {
			sep := "&"
			if q == "" {
				sep = "?"
			}
			q += sep + "prefix=" + url.QueryEscape(prefix)
		}
		var r struct {
			Notes []noteMeta `json:"notes"`
		}
		if err := api("GET", "/notes"+q, nil, &r); err != nil {
			die(err)
		}
		var sb strings.Builder
		for _, m := range r.Notes {
			var n noteFull
			if err := api("GET", "/notes/"+escPath(m.Path)+q, nil, &n); err != nil {
				continue
			}
			fmt.Fprintf(&sb, "\n\n---\n\n<!-- %s -->\n\n", m.Path)
			sb.WriteString(n.Content)
		}
		if toStdout {
			fmt.Print(sb.String())
			return
		}
		if err := os.WriteFile("extbrain-pull.md", []byte(sb.String()), 0o644); err != nil {
			die(err)
		}
		fmt.Printf("✓ %d 篇 → extbrain-pull.md\n", len(r.Notes))
	case "rm":
		var repo string
		f.StringVar(&repo, "repo", "", "仓库名（缺省=默认仓库）")
		f.Parse(reorderFlags(rest))
		if f.NArg() == 0 {
			die(fmt.Errorf("路径必填"))
		}
		if err := api("DELETE", "/notes/"+escPath(f.Arg(0))+repoQ(repo), nil, nil); err != nil {
			die(err)
		}
		fmt.Println("✓ 已删除")
	default:
		fmt.Println("未知子命令:", sub)
	}
}

// repoQ 仓库名 query 串（空=默认仓库=服务端语义，不拼参数）
func repoQ(repo string) string {
	if repo == "" {
		return ""
	}
	return "?repo=" + url.QueryEscape(repo)
}

func escPath(p string) string {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

/* ============ search / me ============ */

func searchCmd(args []string) {
	f := newFlags("search")
	f.Parse(reorderFlags(args))
	if f.NArg() == 0 {
		die(fmt.Errorf("关键词必填"))
	}
	// 服务端 /search 缺省=三域全搜（todos/memos/notes；笔记命中带仓库注记）
	var r struct {
		Todos []struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"todos"`
		Memos []struct {
			ID      int64  `json:"id"`
			Snippet string `json:"snippet"`
		} `json:"memos"`
		Notes []struct {
			RepoName string `json:"repo_name"`
			Path     string `json:"path"`
			Title    string `json:"title"`
			Snippet  string `json:"snippet"`
		} `json:"notes"`
	}
	if err := api("GET", "/search?q="+url.QueryEscape(strings.Join(f.Args(), " ")), nil, &r); err != nil {
		die(err)
	}
	if f.asJSON {
		f.out(r)
		return
	}
	for _, t := range r.Todos {
		fmt.Printf("● 待办 #%d %s\n  %s\n", t.ID, t.Title, t.Snippet)
	}
	for _, m := range r.Memos {
		fmt.Printf("● 便签 #%d\n  %s\n", m.ID, m.Snippet)
	}
	for _, h := range r.Notes {
		loc := h.Path
		if h.RepoName != "" {
			loc = "[" + h.RepoName + "] " + h.Path
		}
		fmt.Printf("● %s\n  %s\n", loc, h.Snippet)
	}
	fmt.Printf("(%d 条)\n", len(r.Todos)+len(r.Memos)+len(r.Notes))
}

func meCmd(args []string) {
	f := newFlags("me")
	f.Parse(reorderFlags(args))
	var probe struct{}
	if err := api("GET", "/todos?limit=1", nil, &probe); err != nil {
		die(err)
	}
	c, _ := loadConfig()
	fmt.Println("✓ 配置有效")
	fmt.Println("server:", c.Server)
	fmt.Println("key:  ", c.keyHint())
}
