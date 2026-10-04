-- 0010 回滚：删 trgm 索引（不 DROP EXTENSION pg_trgm——可能被其它对象引用）
DROP INDEX IF EXISTS idx_todo_remark_trgm;
DROP INDEX IF EXISTS idx_todo_title_trgm;
DROP INDEX IF EXISTS idx_memo_content_trgm;
DROP INDEX IF EXISTS idx_note_content_content_trgm;
DROP INDEX IF EXISTS idx_note_path_trgm;
DROP INDEX IF EXISTS idx_note_title_trgm;
