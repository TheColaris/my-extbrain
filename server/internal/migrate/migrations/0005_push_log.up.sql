-- 0005: 通知推送记录（tl_push_log）——按事件留痕每次投递的成败与原因
CREATE TABLE IF NOT EXISTS tl_push_log (
    id            bigserial PRIMARY KEY,
    user_id       bigint       NOT NULL,
    event_type    varchar(16)  NOT NULL,              -- 事件类型（NotifyEventType）：due_today/overdue/test
    event_key     varchar(64)  NOT NULL DEFAULT '',   -- 事件标识（同事件多渠道共享）：due:<date>/overdue:<date>/test:<nano>
    channel_type  varchar(16)  NOT NULL,              -- 投递渠道（PushChannel）：web/dingtalk/feishu
    channel_id    bigint       NOT NULL DEFAULT 0,    -- 渠道或订阅 id（web=tf_push_subscription.id）
    title         varchar(255) NOT NULL,
    body          varchar(500) NOT NULL DEFAULT '',
    status        varchar(8)   NOT NULL,              -- 投递结果（PushStatus）：ok/fail
    error         varchar(255) NOT NULL DEFAULT '',   -- 失败原因（可执行文案）
    cost_ms       int          NOT NULL DEFAULT 0,
    retry_count   int          NOT NULL DEFAULT 0,    -- 重推次数（重推原地更新本行）
    create_time   timestamptz  NOT NULL DEFAULT now(),
    update_time   timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_push_log_user_time ON tl_push_log (user_id, create_time);
COMMENT ON TABLE  tl_push_log        IS '通知推送记录表（不可变+重推原地更新；保留期同 LOG_RETENTION_DAYS）';
COMMENT ON COLUMN tl_push_log.status IS 'ok=已送达 fail=失败（error 列=可执行原因）';

-- 审计日志与推送记录保留期清理（LOG_RETENTION_DAYS，此前只配未实现）
CREATE INDEX IF NOT EXISTS idx_log_create_time ON tl_api_log (create_time);
