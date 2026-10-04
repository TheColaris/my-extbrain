// Package auth —— Web 会话 JWT（HS256；API Key 认证在 api 中间件层）。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const TokenTTL = 7 * 24 * time.Hour

type Manager struct {
	secret []byte
}

func NewManager(secret string) *Manager {
	return &Manager{secret: []byte(secret)}
}

// RandomSecret 生成 32 字节随机串（JWT_SECRET 未配置时兜底，重启即全体重登）
func RandomSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Claims struct {
	UserID   int64  `json:"uid"`
	NickName string `json:"nick"`
	Ver      int    `json:"ver"` // token 版本（改密/换绑后 +1 → 旧会话全部失效）
	jwt.RegisteredClaims
}

func (m *Manager) Sign(userID int64, nickName string, ver int) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID:   userID,
		NickName: nickName,
		Ver:      ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "web",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(TokenTTL)),
		},
	})
	return t.SignedString(m.secret)
}

func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 { // 钉死 HS256：HS384/512 同密钥签名也不收
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("token 无效或已过期")
	}
	cl, ok := t.Claims.(*Claims)
	if !ok || cl.UserID == 0 {
		return nil, errors.New("token 载荷异常")
	}
	return cl, nil
}
