-- 0007: 笔记分享（tf_note_share）——快照 token 机制
-- 一篇笔记仅一个活跃分享（部分唯一索引：撤销后可重建新 token）；快照冻结内容或 live 跟随原笔记
CREATE TABLE IF NOT EXISTS tf_note_share (
    id           bigserial PRIMARY KEY,
    user_id      bigint       NOT NULL,
    note_id      bigint       NOT NULL,
    token        varchar(64)  NOT NULL,
    title        varchar(255) NOT NULL DEFAULT '',
    content      text         NOT NULL DEFAULT '',          -- 快照内容（mode=snapshot 读取用；live 冻结时留档）
    content_hash char(64)     NOT NULL DEFAULT '',
    tags         text[]       NOT NULL DEFAULT '{}',
    mode         varchar(8)   NOT NULL DEFAULT 'snapshot',  -- ShareMode：snapshot=快照冻结 / live=跟随更新
    expire_time  timestamptz,                                -- NULL=永久
    view_count   bigint       NOT NULL DEFAULT 0,
    is_revoked   smallint     NOT NULL DEFAULT 0,            -- 撤销=终态，行保留（历史）
    create_time  timestamptz  NOT NULL DEFAULT now(),
    update_time  timestamptz  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_note_share_token ON tf_note_share (token);
CREATE UNIQUE INDEX IF NOT EXISTS uk_note_share_active ON tf_note_share (user_id, note_id) WHERE is_revoked = 0;
CREATE INDEX IF NOT EXISTS idx_note_share_user ON tf_note_share (user_id);
COMMENT ON TABLE  tf_note_share        IS '笔记分享表（公开只读快照/live token；一篇笔记一个活跃分享）';
COMMENT ON COLUMN tf_note_share.mode   IS 'snapshot=快照冻结 live=跟随原笔记实时内容';
COMMENT ON COLUMN tf_note_share.expire_time IS 'NULL=永久；过期后公开读取返回失效';
