package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"extbrain-server/internal/model"
)

// v3 待办行为回归：二态状态 / completed_at 流转 / counts / 排序（created|due）/
// 排序偏好按用户持久化 / tags / clear_due / restore。
func TestTodoV3Flow(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_todo")
	// 排序偏好落在 tu_user：种子用户 21（不存在则建，存在则重置偏好）
	db.Exec(`INSERT INTO tu_user (id, phone, password_hash, nick_name) OVERRIDING SYSTEM VALUE
	         VALUES (21, '13900000021', 'x', 't21')
	         ON CONFLICT (id) DO UPDATE SET todo_sort = 'created'`)
	s := &TodoService{DB: db}

	// --- tags 更新（曾出现 500：map update 裸 []string 写 text[]）---
	t1, err := s.Create(ctx, 21, TodoCreate{Title: "A", Tags: []string{"dev"}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	got, err := s.Update(ctx, 21, t1.ID, TodoUpdate{Tags: []string{"工作", "周报"}})
	if err != nil {
		t.Fatalf("tags 更新失败（回归：曾 500）: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "工作" {
		t.Fatalf("tags 更新异常: %v", got.Tags)
	}

	// --- clear_due：显式清除截止时间 ---
	due := time.Now().Add(2 * time.Hour)
	t2, _ := s.Create(ctx, 21, TodoCreate{Title: "B", DueTime: &due})
	time.Sleep(2 * time.Millisecond)
	got, err = s.Update(ctx, 21, t2.ID, TodoUpdate{ClearDue: true})
	if err != nil {
		t.Fatalf("clear_due 失败: %v", err)
	}
	if got.DueTime != nil {
		t.Fatalf("due 应被清除: %v", got.DueTime)
	}
	due2 := time.Now().Add(3 * time.Hour)
	got, _ = s.Update(ctx, 21, t2.ID, TodoUpdate{DueTime: &due2})
	if got.DueTime == nil {
		t.Fatal("due 应被设置")
	}
	t3, _ := s.Create(ctx, 21, TodoCreate{Title: "C"})

	// --- 新建即在途；completed_at 跟随 done 流转 ---
	if t1.Status != model.TodoStatusActive {
		t.Fatalf("新条目应为 active: %s", t1.Status)
	}
	if t3.CompletedAt != nil {
		t.Fatal("新条目 completed_at 应为空")
	}
	done := model.TodoStatusDone
	active := model.TodoStatusActive
	got, _ = s.Update(ctx, 21, t3.ID, TodoUpdate{Status: &done})
	if got.CompletedAt == nil {
		t.Fatal("done 应写入 completed_at")
	}
	got, _ = s.Update(ctx, 21, t3.ID, TodoUpdate{Status: &active})
	if got.CompletedAt != nil {
		t.Fatal("离开 done 应清空 completed_at")
	}

	// --- 非法状态拒绝（doing/todo 已废弃）---
	if _, err := s.Update(ctx, 21, t1.ID, TodoUpdate{Status: ptrStatus("doing")}); err == nil {
		t.Fatal("doing 应被拒绝")
	}

	// --- counts：二态全量统计 ---
	counts, err := s.Counts(ctx, 21)
	if err != nil {
		t.Fatalf("counts 失败: %v", err)
	}
	if counts.Active != 3 || counts.Done != 0 {
		t.Fatalf("counts 异常: %+v", counts)
	}

	// --- sort=created：创建时间倒序 ---
	list, counts2, err := s.List(ctx, 21, TodoFilter{Sort: "created"})
	if err != nil {
		t.Fatalf("list 失败: %v", err)
	}
	if counts2.Active != 3 {
		t.Fatalf("List 应带 counts: %+v", counts2)
	}
	if len(list) != 3 || list[0].ID != t3.ID || list[2].ID != t1.ID {
		t.Fatalf("created 排序应最新在前: %v", listIDs(list))
	}

	// --- sort=due：截止升序在前，无截止按新在前在后 ---
	list, _, _ = s.List(ctx, 21, TodoFilter{Sort: "due"})
	if list[0].ID != t2.ID {
		t.Fatalf("due 排序：有 due 的应排最前，got %d", list[0].ID)
	}
	if list[1].ID != t3.ID || list[2].ID != t1.ID {
		t.Fatalf("due 排序（无 due 新在前）异常: %v", listIDs(list))
	}

	// --- 排序偏好按用户存库（service 层：读写与校验；List 缺省取偏好在 handler 层，HTTP 冒烟覆盖）---
	if err := s.SetSortPref(ctx, 21, "due"); err != nil {
		t.Fatalf("SetSortPref 失败: %v", err)
	}
	if pref, _ := s.SortPref(ctx, 21); pref != "due" {
		t.Fatalf("偏好应持久化: %q", pref)
	}
	if err := s.SetSortPref(ctx, 21, "smart"); err == nil {
		t.Fatal("非法偏好应被拒绝")
	} else {
		var ue *UserError
		if !errors.As(err, &ue) {
			t.Fatalf("非法偏好应返回 UserError: %T", err)
		}
	}
	if pref, _ := s.SortPref(ctx, 21); pref != "due" {
		t.Fatalf("非法请求不应改写偏好: %q", pref)
	}

	// --- 完成 → 已完成视图；restore：撤销删除 ---
	got, _ = s.Update(ctx, 21, t3.ID, TodoUpdate{Status: &done})
	time.Sleep(2 * time.Millisecond)
	got, _ = s.Update(ctx, 21, t1.ID, TodoUpdate{Status: &done})
	_ = got
	doneList, _, _ := s.List(ctx, 21, TodoFilter{Status: model.TodoStatusDone, Sort: "due"})
	if len(doneList) != 2 {
		t.Fatalf("done 应 2 条: %d", len(doneList))
	}
	if err := s.Delete(ctx, 21, t2.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := s.Restore(ctx, 21, t2.ID); err != nil {
		t.Fatalf("restore 失败: %v", err)
	}
	after, _, _ := s.List(ctx, 21, TodoFilter{})
	if len(after) != 3 {
		t.Fatalf("restore 后应 3 条: %d", len(after))
	}
	if _, err := s.Restore(ctx, 22, t2.ID); !errors.Is(err, ErrTodoNotFound) {
		t.Fatalf("跨用户 restore 应 Not Found: %v", err)
	}
	if _, err := s.Restore(ctx, 21, t2.ID); !errors.Is(err, ErrTodoNotFound) {
		t.Fatalf("未删除条目 restore 应 Not Found: %v", err)
	}
}

func ptrStatus(s model.TodoStatus) *model.TodoStatus { return &s }

func listIDs(ts []model.Todo) []int64 {
	out := make([]int64, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}
