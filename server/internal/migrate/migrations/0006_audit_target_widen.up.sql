-- 审计 target 加宽：笔记 path 上限 500，move 的 target 为 "from → to"（可达约 1000+），
-- 255 会超长导致整批审计 INSERT 失败（批量一败俱丢）。PG 加宽 varchar 仅改元数据，无重写。
ALTER TABLE tl_api_log ALTER COLUMN target TYPE VARCHAR(1200);
