package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type NoteHandler struct {
	Notes *service.NoteService
	Repos *service.RepoService // 仓库解析（?repo= 与跨仓库寻址；迁移 0012）
	Todos *service.TodoService // scope=all 全域搜索（路由装配时注入）
	Memos *service.MemoService
	Perm  *service.NotePermService // 目录权限（AI Key 可见性；nil=不启用）
}

// reqKeyID 当前请求的 API Key id（Web JWT 会话=0）。
func reqKeyID(c *gin.Context) int64 {
	v, _ := c.Get(CtxKeyID)
	n, _ := v.(int64)
	return n
}

// permView 解析当前请求的目录权限视图：Web JWT → nil（不受限）；API Key → 按 Key 加载。
// 第二返回值 false = 已写出 500（读规则失败按 fail-closed 处理，不放行）。
func permView(c *gin.Context, p *service.NotePermService) (*service.PermView, bool) {
	if p == nil {
		return nil, true
	}
	v, err := p.View(c.Request.Context(), uid(c), reqKeyID(c))
	if err != nil {
		fail(c, errInternal("目录权限读取失败，请重试"))
		return nil, false
	}
	return v, true
}

// resolveRepo 解析当前请求作用的仓库：?repo= 仓库名（空=默认仓库；裸路径寻址=默认仓库）。
// 第二返回值 false = 已写出响应（仓库不存在 404 / 解析失败 500）。
func (h *NoteHandler) resolveRepo(c *gin.Context) (*model.Repo, bool) {
	return h.repoByParam(c, c.Query("repo"))
}

// annotateRepo 返回面注记：笔记带所属仓库名（repo_name；不落库的展示字段）。
func annotateRepo(n *model.Note, r *model.Repo) *model.Note {
	if n != nil && r != nil {
		n.RepoName = r.Name
	}
	return n
}

// failNoteDenied 目录权限拦截统一文案（AI 可执行：说明原因 + 下一步，不泄露目录内容）。
func failNoteDenied(c *gin.Context) {
	fail(c, errForbidden("该目录对你的 Key 不可见（目录权限限制）。不要反复尝试；如需访问，请让用户在网页面板调整目录权限"))
}

// ListNotes GET /api/v1/notes?prefix=ai/&repo=仓库名&limit=&offset=
// （缺省=默认仓库；path 均为仓库内相对路径；truncated=true 表示还有下一页）
func (h *NoteHandler) List(c *gin.Context) {
	repo, ok := h.resolveRepo(c)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	limit, _ := atoiDefault(c.Query("limit"), 0)
	offset, _ := atoiDefault(c.Query("offset"), 0)
	notes, dirs, truncated, err := h.Notes.ListPage(c.Request.Context(), uid(c), repo.ID, c.Query("prefix"), view, limit, offset)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	for i := range notes {
		notes[i].RepoName = repo.Name
	}
	c.JSON(http.StatusOK, gin.H{"notes": notes, "dirs": dirs, "truncated": truncated})
}

// GetNote GET /api/v1/notes/*path?format=md&repo=（format=md 或 Accept: text/markdown → 纯 MD 原文）
func (h *NoteHandler) Get(c *gin.Context) {
	repo, ok := h.resolveRepo(c)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	n, err := h.Notes.Get(c.Request.Context(), uid(c), repo.ID, c.Param("path"), view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoteNotFound):
			fail(c, errNotFound("笔记不存在："+strings.Trim(c.Param("path"), "/")))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		default:
			respondServiceErr(c, err)
		}
		return
	}
	if c.Query("format") == "md" || strings.Contains(c.GetHeader("Accept"), "text/markdown") {
		c.String(http.StatusOK, "%s", n.Content)
		return
	}
	annotateRepo(&n.Note, repo)
	c.JSON(http.StatusOK, n)
}

// PutNote PUT /api/v1/notes/*path?repo=（upsert；expected_hash 乐观锁不匹配 → 409）
func (h *NoteHandler) Put(c *gin.Context) {
	var in service.NoteUpsert
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {content, title?, tags?, expected_hash?}"))
		return
	}
	repo, ok := h.resolveRepo(c)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	note, hash, err := h.Notes.Upsert(c.Request.Context(), uid(c), repo.ID, c.Param("path"), in, view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrHashConflict):
			fail(c, errConflict("hash_conflict", "内容已被修改：先 GET 拿最新 expected_hash 再提交，或去掉 expected_hash 强制覆盖"))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"note": annotateRepo(note, repo), "content_hash": hash})
}

// DeleteNote DELETE /api/v1/notes/*path?repo=
func (h *NoteHandler) Delete(c *gin.Context) {
	repo, ok := h.resolveRepo(c)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	if err := h.Notes.Delete(c.Request.Context(), uid(c), repo.ID, c.Param("path"), view); err != nil {
		switch {
		case errors.Is(err, service.ErrNoteNotFound):
			fail(c, errNotFound("笔记不存在"))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// MoveNote POST /api/v1/notes/move —— body {from, to, repo?}：重命名 / 移动（原子；目标已存在 → 409）。
// to 支持「仓库名:路径」形态跨仓库移动（仓库名须为本人活跃仓库，名称不含冒号故无歧义）。
func (h *NoteHandler) Move(c *gin.Context) {
	var in struct {
		From string `json:"from" binding:"required"`
		To   string `json:"to" binding:"required"`
		Repo string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {from, to, repo?}"))
		return
	}
	c.Set(CtxAuditTarget, in.From+" → "+in.To) // 审计 Target 取实际路径（路由无 path 参数）
	fromRepo, ok := h.repoByParam(c, in.Repo)
	if !ok {
		return
	}
	toRepo, toPath := fromRepo, in.To
	// to 支持「仓库名:路径」跨仓库：冒号前缀命中本人活跃仓库名即跨仓（SplitCrossRepo 同语义），否则按普通路径
	if sep := strings.IndexByte(in.To, ':'); sep > 0 {
		tr, err2 := h.Repos.ResolveByName(c.Request.Context(), uid(c), in.To[:sep])
		switch {
		case err2 == nil:
			toRepo, toPath = tr, in.To[sep+1:]
		case !errors.Is(err2, service.ErrRepoNotFound):
			respondServiceErr(c, err2)
			return
		}
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	note, err := h.Notes.Move(c.Request.Context(), uid(c), fromRepo.ID, in.From, toRepo.ID, toPath, view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoteNotFound):
			fail(c, errNotFound("笔记不存在："+strings.Trim(in.From, "/")))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		case errors.Is(err, service.ErrPathConflictDeleted):
			fail(c, errConflict("path_conflict", "目标路径被一篇已删除的笔记占用：撤销删除它，或换个名字"))
		case errors.Is(err, service.ErrPathConflict):
			fail(c, errConflict("path_conflict", "目标路径已存在："+strings.Trim(in.To, "/")))
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"note": annotateRepo(note, toRepo)})
}

// RestoreNote POST /api/v1/notes/restore —— body {path, repo?}：撤销删除（软删恢复）
func (h *NoteHandler) Restore(c *gin.Context) {
	var in struct {
		Path string `json:"path" binding:"required"`
		Repo string `json:"repo"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {path, repo?}"))
		return
	}
	c.Set(CtxAuditTarget, in.Path)
	repo, ok := h.repoByParam(c, in.Repo)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	note, err := h.Notes.Restore(c.Request.Context(), uid(c), repo.ID, in.Path, view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoteNotFound):
			fail(c, errNotFound("已删除的笔记不存在："+strings.Trim(in.Path, "/")))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"note": annotateRepo(note, repo)})
}

// repoByParam 按显式仓库名解析（body/query 通用；空=默认仓库）。
// 缺省走 RepoService.Default 取真实行（默认仓库可改名，返回面注记不能写死「默认仓库」）。
func (h *NoteHandler) repoByParam(c *gin.Context, name string) (*model.Repo, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		r, err := h.Repos.Default(c.Request.Context(), uid(c))
		if err != nil {
			fail(c, errInternal("默认仓库读取失败，请重试"))
			return nil, false
		}
		return r, true
	}
	r, err := h.Repos.ResolveByName(c.Request.Context(), uid(c), name)
	if err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			fail(c, errNotFound("仓库不存在："+name))
			return nil, false
		}
		fail(c, errInternal("仓库解析失败，请重试"))
		return nil, false
	}
	return r, true
}

// Search GET /api/v1/search?q=&limit=&scope=&mode=
// scope 语义（与前端/MCP/CLI 契约一致）：
//   - 缺省：Web JWT / scope=all 的 Key → 三域 {todos,memos,notes}；notes 专属 Key → 笔记域 {hits}
//   - scope=note：笔记域 {hits}（库页搜索显式使用）
//   - scope=all：三域 {todos,memos,notes}；notes 专属 Key 调用 → 403（不得借道越权）
//   - 其他值：400
func (h *NoteHandler) Search(c *gin.Context) {
	limit, _ := atoiDefault(c.Query("limit"), 20)
	q := c.Query("q")
	mode := c.Query("mode")
	if !model.SearchMode(mode).Valid() {
		fail(c, errBadRequest("invalid_mode", "mode 是 keyword|vector|hybrid（缺省=自动）"))
		return
	}
	scope := c.Query("scope")
	allAllowed := keyScopeAllowsAll(c)
	switch scope {
	case "", "note":
		if scope == "" && allAllowed {
			break // 缺省且有权跨域：走下方全域分支（CLI/全局搜索兼容）
		}
		view, ok := permView(c, h.Perm)
		if !ok {
			return
		}
		hits, err := h.Notes.Search(c.Request.Context(), uid(c), q, limit, mode, view)
		if err != nil {
			respondServiceErr(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"hits": hits})
		return
	case "all":
		if !allAllowed {
			fail(c, errForbidden("该 Key 只能检索知识库；跨域搜索需要 scope=all 的 Key"))
			return
		}
	default:
		fail(c, errBadRequest("invalid_scope", "scope 是 note|all（缺省=自动）"))
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	todos, memos, notes, err := h.SearchAll(c.Request.Context(), uid(c), q, limit, mode, view)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"todos": todos, "memos": memos, "notes": notes})
}

// SearchAll 全域搜索（供 scope=all；按主体分组；待办/便签域不走向量）
func (h *NoteHandler) SearchAll(ctx context.Context, userID int64, q string, limit int, mode string, view *service.PermView) (
	[]service.TodoSearchHit, []service.MemoSearchHit, []service.SearchHit, error,
) {
	if h.Todos == nil || h.Memos == nil {
		return nil, nil, nil, errInternal("全域搜索组件未装配")
	}
	todos, err := h.Todos.Search(ctx, userID, q, limit)
	if err != nil {
		return nil, nil, nil, err
	}
	memos, err := h.Memos.Search(ctx, userID, q, limit)
	if err != nil {
		return nil, nil, nil, err
	}
	notes, err := h.Notes.Search(ctx, userID, q, limit, mode, view)
	if err != nil {
		return nil, nil, nil, err
	}
	return todos, memos, notes, nil
}

// keyScopeAllowsAll —— Web JWT（无 key_scope）恒 true；API Key 仅 scope=all 为 true。
func keyScopeAllowsAll(c *gin.Context) bool {
	v, ok := c.Get(CtxKeyScope)
	return !ok || v == string(model.KeyScopeAll)
}
