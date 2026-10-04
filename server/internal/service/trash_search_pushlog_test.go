package service

import (
	"context"
	"os"
	"testing"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// 回归覆盖：回收站（列表/来源反查/恢复/彻底删除/清空/保留期）+
// 全域搜索（scope=all 三域）+ 推送记录（留痕/事件聚合/重推更新）。
func TestTrashSearchPushLog(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db := testDB(t)
	ctx := context.Background()
	const uid = int64(31)
	db.Exec("TRUNCATE tf_todo")
	db.Exec("TRUNCATE tf_memo")
	db.Exec("DELETE FROM tf_note WHERE user_id = ?", uid)
	db.Exec("DELETE FROM tf_note_content")
	db.Exec("DELETE FROM tl_api_log WHERE user_id = ?", uid)
	db.Exec("DELETE FROM tl_push_log WHERE user_id = ?", uid)
	db.Exec(`INSERT INTO tu_user (id, phone, password_hash, nick_name) OVERRIDING SYSTEM VALUE
	         VALUES (31, '13900000031', 'x', 't31')
	         ON CONFLICT (id) DO NOTHING`)
	rid := testRepoID(t, db, uid)

	todos := &TodoService{DB: db}
	memos := &MemoService{DB: db}
	notes := &NoteService{DB: db}
	trash := &TrashService{DB: db}
	pushLog := &PushLogService{DB: db}

	// --- 造数据 + 软删 ---
	t1, _ := todos.Create(ctx, uid, TodoCreate{Title: "整理 Supabase RLS", Remark: "含 ops 标签", Tags: []string{"ops"}, Source: model.TodoSourceWeb})
	m1, _ := memos.Create(ctx, uid, MemoCreate{Content: "Supabase pooler 走 IPv4", Tags: []string{"db"}})
	n1, _, _ := notes.Upsert(ctx, uid, rid, "ai/supabase-坑.md", NoteUpsert{Content: "pooler prepared statements 禁用", Tags: []string{"db"}}, nil)
	if err := todos.Delete(ctx, uid, t1.ID); err != nil {
		t.Fatalf("todo delete: %v", err)
	}
	if err := memos.Delete(ctx, uid, m1.ID); err != nil {
		t.Fatalf("memo delete: %v", err)
	}
	if err := notes.Delete(ctx, uid, rid, "ai/supabase-坑.md", nil); err != nil {
		t.Fatalf("note delete: %v", err)
	}

	// --- 来源反查（审计日志伪造：CLI Key=5 删了 todo，Web 删了 memo/note）---
	// client_ip NOT NULL 无默认，缺列 INSERT 会静默失败使来源断言全空——显式补列并检查错误
	mustExec(t, db, `INSERT INTO tl_api_log (user_id, api_key_id, action, target, status_code, client_ip)
	         VALUES (?, 5, 'todo.delete', ?, 200, '9.9.9.9'), (?, 0, 'memo.delete', ?, 200, ''), (?, 0, 'note.delete', ?, 200, '')`,
		uid, itoa(t1.ID), uid, itoa(m1.ID), uid, "ai/supabase-坑.md")
	mustExec(t, db, `INSERT INTO tu_api_key (id, user_id, key_name, key_hash, key_hint, scope) OVERRIDING SYSTEM VALUE
	         VALUES (5, ?, '工作机', 'hash5', 'ak5', 'all') ON CONFLICT (id) DO UPDATE SET key_name = '工作机'`, uid)

	items, counts, err := trash.List(ctx, uid)
	if err != nil {
		t.Fatalf("trash list: %v", err)
	}
	if counts["todo"] != 1 || counts["memo"] != 1 || counts["note"] != 1 {
		t.Fatalf("counts 错: %v", counts)
	}
	if len(items) != 3 {
		t.Fatalf("items 应 3 条，得 %d", len(items))
	}
	byType := map[string]TrashItem{}
	for _, it := range items {
		byType[it.Type] = it
		if it.LeftDays < 0 || it.LeftDays > trashRetentionDays {
			t.Fatalf("left_days 越界: %d", it.LeftDays)
		}
	}
	if src := byType["todo"].Source; src != "CLI · 工作机" {
		t.Fatalf("todo 来源应=CLI · 工作机，得 %q", src)
	}
	if src := byType["memo"].Source; src != "Web" {
		t.Fatalf("memo 来源应=Web，得 %q", src)
	}

	// --- 全域搜索：软删的不出现（三域同语义），活跃的各归各组 ---
	todosHits, err := todos.Search(ctx, uid, "supabase", 20)
	if err != nil {
		t.Fatalf("todo search: %v", err)
	}
	if len(todosHits) != 0 {
		t.Fatalf("软删待办不应命中，得 %d", len(todosHits))
	}
	t2, _ := todos.Create(ctx, uid, TodoCreate{Title: "重启 supabase pooler", Source: model.TodoSourceCLI})
	todosHits, _ = todos.Search(ctx, uid, "supabase", 20)
	if len(todosHits) != 1 || todosHits[0].ID != t2.ID {
		t.Fatalf("todo 搜索应命中 1 条，得 %d", len(todosHits))
	}
	memoHits, _ := memos.Search(ctx, uid, "pooler", 20)
	if len(memoHits) != 0 {
		t.Fatalf("软删便签不应命中，得 %d", len(memoHits))
	}
	m2, _ := memos.Create(ctx, uid, MemoCreate{Content: "pooler 活跃便签", Tags: []string{"db"}})
	memoHits, _ = memos.Search(ctx, uid, "pooler", 20)
	if len(memoHits) != 1 || memoHits[0].ID != m2.ID {
		t.Fatalf("memo 搜索应命中活跃 1 条，得 %d", len(memoHits))
	}
	noteHits, _ := notes.Search(ctx, uid, "supabase", 20, "", nil)
	if len(noteHits) != 0 {
		t.Fatalf("软删笔记不应命中，得 %d", len(noteHits))
	}

	// --- 恢复：todo/memo/note 全部可恢复（笔记无冲突场景）---
	for _, c := range []struct {
		tt model.TrashType
		id int64
	}{{model.TrashTodo, t1.ID}, {model.TrashMemo, m1.ID}, {model.TrashNote, n1.ID}} {
		if err := trash.Restore(ctx, uid, c.tt, c.id); err != nil {
			t.Fatalf("restore %s: %v", c.tt, err)
		}
	}
	if _, counts, _ := trash.List(ctx, uid); counts["todo"]+counts["memo"]+counts["note"] != 0 {
		t.Fatalf("恢复后回收站应空")
	}
	// 恢复后的笔记重新可搜（活跃态），且带正文大小
	noteHits, _ = notes.Search(ctx, uid, "supabase", 20, "", nil)
	if len(noteHits) != 1 || noteHits[0].SizeBytes == 0 || noteHits[0].Path != "ai/supabase-坑.md" {
		t.Fatalf("恢复后 note 搜索应命中 1 条且带大小，得 %+v", noteHits)
	}

	// --- 再删 + 彻底删除一条 ---
	_ = todos.Delete(ctx, uid, t1.ID)
	if err := trash.PurgeItem(ctx, uid, model.TrashTodo, t1.ID); err != nil {
		t.Fatalf("purge item: %v", err)
	}
	var n int64
	db.Model(&model.Todo{}).Where("id = ?", t1.ID).Count(&n)
	if n != 0 {
		t.Fatalf("彻底删除后行应消失")
	}
	if err := trash.PurgeItem(ctx, uid, model.TrashTodo, t1.ID); err == nil {
		t.Fatalf("重复彻底删除应报不存在")
	}

	// --- 清空 ---
	_ = memos.Delete(ctx, uid, m1.ID)
	if err := notes.Delete(ctx, uid, rid, "ai/supabase-坑.md", nil); err != nil {
		t.Fatalf("note 再软删: %v", err)
	}
	out, err := trash.PurgeAll(ctx, uid)
	if err != nil {
		t.Fatalf("purge all: %v", err)
	}
	if out["memo"] != 1 || out["note"] != 1 {
		t.Fatalf("purge all 计数错: %v", out)
	}
	db.Model(&model.NoteContent{}).Where("note_id = ?", n1.ID).Count(&n)
	if n != 0 {
		t.Fatalf("清空后笔记正文应删除")
	}

	// --- 推送记录：留痕 + 事件聚合 + 重推更新 ---
	pushLog.Record(uid, model.NotifyEventDueToday, "due:2026-10-03", model.PushChannelWeb, 9, "今日待办 · 1 项到期", "body", nil, 12)
	pushLog.Record(uid, model.NotifyEventDueToday, "due:2026-10-03", model.PushChannelDingTalk, 3, "今日待办 · 1 项到期", "body", errFake("加签校验失败"), 105)
	events, err := pushLog.List(ctx, uid, PushLogFilter{Days: 7})
	if err != nil {
		t.Fatalf("push log list: %v", err)
	}
	if len(events) != 1 || len(events[0].Channels) != 2 {
		t.Fatalf("事件聚合应 1 事件 2 渠道，得 %+v", events)
	}
	// 两条 Record 同毫秒落库，create_time DESC 排序不稳定——按渠道类型定位失败行，不按索引
	var failRow *PushChannelHit
	for i := range events[0].Channels {
		if events[0].Channels[i].ChannelType == string(model.PushChannelDingTalk) {
			failRow = &events[0].Channels[i]
		}
	}
	if failRow == nil || failRow.Error == "" {
		t.Fatalf("失败渠道应带原因，得 %+v", events[0].Channels)
	}
	stats, _ := pushLog.StatsToday(ctx, uid)
	if stats.Events != 1 || stats.Deliveries != 2 || stats.Fails != 1 {
		t.Fatalf("今日统计错: %+v", stats)
	}
	row, err := pushLog.Get(ctx, uid, failRow.ID)
	if err != nil {
		t.Fatalf("get log: %v", err)
	}
	pushLog.MarkRetry(row, errFake("仍失败: NTP 偏移"), 200)
	row2, _ := pushLog.Get(ctx, uid, row.ID)
	if row2.RetryCount != 1 || row2.Status != string(model.PushStatusFail) {
		t.Fatalf("重推应原地更新 retry=1 fail，得 %+v", row2)
	}

	// --- 保留期清理 ---
	// m1 已被 PurgeAll 物理删除，对仍存活的 m2（活跃，未进 PurgeAll 集合）做超期软删标记
	mustExec(t, db, `UPDATE tf_memo SET is_deleted = 1, update_time = ? WHERE id = ?`, time.Now().AddDate(0, 0, -40), m2.ID)
	deleted, err := trash.CleanupExpired(ctx, 30)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted["memo"] != 1 {
		t.Fatalf("超期清理应删 1 条 memo，得 %v", deleted)
	}
	if nDel, _ := pushLog.CleanupRetention(ctx, 0); nDel < 2 {
		t.Fatalf("0 天保留应清光推送记录，得 %d", nDel)
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

// mustExec 测试里的写操作失败必须立刻暴露（吞错会让断言建立在假数据上）
func mustExec(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
