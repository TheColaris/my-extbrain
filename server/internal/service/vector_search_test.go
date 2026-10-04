package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// ---------- 切分（纯单测） ----------

func TestChunkMarkdown(t *testing.T) {
	if got := ChunkMarkdown("   \n\n  "); got != nil {
		t.Fatalf("空内容应返回 nil，得 %v", got)
	}
	if got := ChunkMarkdown("短内容一段"); len(got) != 1 || got[0] != "短内容一段" {
		t.Fatalf("单段应 1 片，得 %v", got)
	}
	// 标题分节：两节 → 两片
	md := "# 标题一\n正文一\n\n## 标题二\n正文二"
	if got := ChunkMarkdown(md); len(got) != 2 {
		t.Fatalf("标题应分节成 2 片，得 %d：%v", len(got), got)
	}
	// 长正文按目标长度切分，且每片不超硬上限
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("这是一段用于测试切分的长文本内容。\n")
	}
	chunks := ChunkMarkdown(sb.String())
	if len(chunks) < 2 {
		t.Fatalf("长文本应切成多片，得 %d", len(chunks))
	}
	for i, c := range chunks {
		if n := len([]rune(c)); n > chunkMaxRunes {
			t.Fatalf("第 %d 片超上限：%d", i, n)
		}
	}
	// 超长单行 → 滑窗切片（含重叠，末片 ≤ 上限）
	long := strings.Repeat("甲", 4000)
	pieces := splitLongLine(long)
	if len(pieces) != 3 {
		t.Fatalf("4000 字单行应切 3 片，得 %d", len(pieces))
	}
	if len([]rune(pieces[0])) != chunkMaxRunes {
		t.Fatalf("首片应满窗：%d", len([]rune(pieces[0])))
	}
}

// ---------- RRF 融合（纯单测） ----------

func TestFuseRRF(t *testing.T) {
	kw := []SearchHit{
		{Path: "a.md", Title: "A", Snippet: "kw-a"},
		{Path: "b.md", Title: "B", Snippet: "kw-b"},
	}
	kwIDs := []int64{1, 2}
	vec := []VectorHit{
		{NoteID: 2, Path: "b.md", Title: "B", ChunkText: "vec-b"},
		{NoteID: 3, Path: "c.md", Title: "C", ChunkText: "vec-c"},
	}
	out := fuseRRF(kw, kwIDs, vec, 10)
	if len(out) != 3 {
		t.Fatalf("融合应 3 条，得 %d", len(out))
	}
	// 双命中的 b 应排最前（1/(60+2) + 1/(60+1)）
	if out[0].Path != "b.md" || out[0].Source != "both" {
		t.Fatalf("双命中应排首且标 both，得 %+v", out[0])
	}
	// 单路命中各归其源（a=关键词 rank1 与 c=向量 rank2 在 RRF 下几乎同分，只断言集合与来源）
	byPath := map[string]SearchHit{}
	for _, h := range out {
		byPath[h.Path] = h
	}
	if h := byPath["a.md"]; h.Source != "keyword" || h.Snippet != "kw-a" {
		t.Fatalf("纯关键词命中异常：%+v", h)
	}
	if h := byPath["c.md"]; h.Source != "semantic" || !strings.Contains(h.Snippet, "vec-c") {
		t.Fatalf("纯语义命中异常（摘录应来自切片原文）：%+v", h)
	}
	// 双命中保留关键词摘录（带高亮词）
	if byPath["b.md"].Snippet != "kw-b" {
		t.Fatalf("双命中应保留关键词摘录：%+v", byPath["b.md"])
	}
	// limit 生效
	if got := fuseRRF(kw, kwIDs, vec, 1); len(got) != 1 || got[0].Path != "b.md" {
		t.Fatalf("limit=1 应只留首位：%+v", got)
	}
	// 无向量结果 → 关键词原序原样
	if got := fuseRRF(kw, kwIDs, nil, 10); len(got) != 2 || got[0].Path != "a.md" {
		t.Fatalf("无向量时应原样返回：%+v", got)
	}
}

// ---------- 集成：索引 + 混合检索（fake embedder） ----------

// fakeEmbedder 确定性假向量：按关键词分簇（限流族→e0 / docker 族→e1 /
// unless-stopped→e2 空簇 / 其余→e3）。让「语义命中但关键词不中」可被精确构造。
type fakeEmbedder struct {
	dim   int
	calls atomic.Int64 // 累计向量化文本条数（缓存去重断言用）
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls.Add(int64(len(texts)))
	out := make([][]float32, len(texts))
	for i, s := range texts {
		v := make([]float32, f.dim)
		switch {
		case strings.Contains(s, "限流") || strings.Contains(s, "rate limit") || strings.Contains(s, "成本"):
			v[0] = 1
		case strings.Contains(s, "docker") || strings.Contains(s, "compose") || strings.Contains(s, "部署"):
			v[1] = 1
		case strings.Contains(s, "unless-stopped"):
			v[2] = 1 // 无笔记落此簇 → 该查询的向量路必然空（构造纯关键词场景）
		default:
			v[3] = 1
		}
		out[i] = v
	}
	return out, nil
}

func vectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testDB(t)
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_note_chunk, tp_system_config")
	return db
}

// setupVector 装配：配置（enabled + fake 客户端）+ 索引服务 + 检索服务。
func setupVector(t *testing.T, db *gorm.DB) (*SysConfig, *IndexService, *NoteService, *fakeEmbedder) {
	t.Helper()
	ctx := context.Background()
	cfg := NewSysConfig(db)
	if err := cfg.Set(ctx, map[string]string{
		CfgEmbeddingEnabled: "true",
		CfgEmbeddingBaseURL: "http://fake.local/v1",
		CfgEmbeddingAPIKey:  "sk-test",
		CfgEmbeddingModel:   "fake",
		CfgEmbeddingDim:     "1024",
	}); err != nil {
		t.Fatalf("配置写入失败: %v", err)
	}
	if err := cfg.Load(ctx); err != nil {
		t.Fatalf("配置加载失败: %v", err)
	}
	fake := &fakeEmbedder{dim: 1024}
	idx := NewIndexService(db, cfg)
	idx.NewClient = func(EmbeddingConfig) Embedder { return fake }
	vs := NewVectorSearcher(db, cfg)
	vs.NewClient = func(EmbeddingConfig) Embedder { return fake }
	return cfg, idx, &NoteService{DB: db, Index: idx, Vector: vs}, fake
}

func seedNotes(t *testing.T, notes *NoteService, idx *IndexService, uid int64) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	rid := testRepoID(t, notes.DB, uid)
	seed := []struct{ path, content string }{
		{"ai/限流方案.md", "服务端限流用令牌桶，超限直接 429。"},
		{"ops/部署.md", "docker compose 部署，容器重启策略 unless-stopped。"},
	}
	ids := map[string]int64{}
	for _, s := range seed {
		n, _, err := notes.Upsert(ctx, uid, rid, s.path, NoteUpsert{Content: s.content}, nil)
		if err != nil {
			t.Fatalf("seed %s: %v", s.path, err)
		}
		ids[s.path] = n.ID
		if err := idx.indexNote(ctx, n.ID, true); err != nil { // 同步重建（确定性）
			t.Fatalf("索引 %s: %v", s.path, err)
		}
	}
	return ids
}

func TestVectorSearchFlow(t *testing.T) {
	db := vectorTestDB(t)
	ctx := context.Background()
	cfg, idx, notes, _ := setupVector(t, db)
	ids := seedNotes(t, notes, idx, 21)

	// 索引落库：2 篇 → 2 片，hash 与正文一致
	var chunkCnt int64
	db.Model(&model.NoteChunk{}).Count(&chunkCnt)
	if chunkCnt != 2 {
		t.Fatalf("应 2 片切片，得 %d", chunkCnt)
	}
	// hash 一致时跳过（非 force）
	before := time.Now().Add(-time.Second)
	if err := idx.indexNote(ctx, ids["ai/限流方案.md"], false); err != nil {
		t.Fatalf("重复索引: %v", err)
	}
	var updated []time.Time
	db.Model(&model.NoteChunk{}).Where("note_id = ?", ids["ai/限流方案.md"]).Pluck("update_time", &updated)
	if len(updated) != 1 || updated[0].Before(before) {
		t.Fatalf("hash 一致应跳过重建（update_time 不应刷新）: %v", updated)
	}

	// ① 语义命中（关键词不中）：「限流策略」不在正文里，但假向量同簇
	hits, err := notes.Search(ctx, 21, "限流策略", 10, "", nil)
	if err != nil {
		t.Fatalf("混合检索失败: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "ai/限流方案.md" || hits[0].Source != "semantic" {
		t.Fatalf("语义命中异常: %+v", hits)
	}
	if !strings.Contains(hits[0].Snippet, "令牌桶") {
		t.Fatalf("语义摘录应来自切片原文: %q", hits[0].Snippet)
	}
	if hits[0].UpdatedAt.IsZero() || hits[0].SizeBytes == 0 {
		t.Fatalf("语义命中应带更新时间与大小: %+v", hits[0]) // 列名≠字段名的静默空值坑（GORM Scan）
	}
	// ② 双命中：「限流」关键词+向量同簇 → both
	hits, _ = notes.Search(ctx, 21, "限流", 10, "", nil)
	if len(hits) != 1 || hits[0].Source != "both" {
		t.Fatalf("双命中异常: %+v", hits)
	}
	// ③ 纯关键词：「unless-stopped」关键词中，但假向量落在无笔记的空簇（且相似度地板也会滤掉）
	hits, _ = notes.Search(ctx, 21, "unless-stopped", 10, "", nil)
	if len(hits) != 1 || hits[0].Path != "ops/部署.md" || hits[0].Source != "keyword" {
		t.Fatalf("纯关键词命中异常: %+v", hits)
	}
	// ③b 相似度地板：无关笔记（假向量正交=相似度 0）不进语义结果
	if hits, _ = notes.Search(ctx, 21, "限流策略", 10, "vector", nil); len(hits) != 1 {
		t.Fatalf("相似度地板应滤掉无关笔记: %+v", hits)
	}
	// ④ mode=keyword 关闭向量
	if hits, _ = notes.Search(ctx, 21, "限流策略", 10, "keyword", nil); len(hits) != 0 {
		t.Fatalf("keyword 模式不应有语义命中: %+v", hits)
	}
	// ⑤ mode=vector 只走向量
	if hits, _ = notes.Search(ctx, 21, "限流策略", 10, "vector", nil); len(hits) != 1 || hits[0].Source != "semantic" {
		t.Fatalf("vector 模式异常: %+v", hits)
	}
	// ⑥ 跨用户隔离：同查询在用户 22 下无结果
	if hits, _ = notes.Search(ctx, 22, "限流策略", 10, "", nil); len(hits) != 0 {
		t.Fatalf("跨用户不应命中: %+v", hits)
	}
	// ⑦ 软删后不命中（chunk 保留但 join 过滤）
	if err := notes.Delete(ctx, 21, testRepoID(t, notes.DB, 21), "ai/限流方案.md", nil); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if hits, _ = notes.Search(ctx, 21, "限流策略", 10, "", nil); len(hits) != 0 {
		t.Fatalf("软删笔记不应命中: %+v", hits)
	}

	// ⑧ 降级：provider 未启用 → 纯关键词（语义查询 0 命中，关键词查询照常）
	if err := cfg.Set(ctx, map[string]string{CfgEmbeddingEnabled: "false"}); err != nil {
		t.Fatalf("关闭配置失败: %v", err)
	}
	if hits, _ = notes.Search(ctx, 21, "限流策略", 10, "", nil); len(hits) != 0 {
		t.Fatalf("降级后语义查询应无命中: %+v", hits)
	}
	if hits, _ = notes.Search(ctx, 21, "unless-stopped", 10, "", nil); len(hits) != 1 {
		t.Fatalf("降级后关键词应照常: %+v", hits)
	}
}

func TestIndexStatusAndRebuild(t *testing.T) {
	db := vectorTestDB(t)
	ctx := context.Background()
	_, idx, notes, _ := setupVector(t, db)
	seedNotes(t, notes, idx, 21)

	st, err := idx.Status(ctx)
	if err != nil {
		t.Fatalf("状态读取失败: %v", err)
	}
	if st.Notes != 2 || st.Chunks != 2 || st.Pending != 0 || st.Failed != 0 {
		t.Fatalf("状态数字异常: %+v", st)
	}
	if st.IndexDim != 1024 || st.ConfigDim != 1024 {
		t.Fatalf("维度读数异常: index=%d config=%d", st.IndexDim, st.ConfigDim)
	}
	if st.LastIndexAt == nil {
		t.Fatal("最近索引时间应有值")
	}

	// 重建：清空后全量补回（异步；轮询完成）
	if err := db.Exec("TRUNCATE tf_note_chunk").Error; err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if err := idx.ReindexAll(ctx); err != nil {
		t.Fatalf("重建失败: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int64
		db.Model(&model.NoteChunk{}).Count(&n)
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("重建未完成，得 %d 片", n)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 维度对齐：768 → 1024 往返（空表 ALTER + HNSW 重建）
	if err := idx.ensureDim(ctx, 768); err != nil {
		t.Fatalf("维度对齐(768)失败: %v", err)
	}
	if dim, _ := idx.indexDim(ctx); dim != 768 {
		t.Fatalf("列维度应 768，得 %d", dim)
	}
	if err := idx.ensureDim(ctx, 1024); err != nil {
		t.Fatalf("维度对齐(1024)失败: %v", err)
	}
	if dim, _ := idx.indexDim(ctx); dim != 1024 {
		t.Fatalf("列维度应还原 1024，得 %d", dim)
	}
}

// 异步队列：Upsert 投递 → worker 落片（懒补偿兜底路径同时覆盖）
func TestIndexAsyncEnqueue(t *testing.T) {
	db := vectorTestDB(t)
	ctx := context.Background()
	_, idx, notes, _ := setupVector(t, db)
	idx.Start(ctx)

	if _, _, err := notes.Upsert(ctx, 21, testRepoID(t, notes.DB, 21), "ai/异步.md", NoteUpsert{Content: "限流 异步索引测试"}, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var n int64
		db.Model(&model.NoteChunk{}).Count(&n)
		if n >= 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("异步索引超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// 查询向量缓存：同词两次搜索只向量化一次（Web 逐字防抖场景的实际收益）
func TestVecCacheDedup(t *testing.T) {
	db := vectorTestDB(t)
	ctx := context.Background()
	_, idx, notes, fake := setupVector(t, db)
	seedNotes(t, notes, idx, 21)

	before := fake.calls.Load()
	hits1, err := notes.Search(ctx, 21, "限流策略", 10, "", nil)
	if err != nil || len(hits1) != 1 {
		t.Fatalf("首查应命中: %d err=%v", len(hits1), err)
	}
	if got := fake.calls.Load() - before; got != 1 {
		t.Fatalf("首查应恰好 1 次查询向量化，得 %d", got)
	}
	if hits2, _ := notes.Search(ctx, 21, "限流策略", 10, "vector", nil); len(hits2) != 1 {
		t.Fatalf("二查应命中: %+v", hits2)
	}
	if got := fake.calls.Load() - before; got != 1 {
		t.Fatalf("重复查询应命中缓存（不重复调用），累计 %d", got)
	}
}

// 迁移 0010 契约：关键词路 trgm 索引齐备（缺一条=ILIKE 退回全表扫描）
func TestSearchTrgmIndexes(t *testing.T) {
	db := testDB(t)
	var ext int64
	db.Raw(`SELECT count(*) FROM pg_extension WHERE extname = 'pg_trgm'`).Scan(&ext)
	if ext != 1 {
		t.Fatal("pg_trgm 未启用")
	}
	for _, name := range []string{
		"idx_note_title_trgm", "idx_note_path_trgm", "idx_note_content_content_trgm",
		"idx_memo_content_trgm", "idx_todo_title_trgm", "idx_todo_remark_trgm",
	} {
		var n int64
		db.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = ?`, name).Scan(&n)
		if n != 1 {
			t.Fatalf("缺索引 %s（迁移 0010 未生效？）", name)
		}
	}
}

// iterative_scan 可用性判定：pgvector>=0.8 才 SET LOCAL（老版本/异常版本一律降级）
func TestIterScanSupported(t *testing.T) {
	cases := map[string]bool{
		"0.8.2": true, "0.8.7": true, "0.9.0": true, "1.0.0": true,
		"0.7.4": false, "0.6.2": false, "": false, "garbage": false, "0.x": false,
	}
	for ver, want := range cases {
		if got := iterScanSupported(ver); got != want {
			t.Fatalf("iterScanSupported(%q)=%v want %v", ver, got, want)
		}
	}
}
