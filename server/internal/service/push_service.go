package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"extbrain-server/internal/model"

	"github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PushService —— Web Push（VAPID）订阅管理与到期扫描推送。
type PushService struct {
	DB          *gorm.DB
	Cache       *CacheService
	Notify      *NotifyService
	PushLog     *PushLogService
	VapidPublic string
	VapidPriv   string
}

type SubInput struct {
	Endpoint string `json:"endpoint" binding:"required"`
	Keys     struct {
		P256DH string `json:"p256dh" binding:"required"`
		Auth   string `json:"auth" binding:"required"`
	} `json:"keys" binding:"required"`
}

func (s *PushService) Enabled() bool { return s.VapidPublic != "" && s.VapidPriv != "" }

func (s *PushService) Subscribe(ctx context.Context, userID int64, in SubInput) error {
	// endpoint 全局唯一：已被其他账号绑定的订阅拒绝接管（冲突分支不再改绑 user_id，
	// 防知晓他人 endpoint URL 者把推送通道改绑到自己名下）
	var exist model.PushSubscription
	if err := s.DB.WithContext(ctx).Where("endpoint = ?", in.Endpoint).First(&exist).Error; err == nil {
		if exist.UserID != userID {
			return &UserError{"该推送订阅已绑定其他账号"}
		}
	}
	sub := model.PushSubscription{
		UserID: userID, Endpoint: in.Endpoint, P256DH: in.Keys.P256DH, Auth: in.Keys.Auth,
	}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "endpoint"}},
		DoUpdates: clause.Assignments(map[string]any{
			"p256dh": in.Keys.P256DH, "auth": in.Keys.Auth, "is_deleted": 0,
		}),
	}).Create(&sub).Error
}

func (s *PushService) Unsubscribe(ctx context.Context, userID int64, endpoint string) error {
	return s.DB.WithContext(ctx).Model(&model.PushSubscription{}).
		Where("endpoint = ? AND user_id = ?", endpoint, userID).
		Update("is_deleted", 1).Error
}

// webPush 向单个订阅发通知
func (s *PushService) webPush(sub *model.PushSubscription, title, body string) error {
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body})
	rsp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{Auth: sub.Auth, P256dh: sub.P256DH},
	}, &webpush.Options{VAPIDPublicKey: s.VapidPublic, VAPIDPrivateKey: s.VapidPriv, TTL: 24 * 3600})
	if rsp != nil {
		rsp.Body.Close()
		if rsp.StatusCode == 404 || rsp.StatusCode == 410 { // 订阅失效：清理
			s.DB.Model(sub).Updates(map[string]any{"is_deleted": 1})
			return fmt.Errorf("订阅已失效（%d），已自动清理", rsp.StatusCode)
		}
	}
	return err
}

// DueScan 到期扫描：09:00 当日汇总 + 逾期即时（每小时跑）；sys_cache 防重。
// 返回本次推送人数。
func (s *PushService) DueScan(ctx context.Context) (int, error) {
	if !s.Enabled() {
		// Web Push 未配也继续：钉钉/飞书渠道独立生效
		_ = s.Enabled()
	}
	now := time.Now()
	todayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	todayKey := now.Format("2006-01-02")
	var eventType model.NotifyEventType
	var eventKey string

	// 找出有「今日到期未完成」待办的用户
	type userDue struct {
		UserID   int64 `gorm:"column:user_id"`
		DueCount int64 `gorm:"column:due_count"`
		OverdueN int64 `gorm:"column:overdue_n"`
	}
	var uds []userDue
	err := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Select("user_id, COUNT(*) AS due_count, SUM(CASE WHEN due_time < ? THEN 1 ELSE 0 END) AS overdue_n", now).
		Where("is_deleted = 0 AND status <> ? AND due_time IS NOT NULL AND due_time <= ?", model.TodoStatusDone, todayEnd).
		Group("user_id").Scan(&uds).Error
	if err != nil {
		return 0, err
	}

	sent := 0
	for _, ud := range uds {
		digestKey := fmt.Sprintf("push:digest:%d:%s", ud.UserID, todayKey)
		// 09:00 前只推「逾期即时」（且仅当有逾期）；09:00-22:00 推当日汇总（每天一次）
		var title, body string
		if now.Hour() >= 9 && now.Hour() < 22 {
			if s.Cache.GetInt(ctx, digestKey) > 0 {
				continue // 今日汇总已发
			}
			title = fmt.Sprintf("今日待办 · %d 项到期", ud.DueCount)
			body = fmt.Sprintf("其中逾期未完成 %d 项，打开面板处理", ud.OverdueN)
			eventType, eventKey = model.NotifyEventDueToday, "due:"+todayKey
		} else if ud.OverdueN > 0 {
			overdueKey := fmt.Sprintf("push:overdue:%d:%s", ud.UserID, todayKey)
			if s.Cache.GetInt(ctx, overdueKey) > 0 {
				continue
			}
			_ = s.Cache.Set(ctx, overdueKey, "1", 24*time.Hour)
			title = fmt.Sprintf("逾期提醒 · %d 项", ud.OverdueN)
			body = "有待办已过期未完成"
			eventType, eventKey = model.NotifyEventOverdue, "overdue:"+todayKey
		} else {
			continue
		}

		s.pushToUser(ctx, ud.UserID, eventType, eventKey, title, body)
		if now.Hour() >= 9 && now.Hour() < 22 {
			_ = s.Cache.Set(ctx, digestKey, "1", 24*time.Hour)
		}
		sent++
	}
	return sent, nil
}

// pushToUser 推给用户全部启用渠道（机器人 + 浏览器），逐渠道留痕（tl_push_log）
func (s *PushService) pushToUser(ctx context.Context, userID int64, eventType model.NotifyEventType, eventKey, title, body string) {
	if s.Notify != nil {
		var channels []model.NotifyChannel
		s.DB.WithContext(ctx).
			Where("user_id = ? AND is_enabled = 1 AND is_deleted = 0", userID).Find(&channels)
		for i := range channels {
			start := time.Now()
			ch := model.PushChannelDingTalk
			if channels[i].ChannelType == model.ChannelTypeFeishu {
				ch = model.PushChannelFeishu
			}
			err := s.Notify.Send(&channels[i], title, body)
			if s.PushLog != nil {
				s.PushLog.Record(userID, eventType, eventKey, ch, channels[i].ID, title, body, err, time.Since(start).Milliseconds())
			}
			if err != nil {
				log.Printf("[push] 渠道 %d 推送失败: %v", channels[i].ID, err)
			}
		}
	}
	if s.Enabled() {
		var subs []model.PushSubscription
		s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 0", userID).Find(&subs)
		for i := range subs {
			start := time.Now()
			err := s.webPush(&subs[i], title, body)
			if s.PushLog != nil {
				s.PushLog.Record(userID, eventType, eventKey, model.PushChannelWeb, subs[i].ID, title, body, err, time.Since(start).Milliseconds())
			}
			if err != nil {
				log.Printf("[push] 浏览器订阅推送失败: %v", err)
			}
		}
	}
}

var ErrVapidDisabled = errors.New("web push disabled")

// GenerateVapid 生成 VAPID 密钥对（打印到 stdout，写入 env 后重启）
func GenerateVapid() (pub, priv string, err error) {
	return webpush.GenerateVAPIDKeys()
}

// SendToUserForTest 面板「发送测试」：给自己全部启用渠道推一条
func (s *PushService) SendToUserForTest(ctx context.Context, userID int64) {
	s.pushToUser(ctx, userID, model.NotifyEventTest, fmt.Sprintf("test:%d", time.Now().UnixNano()), "我的外脑 · 测试推送", "通知链路已打通 ✓")
}

// RetryChannel 对一条投递记录重推（同渠道同内容）；结果原地更新该行。
func (s *PushService) RetryChannel(ctx context.Context, userID, logID int64) error {
	if s.PushLog == nil {
		return fmt.Errorf("推送记录组件未装配")
	}
	row, err := s.PushLog.Get(ctx, userID, logID)
	if err != nil {
		return err
	}
	ch := model.PushChannel(row.ChannelType)
	start := time.Now()
	var pushErr error
	switch ch {
	case model.PushChannelWeb:
		var sub model.PushSubscription
		q := s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 0", userID)
		if row.ChannelID > 0 {
			q = q.Where("id = ?", row.ChannelID)
		}
		if err := q.Order("id DESC").First(&sub).Error; err != nil {
			pushErr = fmt.Errorf("浏览器订阅不存在或已失效，请到设置页重新订阅")
		} else {
			pushErr = s.webPush(&sub, row.Title, row.Body)
			row.ChannelID = sub.ID
		}
	case model.PushChannelDingTalk, model.PushChannelFeishu:
		var c model.NotifyChannel
		if err := s.DB.WithContext(ctx).
			Where("id = ? AND user_id = ? AND is_deleted = 0", row.ChannelID, userID).First(&c).Error; err != nil {
			pushErr = fmt.Errorf("渠道已被删除，无法重推；可到设置页重新配置")
		} else if c.IsEnabled != 1 {
			pushErr = fmt.Errorf("渠道已停用，启用后再重推")
		} else {
			pushErr = s.Notify.Send(&c, row.Title, row.Body)
		}
	default:
		pushErr = fmt.Errorf("未知渠道 %s", row.ChannelType)
	}
	s.PushLog.MarkRetry(row, pushErr, time.Since(start).Milliseconds())
	return pushErr
}
