package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"extbrain-server/internal/model"

	"github.com/lib/pq"
)

func TestNoteLifecycle(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content")
	s := &NoteService{DB: db}
	r21 := testRepoID(t, db, 21)

	// 创建
	n1, h1, err := s.Upsert(ctx, 21, r21, "ai/glm 使用笔记.md", NoteUpsert{Content: "# GLM\nSonnet 对比笔记"}, nil)
	if err != nil || n1.Path != "ai/glm 使用笔记.md" || n1.Title != "glm 使用笔记.md" {
		t.Fatalf("创建失败: %v %+v", err, n1)
	}
	if h1 == "" {
		t.Fatal("应返回 content_hash")
	}
	// 非法 path；"/a.md" 会被宽容归一为 "a.md"（合法）
	for _, bad := range []string{"", "a/../b.md", "a//b.md"} {
		if _, _, err := s.Upsert(ctx, 21, r21, bad, NoteUpsert{Content: "x"}, nil); err == nil {
			t.Fatalf("path %q 应报错", bad)
		}
	}
	if n, _, err := s.Upsert(ctx, 21, r21, "/归一测试.md", NoteUpsert{Content: "x"}, nil); err != nil || n.Path != "归一测试.md" {
		t.Fatalf("前导 / 应归一: %+v err=%v", n, err)
	}
	// upsert 覆盖 + 乐观锁
	_, _, err = s.Upsert(ctx, 21, r21, "ai/glm 使用笔记.md", NoteUpsert{Content: "v2", ExpectedHash: "wrong"}, nil)
	if err != ErrHashConflict {
		t.Fatalf("错 hash 应 409 语义: %v", err)
	}
	_, h2, _ := s.Upsert(ctx, 21, r21, "ai/glm 使用笔记.md", NoteUpsert{Content: "v2 内容", Title: "自定义标题", ExpectedHash: h1}, nil)
	if h2 == h1 {
		t.Fatal("hash 应更新")
	}
	got, _ := s.Get(ctx, 21, r21, "ai/glm 使用笔记.md", nil)
	if got.Content != "v2 内容" || got.Title != "自定义标题" {
		t.Fatalf("覆盖异常: %+v", got)
	}
	// 前缀列表 + 目录
	s.Upsert(ctx, 21, r21, "ai/prompt.md", NoteUpsert{Content: "x"}, nil)
	s.Upsert(ctx, 21, r21, "ops/docker.md", NoteUpsert{Content: "y"}, nil)
	notes, dirs, err := s.List(ctx, 21, r21, "", nil)
	if err != nil || len(notes) != 4 || len(dirs) != 2 { // 3 条 + 归一测试.md
		t.Fatalf("全量列表: notes=%d dirs=%v err=%v", len(notes), dirs, err)
	}
	notes, dirs, _ = s.List(ctx, 21, r21, "ai", nil)
	if len(notes) != 2 || len(dirs) != 0 {
		t.Fatalf("ai 前缀: notes=%d dirs=%d", len(notes), len(dirs))
	}
	// 跨用户隔离
	if _, err := s.Get(ctx, 22, testRepoID(t, db, 22), "ai/glm 使用笔记.md", nil); err != ErrNoteNotFound {
		t.Fatal("跨用户应 Not Found")
	}
	// 搜索（title 命中 + content 命中摘录）
	hits, err := s.Search(ctx, 21, "glm", 10, "", nil)
	if err != nil || len(hits) != 1 { // path 命中（title 已被覆盖为「自定义标题」）
		t.Fatalf("glm 应命中: %d err=%v", len(hits), err)
	}
	hits, _ = s.Search(ctx, 21, "docker", 10, "", nil)
	if len(hits) != 1 || hits[0].Path != "ops/docker.md" {
		t.Fatalf("docker 命中异常: %+v", hits)
	}
	if _, err := s.Search(ctx, 21, "  ", 10, "", nil); err == nil {
		t.Fatal("空 q 应报错")
	}
	// 删除
	if err := s.Delete(ctx, 21, r21, "ops/docker.md", nil); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := s.Get(ctx, 21, r21, "ops/docker.md", nil); err != ErrNoteNotFound {
		t.Fatal("删除后应 Not Found")
	}
	_ = model.Note{}
}

// v2 知识库：重命名/移动、撤销删除、删后重建同路径（唯一索引含软删行 → 复活语义）
func TestNoteMoveRestoreRevive(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content")
	s := &NoteService{DB: db}
	r31 := testRepoID(t, db, 31)

	_, h1, _ := s.Upsert(ctx, 31, r31, "ai/old.md", NoteUpsert{Content: "v1", Title: "旧名", Tags: []string{"t"}}, nil)

	// 重命名：path 变更，内容 / hash / 标题 / 标签不动
	moved, err := s.Move(ctx, 31, r31, "ai/old.md", r31, "dev/new.md", nil)
	if err != nil || moved.Path != "dev/new.md" || moved.Title != "旧名" {
		t.Fatalf("移动失败: %+v err=%v", moved, err)
	}
	if _, err := s.Get(ctx, 31, r31, "ai/old.md", nil); err != ErrNoteNotFound {
		t.Fatal("旧路径应不存在")
	}
	got, err := s.Get(ctx, 31, r31, "dev/new.md", nil)
	if err != nil || got.Content != "v1" || got.ContentHash != h1 {
		t.Fatalf("移动后内容/hash 应不变: %+v err=%v", got, err)
	}
	// 目标已存在 → 冲突
	if _, _, err := s.Upsert(ctx, 31, r31, "dev/exist.md", NoteUpsert{Content: "x"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(ctx, 31, r31, "dev/new.md", r31, "dev/exist.md", nil); err != ErrPathConflict {
		t.Fatalf("目标已存在应冲突: %v", err)
	}
	// 同路径 / 源不存在 / 非法目标 / 跨用户
	if _, err := s.Move(ctx, 31, r31, "dev/new.md", r31, "dev/new.md", nil); err == nil {
		t.Fatal("同路径应报错")
	}
	if _, err := s.Move(ctx, 31, r31, "dev/none.md", r31, "dev/x.md", nil); err != ErrNoteNotFound {
		t.Fatal("源不存在应 Not Found")
	}
	if _, err := s.Move(ctx, 31, r31, "dev/new.md", r31, "a/../b.md", nil); err == nil {
		t.Fatal("非法目标应报错")
	}
	if _, err := s.Move(ctx, 32, testRepoID(t, db, 32), "dev/new.md", testRepoID(t, db, 32), "dev/stolen.md", nil); err != ErrNoteNotFound {
		t.Fatal("跨用户应 Not Found")
	}

	// 删除 → 撤销恢复
	if err := s.Delete(ctx, 31, r31, "dev/new.md", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Restore(ctx, 31, r31, "dev/new.md", nil); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if got, err := s.Get(ctx, 31, r31, "dev/new.md", nil); err != nil || got.Content != "v1" {
		t.Fatalf("恢复后应可读: %+v err=%v", got, err)
	}
	if _, err := s.Restore(ctx, 31, r31, "dev/new.md", nil); err != ErrNoteNotFound {
		t.Fatal("未删除的笔记 restore 应 Not Found")
	}
	if _, err := s.Restore(ctx, 32, testRepoID(t, db, 32), "dev/new.md", nil); err != ErrNoteNotFound {
		t.Fatal("跨用户 restore 应 Not Found")
	}

	// 删后重建同路径：唯一索引含软删行，必须复活而非 500（历史 bug）
	if err := s.Delete(ctx, 31, r31, "dev/new.md", nil); err != nil {
		t.Fatal(err)
	}
	rev, h2, err := s.Upsert(ctx, 31, r31, "dev/new.md", NoteUpsert{Content: "v2", Title: "重建"}, nil)
	if err != nil {
		t.Fatalf("删后重建应成功（复活）: %v", err)
	}
	if rev.ID == 0 || h2 == h1 {
		t.Fatalf("复活异常: %+v h2=%s", rev, h2)
	}
	got2, err := s.Get(ctx, 31, r31, "dev/new.md", nil)
	if err != nil || got2.Content != "v2" || got2.Title != "重建" {
		t.Fatalf("复活后应为新内容: %+v err=%v", got2, err)
	}
	var cnt int64
	db.Model(&model.Note{}).Where("user_id = ? AND path = ?", 31, "dev/new.md").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("复活后应仍为一行: %d", cnt)
	}
	// 移动到被软删行占用的路径 → 冲突（软删行占唯一键）
	if _, _, err := s.Upsert(ctx, 31, r31, "dev/gone.md", NoteUpsert{Content: "z"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, 31, r31, "dev/gone.md", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(ctx, 31, r31, "dev/new.md", r31, "dev/gone.md", nil); err != ErrPathConflictDeleted {
		t.Fatalf("目标被软删行占用应返回专门错误: %v", err)
	}
}

func TestExcerptBounds(t *testing.T) {
	// 命中在末尾（历史越界 panic 用例）与超短内容
	cases := []struct{ content, q string }{
		{"prefix-padding-padding " + strings.Repeat("字", 80) + " temperature", "temperature"},
		{"temperature", "temperature"},
		{"", "x"},
		{"abc", "不存在"},
	}
	for _, c := range cases {
		got := excerpt(c.content, c.q, 48) // 不 panic 即通过
		if len([]rune(got)) > 48*2+10+2 {
			t.Errorf("摘录超长: %d", len([]rune(got)))
		}
	}
}

// API 契约：空/根级数据下 list 字段恒数组非 null（历史 dirs:null 打崩前端 .map）
func TestListContractArrays(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content")
	s := &NoteService{DB: db}
	r41 := testRepoID(t, db, 41)
	notes, dirs, err := s.List(ctx, 41, r41, "", nil)
	if err != nil || notes == nil || dirs == nil {
		t.Fatalf("空库应返回空数组非 nil: notes=%v dirs=%v err=%v", notes, dirs, err)
	}
	_, _, _ = s.Upsert(ctx, 41, r41, "根级.md", NoteUpsert{Content: "x"}, nil)
	notes, dirs, _ = s.List(ctx, 41, r41, "", nil)
	if notes == nil || dirs == nil {
		t.Fatalf("根级笔记时 dirs 必须是空数组")
	}
	if len(dirs) != 0 || len(notes) != 1 {
		t.Fatalf("根级场景数据异常: notes=%d dirs=%d", len(notes), len(dirs))
	}
}

// 仓库口径（迁移 0012）：fuseRRF 纯语义命中必须携带仓库归属 + filterHits 按命中仓库判定——
// 回归点：曾漏带 repo_id 导致受限仓库经纯语义检索绕过权限（repo 0 无规则=放行）。
func TestSearchRepoAttributionAndFilter(t *testing.T) {
	repoA, repoB := int64(101), int64(102)
	kw := []SearchHit{
		{RepoID: repoA, RepoName: "默认仓库", Path: "ai/kw.md", Title: "kw", Source: "keyword"},
	}
	vec := []VectorHit{
		{NoteID: 1, RepoID: repoB, RepoName: "工作库", Path: "需求/语义.md", Title: "语义", ChunkText: "语义内容", Similarity: 0.8},
		{NoteID: 2, RepoID: repoA, RepoName: "默认仓库", Path: "ai/语义双.md", Title: "双", ChunkText: "双内容", Similarity: 0.7},
	}
	fused := fuseRRF(kw, []int64{2}, vec, 20) // note 2 双命中、note 1 纯语义
	byPath := map[string]SearchHit{}
	for _, h := range fused {
		byPath[h.Path] = h
	}
	// 纯语义命中：仓库归属必须随行
	h1, ok := byPath["需求/语义.md"]
	if !ok {
		t.Fatalf("纯语义命中缺失: %+v", fused)
	}
	if h1.RepoID != repoB || h1.RepoName != "工作库" || h1.Source != "semantic" {
		t.Fatalf("纯语义命中应带仓库归属: %+v", h1)
	}
	// 双命中：hit 以关键词路为准（path=关键词路的 ai/kw.md），仓库归属同样在
	if h2 := byPath["ai/kw.md"]; h2.RepoID != repoA || h2.Source != "both" {
		t.Fatalf("双命中仓库归属异常: %+v", h2)
	}
	// 过滤：View 只对 repoA 的仓库级白名单含 key 7 → repoB 全文（无规则）放行、repoA 白名单内放行
	view := &PermView{keyID: 7, rules: []model.NotePerm{
		{RepoID: repoA, FolderPath: "", Mode: PermModeAllow, KeyIDs: pq.StringArray{"7"}},
	}}
	out := filterHits(fused, view)
	if len(out) != 2 {
		t.Fatalf("两命中分属白名单内仓库与无规则仓库，应全保留: %+v", out)
	}
	// 反例：repoB 配了不含 key 7 的仓库级白名单 → 该命中必须被过滤（防语义绕过）
	view.rules = append(view.rules, model.NotePerm{RepoID: repoB, FolderPath: "", Mode: PermModeAllow, KeyIDs: pq.StringArray{"8"}})
	out = filterHits(fused, view)
	if len(out) != 1 || out[0].RepoID != repoA {
		t.Fatalf("repoB 白名单不含 key7 后应只剩 repoA 命中: %+v", out)
	}
}
