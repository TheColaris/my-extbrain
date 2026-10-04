-- 0008 down
ALTER TABLE tu_user DROP COLUMN IF EXISTS avatar_emoji;
ALTER TABLE tu_user DROP COLUMN IF EXISTS avatar_bg;
ALTER TABLE tu_user DROP COLUMN IF EXISTS token_version;
