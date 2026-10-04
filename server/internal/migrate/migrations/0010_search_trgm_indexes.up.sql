-- 0010 检索扩展性：关键词路 trgm 索引（ILIKE 走索引，百万级不再全表扫正文）
-- 背景：note/todo/memo 三域关键词搜索是 ILIKE '%q%'，无索引=每查全表扫正文；
--       pg_trgm GIN 索引让 ILIKE 走位图索引扫描（中英文均生效，含 2 字短词）。
-- 注意：本迁移在事务内普通 CREATE INDEX（当前量级瞬时完成）；
--       未来大表补建索引须改 CREATE INDEX CONCURRENTLY（届时需迁移工具支持非事务）。

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 知识库（搜索主力：title/path 两列 + 正文独立表）
CREATE INDEX idx_note_title_trgm ON tf_note USING gin (title gin_trgm_ops);
CREATE INDEX idx_note_path_trgm ON tf_note USING gin (path gin_trgm_ops);
CREATE INDEX idx_note_content_content_trgm ON tf_note_content USING gin (content gin_trgm_ops);

-- 便签 / 待办（短文本，量级同步增长，顺手铺平）
CREATE INDEX idx_memo_content_trgm ON tf_memo USING gin (content gin_trgm_ops);
CREATE INDEX idx_todo_title_trgm ON tf_todo USING gin (title gin_trgm_ops);
CREATE INDEX idx_todo_remark_trgm ON tf_todo USING gin (remark gin_trgm_ops);
