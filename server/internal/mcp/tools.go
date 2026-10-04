package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTools 注册工具集（按 Key scope 过滤）：
//   - scope=todo（含便签）：todo_* / memo_*
//   - scope=notes（知识库）：search / note_*
//   - whoami 恒可用
func registerTools(e *env) {
	if e.id.Scope.Allows(model.KeyScopeTodo) {
		registerTodoTools(e)
		registerMemoTools(e)
	}
	if e.id.Scope.Allows(model.KeyScopeNotes) {
		registerSearchTool(e)
		registerNoteTools(e)
	}
	registerWhoami(e)
}

/* ============ 检索 ============ */

type searchIn struct {
	Q     string `json:"q" jsonschema:"检索关键词"`
	Scope string `json:"scope,omitempty" jsonschema:"检索范围：all（默认，全部）/ todo（待办）/ memo（便签）/ note（知识库）"`
	Limit int    `json:"limit,omitempty" jsonschema:"每域返回条数上限（默认 10，最大 50）"`
}

func registerSearchTool(e *env) {
	addTool(e, &sdk.Tool{
		Name:        "search",
		Description: "全文检索外脑（待办 / 便签 / 知识库）。回答用户问题前先搜这里；存新知识前也先用它查重。返回命中摘要与路径。",
		Annotations: ro(),
	}, string(model.ActionOf("search", "query")), func(in searchIn) string { return in.Q },
		func(ctx context.Context, in searchIn) (string, error) {
			q := strings.TrimSpace(in.Q)
			if q == "" {
				return "", &service.UserError{Msg: "q（关键词）必填"}
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 10
			}
			if limit > 50 {
				limit = 50
			}
			scope := in.Scope
			if scope == "" {
				scope = "all"
			}
			// notes 专属 Key 不得借道 search 的 all 缺省越权读待办/便签（与 REST /search 同口径）
			if e.id.Scope != model.KeyScopeAll && scope != "note" {
				scope = "note"
			}

			var sb strings.Builder
			total := 0
			section := func(title string, n int, render func()) {
				if n == 0 {
					return
				}
				total += n
				fmt.Fprintf(&sb, "\n【%s】%d 条\n", title, n)
				render()
			}

			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			switch scope {
			case "todo":
				hits, err := e.d.Todos.Search(ctx, e.id.UserID, q, limit)
				if err != nil {
					return "", err
				}
				section("待办", len(hits), func() { renderTodos(&sb, hits) })
			case "memo":
				hits, err := e.d.Memos.Search(ctx, e.id.UserID, q, limit)
				if err != nil {
					return "", err
				}
				section("便签", len(hits), func() { renderMemos(&sb, hits) })
			case "note":
				hits, err := e.d.Notes.Search(ctx, e.id.UserID, q, limit, "", view)
				if err != nil {
					return "", err
				}
				section("知识库", len(hits), func() { renderNotes(&sb, hits) })
			case "all":
				todos, err := e.d.Todos.Search(ctx, e.id.UserID, q, limit)
				if err != nil {
					return "", err
				}
				memos, err := e.d.Memos.Search(ctx, e.id.UserID, q, limit)
				if err != nil {
					return "", err
				}
				notes, err := e.d.Notes.Search(ctx, e.id.UserID, q, limit, "", view)
				if err != nil {
					return "", err
				}
				section("知识库", len(notes), func() { renderNotes(&sb, notes) })
				section("待办", len(todos), func() { renderTodos(&sb, todos) })
				section("便签", len(memos), func() { renderMemos(&sb, memos) })
			default:
				return "", &service.UserError{Msg: "scope 必须是 all / todo / memo / note"}
			}
			if total == 0 {
				return fmt.Sprintf("未找到与「%s」相关的内容（待办 / 便签 / 知识库均为 0 条）。", q), nil
			}
			return fmt.Sprintf("搜索「%s」共命中 %d 条：\n%s", q, total, sb.String()), nil
		})
}

func renderNotes(sb *strings.Builder, hits []service.SearchHit) {
	for _, h := range hits {
		loc := h.Path
		if h.RepoName != "" { // 跨仓库检索：标注来源仓库（读写指定仓库用 repo 字段，不是拼路径）
			loc = "[" + h.RepoName + "] " + h.Path
		}
		fmt.Fprintf(sb, "- %s（%s，%s 更新）\n", loc, humanBytes(h.SizeBytes), h.UpdatedAt.Format("2006-01-02"))
		if s := strings.TrimSpace(h.Snippet); s != "" {
			fmt.Fprintf(sb, "  %s\n", s)
		}
	}
}

func renderTodos(sb *strings.Builder, hits []service.TodoSearchHit) {
	for _, h := range hits {
		due := ""
		if h.DueTime != nil {
			due = "，截止 " + h.DueTime.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(sb, "- #%d [%s] %s%s\n", h.ID, h.Status, h.Title, due)
		if s := strings.TrimSpace(h.Snippet); s != "" {
			fmt.Fprintf(sb, "  %s\n", s)
		}
	}
}

func renderMemos(sb *strings.Builder, hits []service.MemoSearchHit) {
	for _, h := range hits {
		fmt.Fprintf(sb, "- #%d（%s）\n", h.ID, h.CreateTime.Local().Format("2006-01-02"))
		if s := strings.TrimSpace(h.Snippet); s != "" {
			fmt.Fprintf(sb, "  %s\n", s)
		}
	}
}

/* ============ 待办 ============ */

func registerTodoTools(e *env) {
	type createIn struct {
		Title  string   `json:"title" jsonschema:"待办标题"`
		Due    string   `json:"due,omitempty" jsonschema:"截止时间（可选，北京时间）：2026-10-09（当天 23:59 前）或 2026-10-09 18:00"`
		Tags   []string `json:"tags,omitempty" jsonschema:"标签（可选）"`
		Remark string   `json:"remark,omitempty" jsonschema:"备注（可选，支持 Markdown）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "todo_create",
		Description: "记一条待办。用户说「记一下 / 别忘了 / 提醒我」时用；说了明确时间就带 due。",
		Annotations: rw(),
	}, string(model.ActionOf("todo", "create")), func(in createIn) string { return in.Title },
		func(ctx context.Context, in createIn) (string, error) {
			title := strings.TrimSpace(in.Title)
			if title == "" {
				return "", &service.UserError{Msg: "title（标题）必填"}
			}
			in2 := service.TodoCreate{Title: title, Remark: in.Remark, Tags: in.Tags, Source: model.TodoSourceCLI}
			if strings.TrimSpace(in.Due) != "" {
				t, err := parseDue(in.Due)
				if err != nil {
					return "", err
				}
				in2.DueTime = &t
			}
			t, err := e.d.Todos.Create(ctx, e.id.UserID, in2)
			if err != nil {
				return "", err
			}
			msg := fmt.Sprintf("✓ 已记待办 #%d：%s", t.ID, t.Title)
			if t.DueTime != nil {
				msg += "（截止 " + t.DueTime.Local().Format("2006-01-02 15:04") + "）"
			}
			return msg, nil
		})

	type listIn struct {
		Status string `json:"status,omitempty" jsonschema:"筛选：active（默认，在途）/ done（已完成）"`
		Limit  int    `json:"limit,omitempty" jsonschema:"返回条数上限（默认 20，最大 100）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "todo_list",
		Description: "列待办（默认在途；可查已完成）。",
		Annotations: ro(),
	}, string(model.ActionOf("todo", "read")), nil,
		func(ctx context.Context, in listIn) (string, error) {
			f := service.TodoFilter{Status: model.TodoStatus(in.Status), Limit: in.Limit}
			if f.Status != "" && !f.Status.Valid() {
				return "", &service.UserError{Msg: "status 必须是 active（在途）/ done（已完成）"}
			}
			todos, counts, err := e.d.Todos.List(ctx, e.id.UserID, f)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "待办：在途 %d / 已完成 %d（当前显示 %d 条）\n", counts.Active, counts.Done, len(todos))
			for _, t := range todos {
				due := ""
				if t.DueTime != nil {
					due = " · 截止 " + t.DueTime.Local().Format("2006-01-02 15:04")
				}
				tags := ""
				if len(t.Tags) > 0 {
					tags = " #" + strings.Join(t.Tags, " #")
				}
				fmt.Fprintf(&sb, "%4d [%s] %s%s%s\n", t.ID, t.Status, t.Title, due, tags)
			}
			return sb.String(), nil
		})

	type updateIn struct {
		ID       int64    `json:"id" jsonschema:"待办 id"`
		Status   string   `json:"status,omitempty" jsonschema:"流转：active（恢复在途）/ done（完成）"`
		Title    string   `json:"title,omitempty" jsonschema:"新标题（可选）"`
		Remark   string   `json:"remark,omitempty" jsonschema:"新备注（可选）"`
		Due      string   `json:"due,omitempty" jsonschema:"新截止时间（可选，北京时间）"`
		ClearDue bool     `json:"clear_due,omitempty" jsonschema:"清除截止时间（与 due 二选一）"`
		Tags     []string `json:"tags,omitempty" jsonschema:"替换标签（可选）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "todo_update",
		Description: "更新待办：改标题 / 备注 / 标签 / 截止时间，或流转状态（完成 = status 设 done，恢复 = active）。",
		Annotations: rw(),
	}, string(model.ActionOf("todo", "update")), func(in updateIn) string { return fmt.Sprintf("%d", in.ID) },
		func(ctx context.Context, in updateIn) (string, error) {
			upd := service.TodoUpdate{Tags: in.Tags}
			if in.Status != "" {
				st := model.TodoStatus(in.Status)
				if !st.Valid() {
					return "", &service.UserError{Msg: "status 必须是 active（在途）/ done（已完成）"}
				}
				upd.Status = &st
			}
			if in.Title != "" {
				upd.Title = &in.Title
			}
			if in.Remark != "" {
				upd.Remark = &in.Remark
			}
			if in.ClearDue {
				upd.ClearDue = true
			} else if strings.TrimSpace(in.Due) != "" {
				t, err := parseDue(in.Due)
				if err != nil {
					return "", err
				}
				upd.DueTime = &t
			}
			t, err := e.d.Todos.Update(ctx, e.id.UserID, in.ID, upd)
			if err != nil {
				return "", todoErr(err, in.ID)
			}
			msg := fmt.Sprintf("✓ 已更新 #%d：%s [%s]", t.ID, t.Title, t.Status)
			if t.DueTime != nil {
				msg += "（截止 " + t.DueTime.Local().Format("2006-01-02 15:04") + "）"
			}
			return msg, nil
		})

	type deleteIn struct {
		ID int64 `json:"id" jsonschema:"待办 id"`
	}
	addTool(e, &sdk.Tool{
		Name:        "todo_delete",
		Description: "删除待办（进回收站，30 天内可恢复）。删除前先向用户确认。",
		Annotations: del(),
	}, string(model.ActionOf("todo", "delete")), func(in deleteIn) string { return fmt.Sprintf("%d", in.ID) },
		func(ctx context.Context, in deleteIn) (string, error) {
			if err := e.d.Todos.Delete(ctx, e.id.UserID, in.ID); err != nil {
				return "", todoErr(err, in.ID)
			}
			return fmt.Sprintf("✓ 已删除待办 #%d（回收站保留 30 天，可在网页面板恢复）", in.ID), nil
		})
}

func todoErr(err error, id int64) error {
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return fmt.Errorf("待办 #%d 不存在（用 todo_list 查看）", id)
	}
	return err
}

/* ============ 便签 ============ */

func registerMemoTools(e *env) {
	type createIn struct {
		Content string   `json:"content" jsonschema:"便签内容（≤2000 字）"`
		Tags    []string `json:"tags,omitempty" jsonschema:"标签（可选）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "memo_create",
		Description: "记一条便签：一句话速记、结论、灵感（≤2000 字）。更长的成篇内容用 note_write。",
		Annotations: rw(),
	}, string(model.ActionOf("memo", "create")), nil,
		func(ctx context.Context, in createIn) (string, error) {
			c := strings.TrimSpace(in.Content)
			if c == "" {
				return "", &service.UserError{Msg: "content（内容）必填"}
			}
			m, err := e.d.Memos.Create(ctx, e.id.UserID, service.MemoCreate{Content: c, Tags: in.Tags})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("✓ 已记便签 #%d", m.ID), nil
		})

	type listIn struct {
		Limit int `json:"limit,omitempty" jsonschema:"返回条数上限（默认 20，最大 100）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "memo_list",
		Description: "列最近便签（置顶在前）。",
		Annotations: ro(),
	}, string(model.ActionOf("memo", "read")), nil,
		func(ctx context.Context, in listIn) (string, error) {
			limit := in.Limit
			if limit <= 0 {
				limit = 20
			}
			memos, err := e.d.Memos.List(ctx, e.id.UserID, int64(limit), 0)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "最近便签 %d 条：\n", len(memos))
			for _, m := range memos {
				pin := ""
				if m.IsPinned == 1 {
					pin = "📌 "
				}
				content := strings.ReplaceAll(m.Content, "\n", " ")
				if len([]rune(content)) > 80 {
					content = string([]rune(content)[:80]) + "…"
				}
				fmt.Fprintf(&sb, "%4d %s%s（%s）\n", m.ID, pin, content, m.CreateTime.Local().Format("2006-01-02 15:04"))
			}
			return sb.String(), nil
		})

	type updateIn struct {
		ID      int64    `json:"id" jsonschema:"便签 id"`
		Content string   `json:"content,omitempty" jsonschema:"新内容（可选）"`
		Pinned  *bool    `json:"pinned,omitempty" jsonschema:"置顶开关（可选）"`
		Tags    []string `json:"tags,omitempty" jsonschema:"替换标签（可选）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "memo_update",
		Description: "更新便签：改内容 / 标签 / 置顶。",
		Annotations: rw(),
	}, string(model.ActionOf("memo", "update")), func(in updateIn) string { return fmt.Sprintf("%d", in.ID) },
		func(ctx context.Context, in updateIn) (string, error) {
			upd := service.MemoUpdate{Tags: in.Tags}
			if in.Content != "" {
				upd.Content = &in.Content
			}
			if in.Pinned != nil {
				var v int16
				if *in.Pinned {
					v = 1
				}
				upd.IsPinned = &v
			}
			m, err := e.d.Memos.Update(ctx, e.id.UserID, in.ID, upd)
			if err != nil {
				if isNotFound(err) {
					return "", fmt.Errorf("便签 #%d 不存在（用 memo_list 查看）", in.ID)
				}
				return "", err
			}
			pin := ""
			if m.IsPinned == 1 {
				pin = "（已置顶）"
			}
			return fmt.Sprintf("✓ 已更新便签 #%d%s", m.ID, pin), nil
		})

	type deleteIn struct {
		ID int64 `json:"id" jsonschema:"便签 id"`
	}
	addTool(e, &sdk.Tool{
		Name:        "memo_delete",
		Description: "删除便签（进回收站）。删除前先向用户确认。",
		Annotations: del(),
	}, string(model.ActionOf("memo", "delete")), func(in deleteIn) string { return fmt.Sprintf("%d", in.ID) },
		func(ctx context.Context, in deleteIn) (string, error) {
			if err := e.d.Memos.Delete(ctx, e.id.UserID, in.ID); err != nil {
				if isNotFound(err) {
					return "", fmt.Errorf("便签 #%d 不存在（用 memo_list 查看）", in.ID)
				}
				return "", err
			}
			return fmt.Sprintf("✓ 已删除便签 #%d（回收站保留 30 天）", in.ID), nil
		})
}

/* ============ 知识库 ============ */

func registerNoteTools(e *env) {
	type listIn struct {
		Prefix string `json:"prefix,omitempty" jsonschema:"目录前缀（可选），如 ai 或 ai/；缺省看仓库结构"`
		Repo   string `json:"repo,omitempty" jsonschema:"仓库名（可选；缺省为默认仓库）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_list",
		Description: "浏览知识库：给前缀列出子目录与笔记；不给前缀看仓库结构。不带 repo 时先列全部仓库清单（知识库分仓库：仓库 > 文件夹 > 笔记）。写新笔记前先看既有目录规划路径。",
		Annotations: ro(),
	}, string(model.ActionOf("note", "read")), func(in listIn) string { return in.Repo + "|" + in.Prefix },
		func(ctx context.Context, in listIn) (string, error) {
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			var sb strings.Builder
			if strings.TrimSpace(in.Repo) == "" && strings.TrimSpace(in.Prefix) == "" && e.d.Repos != nil {
				if repos, err := e.d.Repos.List(ctx, e.id.UserID); err == nil && len(repos) > 1 {
					sb.WriteString("【仓库清单】（note_* 工具用 repo 字段指定；缺省为默认仓库）\n")
					for _, r := range repos {
						def := ""
						if r.IsDefault == 1 {
							def = "（默认）"
						}
						fmt.Fprintf(&sb, "· %s%s：%d 篇\n", r.Name, def, r.NoteCount)
					}
					sb.WriteString("\n")
				}
			}
			repo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			notes, dirs, err := e.d.Notes.List(ctx, e.id.UserID, repo.ID, in.Prefix, view)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&sb, "【仓库：%s】\n", repo.Name)
			for _, d := range dirs {
				fmt.Fprintf(&sb, "📁 %s/\n", d)
			}
			for _, n := range notes {
				fmt.Fprintf(&sb, "   %s\n", n.Path)
			}
			fmt.Fprintf(&sb, "（%d 篇笔记，%d 个目录）", len(notes), len(dirs))
			return sb.String(), nil
		})

	type readIn struct {
		Path string `json:"path" jsonschema:"笔记路径（仓库内相对路径），如 ai/xxx.md"`
		Repo string `json:"repo,omitempty" jsonschema:"仓库名（可选；缺省为默认仓库）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_read",
		Description: "读一篇笔记全文。返回首部含 path / hash / 更新时间（hash 可用于 note_write 的 expected_hash 防覆盖）。",
		Annotations: ro(),
	}, string(model.ActionOf("note", "read")), func(in readIn) string { return in.Path },
		func(ctx context.Context, in readIn) (string, error) {
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			repo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			nf, err := e.d.Notes.Get(ctx, e.id.UserID, repo.ID, in.Path, view)
			if err != nil {
				return "", noteErr(err, in.Path)
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "repo: %s\npath: %s\nhash: %s\nupdated: %s\n", repo.Name, nf.Path, nf.ContentHash, nf.UpdateTime.Local().Format("2006-01-02 15:04"))
			if len(nf.Tags) > 0 {
				fmt.Fprintf(&sb, "tags: %s\n", strings.Join(nf.Tags, ", "))
			}
			sb.WriteString("\n")
			sb.WriteString(nf.Content)
			return sb.String(), nil
		})

	type pullIn struct {
		Prefix   string `json:"prefix,omitempty" jsonschema:"目录前缀（可选），如 ai；缺省为整个仓库"`
		Repo     string `json:"repo,omitempty" jsonschema:"仓库名（可选；缺省为默认仓库）"`
		MaxBytes int    `json:"max_bytes,omitempty" jsonschema:"输出字节上限（默认 100000，防爆上下文）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_pull",
		Description: "把某目录下全部笔记拼成一个 Markdown bundle（资料多时一次拿全，省多次 note_read）。",
		Annotations: ro(),
	}, string(model.ActionOf("note", "read")), func(in pullIn) string { return in.Prefix },
		func(ctx context.Context, in pullIn) (string, error) {
			maxBytes := in.MaxBytes
			if maxBytes <= 0 {
				maxBytes = 100_000
			}
			if maxBytes > 5<<20 { // 钳制上限：防一次性 bundle 全库的内存/上下文放大
				maxBytes = 5 << 20
			}
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			repo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			notes, _, err := e.d.Notes.List(ctx, e.id.UserID, repo.ID, in.Prefix, view)
			if err != nil {
				return "", err
			}
			if len(notes) == 0 {
				return "该范围下没有笔记。", nil
			}
			var sb strings.Builder
			used, included := 0, 0
			for _, meta := range notes {
				nf, err := e.d.Notes.Get(ctx, e.id.UserID, repo.ID, meta.Path, view)
				if err != nil {
					continue
				}
				part := fmt.Sprintf("\n\n---\n\n<!-- %s -->\n\n%s", nf.Path, nf.Content)
				if used+len(part) > maxBytes && included > 0 {
					break
				}
				sb.WriteString(part)
				used += len(part)
				included++
			}
			out := fmt.Sprintf("bundle（仓库：%s）共 %d/%d 篇（约 %s）：%s", repo.Name, included, len(notes), humanBytes(used), sb.String())
			if included < len(notes) {
				out += fmt.Sprintf("\n\n（已达输出上限，还有 %d 篇未包含；可缩小 prefix 或提高 max_bytes）", len(notes)-included)
			}
			return out, nil
		})

	type writeIn struct {
		Path         string   `json:"path" jsonschema:"笔记路径（仓库内相对路径），如 ai/xxx.md（末段缺 .md 自动补）"`
		Repo         string   `json:"repo,omitempty" jsonschema:"仓库名（可选；缺省为默认仓库；写前先 note_list 看仓库清单）"`
		Content      string   `json:"content" jsonschema:"Markdown 正文"`
		Title        string   `json:"title,omitempty" jsonschema:"标题（可选，缺省取正文一级标题或文件名）"`
		Tags         []string `json:"tags,omitempty" jsonschema:"标签（可选）"`
		ExpectedHash string   `json:"expected_hash,omitempty" jsonschema:"乐观锁（可选）：先 note_read 拿 hash，写入时若已被改动会拒绝（防覆盖新版本）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_write",
		Description: "写入 / 覆盖一篇知识库笔记（Markdown）。重复主题更新已有路径，不要新建冗余文件；写前先 search 查重。",
		Annotations: upsert(),
	}, string(model.ActionOf("note", "upsert")), func(in writeIn) string { return in.Path },
		func(ctx context.Context, in writeIn) (string, error) {
			if strings.TrimSpace(in.Path) == "" {
				return "", &service.UserError{Msg: "path（路径）必填"}
			}
			if in.Content == "" {
				return "", &service.UserError{Msg: "content（正文）必填"}
			}
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			repo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			note, hash, err := e.d.Notes.Upsert(ctx, e.id.UserID, repo.ID, in.Path, service.NoteUpsert{
				Title: in.Title, Content: in.Content, Tags: in.Tags, ExpectedHash: in.ExpectedHash,
			}, view)
			if err != nil {
				if err == service.ErrHashConflict {
					return "", fmt.Errorf("写入被拒：笔记「%s」在你读取后已被更新（hash 不匹配）。先 note_read 取最新版合并，再写入", in.Path)
				}
				return "", noteErr(err, in.Path)
			}
			return fmt.Sprintf("✓ 已保存 [%s] %s（%s，hash %s）", repo.Name, note.Path, humanBytes(len(in.Content)), hash[:12]), nil
		})

	type moveIn struct {
		From string `json:"from" jsonschema:"原路径（仓库内相对路径）"`
		To   string `json:"to" jsonschema:"新路径；跨仓库移动用「仓库名:路径」形态"`
		Repo string `json:"repo,omitempty" jsonschema:"from 所属仓库名（可选；缺省为默认仓库）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_move",
		Description: "重命名 / 移动笔记（from → to，原子操作）；跨仓库移动 to 写「仓库名:路径」。",
		Annotations: rw(),
	}, string(model.ActionOf("note", "move")), func(in moveIn) string { return in.From + " → " + in.To },
		func(ctx context.Context, in moveIn) (string, error) {
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			fromRepo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			toRepo, toPath := fromRepo, in.To
			// to 支持「仓库名:路径」跨仓库：冒号前缀命中本人活跃仓库名即跨仓
			if sep := strings.IndexByte(in.To, ':'); sep > 0 {
				tr, err2 := e.d.Repos.ResolveByName(ctx, e.id.UserID, in.To[:sep])
				if err2 != nil && !errors.Is(err2, service.ErrRepoNotFound) {
					return "", err2
				}
				if err2 == nil {
					toRepo, toPath = tr, in.To[sep+1:]
				}
			}
			note, err := e.d.Notes.Move(ctx, e.id.UserID, fromRepo.ID, in.From, toRepo.ID, toPath, view)
			if err != nil {
				switch {
				case errors.Is(err, service.ErrNoteDenied):
					return "", fmt.Errorf("该目录对你的 Key 不可见（目录权限限制）。不要反复尝试；如需访问，请让用户在网页面板调整目录权限")
				case isNotFound(err):
					return "", fmt.Errorf("笔记不存在：%s（用 note_list 查看）", strings.Trim(in.From, "/"))
				case err == service.ErrPathConflictDeleted:
					return "", fmt.Errorf("目标路径被一篇已删除的笔记占用：去回收站恢复它，或换个名字")
				case err == service.ErrPathConflict:
					return "", fmt.Errorf("目标路径已存在：%s（换个名字，或先处理已有笔记）", strings.Trim(in.To, "/"))
				}
				return "", err
			}
			return fmt.Sprintf("✓ 已移动 → [%s] %s", toRepo.Name, note.Path), nil
		})

	type deleteIn struct {
		Path string `json:"path" jsonschema:"笔记路径（仓库内相对路径）"`
		Repo string `json:"repo,omitempty" jsonschema:"仓库名（可选；缺省为默认仓库）"`
	}
	addTool(e, &sdk.Tool{
		Name:        "note_delete",
		Description: "删除笔记（进回收站，30 天内可恢复）。删除前先向用户确认。",
		Annotations: del(),
	}, string(model.ActionOf("note", "delete")), func(in deleteIn) string { return in.Path },
		func(ctx context.Context, in deleteIn) (string, error) {
			view, err := e.permView(ctx)
			if err != nil {
				return "", err
			}
			repo, err := e.resolveRepo(ctx, in.Repo)
			if err != nil {
				return "", err
			}
			if err := e.d.Notes.Delete(ctx, e.id.UserID, repo.ID, in.Path, view); err != nil {
				return "", noteErr(err, in.Path)
			}
			return fmt.Sprintf("✓ 已删除 [%s] %s（回收站保留 30 天，可在网页面板恢复）", repo.Name, strings.Trim(in.Path, "/")), nil
		})
}

func noteErr(err error, path string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, service.ErrNoteDenied) {
		return fmt.Errorf("该目录对你的 Key 不可见（目录权限限制）。不要反复尝试；如需访问，请让用户在网页面板调整目录权限")
	}
	if isNotFound(err) {
		return fmt.Errorf("笔记不存在：%s（用 note_list 查看已有路径）", strings.Trim(path, "/"))
	}
	return err
}

/* ============ 身份自检 ============ */

func registerWhoami(e *env) {
	addTool(e, &sdk.Tool{
		Name:        "whoami",
		Description: "连接与身份自检：返回当前用户、凭证与权限范围。接入后先用它验证链路。",
		Annotations: ro(),
	}, string(model.ActionOf("auth", "read")), nil,
		func(ctx context.Context, _ struct{}) (string, error) {
			scopeCN := map[model.KeyScope]string{
				model.KeyScopeAll:   "全部（待办 + 便签 + 知识库）",
				model.KeyScopeTodo:  "待办与便签",
				model.KeyScopeNotes: "知识库",
			}[e.id.Scope]
			var sb strings.Builder
			sb.WriteString("✓ 已连接「我的外脑」\n")
			if e.d.Account != nil {
				if u, err := e.d.Account.Me(ctx, e.id.UserID); err == nil && u != nil {
					fmt.Fprintf(&sb, "用户: %s\n", u.NickName)
				}
			}
			fmt.Fprintf(&sb, "凭证: Key「%s」（%s）\n权限: %s\n服务: extbrain-server %s", e.id.KeyName, e.id.KeyHint, scopeCN, e.d.Version)
			return sb.String(), nil
		})
}

/* ============ 工具函数 ============ */

// resolveRepo 解析工具入参的 repo 字段（空=默认仓库；未知仓库名→可执行错误）。
// 返回整仓：输出面要带仓库名。
func (e *env) resolveRepo(ctx context.Context, name string) (*model.Repo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		id, err := e.d.Repos.DefaultID(ctx, e.id.UserID)
		if err != nil {
			return nil, err
		}
		// 缺省仓名不查库（约定文案）；清单场景 note_list 自己会拉全量
		return &model.Repo{ID: id, Name: "默认仓库"}, nil
	}
	r, err := e.d.Repos.ResolveByName(ctx, e.id.UserID, name)
	if err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			return nil, &service.UserError{Msg: "仓库不存在：" + name + "（note_list 不带 repo 可看仓库清单）"}
		}
		return nil, err
	}
	return r, nil
}

func isNotFound(err error) bool {
	return err == service.ErrNoteNotFound || err == service.ErrTodoNotFound || err == service.ErrMemoNotFound
}

// parseDue 解析截止时间（北京时间/服务器本地时区）：
//   - 2026-10-09          → 当天 23:59（“当天截止”语义，不会一早被判逾期）
//   - 2026-10-09 18:00    → 本地 18:00
//   - RFC3339（带时区）    → 原样
func parseDue(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.Local), nil
	}
	return time.Time{}, &service.UserError{Msg: "时间格式不识别：" + s + "（支持 2026-10-09 / 2026-10-09 18:00）"}
}

func humanBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
