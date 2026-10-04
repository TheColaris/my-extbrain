package service

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"
)

// CLIAuthService —— CLI 设备授权（gh auth login 同款流程）：
// start 发码（公开）→ 用户在浏览器登录并 approve（JWT，自动签发一把 Key）→ poll 一次性取走 Key。
type CLIAuthService struct {
	Cache *CacheService
	Keys  *APIKeyService
}

const (
	cliAuthTTL    = 5 * time.Minute
	cliAuthPrefix = "cli_auth:"
	codeChars     = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // 去易混 I/O/0/1
)

var (
	ErrCLIAuthExpired  = errors.New("授权请求不存在或已过期，请在终端重新发起")
	ErrCLIAuthConsumed = errors.New("授权码已被使用")
)

func (s *CLIAuthService) key(code string) string {
	return cliAuthPrefix + strings.ToUpper(strings.TrimSpace(code))
}

// Start 生成 8 位授权码，状态=pending
func (s *CLIAuthService) Start(ctx context.Context) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, x := range b {
		sb.WriteByte(codeChars[int(x)%len(codeChars)])
	}
	code := sb.String()
	if err := s.Cache.Set(ctx, s.key(code), "pending", cliAuthTTL); err != nil {
		return "", err
	}
	return code, nil
}

// Approve 浏览器侧确认：校验 pending → 自动签发一把 Key → 状态置 approved:key
func (s *CLIAuthService) Approve(ctx context.Context, userID int64, code string) error {
	v, err := s.Cache.Get(ctx, s.key(code))
	if err != nil {
		return ErrCLIAuthExpired
	}
	if v != "pending" {
		return ErrCLIAuthConsumed
	}
	_, full, err := s.Keys.Issue(ctx, userID, IssueInput{Name: "CLI 授权"})
	if err != nil {
		return err
	}
	return s.Cache.Set(ctx, s.key(code), "approved:"+full, cliAuthTTL)
}

// Poll 终端侧轮询：pending → 等待；approved → 一次性取走 Key（原子取删，取走即失效）。
func (s *CLIAuthService) Poll(ctx context.Context, code string) (status string, fullKey string, err error) {
	// 原子取删：两个并发 poll 只有一个能拿到 approved 行（此前 Get+Del 两步可被竞态击穿）
	if v, ok, err := s.Cache.PopPrefix(ctx, s.key(code), "approved:"); err != nil {
		return "expired", "", nil
	} else if ok {
		return "approved", strings.TrimPrefix(v, "approved:"), nil
	}
	if v, err := s.Cache.Get(ctx, s.key(code)); err == nil && v == "pending" {
		return "pending", "", nil
	}
	return "expired", "", nil
}
