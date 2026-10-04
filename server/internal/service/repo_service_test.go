package service

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// testRepoID 确保 userID 有默认仓库并返回 id（仓库化后 notes 相关测试的固定前置）。
func testRepoID(t *testing.T, db *gorm.DB, userID int64) int64 {
	t.Helper()
	id, err := (&RepoService{DB: db}).DefaultID(context.Background(), userID)
	if err != nil {
		t.Fatalf("默认仓库创建失败: %v", err)
	}
	return id
}

// 仓库服务：CRUD / 默认仓库 / 名称唯一 / 数量配额 / 删除守卫 / 跨仓库寻址解析。
func TestRepoServiceFlow(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_note_perm, tf_repo")
	sys := NewSysConfig(db)
	s := &RepoService{DB: db, Sys: sys}
	notes := &NoteService{DB: db}
	uid := int64(401)

	// 默认仓库：懒建幂等
	d1, err := s.DefaultID(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	d2, _ := s.DefaultID(ctx, uid)
	if d1 != d2 {
		t.Fatal("DefaultID 应幂等")
	}

	// 建仓 + 配额（默认 3，不含默认仓库）
	r1, err := s.Create(ctx, uid, "工作库", "公司资料")
	if err != nil || r1.IsDefault != 0 {
		t.Fatalf("建仓: %v %+v", err, r1)
	}
	if _, err := s.Create(ctx, uid, "学习库", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, uid, "临时收集", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, uid, "超额仓库", ""); err == nil {
		t.Fatal("超过默认配额（3）应被拒绝")
	} else {
		if ue, ok := err.(*UserError); !ok || !strings.Contains(ue.Msg, "上限") {
			t.Fatalf("配额错误文案异常: %v", err)
		}
	}
	// 配额可在库中调（repo.quota）：调到 5 后第 4 个可建；显式 0=不限制
	if err := sys.Set(ctx, map[string]string{CfgRepoQuota: "5"}); err != nil {
		t.Fatal(err)
	}
	r4, err := s.Create(ctx, uid, "超额仓库", "")
	if err != nil {
		t.Fatalf("配额调到 5 后应可建第 4 个: %v", err)
	}
	if err := sys.Set(ctx, map[string]string{CfgRepoQuota: "0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, uid, "不限制仓", ""); err != nil {
		t.Fatalf("显式 0 应不限制: %v", err)
	}
	_ = r4

	// 名称唯一（活跃）：软删释放名字
	if _, err := s.Create(ctx, uid, "工作库", ""); err == nil {
		t.Fatal("重名应拒绝")
	}
	if err := s.Delete(ctx, uid, r1.ID); err != nil {
		t.Fatalf("删空仓库应成功: %v", err)
	}
	if _, err := s.Create(ctx, uid, "工作库", "重建"); err != nil {
		t.Fatalf("软删后同名应可重建: %v", err)
	}
	r1b, _ := s.ResolveByName(ctx, uid, "工作库")
	if r1b.ID == r1.ID {
		t.Fatal("重建后应为新行")
	}

	// 改名 + 默认仓库可改名
	if _, err := s.Update(ctx, uid, r1b.ID, strPtr("项目库"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, uid, d1, strPtr("我的默认库"), nil); err != nil {
		t.Fatalf("默认仓库应可改名: %v", err)
	}
	if _, err := s.Update(ctx, uid, r1b.ID, strPtr("我的默认库"), nil); err == nil {
		t.Fatal("改名撞已有名应拒绝")
	}
	if _, err := s.Update(ctx, uid, r1b.ID, strPtr("含:冒号"), nil); err == nil {
		t.Fatal("仓库名含冒号应拒绝（跨仓库寻址分隔符）")
	}

	// 删除守卫：默认仓库不可删；非空仓库禁删
	if err := s.Delete(ctx, uid, d1); err == nil {
		t.Fatal("默认仓库不可删")
	}
	if _, _, err := notes.Upsert(ctx, uid, r1b.ID, "需求/仓库化.md", NoteUpsert{Content: "x"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, uid, r1b.ID); err == nil {
		t.Fatal("非空仓库应禁删")
	} else if ue, ok := err.(*UserError); !ok || !strings.Contains(ue.Msg, "篇笔记") {
		t.Fatalf("非空禁删文案异常: %v", err)
	}
	// 清空后可删
	if err := notes.Delete(ctx, uid, r1b.ID, "需求/仓库化.md", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, uid, r1b.ID); err != nil {
		t.Fatalf("清空后应可删: %v", err)
	}

	// 列表 + 笔记数
	views, err := s.List(ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) < 2 || views[0].IsDefault != 1 {
		t.Fatalf("列表应默认在前: %+v", views)
	}

	// 跨仓库寻址解析：名称命中即跨仓库；未命中=路径本身含冒号
	id, rest, ok, err := s.SplitCrossRepo(ctx, uid, "学习库:需求/a.md")
	if err != nil || !ok {
		t.Fatalf("应识别跨仓库形态: %v ok=%v", err, ok)
	}
	lr, _ := s.ResolveByName(ctx, uid, "学习库")
	if id != lr.ID || rest != "需求/a.md" {
		t.Fatalf("解析错: id=%d rest=%q", id, rest)
	}
	if _, _, ok, _ := s.SplitCrossRepo(ctx, uid, "不存在仓:a.md"); ok {
		t.Fatal("前缀非仓库名应按普通路径处理")
	}

	// 跨仓库移动（同路径在两个仓库共存合法）
	if _, _, err := notes.Upsert(ctx, uid, d1, "ai/搬家.md", NoteUpsert{Content: "v1"}, nil); err != nil {
		t.Fatal(err)
	}
	n2repo, err := s.Create(ctx, uid, "搬入仓", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Move(ctx, uid, d1, "ai/搬家.md", n2repo.ID, "ai/搬家.md", nil); err != nil {
		t.Fatalf("跨仓库移动应成功: %v", err)
	}
	if _, err := notes.Get(ctx, uid, d1, "ai/搬家.md", nil); err != ErrNoteNotFound {
		t.Fatal("原仓库应查不到")
	}
	if _, err := notes.Get(ctx, uid, n2repo.ID, "ai/搬家.md", nil); err != nil {
		t.Fatalf("新仓库应可读: %v", err)
	}

	// 同名笔记在不同仓库共存（uk 到 repo 维度）
	if _, _, err := notes.Upsert(ctx, uid, d1, "ai/搬家.md", NoteUpsert{Content: "重建"}, nil); err != nil {
		t.Fatalf("同路径在不同仓库应可共存: %v", err)
	}
}
