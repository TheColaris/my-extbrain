-- 0013: E2E 洁净室信箱（tl_e2e_mailbox）——E2E 模式下 webhook/浏览器推送不外发，截获落此表供旅程断言。
-- 仅洁净环境写入（三重门全开时 Capture 才会落行）；生产库为空表，无行为差异。
CREATE TABLE IF NOT EXISTS tl_e2e_mailbox (
    id          bigserial PRIMARY KEY,
    channel     varchar(20)  NOT NULL,              -- 截获通道（E2EChannel）：webhook/webpush
    target      varchar(500) NOT NULL DEFAULT '',   -- 目标标识：webhook URL / push endpoint
    title       varchar(255) NOT NULL DEFAULT '',
    body        varchar(2000) NOT NULL DEFAULT '',
    payload     text         NOT NULL DEFAULT '',   -- 完整外发载荷（请求体 JSON）
    create_time timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_e2e_mailbox_channel_time ON tl_e2e_mailbox (channel, id DESC);
COMMENT ON TABLE  tl_e2e_mailbox         IS 'E2E 洁净室信箱（三重门全开时截获外发内容；生产恒空表）';
COMMENT ON COLUMN tl_e2e_mailbox.channel IS '截获通道：webhook=钉钉/飞书机器人 webpush=浏览器推送';
COMMENT ON COLUMN tl_e2e_mailbox.target  IS '目标标识：webhook URL（含签名参数）/ push endpoint';
COMMENT ON COLUMN tl_e2e_mailbox.payload IS '完整外发载荷 JSON（断言加签参数/消息结构用）';
