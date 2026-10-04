-- 0005 down：删推送记录表与补充索引
DROP INDEX IF EXISTS idx_log_create_time;
DROP TABLE IF EXISTS tl_push_log;
