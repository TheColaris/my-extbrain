-- 0008: 账户资料扩展（头像表情/底色 + 会话版本）
-- token_version：改密/换绑后 +1，旧 JWT（带旧版本）全部失效=其他设备退出登录
ALTER TABLE tu_user ADD COLUMN IF NOT EXISTS avatar_emoji varchar(8)  NOT NULL DEFAULT '';
ALTER TABLE tu_user ADD COLUMN IF NOT EXISTS avatar_bg    varchar(16) NOT NULL DEFAULT '';
ALTER TABLE tu_user ADD COLUMN IF NOT EXISTS token_version int        NOT NULL DEFAULT 0;
COMMENT ON COLUMN tu_user.avatar_emoji  IS '头像表情（emoji，空=字母默认）';
COMMENT ON COLUMN tu_user.avatar_bg     IS '头像底色（主题色名：yellow/pink/lime/cyan/purple/red/blue，空=白）';
COMMENT ON COLUMN tu_user.token_version IS '会话版本：改密/换绑时 +1，JWT 携带版本，不匹配即 401';
