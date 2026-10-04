-- 回滚：收窄回 255（若已有超长数据需先手工清理）
ALTER TABLE tl_api_log ALTER COLUMN target TYPE VARCHAR(255);
