package service

import (
	"context"
	"errors"
	"strings"

	"extbrain-server/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AccountService —— 账户资料：绑定/换绑手机号与邮箱、改密。
type AccountService struct {
	DB   *gorm.DB
	Auth *AuthService // 复用通道识别与占用检查
}

func (s *AccountService) Bind(ctx context.Context, userID int64, ch model.BindChannel, value, currentPwd string) (*model.User, error) {
	if ch != model.BindChannelPhone && ch != model.BindChannelEmail {
		return nil, &UserError{"type 必须是 phone 或 email"}
	}
	ident := strings.TrimSpace(strings.ToLower(value))
	if ch == model.BindChannelPhone && !phoneRe.MatchString(ident) {
		return nil, &UserError{"手机号格式不合法"}
	}
	if ch == model.BindChannelEmail && !emailRe.MatchString(ident) {
		return nil, &UserError{"邮箱格式不合法"}
	}
	var u model.User
	if err := s.DB.WithContext(ctx).Where("id = ? AND is_deleted = 0", userID).First(&u).Error; err != nil {
		return nil, ErrUserNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPwd)) != nil {
		return nil, &UserError{"当前密码不正确"}
	}
	// 目标通道占用检查
	var cnt int64
	s.DB.WithContext(ctx).Model(&model.User{}).
		Where("(phone = ? OR email = ?) AND id <> ? AND is_deleted = 0", ident, ident, userID).Count(&cnt)
	if cnt > 0 {
		return nil, &UserError{"该账号已被其他用户绑定"}
	}
	updates := map[string]any{"token_version": gorm.Expr("token_version + 1")} // 换绑=安全操作：踢其他设备
	if ch == model.BindChannelPhone {
		updates["phone"] = ident
	} else {
		updates["email"] = ident
	}
	if err := s.DB.WithContext(ctx).Model(&u).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.DB.WithContext(ctx).First(&u, userID).Error
	return &u, nil
}

func (s *AccountService) ChangePassword(ctx context.Context, userID int64, oldPwd, newPwd string) (*model.User, error) {
	if len(newPwd) < pwdMinLen {
		return nil, &UserError{"新密码至少 8 位"}
	}
	var u model.User
	if err := s.DB.WithContext(ctx).Where("id = ? AND is_deleted = 0", userID).First(&u).Error; err != nil {
		return nil, ErrUserNotFound
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPwd)) != nil {
		return nil, &UserError{"当前密码不正确"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	// 改密=安全操作：token_version +1（旧会话全失效；调用方用返回的版本给当前设备顺发新 token）
	if err := s.DB.WithContext(ctx).Model(&u).Updates(map[string]any{
		"password_hash": string(hash),
		"token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateProfileInput 部分更新（nil=不动该字段）
type UpdateProfileInput struct {
	NickName    *string `json:"nick_name"`
	AvatarEmoji *string `json:"avatar_emoji"`
	AvatarBg    *string `json:"avatar_bg"`
}

// UpdateProfile 更新昵称 / 头像表情 / 头像底色（三者任意子集）。
func (s *AccountService) UpdateProfile(ctx context.Context, userID int64, in UpdateProfileInput) (*model.User, error) {
	updates := map[string]any{}
	if in.NickName != nil {
		nick := strings.TrimSpace(*in.NickName)
		if nick == "" || len([]rune(nick)) > 64 {
			return nil, &UserError{"昵称 1-64 字"}
		}
		updates["nick_name"] = nick
	}
	if in.AvatarEmoji != nil {
		e := strings.TrimSpace(*in.AvatarEmoji)
		if len([]rune(e)) > 4 {
			return nil, &UserError{"头像表情过长（≤4 字符）"}
		}
		updates["avatar_emoji"] = e
	}
	if in.AvatarBg != nil {
		b := model.AvatarBg(strings.TrimSpace(*in.AvatarBg))
		if !b.Valid() {
			return nil, &UserError{"头像底色不合法（yellow/pink/green/blue/orange/purple/red 或空）"}
		}
		updates["avatar_bg"] = string(b)
	}
	if len(updates) == 0 {
		return nil, &UserError{"没有要更新的字段（nick_name/avatar_emoji/avatar_bg）"}
	}
	var u model.User
	if err := s.DB.WithContext(ctx).Model(&u).Where("id = ? AND is_deleted = 0", userID).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).First(&u, userID).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

// Me 当前用户信息（token 探活）
func (s *AccountService) Me(ctx context.Context, userID int64) (*model.User, error) {
	var u model.User
	if err := s.DB.WithContext(ctx).Where("id = ? AND is_deleted = 0", userID).First(&u).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

var ErrNickRequired = errors.New("nick required")
