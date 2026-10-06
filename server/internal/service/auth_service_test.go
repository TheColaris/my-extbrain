package service

import (
	"context"
	"os"
	"testing"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/migrate"
	"extbrain-server/internal/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 集成测试库：TEST_DATABASE_URL（本地 compose 的 db，宿主 5433）；未配置则跳过。
// 首次连接先跑 SQL 迁移，保证测试库 schema 与 migrations 一致。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("测试库不可达: %v", err)
	}
	if err := migrate.Up(url); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	db.Exec("TRUNCATE tu_user, tu_api_key, sys_cache, tf_repo")
	return db
}

func TestRegisterLoginFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	cs := &CacheService{DB: db}
	svc := &AuthService{DB: db, Cache: cs, JWT: auth.NewManager("test-secret")}

	// 手机号注册已下掉（存量手机号账号登录保留，见下）
	if _, err := svc.Register(ctx, RegisterInput{Account: "13800000001", Password: "password123"}); err == nil {
		t.Fatal("手机号注册应被拒绝（当前仅支持邮箱注册）")
	}

	// 邮箱注册：验证码校验（测试直放缓存模拟发码）
	if err := cs.Set(ctx, emailCodeKey("user@example.com"), "123456", emailCodeTTL); err != nil {
		t.Fatalf("预置验证码失败: %v", err)
	}
	u2, err := svc.Register(ctx, RegisterInput{Account: "User@Example.COM ", Password: "password123", EmailCode: "123456"})
	if err != nil || u2.Email == nil || *u2.Email != "user@example.com" {
		t.Fatalf("邮箱注册失败（应归一小写+去空白）: %v email=%v", err, u2.Email)
	}
	// 验证码一次性消费
	if _, err := cs.Get(ctx, emailCodeKey("user@example.com")); err == nil {
		t.Fatal("验证码应已被消费删除")
	}

	// 缺码 / 错码
	if _, err := svc.Register(ctx, RegisterInput{Account: "a@b.com", Password: "password123"}); err == nil {
		t.Fatal("缺验证码应报错")
	}
	_ = cs.Set(ctx, emailCodeKey("a@b.com"), "654321", emailCodeTTL)
	if _, err := svc.Register(ctx, RegisterInput{Account: "a@b.com", Password: "password123", EmailCode: "000000"}); err == nil {
		t.Fatal("错误验证码应报错")
	}

	// 重复注册（占用检查先于验证码，命中"已注册"）
	if _, err := svc.Register(ctx, RegisterInput{Account: "user@example.com", Password: "password123", EmailCode: "123456"}); err == nil {
		t.Fatal("重复邮箱应报错")
	}

	// 非法输入
	if _, err := svc.Register(ctx, RegisterInput{Account: "12345", Password: "password123", EmailCode: "123456"}); err == nil {
		t.Fatal("非法账号应报错")
	}
	if _, err := svc.Register(ctx, RegisterInput{Account: "c@d.com", Password: "short", EmailCode: "123456"}); err == nil {
		t.Fatal("短密码应报错")
	}

	// 存量手机号账号：直接建行模拟（注册入口已关），登录仍可用
	phone := "13800000001"
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if err := db.Create(&model.User{Phone: &phone, PasswordHash: string(hash), NickName: "legacy"}).Error; err != nil {
		t.Fatalf("存量账号建行失败: %v", err)
	}
	_, token, err := svc.Login(ctx, LoginInput{Account: "13800000001", Password: "password123"}, "1.2.3.4")
	if err != nil || token == "" {
		t.Fatalf("存量手机号账号登录失败: %v", err)
	}
	if _, err := svc.JWT.Parse(token); err != nil {
		t.Fatalf("token 解析失败: %v", err)
	}

	// 密码错误 + 失败锁定
	for i := 0; i < loginFailLimit; i++ {
		if _, _, err := svc.Login(ctx, LoginInput{Account: "13800000001", Password: "wrong-pass"}, "1.2.3.4"); err == nil {
			t.Fatal("错误密码不应登录成功")
		}
	}
	if _, _, err := svc.Login(ctx, LoginInput{Account: "13800000001", Password: "password123"}, "1.2.3.4"); err == nil {
		t.Fatal("达到失败上限后正确密码也应被锁")
	}
}

func TestCacheIncrFixedWindow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	cs := &CacheService{DB: db}

	n, err := cs.Incr(ctx, "test:incr", loginFailWindow)
	if err != nil || n != 1 {
		t.Fatalf("首次 Incr 应为 1: n=%d err=%v", n, err)
	}
	n, _ = cs.Incr(ctx, "test:incr", loginFailWindow)
	n, _ = cs.Incr(ctx, "test:incr", loginFailWindow)
	if n != 3 {
		t.Fatalf("三次 Incr 应为 3: %d", n)
	}
	cs.Set(ctx, "test:str", "v", loginFailWindow)
	if v, err := cs.Get(ctx, "test:str"); err != nil || v != "v" {
		t.Fatalf("Get 失败: %v %q", err, v)
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ks := &APIKeyService{DB: db}

	k, full, err := ks.Issue(ctx, 1, IssueInput{Name: "工作机"})
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	if len(full) != len("ak_live_")+64 || full[:8] != "ak_live_" {
		t.Fatalf("Key 格式异常: %q", full)
	}
	if k.KeyHint == "" || k.Scope != model.KeyScopeAll {
		t.Fatalf("hint/scope 默认值异常: %+v", k)
	}

	// 认证：完整 Key 可反查
	got, err := ks.VerifyByRawKey(ctx, full)
	if err != nil || got.ID != k.ID {
		t.Fatalf("Verify 失败: %v", err)
	}
	if _, err := ks.VerifyByRawKey(ctx, "ak_live_deadbeef"); err == nil {
		t.Fatal("伪造 Key 不应通过")
	}

	// 编辑：改 scope
	ns := model.KeyScopeNotes
	upd, err := ks.Update(ctx, 1, k.ID, UpdateInput{Scope: &ns, Name: strPtr("服务器")})
	if err != nil || upd.Scope != model.KeyScopeNotes || upd.KeyName != "服务器" {
		t.Fatalf("编辑失败: %v %+v", err, upd)
	}
	// scope 越权值
	bad := model.KeyScope("root")
	if _, err := ks.Update(ctx, 1, k.ID, UpdateInput{Scope: &bad}); err == nil {
		t.Fatal("非法 scope 应报错")
	}

	// 吊销后不可认证
	if err := ks.Revoke(ctx, 1, k.ID); err != nil {
		t.Fatalf("吊销失败: %v", err)
	}
	if _, err := ks.VerifyByRawKey(ctx, full); err == nil {
		t.Fatal("吊销后不应通过认证")
	}
	// 他人 key 不可操作
	if err := ks.Revoke(ctx, 2, k.ID); err != ErrKeyNotFound {
		t.Fatalf("跨用户吊销应 ErrKeyNotFound: %v", err)
	}
}

func TestMemRateLimiter(t *testing.T) {
	r := NewMemRateLimiter(3, 1000000) // 窗口足够大
	for i := 0; i < 3; i++ {
		if !r.Allow("k1") {
			t.Fatal("前 3 次应放行")
		}
	}
	if r.Allow("k1") {
		t.Fatal("第 4 次应限流")
	}
	if !r.Allow("k2") {
		t.Fatal("不同 key 不互相影响")
	}
}

func strPtr(s string) *string { return &s }
