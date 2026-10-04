package service

import (
	"context"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// PushLogService —— 通知推送记录：每次投递留痕（成败+原因），按事件聚合查询，支持重推。
type PushLogService struct{ DB *gorm.DB }

// PushLog GORM 模型（tl_push_log，迁移 0005）
type PushLog = model.PushLog

// Record 落一条投递记录；失败不阻断主流程（推送本身已结束，记录失败只打日志语义由调用方忽略）。
func (s *PushLogService) Record(userID int64, eventType model.NotifyEventType, eventKey string,
	channel model.PushChannel, channelID int64, title, body string, err error, costMs int64) {
	st := model.PushStatusOK
	msg := ""
	if err != nil {
		st = model.PushStatusFail
		msg = errShort(err.Error())
		if len(msg) > 255 {
			msg = msg[:255]
		}
	}
	row := &model.PushLog{
		UserID: userID, EventType: string(eventType), EventKey: eventKey,
		ChannelType: string(channel), ChannelID: channelID,
		Title: cut(title, 255), Body: cut(body, 500),
		Status: string(st), Error: msg, CostMs: int32(costMs),
	}
	_ = s.DB.Create(row).Error
}

// PushChannelHit 事件聚合里的单渠道投递
type PushChannelHit struct {
	ID          int64     `json:"id"`
	ChannelType string    `json:"channel_type"`
	ChannelID   int64     `json:"channel_id"`
	Status      string    `json:"status"`
	Error       string    `json:"error"`
	CostMs      int32     `json:"cost_ms"`
	RetryCount  int       `json:"retry_count"`
	CreateTime  time.Time `json:"create_time"`
}

// PushEvent 事件聚合行（一条提醒 · 多渠道投递）
type PushEvent struct {
	EventKey   string           `json:"event_key"`
	EventType  string           `json:"event_type"`
	Title      string           `json:"title"`
	Body       string           `json:"body"`
	CreateTime time.Time        `json:"create_time"`
	Channels   []PushChannelHit `json:"channels"`
}

// PushLogFilter 查询过滤（零值=不限）
type PushLogFilter struct {
	Channel string // web/dingtalk/feishu
	Result  string // fail=仅含失败渠道的事件
	Type    string // due_today/overdue/test
	Days    int    // 时间窗（默认 7，上限 90）
	Limit   int
}

// List 按事件聚合返回（事件=同 event_key 的全部渠道投递；时间倒序）。
func (s *PushLogService) List(ctx context.Context, userID int64, f PushLogFilter) ([]PushEvent, error) {
	if f.Days <= 0 || f.Days > 90 {
		f.Days = 7
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	since := time.Now().AddDate(0, 0, -f.Days)
	q := s.DB.WithContext(ctx).Model(&model.PushLog{}).Where("user_id = ? AND create_time >= ?", userID, since)
	if f.Channel != "" {
		q = q.Where("channel_type = ?", f.Channel)
	}
	if f.Type != "" {
		q = q.Where("event_type = ?", f.Type)
	}
	if f.Result == "fail" {
		q = q.Where("status = ?", string(model.PushStatusFail))
	}
	var rows []model.PushLog
	if err := q.Order("create_time DESC").Limit(f.Limit * 8).Find(&rows).Error; err != nil {
		return nil, err
	}
	events := make([]PushEvent, 0, 8)
	byKey := map[string]*PushEvent{}
	for _, r := range rows {
		ev, ok := byKey[r.EventKey]
		if !ok {
			ev = &PushEvent{EventKey: r.EventKey, EventType: r.EventType, Title: r.Title, Body: r.Body, CreateTime: r.CreateTime, Channels: []PushChannelHit{}}
			events = append(events, *ev)
			ev = &events[len(events)-1]
			// 必须指向切片元素：指向堆对象会让后续渠道写进拷贝前的孤儿对象（多渠道只剩 1 条）
			byKey[r.EventKey] = ev
		}
		ev.Channels = append(ev.Channels, PushChannelHit{
			ID: r.ID, ChannelType: r.ChannelType, ChannelID: r.ChannelID,
			Status: r.Status, Error: r.Error, CostMs: r.CostMs, RetryCount: r.RetryCount, CreateTime: r.CreateTime,
		})
	}
	return events, nil
}

// PushStats 今日数字带（只放可数的量：提醒/投递/失败）
type PushStats struct {
	Events     int64 `json:"events"`
	Deliveries int64 `json:"deliveries"`
	Fails      int64 `json:"fails"`
}

// StatsToday 今日统计（自然日，服务器时区）
func (s *PushLogService) StatsToday(ctx context.Context, userID int64) (PushStats, error) {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var st PushStats
	err := s.DB.WithContext(ctx).Model(&model.PushLog{}).
		Select("COUNT(DISTINCT event_key) AS events, COUNT(*) AS deliveries, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS fails", string(model.PushStatusFail)).
		Where("user_id = ? AND create_time >= ?", userID, dayStart).
		Scan(&st).Error
	return st, err
}

// Get 取单条（重推前校验归属）
func (s *PushLogService) Get(ctx context.Context, userID, id int64) (*model.PushLog, error) {
	var row model.PushLog
	if err := s.DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&row).Error; err != nil {
		return nil, ErrPushLogNotFound
	}
	return &row, nil
}

// MarkRetry 重推后原地更新本行（重试计数+1，历史不另起行）
func (s *PushLogService) MarkRetry(row *model.PushLog, err error, costMs int64) {
	st := model.PushStatusOK
	msg := ""
	if err != nil {
		st = model.PushStatusFail
		msg = errShort(err.Error())
		if len(msg) > 255 {
			msg = msg[:255]
		}
	}
	s.DB.Model(row).Updates(map[string]any{
		"status": string(st), "error": msg, "cost_ms": int32(costMs),
		"retry_count": row.RetryCount + 1, "update_time": time.Now(),
	})
}

// CleanupRetention 保留期清理（与审计日志同口径，默认 90 天）
func (s *PushLogService) CleanupRetention(ctx context.Context, days int) (int64, error) {
	res := s.DB.WithContext(ctx).Where("create_time < ?", time.Now().AddDate(0, 0, -days)).Delete(&model.PushLog{})
	return res.RowsAffected, res.Error
}
