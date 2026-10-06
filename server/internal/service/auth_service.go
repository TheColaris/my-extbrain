package service

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 账号通道识别与校验
var (
	phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)
	emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

const (
	pwdMinLen       = 8
	loginFailLimit  = 5                // 连续失败 5 次
	loginFailWindow = 15 * time.Minute // 锁 15 分钟（窗口即锁期）
)

type AuthService struct {
	DB    *gorm.DB
	Cache *CacheService
	JWT   *auth.Manager
	Email *EmailService
}

type RegisterInput struct {
	Account   string            `json:"account" binding:"required"`
	Password  string            `json:"password" binding:"required"`
	EmailCode string            `json:"email_code"` // 邮箱注册必填（POST /auth/send-email-code 下发）
	BindCh    model.BindChannel // phone/email（注册时由 account 类型识别，不由客户端指定）
}

type LoginInput struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// DetectChannel account 自动识别：含 @ = email，11 位手机号 = phone
func DetectChannel(account string) (model.BindChannel, string, bool) {
	a := strings.TrimSpace(strings.ToLower(account))
	switch {
	case emailRe.MatchString(a):
		return model.BindChannelEmail, a, true
	case phoneRe.MatchString(a):
		return model.BindChannelPhone, a, true
	default:
		return "", "", false
	}
}

// Register 注册（仅邮箱 + 验证码验证；手机号注册已下掉，存量手机号账号不受影响）
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	ch, ident, ok := DetectChannel(in.Account)
	if !ok {
		return nil, &UserError{"account 必须是合法邮箱"}
	}
	if ch != model.BindChannelEmail {
		return nil, &UserError{"当前仅支持邮箱注册"}
	}
	if len(in.Password) < pwdMinLen {
		return nil, &UserError{"密码至少 8 位"}
	}
	// 通道占用检查（两通道都查，命中即冲突）
	var cnt int64
	s.DB.WithContext(ctx).Model(&model.User{}).
		Where("phone = ? AND is_deleted = 0", ident).
		Or("email = ? AND is_deleted = 0", ident).
		Count(&cnt)
	if cnt > 0 {
		return nil, &UserError{"该账号已注册"}
	}
	// 邮箱验证码校验（一次性消费即删；失败即拒，不建号）
	if err := s.consumeEmailCode(ctx, ident, in.EmailCode); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{PasswordHash: string(hash), NickName: defaultNick(ident), Email: &ident}
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 首个注册用户自动成为平台管理员（自托管开箱即用；存量库由迁移 0009 补齐）。
		// Count 粗判 + sys_cache 原子计数收口：并发注册只有一个请求拿到 1（根除双 admin TOCTOU，
		// 无需唯一索引；缓存清空且库无 admin 的双场景才可能重发，正常演化下安全）
		var admins int64
		if err := tx.Model(&model.User{}).Where("is_admin = 1").Count(&admins).Error; err != nil {
			return err
		}
		if admins == 0 {
			if n, _ := s.Cache.Incr(ctx, "admin_bootstrap", 24*time.Hour); n == 1 {
				u.IsAdmin = 1
			}
		}
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		// 每用户一个默认仓库（迁移 0012；裸路径寻址=默认仓库）
		return tx.Create(&model.Repo{UserID: u.ID, Name: "默认仓库", Description: "开启仓库功能时自动创建；存量笔记都在这里", IsDefault: 1}).Error
	}); err != nil {
		return nil, err
	}
	return u, nil
}

// 注册验证码参数：10 分钟有效；同邮箱 60 秒重发冷却；全局日上限（Resend 免费层 100 封/天，留余量）。
const (
	emailCodeTTL      = 10 * time.Minute
	emailCodeCool     = time.Minute
	emailCodeDailyCap = 90
)

func emailCodeKey(email string) string { return "email_code:" + email }

// SendEmailCode 发送注册验证码：格式校验 → 冷却 → 日上限 → 生成存缓存 → 发信（失败作废码、允许立即重试）。
func (s *AuthService) SendEmailCode(ctx context.Context, email string) error {
	a := strings.TrimSpace(strings.ToLower(email))
	if !emailRe.MatchString(a) {
		return &UserError{"邮箱格式不正确"}
	}
	if s.Email == nil || !s.Email.Ready() {
		return &UserError{"邮件服务未配置或未启用，请联系管理员"}
	}
	if v, _ := s.Cache.Get(ctx, "email_code_cool:"+a); v != "" {
		return &UserError{"发送过于频繁，请 1 分钟后再试"}
	}
	n, err := s.Cache.Incr(ctx, "email_code_daily:"+time.Now().Format("20060102"), 48*time.Hour)
	if err != nil {
		return err
	}
	if n > emailCodeDailyCap {
		return &UserError{"今日验证码发送量已达上限，请明天再试"}
	}
	code := randomCode6()
	if err := s.Cache.Set(ctx, emailCodeKey(a), code, emailCodeTTL); err != nil {
		return err
	}
	if err := s.Email.SendCode(ctx, a, code, int(emailCodeTTL.Minutes())); err != nil {
		s.Cache.Del(ctx, emailCodeKey(a))
		var ue *UserError
		if errors.As(err, &ue) {
			return err
		}
		return &UserError{"验证码邮件发送失败，请稍后重试"}
	}
	_ = s.Cache.Set(ctx, "email_code_cool:"+a, "1", emailCodeCool)
	return nil
}

// consumeEmailCode 校验并消费邮箱验证码（一次性；空/过期/不匹配均报错）。
func (s *AuthService) consumeEmailCode(ctx context.Context, email, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return &UserError{"请先获取邮箱验证码"}
	}
	saved, err := s.Cache.Get(ctx, emailCodeKey(email))
	if err != nil || saved == "" {
		return &UserError{"验证码已过期，请重新获取"}
	}
	if saved != code {
		return &UserError{"验证码错误"}
	}
	s.Cache.Del(ctx, emailCodeKey(email))
	return nil
}

// Login 登录：account 自动识别通道；同 (账号, IP) 组合连续失败 loginFailLimit 次锁定 loginFailWindow。
// 锁键含 IP：攻击者无法从自己的 IP 锁死受害者的正常登录（换 IP 重试由登录端点 per-IP 限流兜住）。
func (s *AuthService) Login(ctx context.Context, in LoginInput, clientIP string) (*model.User, string, error) {
	_, ident, ok := DetectChannel(in.Account)
	if !ok {
		return nil, "", &UserError{"账号格式不合法（手机号或邮箱）"}
	}
	failKey := "pwd_fail:" + ident + ":" + clientIP
	if n := s.Cache.GetInt(ctx, failKey); n >= loginFailLimit {
		return nil, "", &UserError{"失败次数过多，请 15 分钟后再试"}
	}
	var u model.User
	q := s.DB.WithContext(ctx).Where("is_deleted = 0")
	if strings.Contains(ident, "@") {
		q = q.Where("email = ?", ident)
	} else {
		q = q.Where("phone = ?", ident)
	}
	if err := q.First(&u).Error; err != nil {
		s.bumpFail(ctx, failKey)
		return nil, "", &UserError{"账号或密码错误"}
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		s.bumpFail(ctx, failKey)
		return nil, "", &UserError{"账号或密码错误"}
	}
	s.Cache.Del(ctx, failKey)
	token, err := s.JWT.Sign(u.ID, u.NickName, u.TokenVersion)
	if err != nil {
		return nil, "", err
	}
	return &u, token, nil
}

func (s *AuthService) bumpFail(ctx context.Context, key string) {
	_, _ = s.Cache.Incr(ctx, key, loginFailWindow)
}

func defaultNick(ident string) string {
	if i := strings.IndexByte(ident, '@'); i > 0 {
		return ident[:i]
	}
	return "用户" + ident[len(ident)-4:]
}

// UserError 面向用户的业务错误（message 可直接展示/执行）
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

var ErrUserNotFound = errors.New("user not found")
