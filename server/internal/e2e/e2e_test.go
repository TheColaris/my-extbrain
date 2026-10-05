package e2e

import (
	"os"
	"strings"
	"testing"

	"extbrain-server/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDB 与 service 层集成测试同口径：TEST_DATABASE_URL；未配置则跳过。
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
	if err := db.AutoMigrate(&model.SystemConfig{}, &model.E2EMailbox{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return db
}

// forceEnable 走正经门路径强制启用（写标记行 + 三门 Evaluate），测试结束复位。
func forceEnable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Create(&model.SystemConfig{ConfigKey: MarkerKey, ConfigValue: "1"}).Error; err != nil {
		t.Fatalf("写标记行失败: %v", err)
	}
	if err := Evaluate(true, "postgres://u:p@127.0.0.1:5432/"+CleanDBName, db); err != nil {
		t.Fatalf("强制启用失败: %v", err)
	}
	t.Cleanup(func() {
		db.Where("config_key = ?", MarkerKey).Delete(&model.SystemConfig{})
		mu.Lock()
		enabled = false
		mu.Unlock()
	})
}

func TestEvaluateGate1Off(t *testing.T) {
	// 门①不开：静默禁用，无论门②③如何
	if err := Evaluate(false, "postgres://u:p@127.0.0.1:5432/some-db", nil); err != nil {
		t.Fatalf("门①关闭应静默通过: %v", err)
	}
	if Enabled() {
		t.Fatal("门①关闭时不应启用")
	}
}

func TestEvaluateGate2DBName(t *testing.T) {
	// 门②：E2E_MODE=true 但库名不是洁净库 → 必须报错（fail-closed）
	err := Evaluate(true, "postgres://u:p@127.0.0.1:5432/extbrain?sslmode=disable", nil)
	if err == nil || !strings.Contains(err.Error(), "门②") {
		t.Fatalf("库名不符应报门②错误, got: %v", err)
	}
	if Enabled() {
		t.Fatal("门②失败时不应启用")
	}
}

func TestEvaluateGate3Marker(t *testing.T) {
	db := testDB(t)
	// 门③测试自管理标记行：先清（集成库可能被洁净室 seed 过），结束再清并复位运行时
	db.Where("config_key = ?", MarkerKey).Delete(&model.SystemConfig{})
	t.Cleanup(func() {
		db.Where("config_key = ?", MarkerKey).Delete(&model.SystemConfig{})
		mu.Lock()
		enabled = false
		mu.Unlock()
	})

	// 门③：库名对但无标记行 → 报错
	cleanURL := "postgres://u:p@127.0.0.1:5432/" + CleanDBName
	if err := Evaluate(true, cleanURL, db); err == nil || !strings.Contains(err.Error(), "门③") {
		t.Fatalf("缺标记行应报门③错误, got: %v", err)
	}
	if Enabled() {
		t.Fatal("门③失败时不应启用")
	}

	// 写入标记行 → 三门全过 → enabled
	if err := db.Create(&model.SystemConfig{ConfigKey: MarkerKey, ConfigValue: "1"}).Error; err != nil {
		t.Fatalf("写标记行失败: %v", err)
	}
	if err := Evaluate(true, cleanURL, db); err != nil {
		t.Fatalf("三门全过应启用: %v", err)
	}
	if !Enabled() {
		t.Fatal("三门全过应 Enabled=true")
	}
}

func TestDBName(t *testing.T) {
	cases := []struct{ url, want string }{
		{"postgres://u:p@127.0.0.1:5432/extbrain?sslmode=disable", "extbrain"},
		{"postgres://u:p@host/db", "db"},
		{"postgres://u:p@host/extbrain_e2e", CleanDBName},
		{"not-a-url", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := dbName(c.url); got != c.want {
			t.Fatalf("dbName(%q)=%q want %q", c.url, got, c.want)
		}
	}
}

func TestCaptureDisabledNoop(t *testing.T) {
	// 未启用时 Capture 必须是无害 no-op（不落库不 panic）
	mu.Lock()
	enabled = false
	mu.Unlock()
	Capture(nil, ChannelWebhook, "https://x", "t", "b", "{}")
}

func TestCapturePersists(t *testing.T) {
	db := testDB(t)
	forceEnable(t, db)
	t.Cleanup(func() { db.Exec("TRUNCATE tl_e2e_mailbox") })

	longTarget := strings.Repeat("t", 600)
	longTitle := strings.Repeat("标", 300)
	Capture(db, ChannelWebhook, longTarget, longTitle, "body 内容", `{"payload":"x"}`)

	var rows []model.E2EMailbox
	if err := db.Order("id DESC").Limit(1).Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("信箱应落一行: err=%v rows=%d", err, len(rows))
	}
	r := rows[0]
	if r.Channel != ChannelWebhook || r.Target != strings.Repeat("t", 500) {
		t.Fatalf("channel/target 不符: %+v", r)
	}
	if r.Title != strings.Repeat("标", 255) {
		t.Fatalf("title 应按 rune 截断到 255: len=%d", len([]rune(r.Title)))
	}
	if r.Body != "body 内容" || r.Payload != `{"payload":"x"}` {
		t.Fatalf("body/payload 不符: %+v", r)
	}

	// 捕获计数：多落一条，条数增长
	Capture(db, ChannelWebPush, "https://fcm.test/sub", "t2", "b2", "{}")
	var n int64
	db.Model(&model.E2EMailbox{}).Count(&n)
	if n != 2 {
		t.Fatalf("两次 Capture 应落两行, got %d", n)
	}
}

func TestCut(t *testing.T) {
	if got := cut("hello", 3); got != "hel" {
		t.Fatalf("cut(ascii)=%q", got)
	}
	if got := cut("旅程截断测试", 2); got != "旅程" {
		t.Fatalf("cut(中文 rune)=%q", got)
	}
	if got := cut("短", 10); got != "短" {
		t.Fatalf("短串不应截断: %q", got)
	}
	if got := cut("", 5); got != "" {
		t.Fatalf("空串: %q", got)
	}
}

func TestCaptureDBErrorStillSilent(t *testing.T) {
	db := testDB(t)
	forceEnable(t, db)
	// 表不存在 → Create 失败 → 只打日志，绝不向上返回（E2E 底线：调用方语义恒为已捕获）
	db.Exec("DROP TABLE tl_e2e_mailbox")
	Capture(db, ChannelWebhook, "https://x.test", "t", "b", "{}")
	// 共享集成库：恢复表，避免影响其他包的测试
	if err := db.AutoMigrate(&model.E2EMailbox{}); err != nil {
		t.Fatalf("恢复信箱表失败: %v", err)
	}
}
