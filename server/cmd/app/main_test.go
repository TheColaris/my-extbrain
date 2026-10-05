package main

import (
	"os"
	"testing"

	"extbrain-server/internal/e2e"
	"extbrain-server/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 洁净模式限流放宽：默认值（生产口径）与三门全开后的放宽值都要锁死。
func TestRateLimitDefaults(t *testing.T) {
	// 门①关闭（默认）→ 生产默认配额
	if err := e2e.Evaluate(false, "", nil); err != nil {
		t.Fatalf("门①关闭应静默: %v", err)
	}
	if e2ePubLimit() != 10 {
		t.Fatalf("普通模式 pub 限流应 10, got %d", e2ePubLimit())
	}
	if e2eV1Limit() != 240 {
		t.Fatalf("普通模式 v1 限流应 240, got %d", e2eV1Limit())
	}
}

func TestRateLimitE2ERelaxed(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db, err := gorm.Open(postgres.Open(url), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("测试库不可达: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	db.Where("config_key = ?", e2e.MarkerKey).Delete(&model.SystemConfig{})
	t.Cleanup(func() {
		db.Where("config_key = ?", e2e.MarkerKey).Delete(&model.SystemConfig{})
		_ = e2e.Evaluate(false, "", nil)
	})

	if err := db.Create(&model.SystemConfig{ConfigKey: e2e.MarkerKey, ConfigValue: "1"}).Error; err != nil {
		t.Fatalf("写标记行失败: %v", err)
	}
	cleanURL := "postgres://u:p@127.0.0.1:5432/" + e2e.CleanDBName
	if err := e2e.Evaluate(true, cleanURL, db); err != nil {
		t.Fatalf("三门全过应启用: %v", err)
	}
	if e2ePubLimit() != 200 {
		t.Fatalf("洁净模式 pub 限流应放宽到 200, got %d", e2ePubLimit())
	}
	if e2eV1Limit() != 1200 {
		t.Fatalf("洁净模式 v1 限流应放宽到 1200, got %d", e2eV1Limit())
	}
}
