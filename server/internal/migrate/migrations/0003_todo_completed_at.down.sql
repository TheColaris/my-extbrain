-- 回滚：删除 completed_at 列（数据丢失，仅回滚用）。
ALTER TABLE tf_todo DROP COLUMN IF EXISTS completed_at;
