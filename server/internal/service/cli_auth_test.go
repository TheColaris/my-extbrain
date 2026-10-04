package service

import (
	"context"
	"os"
	"testing"
)

func TestCLIAuthFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置")
	}
	db := testDB(t)
	ctx := context.Background()
	svc := &CLIAuthService{Cache: &CacheService{DB: db}, Keys: &APIKeyService{DB: db}}

	code, err := svc.Start(ctx)
	if err != nil || len(code) != 8 {
		t.Fatalf("start: %v code=%q", err, code)
	}
	// 未授权 → pending
	st, key, _ := svc.Poll(ctx, code)
	if st != "pending" || key != "" {
		t.Fatalf("应 pending: %s %s", st, key)
	}
	// 授权（自动签发 Key）
	if err := svc.Approve(ctx, 51, code); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 重复授权应被拒（已消费 pending 状态）
	if err := svc.Approve(ctx, 51, code); err == nil {
		t.Fatal("重复 approve 应报错")
	}
	// poll 一次性取走
	st, key, _ = svc.Poll(ctx, code)
	if st != "approved" || key == "" {
		t.Fatalf("应 approved 带 key: %s %q", st, key)
	}
	if len(key) != len("ak_live_")+64 {
		t.Fatalf("key 格式异常: %q", key)
	}
	// 二次 poll → 过期/失效
	st, _, _ = svc.Poll(ctx, code)
	if st != "expired" {
		t.Fatalf("二次 poll 应失效: %s", st)
	}
	// 不存在的码
	st, _, _ = svc.Poll(ctx, "ZZZZZZZZ")
	if st != "expired" {
		t.Fatalf("未知码应 expired: %s", st)
	}
}
