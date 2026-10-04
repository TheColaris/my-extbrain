package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CacheService —— sys_cache（UNLOGGED）键值缓存：
// 惰性过期（读取时判定）、计数器原子自增（ON CONFLICT）、重启即清空。
type CacheService struct{ DB *gorm.DB }

var ErrCacheExpired = errors.New("cache expired")

// Get 读缓存；不存在或已过期返回 ErrCacheExpired（顺手删除过期行）。
func (s *CacheService) Get(ctx context.Context, key string) (string, error) {
	var e model.CacheEntry
	if err := s.DB.WithContext(ctx).First(&e, "cache_key = ?", key).Error; err != nil {
		return "", ErrCacheExpired
	}
	if time.Now().After(e.ExpireTime) {
		s.DB.WithContext(ctx).Delete(&model.CacheEntry{}, "cache_key = ?", key)
		return "", ErrCacheExpired
	}
	return e.CacheValue, nil
}

// Set 覆盖写缓存
func (s *CacheService) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	e := model.CacheEntry{CacheKey: key, CacheValue: value, ExpireTime: time.Now().Add(ttl)}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cache_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"cache_value", "expire_time"}),
	}).Create(&e).Error
}

// Incr 固定窗口计数器：原子自增并回读新值（不存在从 1 起）；
// 窗口从首次创建起算（冲突分支不动 expire_time）。
func (s *CacheService) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if err := s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "cache_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"cache_value": gorm.Expr("(sys_cache.cache_value::bigint + 1)::text"),
		}),
	}).Create(&model.CacheEntry{CacheKey: key, CacheValue: "1", ExpireTime: time.Now().Add(ttl)}).Error; err != nil {
		return 0, err
	}
	var e model.CacheEntry
	if err := s.DB.WithContext(ctx).First(&e, "cache_key = ?", key).Error; err != nil {
		return 0, err
	}
	n, _ := strconv.ParseInt(e.CacheValue, 10, 64)
	return n, nil
}

// GetInt 读整数缓存（0 = 不存在/过期）
func (s *CacheService) GetInt(ctx context.Context, key string) int64 {
	v, err := s.Get(ctx, key)
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// Del 删除缓存（解锁/重置用）
func (s *CacheService) Del(ctx context.Context, key string) {
	s.DB.WithContext(ctx).Delete(&model.CacheEntry{}, "cache_key = ?", key)
}

// PopPrefix 原子取删：仅当 cache_value 以 prefix 开头时删除并返回该值；
// 不存在/过期/前缀不符返回 ok=false（行保留）。DELETE…RETURNING 保证并发只有一个调用方取到。
// prefix 须不含 LIKE 通配符（调用方传字面量前缀）。
func (s *CacheService) PopPrefix(ctx context.Context, key, prefix string) (string, bool, error) {
	var v string
	err := s.DB.WithContext(ctx).Raw(
		`DELETE FROM sys_cache WHERE cache_key = ? AND cache_value LIKE ? RETURNING cache_value`,
		key, prefix+"%").Scan(&v).Error
	if err != nil {
		return "", false, err
	}
	if v == "" {
		return "", false, nil
	}
	return v, true, nil
}
