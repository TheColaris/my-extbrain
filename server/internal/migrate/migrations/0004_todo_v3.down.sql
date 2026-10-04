-- 回滚 0004：先还原状态默认值与注释，active 回写 todo；排序偏好列删除。
-- 注意：原 doing 条目回滚后变为 todo（doing 语义已在升级时并入 active，down 不可逆恢复）。

ALTER TABLE tf_todo ALTER COLUMN status SET DEFAULT 'todo';
UPDATE tf_todo SET status = 'todo', update_time = now() WHERE status = 'active';
COMMENT ON COLUMN tf_todo.status IS '状态：todo=待办 doing=进行中 done=已完成';

ALTER TABLE tu_user DROP COLUMN IF EXISTS todo_sort;
