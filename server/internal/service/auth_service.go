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
}

type RegisterInput struct {
	Account  string            `json:"account" binding:"required"`
	Password string            `json:"password" binding:"required"`
	BindCh   model.BindChannel // phone/email（注册时由 account 类型识别，不由客户端指定）
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

// Register 注册（手机号或邮箱二选一 + 密码）
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	ch, ident, ok := DetectChannel(in.Account)
	if !ok {
		return nil, &UserError{"account 必须是合法手机号（1[3-9] 开头 11 位）或邮箱"}
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
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{PasswordHash: string(hash), NickName: defaultNick(ident)}
	if ch == model.BindChannelPhone {
		u.Phone = &ident
	} else {
		u.Email = &ident
	}
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
