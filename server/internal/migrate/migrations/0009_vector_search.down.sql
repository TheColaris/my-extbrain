-- 0009 回滚：删切片表/配置表/管理员列（不 DROP EXTENSION vector——可能被其它对象引用）
DROP TABLE IF EXISTS tf_note_chunk;
DROP INDEX IF EXISTS uk_system_config_key;
DROP TABLE IF EXISTS tp_system_config;
ALTER TABLE tu_user DROP COLUMN IF EXISTS is_admin;
