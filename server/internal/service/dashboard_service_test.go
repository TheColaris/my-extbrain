package service

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDashboardDaily(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_todo, tf_memo")
	ts := &TodoService{DB: db}
	ms := &MemoService{DB: db}
	_, _ = ts.Create(ctx, 31, TodoCreate{Title: "t1"})
	_, _ = ts.Create(ctx, 31, TodoCreate{Title: "t2"})
	_, _ = ms.Create(ctx, 31, MemoCreate{Content: "m1"})

	dsvc := &DashboardService{DB: db}
	d, err := dsvc.Summary(ctx, 31, "x")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(d.Daily) != 14 {
		t.Fatalf("应补齐 14 天桶: %d", len(d.Daily))
	}
	today := d.Daily[13]
	if today.Todos != 2 || today.Memos != 1 {
		t.Fatalf("今日桶应 todo=2 memo=1: %+v", today)
	}
	// 他人数据不混入
	d2, _ := dsvc.Summary(ctx, 32, "y")
	if d2.Daily[13].Todos != 0 {
		t.Fatal("跨用户数据不应混入")
	}
	_ = time.Now
}
