package service

import (
	"context"
	"os"
	"testing"

	"extbrain-server/internal/model"
)

func TestLogQuery(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tl_api_log")
	// 造数：3 条 key 操作 + 2 条 Web 会话
	rows := []model.APILog{
		{UserID: 7, APIKeyID: 1, KeyHint: "ak_x…1", Action: "note.upsert", Target: "a.md", StatusCode: 200, CostMs: 9, ClientIP: "1.1.1.1", IPRegion: "美国"},
		{UserID: 7, APIKeyID: 1, KeyHint: "ak_x…1", Action: "todo.create", StatusCode: 200, CostMs: 6, ClientIP: "1.1.1.1"},
		{UserID: 7, APIKeyID: 2, KeyHint: "ak_x…2", Action: "todo.create", StatusCode: 200, CostMs: 5, ClientIP: "2.2.2.2"},
		{UserID: 7, APIKeyID: 0, Action: "web.login", StatusCode: 200, CostMs: 18, ClientIP: "3.3.3.3"},
		{UserID: 8, APIKeyID: 9, Action: "todo.create", StatusCode: 200, CostMs: 1, ClientIP: "9.9.9.9"}, // 他人数据
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("造数失败: %v", err)
		}
	}
	s := &LogQueryService{DB: db}

	p, err := s.List(ctx, 7, LogFilter{Limit: 10})
	if err != nil || len(p.Logs) != 4 {
		t.Fatalf("只查本人：got %d err %v", len(p.Logs), err)
	}
	p, _ = s.List(ctx, 7, LogFilter{KeyID: 1, Limit: 10})
	if len(p.Logs) != 2 {
		t.Fatalf("key_id 筛选：got %d", len(p.Logs))
	}
	p, _ = s.List(ctx, 7, LogFilter{KeyID: -1, Limit: 10})
	if len(p.Logs) != 1 || p.Logs[0].Action != "web.login" {
		t.Fatalf("key_id=-1 应只回 Web 会话：got %+v", p.Logs)
	}
	p, _ = s.List(ctx, 7, LogFilter{Action: "todo.create", Limit: 10})
	if len(p.Logs) != 2 {
		t.Fatalf("action 筛选：got %d", len(p.Logs))
	}
	p, _ = s.List(ctx, 7, LogFilter{Limit: 2})
	if len(p.Logs) != 2 || p.NextBeforeID == 0 {
		t.Fatalf("分页应有 next_before_id：len=%d next=%d", len(p.Logs), p.NextBeforeID)
	}
	last := p.Logs[0].ID
	p2, _ := s.List(ctx, 7, LogFilter{Limit: 10, BeforeID: p.NextBeforeID})
	if len(p2.Logs) != 2 || p2.Logs[0].ID >= last {
		t.Fatalf("游标翻页异常：%+v", p2.Logs)
	}
}
