package service

import (
	"context"
	"errors"
	"os"
	"testing"
)

// 目录权限矩阵（迁移 0012 仓库化）：最深前缀优先（仓库规则→目录规则）/ 白名单 / 读写同权 /
// 搜索与列表过滤 / 规则 CRUD 校验 / 规则跨仓库隔离。
func TestNotePermMatrix(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_note_perm, tu_api_key, tf_repo")

	notes := &NoteService{DB: db}
	perm := &NotePermService{DB: db}
	keys := &APIKeyService{DB: db}
	repos := &RepoService{DB: db}

	uid := int64(301)
	rid, err := repos.DefaultID(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := repos.Create(ctx, uid, "第二仓库", "")
	if err != nil {
		t.Fatal(err)
	}
	seed := map[string]string{
		"根级.md":                  "根级内容",
		"research/公开综述.md":       "综述内容",
		"research/papers/深论文.md": "论文内容",
		"finance/账目.md":          "账目内容",
	}
	for p, c := range seed {
		if _, _, err := notes.Upsert(ctx, uid, rid, p, NoteUpsert{Content: c}, nil); err != nil {
			t.Fatalf("种子笔记 %s: %v", p, err)
		}
	}
	if _, _, err := notes.Upsert(ctx, uid, r2.ID, "research/另一仓库综述.md", NoteUpsert{Content: "第二仓库综述"}, nil); err != nil {
		t.Fatal(err)
	}

	k1, raw1, err := keys.Issue(ctx, uid, IssueInput{Name: "K1"})
	if err != nil {
		t.Fatal(err)
	}
	k2, raw2, err := keys.Issue(ctx, uid, IssueInput{Name: "K2"})
	if err != nil {
		t.Fatal(err)
	}
	k3, raw3, err := keys.Issue(ctx, uid, IssueInput{Name: "K3"})
	if err != nil {
		t.Fatal(err)
	}

	// 规则（默认仓库）：仓库级''=[K1]；research/=[K1,K2]；finance/=空（完全封闭）
	put := func(repoID int64, folder string, ids []int64) {
		if err := perm.Put(ctx, uid, NotePermInput{RepoID: repoID, FolderPath: folder, Mode: PermModeAllow, KeyIDs: ids}); err != nil {
			t.Fatalf("Put %d/%s: %v", repoID, folder, err)
		}
	}
	put(rid, "", []int64{k1.ID})
	put(rid, "research", []int64{k1.ID, k2.ID})
	put(rid, "finance", nil)

	viewOf := func(raw string) *PermView {
		keyID := int64(0)
		switch raw {
		case raw1:
			keyID = k1.ID
		case raw2:
			keyID = k2.ID
		case raw3:
			keyID = k3.ID
		}
		v, err := perm.View(ctx, uid, keyID)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v1, v2, v3 := viewOf(raw1), viewOf(raw2), viewOf(raw3)

	// —— Allowed 矩阵（最深前缀优先；仓库规则=folder_path ''）——
	cases := []struct {
		name string
		v    *PermView
		repo int64
		path string
		want bool
	}{
		{"K1 根级", v1, rid, "根级.md", true},
		{"K1 research", v1, rid, "research/公开综述.md", true},
		{"K1 finance（空白名单=对所有人封闭，最深规则优先）", v1, rid, "finance/账目.md", false},
		{"K2 根级（仓库白名单无 K2）", v2, rid, "根级.md", false},
		{"K2 research（直接规则）", v2, rid, "research/公开综述.md", true},
		{"K2 research/papers（继承最深 research/ 规则）", v2, rid, "research/papers/深论文.md", true},
		{"K2 finance（空白名单=封闭）", v2, rid, "finance/账目.md", false},
		{"K3 有真 Key 但不在任何白名单（仓库规则已配置→全仓不可见）", v3, rid, "根级.md", false},
		{"K3 research 不可见", v3, rid, "research/公开综述.md", false},
		{"非法路径一律拒绝", v1, rid, "../etc/passwd", false},
		// 跨仓库隔离：第二仓库无任何规则=全开放，A 仓库规则不影响 B 仓库
		{"K3 第二仓库不受默认仓库规则影响", v3, r2.ID, "research/另一仓库综述.md", true},
		{"K1 第二仓库开放", v1, r2.ID, "research/另一仓库综述.md", true},
	}
	for _, c := range cases {
		if got := c.v.Allowed(c.repo, c.path); got != c.want {
			t.Errorf("%s: Allowed(%d,%s)=%v want %v", c.name, c.repo, c.path, got, c.want)
		}
	}

	// —— 读写同权：不可见写/删/恢复均拒绝 ——
	if _, _, err := notes.Upsert(ctx, uid, rid, "根级.md", NoteUpsert{Content: "覆盖"}, v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 覆盖根级应 ErrNoteDenied，得 %v", err)
	}
	if _, err := notes.Get(ctx, uid, rid, "finance/账目.md", v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 读 finance 应 ErrNoteDenied，得 %v", err)
	}
	if err := notes.Delete(ctx, uid, rid, "根级.md", v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 删根级应 ErrNoteDenied，得 %v", err)
	}
	if _, err := notes.Move(ctx, uid, rid, "根级.md", rid, "research/挪入.md", v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 从根级移出应 ErrNoteDenied，得 %v", err)
	}
	if _, err := notes.Move(ctx, uid, rid, "research/公开综述.md", rid, "finance/挪入.md", v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 挪入封闭目录应 ErrNoteDenied，得 %v", err)
	}
	// 跨仓库移动到无规则仓库应放行（K2 对 r2 全开放）
	if _, err := notes.Move(ctx, uid, rid, "research/公开综述.md", r2.ID, "搬入.md", v2); err != nil {
		t.Fatalf("K2 跨仓库移到开放仓库应放行: %v", err)
	}
	// 允许侧：K2 在 research/ 内读写正常
	if _, _, err := notes.Upsert(ctx, uid, rid, "research/papers/新论文.md", NoteUpsert{Content: "新"}, v2); err != nil {
		t.Fatalf("K2 写 research 应放行: %v", err)
	}

	// —— List：受限笔记行 + 目录名都不泄露 ——
	rootNotes, rootDirs, err := notes.List(ctx, uid, rid, "", v2)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range rootNotes {
		if !v2.Allowed(n.RepoID, n.Path) {
			t.Fatalf("List 泄露受限笔记: %s", n.Path)
		}
	}
	for _, d := range rootDirs {
		if d == "finance" {
			t.Fatal("List 目录聚合泄露 finance（封闭目录名不可见）")
		}
	}
	fNotes, fDirs, err := notes.List(ctx, uid, rid, "finance", v2)
	if err != nil {
		t.Fatal(err)
	}
	if len(fNotes) != 0 || len(fDirs) != 0 {
		t.Fatalf("K2 列 finance 应为空: notes=%d dirs=%v", len(fNotes), fDirs)
	}

	// —— Search：关键词路统一后过滤（命中带仓库，按命中仓库判定）——
	kwHits, err := notes.Search(ctx, uid, "账目", 20, "keyword", v2)
	if err != nil {
		t.Fatal(err)
	}
	if len(kwHits) != 0 {
		t.Fatalf("K2 搜索不应命中 finance: %+v", kwHits)
	}
	// 注：上一节已把 research/公开综述.md 跨仓库移动为第二仓库的 搬入.md，
	// 所以「综述」的两处命中（搬入.md + 另一仓库综述.md）都在第二仓库。
	kwHits1, err := notes.Search(ctx, uid, "综述", 20, "keyword", v2)
	if err != nil || len(kwHits1) != 2 {
		t.Fatalf("K2 搜索综述应命中 2 条: %d err=%v", len(kwHits1), err)
	}
	for _, h := range kwHits1 {
		if h.RepoName == "" {
			t.Fatal("搜索命中应带仓库名")
		}
		if h.RepoName != "第二仓库" {
			t.Fatalf("综述命中应都在第二仓库: %+v", h)
		}
	}
	// 论文（research/papers/深论文.md + 新论文.md）仍在默认仓库且对 K2 可见
	kwPaper, err := notes.Search(ctx, uid, "论文", 20, "keyword", v2)
	if err != nil || len(kwPaper) != 2 {
		t.Fatalf("K2 搜索论文应命中 2 条: %d err=%v", len(kwPaper), err)
	}
	for _, h := range kwPaper {
		if h.RepoName != "默认仓库" {
			t.Fatalf("论文命中应在默认仓库: %+v", h)
		}
	}

	// —— Restore：不可见目录的软删笔记不可恢复 ——
	if err := notes.Delete(ctx, uid, rid, "finance/账目.md", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Restore(ctx, uid, rid, "finance/账目.md", v2); !errors.Is(err, ErrNoteDenied) {
		t.Fatalf("K2 恢复 finance 应 ErrNoteDenied，得 %v", err)
	}
	if _, err := notes.Restore(ctx, uid, rid, "finance/账目.md", nil); err != nil {
		t.Fatalf("本人（不受限）恢复应成功: %v", err)
	}

	// —— Put 校验：吊销 Key / open 删规则 / 仓库归属 ——
	if err := keys.Revoke(ctx, uid, k2.ID); err != nil {
		t.Fatal(err)
	}
	if err := perm.Put(ctx, uid, NotePermInput{RepoID: rid, FolderPath: "research", Mode: PermModeAllow, KeyIDs: []int64{k2.ID}}); err == nil {
		t.Fatal("吊销 Key 应被拒绝")
	}
	if err := perm.Put(ctx, uid, NotePermInput{RepoID: 99999, FolderPath: "", Mode: PermModeAllow, KeyIDs: []int64{k1.ID}}); err == nil {
		t.Fatal("规则挂到不存在的仓库应被拒绝")
	}
	if err := perm.Put(ctx, uid, NotePermInput{RepoID: rid, FolderPath: "research", Mode: PermModeOpen}); err != nil {
		t.Fatal(err)
	}
	rules, err := perm.List(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rules {
		if r.RepoID == rid && r.FolderPath == "research" {
			t.Fatal("mode=open 应删除 research 规则")
		}
	}
	// open 后 K2 经仓库级规则判定：''=[K1] → 仍不可见 research
	if v2again, _ := perm.View(ctx, uid, k2.ID); v2again.Allowed(rid, "research/公开综述.md") {
		t.Fatal("规则删除后 K2 应回落仓库级规则（不可见）")
	}

	// —— 跨用户：规则不跨用户 ——
	other := int64(302)
	otherRid, err := repos.DefaultID(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := notes.Upsert(ctx, other, otherRid, "other/笔记.md", NoteUpsert{Content: "x"}, nil); err != nil {
		t.Fatal(err)
	}
	vOther, err := perm.View(ctx, other, k1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !vOther.Allowed(otherRid, "other/笔记.md") {
		t.Fatal("其他用户不受 A 的规则影响")
	}

	_ = v3
}
