package service

import (
	"context"
	"testing"
	"time"

	"extbrain-server/internal/model"
)

// 笔记分享：创建 upsert（token 保留）/ 公开读取（snapshot 冻结 · live 跟随）/ 过期 / 撤销 / 计数
func TestNoteShareFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_note_share")
	s := &ShareService{DB: db, Notes: &NoteService{DB: db}}
	uid := int64(51)
	rid := testRepoID(t, db, uid)

	mk, _, err := s.Notes.Upsert(ctx, uid, rid, "分享/测试笔记.md", NoteUpsert{Content: "v1 内容", Title: "测试笔记", Tags: []string{"分享"}}, nil)
	if err != nil {
		t.Fatalf("建笔记失败: %v", err)
	}

	// 1. 创建：默认参数 + 快照冻结
	sh, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", model.ShareModeSnapshot, 30, nil)
	if err != nil || sh.Token == "" || len(sh.Token) != 32 {
		t.Fatalf("创建分享失败: %v %+v", err, sh)
	}
	if sh.Title != "测试笔记" || sh.Content != "v1 内容" || sh.Mode != model.ShareModeSnapshot {
		t.Fatalf("快照字段异常: %+v", sh)
	}
	if sh.ExpireTime == nil || !sh.ExpireTime.After(time.Now()) {
		t.Fatalf("30 天过期应写入: %v", sh.ExpireTime)
	}
	tok1 := sh.Token

	// 2. 公开读取：内容=快照 + 计数
	v, err := s.PublicGet(ctx, tok1)
	if err != nil || v.Content != "v1 内容" || v.Title != "测试笔记" {
		t.Fatalf("公开读取失败: %v %+v", err, v)
	}
	// 3. 源笔记更新 → snapshot 不变
	s.Notes.Upsert(ctx, uid, rid, "分享/测试笔记.md", NoteUpsert{Content: "v2 内容"}, nil)
	v, _ = s.PublicGet(ctx, tok1)
	if v.Content != "v1 内容" {
		t.Fatalf("snapshot 应冻结: %q", v.Content)
	}
	// 4. 再次分享 = upsert：保留 token + 重冻快照
	sh2, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", model.ShareModeLive, 0, nil)
	if err != nil {
		t.Fatalf("更新分享失败: %v", err)
	}
	if sh2.Token != tok1 {
		t.Fatalf("token 应保留: %s != %s", sh2.Token, tok1)
	}
	if sh2.ExpireTime != nil {
		t.Fatalf("改永久应清过期: %v", sh2.ExpireTime)
	}
	// 5. live：跟随源笔记
	v, err = s.PublicGet(ctx, tok1)
	if err != nil || v.Content != "v2 内容" || v.Mode != model.ShareModeLive {
		t.Fatalf("live 应跟随: %v %+v", err, v)
	}
	// 6. live 源笔记删除 → 失效（source_deleted）
	s.Notes.Delete(ctx, uid, rid, "分享/测试笔记.md", nil)
	if _, err = s.PublicGet(ctx, tok1); err != ErrShareSourceDeleted {
		t.Fatalf("live 源删除应失效: %v", err)
	}
	s.Notes.Restore(ctx, uid, rid, "分享/测试笔记.md", nil)

	// 7. owner 查询（按路径）
	got, err := s.GetByPath(ctx, uid, rid, "分享/测试笔记.md")
	if err != nil || got == nil || got.Token != tok1 {
		t.Fatalf("owner 查询: %v %+v", err, got)
	}
	// 8. 撤销 → 公开 404 语义；owner 查询为 nil；可重建（新 token）
	if err := s.Revoke(ctx, uid, tok1); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}
	if err := s.Revoke(ctx, uid, tok1); err != ErrShareNotFound {
		t.Fatalf("重复撤销应 not found: %v", err)
	}
	if _, err := s.PublicGet(ctx, tok1); err != ErrShareNotFound {
		t.Fatalf("撤销后公开读取应失效: %v", err)
	}
	got, _ = s.GetByPath(ctx, uid, rid, "分享/测试笔记.md")
	if got != nil {
		t.Fatalf("撤销后 owner 查询应 nil: %+v", got)
	}
	sh3, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", model.ShareModeSnapshot, 0, nil)
	if err != nil || sh3.Token == tok1 {
		t.Fatalf("撤销后重建应新 token: %v %+v", err, sh3)
	}

	// 9. 过期：手工置过期 → expired
	past := time.Now().Add(-time.Minute)
	db.Model(&model.NoteShare{}).Where("token = ?", sh3.Token).UpdateColumn("expire_time", past)
	if _, err := s.PublicGet(ctx, sh3.Token); err != ErrShareExpired {
		t.Fatalf("过期应失效: %v", err)
	}

	// 10. 参数校验 + 跨用户隔离
	if _, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", "bad", 0, nil); err == nil {
		t.Fatal("非法 mode 应报错")
	}
	if _, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", model.ShareModeSnapshot, 99999, nil); err == nil {
		t.Fatal("超限 expire_days 应报错")
	}
	if _, err := s.Upsert(ctx, uid, rid, "分享/不存在.md", model.ShareModeSnapshot, 0, nil); err != ErrNoteNotFound {
		t.Fatalf("不存在的笔记应报错: %v", err)
	}
	if err := s.Revoke(ctx, uid+1, sh3.Token); err != ErrShareNotFound {
		t.Fatalf("跨用户撤销应拒绝: %v", err)
	}
	if got, _ := s.GetByPath(ctx, uid+1, testRepoID(t, db, uid+1), "分享/测试笔记.md"); got != nil {
		t.Fatalf("跨用户查询应 nil: %+v", got)
	}

	// 11. 一笔记一活跃分享：清表重建后，直接插第二条活跃行应撞唯一索引
	db.Exec("TRUNCATE tf_note_share")
	if _, err := s.Upsert(ctx, uid, rid, "分享/测试笔记.md", model.ShareModeSnapshot, 0, nil); err != nil {
		t.Fatalf("重建失败: %v", err)
	}
	dup := model.NoteShare{UserID: uid, NoteID: mk.ID, Token: "dup" + tok1, Mode: model.ShareModeSnapshot}
	if db.Create(&dup).Error == nil { //nolint:revive // 断言部分唯一索引生效
		t.Fatal("部分唯一索引应拦截第二条活跃分享")
	}

	// 12. 计数：公开读 3 次 → view_count 恰好 +3
	var active string
	db.Model(&model.NoteShare{}).Select("token").Where("user_id = ? AND is_revoked = 0", uid).Scan(&active)
	var before int64
	db.Model(&model.NoteShare{}).Select("view_count").Where("token = ?", active).Scan(&before)
	for i := 0; i < 3; i++ {
		if _, err := s.PublicGet(ctx, active); err != nil {
			t.Fatalf("读取失败: %v", err)
		}
	}
	var after int64
	db.Model(&model.NoteShare{}).Select("view_count").Where("token = ?", active).Scan(&after)
	if after != before+3 {
		t.Fatalf("计数异常: before=%d after=%d", before, after)
	}
}

// 分享复制到知识库：跨用户 / 防冲突（含软删占位）/ 标题清洗 / live 最新 / 失效三分 / 自复制
func TestNoteShareCopy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	// 12.5 复制到知识库：跨用户 / 防冲突 / 标题清洗 / live 最新 / 失效三分
	t.Run("CopyToLibrary", func(t *testing.T) {
		db.Exec("TRUNCATE tf_note, tf_note_content, tf_note_share")
		notes := &NoteService{DB: db}
		ss := &ShareService{DB: db, Notes: notes}
		owner, other := int64(61), int64(62)
		ridO := testRepoID(t, db, owner)
		ridT := testRepoID(t, db, other)

		notes.Upsert(ctx, owner, ridO, "复制/原文.md", NoteUpsert{Content: "v1", Title: "带/斜杠的标题", Tags: []string{"t1"}}, nil)
		sh, err := ss.Upsert(ctx, owner, ridO, "复制/原文.md", model.ShareModeSnapshot, 0, nil)
		if err != nil {
			t.Fatalf("建分享失败: %v", err)
		}
		// 复制到 other 的库：路径=标题清洗（/ → -）+ 内容/标签一致
		n1, err := ss.CopyToLibrary(ctx, other, ridT, sh.Token, nil)
		if err != nil {
			t.Fatalf("复制失败: %v", err)
		}
		if n1.Path != "带-斜杠的标题.md" || n1.UserID != other {
			t.Fatalf("复制路径/归属异常: %+v", n1)
		}
		got, _ := notes.Get(ctx, other, ridT, n1.Path, nil)
		if got.Content != "v1" || len(got.Tags) != 1 || got.Tags[0] != "t1" {
			t.Fatalf("复制内容异常: %+v", got)
		}
		// 再复制：自动 -2
		n2, err := ss.CopyToLibrary(ctx, other, ridT, sh.Token, nil)
		if err != nil || n2.Path != "带-斜杠的标题-2.md" {
			t.Fatalf("防冲突后缀异常: %v %+v", err, n2)
		}
		// 软删占位也应绕开（不复活覆盖）
		notes.Delete(ctx, other, ridT, "带-斜杠的标题-3.md", nil)
		notes.Upsert(ctx, other, ridT, "带-斜杠的标题-3.md", NoteUpsert{Content: "occupied"}, nil)
		notes.Delete(ctx, other, ridT, "带-斜杠的标题-3.md", nil)
		n3, err := ss.CopyToLibrary(ctx, other, ridT, sh.Token, nil)
		if err != nil || n3.Path != "带-斜杠的标题-4.md" {
			t.Fatalf("软删占位应绕开: %v %+v", err, n3)
		}
		// live 复制=当前最新内容
		notes.Upsert(ctx, owner, ridO, "复制/原文.md", NoteUpsert{Content: "v2 最新"}, nil)
		sh2, _ := ss.Upsert(ctx, owner, ridO, "复制/原文.md", model.ShareModeLive, 0, nil)
		n4, err := ss.CopyToLibrary(ctx, other, ridT, sh2.Token, nil)
		if err != nil {
			t.Fatalf("live 复制失败: %v", err)
		}
		got4, _ := notes.Get(ctx, other, ridT, n4.Path, nil)
		if got4.Content != "v2 最新" {
			t.Fatalf("live 应复制最新: %q", got4.Content)
		}
		// 失效三分：撤销 / 过期 / live 源删
		ss.Revoke(ctx, owner, sh2.Token)
		if _, err := ss.CopyToLibrary(ctx, other, ridT, sh2.Token, nil); err != ErrShareNotFound {
			t.Fatalf("撤销后复制应 not found: %v", err)
		}
		sh3, _ := ss.Upsert(ctx, owner, ridO, "复制/原文.md", model.ShareModeSnapshot, 0, nil)
		past := time.Now().Add(-time.Minute)
		db.Model(&model.NoteShare{}).Where("token = ?", sh3.Token).UpdateColumn("expire_time", past)
		if _, err := ss.CopyToLibrary(ctx, other, ridT, sh3.Token, nil); err != ErrShareExpired {
			t.Fatalf("过期复制应 expired: %v", err)
		}
		sh4, _ := ss.Upsert(ctx, owner, ridO, "复制/原文.md", model.ShareModeLive, 0, nil)
		notes.Delete(ctx, owner, ridO, "复制/原文.md", nil)
		if _, err := ss.CopyToLibrary(ctx, other, ridT, sh4.Token, nil); err != ErrShareSourceDeleted {
			t.Fatalf("live 源删复制应 source_deleted: %v", err)
		}
		// 自己复制自己：owner 已有同名原文 → 自动后缀（不覆盖）
		notes.Restore(ctx, owner, ridO, "复制/原文.md", nil)
		sh5, _ := ss.Upsert(ctx, owner, ridO, "复制/原文.md", model.ShareModeSnapshot, 0, nil)
		n5, err := ss.CopyToLibrary(ctx, owner, ridO, sh5.Token, nil)
		if err != nil || n5.Path != "原文.md" {
			// sh5 的 title 来自最近一次 Upsert 的缺省（path.Base=原文.md）→ 剥扩展名后根级新建
			t.Fatalf("自复制异常: %v %+v", err, n5)
		}
	})
}
