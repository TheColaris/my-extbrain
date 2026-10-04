-- sys_cache 系统缓存表（UNLOGGED）
-- 键值 + TTL 惰性清理，
-- 登录失败锁 / 限流计数 / 验证码共用；UNLOGGED 重启即清空（此类数据可丢）。

CREATE UNLOGGED TABLE sys_cache (
  cache_key   VARCHAR(255) PRIMARY KEY,
  cache_value TEXT        NOT NULL,
  expire_time TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_cache_expire ON sys_cache (expire_time);
COMMENT ON TABLE  sys_cache              IS '系统缓存表（UNLOGGED：重启即清空；登录失败锁/限流/验证码共用）';
COMMENT ON COLUMN sys_cache.cache_key   IS '缓存键，带用途前缀：pwd_fail: / rl: / code:';
COMMENT ON COLUMN sys_cache.cache_value IS '缓存值（文本，结构由调用方约定）';
COMMENT ON COLUMN sys_cache.expire_time IS '过期时间；读取时惰性判定，过期即删';
