-- 0004 待办 v3：状态二态化 + 排序偏好按用户存库
-- 状态模型 todo/doing/done → active/done：doing 并入 active（completed_at 本为空，无信息丢失）。
-- 回滚：active 回写 todo（原 doing 的信息不可恢复，down 仅保结构可退）。

UPDATE tf_todo SET status = 'active', update_time = now() WHERE status IN ('todo', 'doing');
ALTER TABLE tf_todo ALTER COLUMN status SET DEFAULT 'active';
COMMENT ON COLUMN tf_todo.status IS '状态：active=在途 done=已完成';

ALTER TABLE tu_user ADD COLUMN todo_sort VARCHAR(16) NOT NULL DEFAULT 'created';
COMMENT ON COLUMN tu_user.todo_sort IS '待办列表排序偏好：created=创建时间倒序（默认） due=截止升序无截止在后';
