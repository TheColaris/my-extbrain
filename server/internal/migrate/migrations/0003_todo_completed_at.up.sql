-- tf_todo 增加 completed_at：完成时间。
-- 语义：status 流转到 done 时写入；从 done 恢复为非 done 时清空。
-- 幂等：IF NOT EXISTS（重复执行安全）。

ALTER TABLE tf_todo ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ NULL;
COMMENT ON COLUMN tf_todo.completed_at IS '完成时间；done 时写入，恢复非 done 时清空';
