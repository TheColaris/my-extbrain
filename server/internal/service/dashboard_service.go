package service

import (
	"context"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// DashboardService —— 仪表盘聚合（一次请求返回数字带与首页列表，查询次数固定）。
type DashboardService struct{ DB *gorm.DB }

type Dashboard struct {
	TodoCount   int64        `json:"todo_count"`
	MemoCount   int64        `json:"memo_count"`
	NoteCount   int64        `json:"note_count"`
	DueToday    int64        `json:"due_today"`
	TodayTodos  []model.Todo `json:"today_todos"` // 今日到期（含已逾期未完成）
	RecentMemos []model.Memo `json:"recent_memos"`
	Daily       []DayCount   `json:"daily"` // 最近 14 天新建趋势（待办+便签）
}

type DayCount struct {
	Date  string `json:"date"` // MM-DD
	Todos int64  `json:"todos"`
	Memos int64  `json:"memos"`
}

func (s *DashboardService) Summary(ctx context.Context, userID int64, nick string) (*Dashboard, error) {
	d := &Dashboard{}
	now := time.Now()
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())

	q := func(m any) *gorm.DB {
		return s.DB.WithContext(ctx).Model(m).Where("user_id = ? AND is_deleted = 0", userID)
	}
	if err := q(&model.Todo{}).Where("status <> ?", model.TodoStatusDone).Count(&d.TodoCount).Error; err != nil {
		return nil, err
	}
	if err := q(&model.Memo{}).Count(&d.MemoCount).Error; err != nil {
		return nil, err
	}
	if err := q(&model.Note{}).Count(&d.NoteCount).Error; err != nil {
		return nil, err
	}
	if err := q(&model.Todo{}).
		Where("due_time IS NOT NULL AND due_time <= ? AND status <> ?", todayEnd, model.TodoStatusDone).
		Count(&d.DueToday).Error; err != nil {
		return nil, err
	}
	if err := q(&model.Todo{}).
		Where("due_time IS NOT NULL AND due_time <= ? AND status <> ?", todayEnd, model.TodoStatusDone).
		Order("due_time ASC").Limit(8).Find(&d.TodayTodos).Error; err != nil {
		return nil, err
	}
	if err := q(&model.Memo{}).Order("id DESC").Limit(5).Find(&d.RecentMemos).Error; err != nil {
		return nil, err
	}
	// 14 天趋势：取原始时间戳在 Go 侧按本地时区分桶（避免 DB/应用时区错位）
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -13)
	bucket := map[string]*DayCount{}
	order := make([]string, 0, 14)
	for i := 0; i < 14; i++ {
		key := start.AddDate(0, 0, i).Format("01-02")
		bucket[key] = &DayCount{Date: key}
		order = append(order, key)
	}
	var times []struct {
		CreateTime time.Time
	}
	if err := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Select("create_time").
		Where("user_id = ? AND is_deleted = 0 AND create_time >= ?", userID, start).
		Scan(&times).Error; err != nil {
		return nil, err
	}
	for _, r := range times {
		if b, ok := bucket[r.CreateTime.In(now.Location()).Format("01-02")]; ok {
			b.Todos++
		}
	}
	times = nil
	var mtimes []struct {
		CreateTime time.Time
	}
	if err := s.DB.WithContext(ctx).Model(&model.Memo{}).
		Select("create_time").
		Where("user_id = ? AND is_deleted = 0 AND create_time >= ?", userID, start).
		Scan(&mtimes).Error; err != nil {
		return nil, err
	}
	for _, r := range mtimes {
		if b, ok := bucket[r.CreateTime.In(now.Location()).Format("01-02")]; ok {
			b.Memos++
		}
	}
	d.Daily = make([]DayCount, 0, 14)
	for _, k := range order {
		d.Daily = append(d.Daily, *bucket[k])
	}
	return d, nil
}
