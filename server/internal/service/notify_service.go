package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// NotifyService —— 机器人渠道 CRUD 与钉钉/飞书 webhook 推送（加签）。
type NotifyService struct {
	DB      *gorm.DB
	Client  *http.Client
	PushLog *PushLogService // 渠道测试也留痕（nil=不留痕）
}

var (
	ErrChannelNotFound = errors.New("channel not found")
	ErrPushLogNotFound = errors.New("push log not found")
)

func NewNotifyService(db *gorm.DB, pushLog *PushLogService) *NotifyService {
	return &NotifyService{DB: db, Client: &http.Client{Timeout: 10 * time.Second}, PushLog: pushLog}
}

type ChannelInput struct {
	ChannelType model.ChannelType `json:"channel_type" binding:"required"`
	WebhookURL  string            `json:"webhook_url" binding:"required"`
	Secret      string            `json:"secret"`
}

func (s *NotifyService) List(ctx context.Context, userID int64) ([]model.NotifyChannel, error) {
	cs := make([]model.NotifyChannel, 0)
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("id DESC").Find(&cs).Error
	return cs, err
}

func (s *NotifyService) Create(ctx context.Context, userID int64, in ChannelInput) (*model.NotifyChannel, error) {
	if in.ChannelType != model.ChannelTypeDingTalk && in.ChannelType != model.ChannelTypeFeishu {
		return nil, &UserError{"channel_type 必须是 dingtalk 或 feishu"}
	}
	if !strings.HasPrefix(in.WebhookURL, "https://") {
		return nil, &UserError{"webhook_url 必须是 https:// 开头的机器人 Webhook 地址"}
	}
	c := &model.NotifyChannel{
		UserID: userID, ChannelType: in.ChannelType, WebhookURL: in.WebhookURL,
		Secret: strings.TrimSpace(in.Secret), IsEnabled: 1,
	}
	if err := s.DB.WithContext(ctx).Create(c).Error; err != nil {
		return nil, err
	}
	// 创建即测试（结果回写）
	s.Test(ctx, userID, c.ID)
	_ = s.DB.WithContext(ctx).First(c, c.ID).Error
	return c, nil
}

func (s *NotifyService) Toggle(ctx context.Context, userID, id int64, enabled bool) error {
	res := s.DB.WithContext(ctx).Model(&model.NotifyChannel{}).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		Updates(map[string]any{"is_enabled": boolInt(enabled), "update_time": time.Now()})
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return res.Error
}

func (s *NotifyService) Delete(ctx context.Context, userID, id int64) error {
	res := s.DB.WithContext(ctx).Model(&model.NotifyChannel{}).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		Updates(map[string]any{"is_deleted": 1, "update_time": time.Now()})
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return res.Error
}

// Test 发送测试消息并回写结果（渠道测试同样留痕到 tl_push_log）
func (s *NotifyService) Test(ctx context.Context, userID, id int64) error {
	c, err := s.get(ctx, userID, id)
	if err != nil {
		return err
	}
	start := time.Now()
	err = s.Send(c, "我的外脑 · 测试推送", "通知链路已打通 ✓")
	if s.PushLog != nil {
		ch := model.PushChannelDingTalk
		if c.ChannelType == model.ChannelTypeFeishu {
			ch = model.PushChannelFeishu
		}
		s.PushLog.Record(userID, model.NotifyEventTest, fmt.Sprintf("test:%d", time.Now().UnixNano()),
			ch, c.ID, "我的外脑 · 测试推送", "通知链路已打通 ✓", err, time.Since(start).Milliseconds())
	}
	s.recordResult(c, err)
	return err
}

func (s *NotifyService) get(ctx context.Context, userID, id int64) (*model.NotifyChannel, error) {
	var c model.NotifyChannel
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		First(&c).Error; err != nil {
		return nil, ErrChannelNotFound
	}
	return &c, nil
}

func (s *NotifyService) recordResult(c *model.NotifyChannel, err error) {
	result := "成功"
	if err != nil {
		result = err.Error()
		if len(result) > 200 {
			result = result[:200]
		}
	}
	now := time.Now()
	s.DB.Model(&model.NotifyChannel{}).Where("id = ?", c.ID).
		Updates(map[string]any{"last_push_time": now, "last_push_result": result, "update_time": now})
}

// Send 按渠道签名推送 markdown 消息
func (s *NotifyService) Send(c *model.NotifyChannel, title, text string) error {
	switch c.ChannelType {
	case model.ChannelTypeDingTalk:
		return s.sendDingTalk(c, title, text)
	case model.ChannelTypeFeishu:
		return s.sendFeishu(c, title, text)
	}
	return fmt.Errorf("未知渠道类型 %s", c.ChannelType)
}

func (s *NotifyService) sendDingTalk(c *model.NotifyChannel, title, text string) error {
	body := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": title, "text": fmt.Sprintf("### %s\n%s", title, text)},
	}
	u := c.WebhookURL
	if c.Secret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		mac := hmac.New(sha256.New, []byte(c.Secret))
		mac.Write([]byte(ts + "\n" + c.Secret))
		sign := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
		u += fmt.Sprintf("&timestamp=%s&sign=%s", ts, sign)
	}
	return s.postJSON(u, body)
}

func (s *NotifyService) sendFeishu(c *model.NotifyChannel, title, text string) error {
	body := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header":   map[string]any{"title": map[string]string{"tag": "plain_text", "content": title}},
			"elements": []any{map[string]any{"tag": "markdown", "content": text}},
		},
	}
	if c.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(ts+"\n"+c.Secret)) // 飞书：key=timestamp+"\n"+secret，数据为空
		mac.Write(nil)
		body["timestamp"] = ts
		body["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	return s.postJSON(c.WebhookURL, body)
}

func (s *NotifyService) postJSON(u string, body any) error {
	b, _ := json.Marshal(body)
	resp, err := s.Client.Post(u, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("网络错误: %s", errShort(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var r struct {
		ErrCode int    `json:"errcode"`
		Code    int    `json:"code"`
		ErrMsg  string `json:"errmsg"`
		Msg     string `json:"msg"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&r)
	if r.ErrCode != 0 || r.Code != 0 {
		msg := r.ErrMsg
		if msg == "" {
			msg = r.Msg
		}
		return fmt.Errorf("机器人返回错误 %d: %s", max(r.ErrCode, r.Code), msg)
	}
	return nil
}

func boolInt(b bool) int16 {
	if b {
		return 1
	}
	return 0
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func errShort(s string) string {
	if len(s) > 120 {
		return s[:120]
	}
	return s
}
