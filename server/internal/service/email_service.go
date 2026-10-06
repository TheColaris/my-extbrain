package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"extbrain-server/internal/e2e"

	"gorm.io/gorm"
)

// EmailService —— Resend 发信（注册验证码 / 平台测试发信）。
// 配置走 SysConfig 热更新；E2E 洁净室截获不外发（绝不回落真实外发）。
type EmailService struct {
	DB   *gorm.DB
	Cfg  *SysConfig
	HTTP *http.Client
	// Endpoint 发信端点（缺省 Resend；测试注入 mock）
	Endpoint string
}

func NewEmailService(db *gorm.DB, cfg *SysConfig) *EmailService {
	return &EmailService{DB: db, Cfg: cfg, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

const resendEndpoint = "https://api.resend.com/emails"

func (s *EmailService) endpoint() string {
	if s.Endpoint != "" {
		return s.Endpoint
	}
	return resendEndpoint
}

// Ready 邮件服务是否可用（配置齐备）。
func (s *EmailService) Ready() bool { return s.Cfg.Email().Ready() }

// Send 发送一封邮件（HTML + 纯文本双版本）。配置未启用返回 UserError（文案可直接展示）。
func (s *EmailService) Send(ctx context.Context, to, subject, textBody, htmlBody string) error {
	c := s.Cfg.Email()
	if !c.Ready() {
		return &UserError{"邮件服务未配置或未启用，请联系管理员"}
	}
	// E2E 洁净室：不真发，截获落信箱后按成功返回（绝不回落真实外发）
	if e2e.Enabled() {
		e2e.Capture(s.DB, e2e.ChannelEmail, to, subject, textBody, htmlBody)
		return nil
	}
	from := c.FromAddr
	if c.FromName != "" {
		from = fmt.Sprintf("%s <%s>", c.FromName, c.FromAddr)
	}
	payload, err := json.Marshal(map[string]any{
		"from":    from,
		"to":      []string{to},
		"subject": subject,
		"text":    textBody,
		"html":    htmlBody,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	rsp, err := s.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("发信请求失败: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode < 200 || rsp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(rsp.Body).Decode(&e)
		msg := strings.TrimSpace(e.Message)
		if msg == "" {
			msg = rsp.Status
		}
		return fmt.Errorf("Resend 拒绝（%d）: %s", rsp.StatusCode, msg)
	}
	return nil
}

// SendCode 发送注册验证码（HTML 模板）。
func (s *EmailService) SendCode(ctx context.Context, to, code string, ttlMinutes int) error {
	text := fmt.Sprintf("你的注册验证码是 %s（%d 分钟内有效）。若非本人操作，忽略此邮件即可。", code, ttlMinutes)
	return s.Send(ctx, to, "外脑注册验证码", text, renderEmailCode(code, ttlMinutes))
}

// Test 平台管理页「测试发信」。
func (s *EmailService) Test(ctx context.Context, to string) error {
	text := "这是一封来自「我的外脑」的测试邮件：收到即表示邮件服务配置可用。"
	return s.Send(ctx, to, "外脑邮件服务测试", text, renderEmailTest())
}

// randomCode6 生成 6 位数字验证码（crypto/rand）。
func randomCode6() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		n = big.NewInt(time.Now().UnixNano() % 1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// renderEmailCode 注册验证码邮件（设计稿=prototype/email-code.html；
// inline style + table 布局保证各邮箱客户端兼容，禁引外链图片/样式）。
func renderEmailCode(code string, ttlMinutes int) string {
	return strings.NewReplacer(
		"{{CODE}}", code,
		"{{TTL}}", strconv.Itoa(ttlMinutes),
	).Replace(emailCodeHTML)
}

const emailCodeHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:0;background:#efe9dc;">
<div style="padding:36px 16px;background:#efe9dc;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:540px;margin:0 auto;">
    <tr>
      <td style="background:#ffffff;border:2px solid #000000;border-radius:12px;padding:34px 38px;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Helvetica Neue',Arial,sans-serif;">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0">
          <tr>
            <td style="width:28px;">
              <div style="width:26px;height:26px;line-height:26px;text-align:center;background:#ffdc58;border:2px solid #000000;border-radius:7px;font-size:14px;font-weight:800;color:#000000;">脑</div>
            </td>
            <td style="padding-left:9px;font-size:15px;font-weight:800;color:#000000;letter-spacing:-0.01em;">我的外脑</td>
          </tr>
        </table>
        <div style="margin-top:26px;font-size:20px;font-weight:800;color:#000000;letter-spacing:-0.01em;">注册验证码</div>
        <div style="margin-top:6px;font-size:13px;line-height:1.6;color:#55504a;">请在注册页输入以下验证码，完成邮箱验证：</div>
        <div style="margin-top:20px;background:#ffdc58;border:2px solid #000000;border-radius:10px;padding:22px 0;text-align:center;font-family:ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-size:34px;font-weight:800;letter-spacing:12px;text-indent:12px;color:#000000;">{{CODE}}</div>
        <div style="margin-top:16px;font-size:13px;line-height:1.7;color:#55504a;">验证码 {{TTL}} 分钟内有效。若非本人操作，忽略此邮件即可。</div>
      </td>
    </tr>
    <tr>
      <td style="padding:16px 6px 0;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Helvetica Neue',Arial,sans-serif;font-size:12px;line-height:1.8;color:#8a857d;">
        我的外脑 · my-extbrain.bot.cd<br>
        这是一封系统邮件，请勿回复。
      </td>
    </tr>
  </table>
</div>
</body>
</html>`

// renderEmailTest 测试发信邮件（同风格简化版）。
func renderEmailTest() string {
	return emailTestHTML
}

const emailTestHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<body style="margin:0;padding:0;background:#efe9dc;">
<div style="padding:36px 16px;background:#efe9dc;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:540px;margin:0 auto;">
    <tr>
      <td style="background:#ffffff;border:2px solid #000000;border-radius:12px;padding:34px 38px;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Helvetica Neue',Arial,sans-serif;">
        <table role="presentation" cellpadding="0" cellspacing="0" border="0">
          <tr>
            <td style="width:28px;">
              <div style="width:26px;height:26px;line-height:26px;text-align:center;background:#ffdc58;border:2px solid #000000;border-radius:7px;font-size:14px;font-weight:800;color:#000000;">脑</div>
            </td>
            <td style="padding-left:9px;font-size:15px;font-weight:800;color:#000000;letter-spacing:-0.01em;">我的外脑</td>
          </tr>
        </table>
        <div style="margin-top:26px;font-size:20px;font-weight:800;color:#000000;letter-spacing:-0.01em;">邮件服务测试</div>
        <div style="margin-top:10px;font-size:13px;line-height:1.7;color:#55504a;">收到此邮件即表示邮件服务配置可用。</div>
      </td>
    </tr>
    <tr>
      <td style="padding:16px 6px 0;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Hiragino Sans GB','Microsoft YaHei','Helvetica Neue',Arial,sans-serif;font-size:12px;line-height:1.8;color:#8a857d;">
        我的外脑 · my-extbrain.bot.cd<br>
        这是一封系统邮件，请勿回复。
      </td>
    </tr>
  </table>
</div>
</body>
</html>`
