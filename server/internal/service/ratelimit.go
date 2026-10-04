package service

import (
	"sync"
	"time"
)

// MemRateLimiter —— 每 API Key 固定窗口内存限流（单机部署；分布式留 v2 Redis/sys_cache）。
// 默认 60 次/分钟，超限 429。
type MemRateLimiter struct {
	mu     sync.Mutex
	bucket map[string]*winCounter
	limit  int
	window time.Duration
}

type winCounter struct {
	count int
	start time.Time
}

func NewMemRateLimiter(limit int, window time.Duration) *MemRateLimiter {
	return &MemRateLimiter{bucket: map[string]*winCounter{}, limit: limit, window: window}
}

// Allow 通过返回 true；超限 false。key 建议 "k<api_key_id>"。
func (r *MemRateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	w, ok := r.bucket[key]
	if !ok || now.Sub(w.start) >= r.window {
		if len(r.bucket) > 100_000 { // 防无限膨胀：超量整体重置
			r.bucket = map[string]*winCounter{}
		}
		r.bucket[key] = &winCounter{count: 1, start: now}
		return true
	}
	w.count++
	return w.count <= r.limit
}
