package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

const keyPrefix = "ak_live_"

var keyNameRe = regexp.MustCompile(`^.{1,64}$`)

// APIKeyService —— 签发/列表/编辑（名称+权限）/吊销。
type APIKeyService struct{ DB *gorm.DB }

type IssueInput struct {
	Name       string `json:"name" binding:"required"`
	ExpireDays int    `json:"expire_days"` // 0=永不过期；7/30/90 合法
}

type UpdateInput struct {
	Name  *string         `json:"name"`
	Scope *model.KeyScope `json:"scope"`
}

// Issue 签发：返回一次性的完整 Key（服务端只存 SHA-256）。
var validExpireDays = map[int]bool{0: true, 7: true, 30: true, 90: true}

func (s *APIKeyService) Issue(ctx context.Context, userID int64, in IssueInput) (*model.APIKey, string, error) {
	if !keyNameRe.MatchString(in.Name) {
		return nil, "", &UserError{"密钥名称 1-64 字符"}
	}
	if !validExpireDays[in.ExpireDays] {
		return nil, "", &UserError{"expire_days 仅支持 0（永久）/ 7 / 30 / 90"}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", err
	}
	full := keyPrefix + hex.EncodeToString(raw)
	k := &model.APIKey{
		UserID:  userID,
		KeyName: in.Name,
		KeyHash: sha256Hex(full),
		KeyHint: hint(full),
		Scope:   model.KeyScopeAll,
	}
	if in.ExpireDays > 0 {
		t := time.Now().AddDate(0, 0, in.ExpireDays)
		k.ExpireTime = &t
	}
	if err := s.DB.WithContext(ctx).Create(k).Error; err != nil {
		return nil, "", err
	}
	return k, full, nil
}

func (s *APIKeyService) List(ctx context.Context, userID int64) ([]model.APIKey, error) {
	ks := make([]model.APIKey, 0)
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 0 AND is_revoked = 0", userID).
		Order("id DESC").Find(&ks).Error
	return ks, err
}

// Update 编辑名称/权限（PATCH 语义：nil=不改）
func (s *APIKeyService) Update(ctx context.Context, userID, keyID int64, in UpdateInput) (*model.APIKey, error) {
	if in.Name != nil && !keyNameRe.MatchString(*in.Name) {
		return nil, &UserError{"密钥名称 1-64 字符"}
	}
	if in.Scope != nil && !in.Scope.Valid() {
		return nil, &UserError{"scope 必须是 all / todo / notes"}
	}
	var k model.APIKey
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = 0 AND is_revoked = 0", keyID, userID).
		First(&k).Error; err != nil {
		return nil, ErrKeyNotFound
	}
	updates := map[string]any{}
	if in.Name != nil {
		updates["key_name"] = *in.Name
	}
	if in.Scope != nil {
		updates["scope"] = *in.Scope
	}
	if len(updates) == 0 {
		return &k, nil
	}
	updates["update_time"] = time.Now()
	if err := s.DB.WithContext(ctx).Model(&k).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.DB.WithContext(ctx).First(&k, k.ID).Error
	return &k, nil
}

// Revoke 吊销（不可逆；日志留痕）
func (s *APIKeyService) Revoke(ctx context.Context, userID, keyID int64) error {
	res := s.DB.WithContext(ctx).Model(&model.APIKey{}).
		Where("id = ? AND user_id = ? AND is_revoked = 0 AND is_deleted = 0", keyID, userID).
		Updates(map[string]any{"is_revoked": 1, "update_time": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrKeyNotFound
	}
	return nil
}

// VerifyByRawKey 认证中间件用：完整 Key → 记录（含吊销/过期校验）；不更新 last_use_time（由审计中间件节流更新）。
func (s *APIKeyService) VerifyByRawKey(ctx context.Context, fullKey string) (*model.APIKey, error) {
	var k model.APIKey
	err := s.DB.WithContext(ctx).
		Where("key_hash = ? AND is_revoked = 0 AND is_deleted = 0", sha256Hex(fullKey)).
		First(&k).Error
	if err != nil {
		return nil, errors.New("API Key 无效或已吊销")
	}
	if k.ExpireTime != nil && time.Now().After(*k.ExpireTime) {
		return nil, errors.New("API Key 已过期")
	}
	return &k, nil
}

var ErrKeyNotFound = errors.New("key not found")

// TouchLastUse 节流更新最近使用时间（认证中间件调用，>60s 一写）
func (s *APIKeyService) TouchLastUse(ctx context.Context, keyID int64) {
	now := time.Now()
	s.DB.WithContext(ctx).Model(&model.APIKey{}).Where("id = ?", keyID).
		Updates(map[string]any{"last_use_time": now, "update_time": now})
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hint(full string) string { return full[:13] + "…" + full[len(full)-4:] }
