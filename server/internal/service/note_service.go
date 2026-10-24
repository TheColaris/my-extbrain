package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"extbrain-server/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NoteService —— 纯 MD 知识库：按 path 组织，upsert 幂等 + content_hash 乐观锁。
// Index/Vector 为可选装配（nil=未启用向量检索，全部行为与 v1 一致）。
type NoteService struct {
	DB     *gorm.DB
	Index  *IndexService
	Vector *VectorSearcher
}

var (
	ErrNoteNotFound = errors.New("note not found")
	ErrHashConflict = errors.New("hash conflict")
	ErrPathConflict = errors.New("path conflict")
	// ErrPathConflictDeleted：目标路径被「已删除」的笔记占用（唯一索引含软删行）——
	// 界面上看不见，需要更明确的提示（撤销删除或换个名字）
	ErrPathConflictDeleted = errors.New("path conflict with deleted note")
	// ErrNoteDenied：目录权限拦截（AI Key 对该目录不可见；读写同权）——403
	ErrNoteDenied = errors.New("note denied by folder permission")
	// errUpsertRace 内部哨兵：并发新建同路径撞唯一索引，调用方重试走覆盖分支
	errUpsertRace  = errors.New("note upsert race")
	pathMax        = 500
	noteTitleMax   = 255
	noteContentMax = 1536 * 1024 // 单篇正文上限（请求体全局限 2MB 之内）
)

const (
	noteListDefault = 500  // 单页默认条数（前端/MCP 循环翻页）
	noteListMax     = 1000 // 单页硬上限
)

type NoteUpsert struct {
	Title        string   `json:"title"`
	Content      string   `json:"content" binding:"required"`
	Tags         []string `json:"tags"`
	ExpectedHash string   `json:"expected_hash"` // 乐观锁：非空且不匹配当前 → 409；空 = 强制覆盖
}

// CleanPath 规范化路径：去首尾 /、禁非法段；空路径报错。
// 注意：目录前缀（List/权限规则）也走本函数，因此这里不做任何扩展名补全。
func CleanPath(p string) (string, error) {
	p = strings.Trim(p, "/")
	p = strings.TrimPrefix(p, "./")
	if p == "" {
		return "", &UserError{"path 不能为空"}
	}
	segs := strings.Split(p, "/")
	for _, s := range segs {
		if s == "" || s == "." || s == ".." {
			return "", &UserError{"path 含非法段（空/./..）"}
		}
	}
	if len(p) > pathMax {
		return "", &UserError{"path 过长（≤500）"}
	}
	return p, nil
}

// CleanNotePath 笔记路径归一：在 CleanPath 基础上给末段无扩展名的路径补 .md
// （与前端新建弹窗、MCP note_write 的文档口径一致；目录前缀仍用 CleanPath）。
func CleanNotePath(p string) (string, error) {
	p, err := CleanPath(p)
	if err != nil {
		return "", err
	}
	segs := strings.Split(p, "/")
	last := segs[len(segs)-1]
	if !strings.Contains(last, ".") {
		segs[len(segs)-1] = last + ".md"
		p = strings.Join(segs, "/")
		if len(p) > pathMax {
			return "", &UserError{"path 过长（≤500）"}
		}
	}
	return p, nil
}

// Upsert 按 (user, repo, path) 创建或覆盖；事务写 note + note_content。path 为仓库内相对路径。
// view=目录权限视图（nil=不受限，Web 面板；下同）——读写同权，不可见即拒绝。
// 并发新建同路径：唯一索引冲突后重试一次，第二次会走到覆盖分支（不再 500）。
func (s *NoteService) Upsert(ctx context.Context, userID, repoID int64, rawPath string, in NoteUpsert, view *PermView) (*model.Note, string, error) {
	p, err := CleanNotePath(rawPath)
	if err != nil {
		return nil, "", err
	}
	if !view.Allowed(repoID, p) {
		return nil, "", ErrNoteDenied
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = path.Base(p)
	}
	if len([]rune(title)) > noteTitleMax {
		return nil, "", &UserError{"标题过长（≤255 字）"}
	}
	content := strings.ReplaceAll(in.Content, "\r\n", "\n")
	hash := hashHex(content)

	var note model.Note
	for attempt := 0; attempt < 3; attempt++ {
		note = model.Note{}
		err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 不带 is_deleted 过滤：唯一索引 uk_note_repo_path 含软删行，同路径直接 Create 会撞键
			if err := tx.Where("user_id = ? AND repo_id = ? AND path = ?", userID, repoID, p).First(&note).Error; err != nil {
				if err != gorm.ErrRecordNotFound {
					return err
				}
				// 新建
				note = model.Note{UserID: userID, RepoID: repoID, Path: p, Title: title, Tags: normalizeTags(in.Tags)}
				if err := tx.Create(&note).Error; err != nil {
					if isUniquePathViolation(err) {
						return errUpsertRace
					}
					return err
				}
				return tx.Create(&model.NoteContent{NoteID: note.ID, Content: content, ContentHash: hash}).Error
			}
			if note.IsDeleted == 1 {
				// 复活软删笔记（语义=新建覆盖；expected_hash 对已删笔记无意义，不校验）
				note.IsDeleted = 0
				note.Title = title
				note.Tags = normalizeTags(in.Tags)
				note.UpdateTime = time.Now()
				if err := tx.Save(&note).Error; err != nil {
					return err
				}
				return tx.Model(&model.NoteContent{}).Where("note_id = ?", note.ID).
					Updates(map[string]any{"content": content, "content_hash": hash, "update_time": time.Now()}).Error
			}
			// 覆盖：乐观锁
			if in.ExpectedHash != "" {
				var cur model.NoteContent
				if err := tx.First(&cur, "note_id = ?", note.ID).Error; err == nil && cur.ContentHash != in.ExpectedHash {
					return ErrHashConflict
				}
			}
			note.Title = title
			note.Tags = normalizeTags(in.Tags)
			note.UpdateTime = time.Now()
			if err := tx.Save(&note).Error; err != nil {
				return err
			}
			return tx.Model(&model.NoteContent{}).Where("note_id = ?", note.ID).
				Updates(map[string]any{"content": content, "content_hash": hash, "update_time": time.Now()}).Error
		})
		if !errors.Is(err, errUpsertRace) {
			break
		}
	}
	if err != nil {
		if errors.Is(err, errUpsertRace) {
			return nil, "", &UserError{"并发写入冲突，请重试"}
		}
		return nil, "", err
	}
	if s.Index != nil { // 提交后投递异步重建（失败只记日志，绝不阻塞保存）
		s.Index.Enqueue(note.ID)
	}
	return &note, hash, nil
}

type NoteFull struct {
	model.Note
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
}

func (s *NoteService) Get(ctx context.Context, userID, repoID int64, rawPath string, view *PermView) (*NoteFull, error) {
	p, err := CleanNotePath(rawPath)
	if err != nil {
		return nil, err
	}
	if !view.Allowed(repoID, p) {
		return nil, ErrNoteDenied
	}
	var note model.Note
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND repo_id = ? AND path = ? AND is_deleted = 0", userID, repoID, p).
		First(&note).Error; err != nil {
		return nil, ErrNoteNotFound
	}
	var c model.NoteContent
	if err := s.DB.WithContext(ctx).First(&c, "note_id = ?", note.ID).Error; err != nil {
		return nil, ErrNoteNotFound
	}
	return &NoteFull{Note: note, Content: c.Content, ContentHash: c.ContentHash}, nil
}

// List 前缀列表（元信息，不含正文）+ 目录聚合（一级目录/文件）。
// 受限目录的笔记行与目录名一并过滤（目录名本身不可泄露）。
// 兼容包装：默认第一页（500 条）；需要完整结果时用 ListPage 翻页。
func (s *NoteService) List(ctx context.Context, userID, repoID int64, prefix string, view *PermView) ([]model.Note, []string, error) {
	notes, dirs, _, err := s.ListPage(ctx, userID, repoID, prefix, view, noteListDefault, 0)
	return notes, dirs, err
}

// ListPage 分页版前缀列表：limit/offset + truncated 标记（调用方按需继续翻页）。
func (s *NoteService) ListPage(ctx context.Context, userID, repoID int64, prefix string, view *PermView, limit, offset int) ([]model.Note, []string, bool, error) {
	if limit <= 0 || limit > noteListMax {
		limit = noteListDefault
	}
	if offset < 0 {
		offset = 0
	}
	if prefix != "" {
		var err error
		if prefix, err = CleanPath(prefix); err != nil {
			return nil, nil, false, err
		}
		prefix += "/"
	}
	notes := make([]model.Note, 0)
	q := s.DB.WithContext(ctx).Where("user_id = ? AND repo_id = ? AND is_deleted = 0", userID, repoID)
	if prefix != "" {
		q = q.Where("path LIKE ?", prefix+"%")
	}
	if err := q.Order("path ASC").Limit(limit + 1).Offset(offset).Find(&notes).Error; err != nil {
		return nil, nil, false, err
	}
	truncated := len(notes) > limit
	if truncated {
		notes = notes[:limit]
	}
	if view != nil {
		kept := make([]model.Note, 0, len(notes))
		for _, n := range notes {
			if view.Allowed(repoID, n.Path) {
				kept = append(kept, n)
			}
		}
		notes = kept
	}
	// 目录：取每条 path 去掉 prefix 后的首段
	seen := map[string]bool{}
	dirs := make([]string, 0) // 契约：JSON 恒数组（nil→null 会打崩前端 .map）
	for _, n := range notes {
		rest := strings.TrimPrefix(n.Path, prefix)
		if i := strings.IndexByte(rest, '/'); i > 0 {
			d := rest[:i]
			if !seen[d] {
				seen[d] = true
				if view == nil || view.Allowed(repoID, prefix+d) {
					dirs = append(dirs, d)
				}
			}
		}
	}
	return notes, dirs, truncated, nil
}

func (s *NoteService) Delete(ctx context.Context, userID, repoID int64, rawPath string, view *PermView) error {
	p, err := CleanNotePath(rawPath)
	if err != nil {
		return err
	}
	if !view.Allowed(repoID, p) {
		return ErrNoteDenied
	}
	res := s.DB.WithContext(ctx).Model(&model.Note{}).
		Where("user_id = ? AND repo_id = ? AND path = ? AND is_deleted = 0", userID, repoID, p).
		Update("is_deleted", 1)
	if res.RowsAffected == 0 {
		return ErrNoteNotFound
	}
	// 软删即清切片：搜索侧本就按 is_deleted 过滤，留着只会让索引状态虚高；
	// 恢复（Restore/Upsert 复活）会重新投递重建。
	if res.Error == nil {
		s.DB.WithContext(ctx).Exec(
			`DELETE FROM tf_note_chunk WHERE note_id IN
			 (SELECT id FROM tf_note WHERE user_id = ? AND repo_id = ? AND path = ?)`, userID, repoID, p)
	}
	return res.Error
}

// Move 重命名 / 移动：仅改 path 与 repo_id（正文、hash、标签、标题不动）。
// toRepoID 可与 fromRepoID 不同（跨仓库移动，to 为目标仓库内相对路径；
// 「仓库名:路径」形态的解析在调用方 RepoService.SplitCrossRepo 完成）。
// 目标被占用（含软删行——唯一索引含软删）→ ErrPathConflict。
func (s *NoteService) Move(ctx context.Context, userID, fromRepoID int64, rawFrom string, toRepoID int64, rawTo string, view *PermView) (*model.Note, error) {
	from, err := CleanNotePath(rawFrom)
	if err != nil {
		return nil, err
	}
	to, err := CleanNotePath(rawTo)
	if err != nil {
		return nil, err
	}
	if !view.Allowed(fromRepoID, from) || !view.Allowed(toRepoID, to) {
		return nil, ErrNoteDenied
	}
	if fromRepoID == toRepoID && from == to {
		return nil, &UserError{"新路径与原路径相同"}
	}
	var note model.Note
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND repo_id = ? AND path = ? AND is_deleted = 0", userID, fromRepoID, from).First(&note).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrNoteNotFound
			}
			return err
		}
		var occupied model.Note
		switch err := tx.Where("user_id = ? AND repo_id = ? AND path = ?", userID, toRepoID, to).First(&occupied).Error; {
		case err == nil:
			if occupied.IsDeleted == 1 {
				return ErrPathConflictDeleted
			}
			return ErrPathConflict
		case err != gorm.ErrRecordNotFound:
			return err
		}
		note.RepoID = toRepoID
		note.Path = to
		note.UpdateTime = time.Now()
		if err := tx.Save(&note).Error; err != nil {
			if isUniquePathViolation(err) { // 并发移动撞目标路径：与「目标已存在」同语义
				return ErrPathConflict
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &note, nil
}

// Restore 撤销删除（is_deleted 1→0）。唯一索引含软删行，同仓库同路径不可能被活跃笔记占用。
// 回收站恢复走 TrashService（按 note id）；本方法供 API {path, repo} 语义。
func (s *NoteService) Restore(ctx context.Context, userID, repoID int64, rawPath string, view *PermView) (*model.Note, error) {
	p, err := CleanNotePath(rawPath)
	if err != nil {
		return nil, err
	}
	if !view.Allowed(repoID, p) {
		return nil, ErrNoteDenied
	}
	res := s.DB.WithContext(ctx).Model(&model.Note{}).
		Where("user_id = ? AND repo_id = ? AND path = ? AND is_deleted = 1", userID, repoID, p).
		Updates(map[string]any{"is_deleted": 0, "update_time": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNoteNotFound
	}
	var note model.Note
	if err := s.DB.WithContext(ctx).Where("user_id = ? AND repo_id = ? AND path = ?", userID, repoID, p).First(&note).Error; err != nil {
		return nil, ErrNoteNotFound
	}
	if s.Index != nil { // 软删时已清切片，恢复后重建
		s.Index.Enqueue(note.ID)
	}
	return &note, nil
}

// SearchHit 搜索命中（含摘录；跨仓库检索——repo_id/repo_name 供展示与权限判定）
type SearchHit struct {
	RepoID    int64     `json:"repo_id"`
	RepoName  string    `json:"repo_name"`
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	SizeBytes int       `json:"size_bytes"` // 正文字节数（UTF-8）
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"` // keyword=纯关键词 / semantic=纯语义 / both=双命中（前端「语义」徽章依据）
}

// rrfK RRF 常数（排名融合免调参；关键词分与余弦分不可比，禁止分数相加）
const rrfK = 60

// Search 检索。mode：""/auto=provider 可用则混合、否则纯关键词；keyword；vector（不可用时降级）。
// hybrid=关键词与向量两路 goroutine 并行 → RRF 融合；向量路失败静默降级，绝不阻断搜索。
func (s *NoteService) Search(ctx context.Context, userID int64, q string, limit int, mode string, view *PermView) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, &UserError{"q 不能为空"}
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	vecOK := s.Vector != nil && s.Vector.Enabled()
	useVec := vecOK && mode != "keyword"
	useKW := mode != "vector"
	if !useVec && !useKW {
		useKW = true // 请求 vector 但 provider 不可用 → 降级关键词
	}

	var (
		kwHits []SearchHit
		kwIDs  []int64
		kwErr  error
		vHits  []VectorHit
		vecErr error
	)
	switch {
	case useVec && useKW:
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); kwHits, kwIDs, kwErr = s.searchKeyword(ctx, userID, q, limit) }()
		go func() { defer wg.Done(); vHits, vecErr = s.Vector.Search(ctx, userID, q, limit) }()
		wg.Wait()
	case useKW:
		kwHits, kwIDs, kwErr = s.searchKeyword(ctx, userID, q, limit)
	default:
		vHits, vecErr = s.Vector.Search(ctx, userID, q, limit)
	}
	if vecErr != nil {
		gin.DefaultErrorWriter.Write([]byte("[search] 向量检索降级（" + vecErr.Error() + "）\n"))
	}
	if kwErr != nil {
		return nil, kwErr
	}
	if len(vHits) == 0 {
		return filterHits(kwHits, view), nil
	}
	return filterHits(fuseRRF(kwHits, kwIDs, vHits, limit), view), nil
}

// filterHits 搜索结果的目录权限后过滤（关键词路与向量路统一收口，防语义检索绕过）。
// 过滤发生在 limit 截断之后：命中受限目录时本页条数可能变少，属预期。
func filterHits(hits []SearchHit, view *PermView) []SearchHit {
	if view == nil {
		return hits
	}
	out := make([]SearchHit, 0, len(hits))
	for _, h := range hits {
		if view.Allowed(h.RepoID, h.Path) {
			out = append(out, h)
		}
	}
	return out
}

// searchKeyword 关键词路（ILIKE 跨 title/path/content；update_time 倒序=融合排名的稳定依据）
// 正文只回窗口片段（position 定位命中点前后 240 字符），避免把每篇全文（可达 1.5MB）读进内存。
func (s *NoteService) searchKeyword(ctx context.Context, userID int64, q string, limit int) ([]SearchHit, []int64, error) {
	like := "%" + q + "%"
	var rows []struct {
		ID         int64
		RepoID     int64
		RepoName   string
		Path       string
		Title      string
		Snippet    string
		SizeBytes  int
		UpdateTime time.Time
	}
	err := s.DB.WithContext(ctx).Table("tf_note n").
		Select(`n.id, n.repo_id, n.path, n.title, n.update_time, r.name AS repo_name,
			octet_length(c.content) AS size_bytes,
			CASE WHEN position(lower(?) in lower(c.content)) > 0
				THEN substring(c.content from greatest(1, position(lower(?) in lower(c.content)) - 96) for 240)
				ELSE left(c.content, 240) END AS snippet`, q, q).
		Joins("JOIN tf_note_content c ON c.note_id = n.id").
		Joins("JOIN tf_repo r ON r.id = n.repo_id").
		Where("n.user_id = ? AND n.is_deleted = 0 AND (n.title ILIKE ? OR n.path ILIKE ? OR c.content ILIKE ?)", userID, like, like, like).
		Order("n.update_time DESC").
		Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	hits := make([]SearchHit, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, SearchHit{
			RepoID: r.RepoID, RepoName: r.RepoName,
			Path: r.Path, Title: r.Title, Snippet: excerpt(r.Snippet, q, 48),
			SizeBytes: r.SizeBytes, UpdatedAt: r.UpdateTime, Source: string(model.SearchSourceKeyword),
		})
		ids = append(ids, r.ID)
	}
	return hits, ids, nil
}

type fusedItem struct {
	hit    SearchHit
	score  float64
	hasKW  bool
	hasVec bool
}

// fuseRRF 两路按排名融合：score=Σ 1/(k+rank)，同一篇双命中天然排前。
// 摘录优先关键词路（含高亮词）；纯语义命中用切片原文。
func fuseRRF(kw []SearchHit, kwIDs []int64, vec []VectorHit, limit int) []SearchHit {
	idx := map[int64]int{}
	items := make([]fusedItem, 0, len(kw)+len(vec))
	get := func(id int64) *fusedItem {
		if i, ok := idx[id]; ok {
			return &items[i]
		}
		items = append(items, fusedItem{})
		idx[id] = len(items) - 1
		return &items[len(items)-1]
	}
	for i, h := range kw {
		it := get(kwIDs[i])
		it.hit = h
		it.hasKW = true
		it.score += 1.0 / float64(rrfK+i+1)
	}
	seen := map[int64]bool{} // 同笔记多切片只取最佳（向量已按距离升序）
	for i, vh := range vec {
		if seen[vh.NoteID] {
			continue
		}
		seen[vh.NoteID] = true
		it := get(vh.NoteID)
		if !it.hasKW {
			// 纯语义命中：repo 归属必须随行（漏带会让 filterHits 按 repo_id=0 判定=无规则放行，
			// 受限仓库经语义检索泄露；同时前端丢仓库标记）
			it.hit = SearchHit{
				RepoID: vh.RepoID, RepoName: vh.RepoName,
				Path: vh.Path, Title: vh.Title, Snippet: SemanticSnippet(vh.ChunkText),
				SizeBytes: vh.SizeBytes, UpdatedAt: vh.UpdatedAt,
			}
		}
		it.hasVec = true
		it.score += 1.0 / float64(rrfK+i+1)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]SearchHit, 0, len(items))
	for _, it := range items {
		switch {
		case it.hasKW && it.hasVec:
			it.hit.Source = string(model.SearchSourceBoth)
		case it.hasVec:
			it.hit.Source = string(model.SearchSourceSemantic)
		default:
			it.hit.Source = string(model.SearchSourceKeyword)
		}
		out = append(out, it.hit)
	}
	return out
}

func excerpt(content, q string, around int) string {
	n := len(content)
	i := strings.Index(strings.ToLower(content), strings.ToLower(q))
	if i < 0 || n == 0 {
		return cut(content, 96)
	}
	start := i - around
	if start < 0 {
		start = 0
	}
	end := i + len(q) + around
	if end > n { // 命中靠末尾时收敛边界（ToLower 极少数字符变长，clamp 兜底防越界）
		end = n
	}
	if start >= end {
		start, end = 0, min(96, n)
	}
	return "…" + cut(content[start:end], 2*around+8) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

var _ = fmt.Sprintf
