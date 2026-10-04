package service

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"extbrain-server/internal/model"
)

func TestTodoFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_todo")
	s := &TodoService{DB: db}
	due := time.Now().Add(2 * time.Hour)

	todo, err := s.Create(ctx, 11, TodoCreate{Title: " 写周报 ", DueTime: &due, Tags: []string{" 工作 ", "", "工作"}, Source: model.TodoSourceCLI})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if todo.Title != "写周报" || todo.Status != model.TodoStatusActive || todo.Source != model.TodoSourceCLI {
		t.Fatalf("默认值/归一异常: %+v", todo)
	}
	if len(todo.Tags) != 1 || todo.Tags[0] != "工作" {
		t.Fatalf("标签应去空去重: %v", todo.Tags)
	}
	if _, err := s.Create(ctx, 11, TodoCreate{Title: "  "}); err == nil {
		t.Fatal("空白标题应报错")
	}

	// 流转 active -> done -> active（二态可逆），终态留 done 供筛选断言
	done := model.TodoStatusDone
	active := model.TodoStatusActive
	got, err := s.Update(ctx, 11, todo.ID, TodoUpdate{Status: &done})
	if err != nil || got.Status != model.TodoStatusDone {
		t.Fatalf("流转 done 失败: %v", err)
	}
	if got, err = s.Update(ctx, 11, todo.ID, TodoUpdate{Status: &active}); err != nil || got.Status != model.TodoStatusActive {
		t.Fatalf("恢复 active 失败: %v", err)
	}
	if got.CompletedAt != nil {
		t.Fatal("恢复 active 应清空 completed_at")
	}
	if _, err := s.Update(ctx, 11, todo.ID, TodoUpdate{Status: &done}); err != nil {
		t.Fatalf("再置 done 失败: %v", err)
	}
	// 非法状态
	bad := model.TodoStatus("finished")
	if _, err := s.Update(ctx, 11, todo.ID, TodoUpdate{Status: &bad}); err == nil {
		t.Fatal("非法 status 应报错")
	}
	// 跨用户隔离
	if _, err := s.Update(ctx, 12, todo.ID, TodoUpdate{Status: &done}); err != ErrTodoNotFound {
		t.Fatalf("跨用户应 Not Found: %v", err)
	}
	// 列表筛选
	t2, _ := s.Create(ctx, 11, TodoCreate{Title: "另一条"})
	openTodos, _, _ := s.List(ctx, 11, TodoFilter{})
	if len(openTodos) != 2 {
		t.Fatalf("应有 2 条: %d", len(openTodos))
	}
	onlyDone, _, _ := s.List(ctx, 11, TodoFilter{Status: model.TodoStatusDone})
	if len(onlyDone) != 1 || onlyDone[0].ID != todo.ID {
		t.Fatalf("status 筛选异常")
	}
	if err := s.Delete(ctx, 11, t2.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	left, _, _ := s.List(ctx, 11, TodoFilter{})
	if len(left) != 1 {
		t.Fatalf("删除后应剩 1 条")
	}
}

func TestMemoFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_memo")
	s := &MemoService{DB: db}

	m, err := s.Create(ctx, 11, MemoCreate{Content: " 一条便签 ", Tags: []string{"想法"}})
	if err != nil || m.Content != "一条便签" {
		t.Fatalf("创建失败: %v", err)
	}
	long := strings.Repeat("字", 2001)
	if _, err := s.Create(ctx, 11, MemoCreate{Content: long}); err == nil {
		t.Fatal("超长便签应引导转知识库")
	}
	// 置顶
	one := int16(1)
	if _, err := s.Update(ctx, 11, m.ID, MemoUpdate{IsPinned: &one}); err != nil {
		t.Fatalf("置顶失败: %v", err)
	}
	m2, _ := s.Create(ctx, 11, MemoCreate{Content: "第二条"})
	list, _ := s.List(ctx, 11, 0, 0)
	if len(list) != 2 || list[0].ID != m.ID {
		t.Fatalf("置顶应排首: %+v", list)
	}
	if err := s.Delete(ctx, 11, m2.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
}
