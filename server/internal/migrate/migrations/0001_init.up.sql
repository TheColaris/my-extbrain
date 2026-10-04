-- my-extbrain 初始迁移（up）
-- 规范：项目建表规范 + PG 适配条款（timestamptz / smallint / text[] / 无物理外键）
-- 说明：日志表 tl_api_log 不可变，update_time 按规范保留、值恒等于 create_time。

-- ============ 用户域 ============

CREATE TABLE tu_user (
  id            BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  phone         VARCHAR(20)  NULL,
  email         VARCHAR(255) NULL,
  password_hash VARCHAR(255) NOT NULL,
  nick_name     VARCHAR(64)  NOT NULL DEFAULT '',
  is_deleted    SMALLINT     NOT NULL DEFAULT 0,
  create_time   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_user_phone ON tu_user (phone) WHERE phone IS NOT NULL;
CREATE UNIQUE INDEX uk_user_email ON tu_user (email) WHERE email IS NOT NULL;
COMMENT ON TABLE  tu_user            IS '用户表';
COMMENT ON COLUMN tu_user.phone      IS '手机号；NULL=未绑定；phone/email 至少绑定一个';
COMMENT ON COLUMN tu_user.email      IS '邮箱；NULL=未绑定';
COMMENT ON COLUMN tu_user.password_hash IS '密码哈希（bcrypt）';
COMMENT ON COLUMN tu_user.nick_name  IS '昵称';
COMMENT ON COLUMN tu_user.is_deleted IS '逻辑删除：1=已删 0=正常';

CREATE TABLE tu_api_key (
  id            BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id       BIGINT       NOT NULL,
  key_name      VARCHAR(64)  NOT NULL,
  key_hash      CHAR(64)     NOT NULL,
  key_hint      VARCHAR(32)  NOT NULL,
  scope         VARCHAR(8)   NOT NULL DEFAULT 'all',
  expire_time   TIMESTAMPTZ  NULL,
  last_use_time TIMESTAMPTZ  NULL,
  is_revoked    SMALLINT     NOT NULL DEFAULT 0,
  is_deleted    SMALLINT     NOT NULL DEFAULT 0,
  create_time   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_api_key_hash ON tu_api_key (key_hash);
CREATE INDEX idx_api_key_user ON tu_api_key (user_id);
COMMENT ON TABLE  tu_api_key             IS 'API 密钥表';
COMMENT ON COLUMN tu_api_key.user_id     IS '归属用户 id（无物理外键，应用层保障）';
COMMENT ON COLUMN tu_api_key.key_name    IS '密钥名称';
COMMENT ON COLUMN tu_api_key.key_hash    IS '完整 Key 的 SHA-256 十六进制（64 定长）';
COMMENT ON COLUMN tu_api_key.key_hint    IS '展示用提示，如 ak_live_3f9a…e21b';
COMMENT ON COLUMN tu_api_key.scope      IS '权限：all=全部 todo=待办与便签 notes=知识库';
COMMENT ON COLUMN tu_api_key.expire_time IS '过期时间；NULL=永不过期';
COMMENT ON COLUMN tu_api_key.last_use_time IS '最近使用时间';
COMMENT ON COLUMN tu_api_key.is_revoked  IS '已吊销：1=是 0=否';

-- ============ 功能域 ============

CREATE TABLE tf_todo (
  id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT      NOT NULL,
  title       VARCHAR(255) NOT NULL,
  remark      VARCHAR(1000) NOT NULL DEFAULT '',
  status      VARCHAR(8)  NOT NULL DEFAULT 'todo',
  due_time    TIMESTAMPTZ NULL,
  tags        TEXT[]      NOT NULL DEFAULT '{}',
  source      VARCHAR(8)  NOT NULL DEFAULT 'web',
  api_key_id  BIGINT      NULL,
  is_deleted  SMALLINT    NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_todo_user_status ON tf_todo (user_id, status);
COMMENT ON TABLE  tf_todo          IS '待办表';
COMMENT ON COLUMN tf_todo.status   IS '状态：todo=待办 doing=进行中 done=已完成';
COMMENT ON COLUMN tf_todo.due_time IS '截止时间；NULL=无截止';
COMMENT ON COLUMN tf_todo.tags     IS '标签数组';
COMMENT ON COLUMN tf_todo.source   IS '来源：web=面板 cli=命令行';
COMMENT ON COLUMN tf_todo.api_key_id IS '经由的密钥 id；NULL=Web 会话创建';

CREATE TABLE tf_memo (
  id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT      NOT NULL,
  content     VARCHAR(2000) NOT NULL,
  tags        TEXT[]      NOT NULL DEFAULT '{}',
  is_pinned   SMALLINT    NOT NULL DEFAULT 0,
  is_deleted  SMALLINT    NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_memo_user_time ON tf_memo (user_id, create_time);
COMMENT ON TABLE  tf_memo           IS '便签表（碎片速记，超 2000 字引导转知识库）';
COMMENT ON COLUMN tf_memo.is_pinned IS '置顶：1=是 0=否';

CREATE TABLE tf_note (
  id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT       NOT NULL,
  path        VARCHAR(500) NOT NULL,
  title       VARCHAR(255) NOT NULL,
  tags        TEXT[]       NOT NULL DEFAULT '{}',
  is_deleted  SMALLINT     NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_note_user_path ON tf_note (user_id, path);
COMMENT ON TABLE  tf_note        IS '知识库笔记表（纯 Markdown，按 path 组织）';
COMMENT ON COLUMN tf_note.path   IS '笔记路径，如 ai/glm-5.3-使用笔记.md；用户内唯一';

CREATE TABLE tf_note_content (
  note_id      BIGINT      PRIMARY KEY,
  content      TEXT        NOT NULL,
  content_hash CHAR(64)    NOT NULL,
  update_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);
COMMENT ON TABLE  tf_note_content          IS '笔记正文表（规范：超长文本独立成表 1:1）';
COMMENT ON COLUMN tf_note_content.note_id  IS '对应 tf_note.id';
COMMENT ON COLUMN tf_note_content.content_hash IS '正文 SHA-256（乐观锁，PUT 可带 expected_hash 校验）';

CREATE TABLE tf_notify_channel (
  id               BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id          BIGINT       NOT NULL,
  channel_type     VARCHAR(8)   NOT NULL,
  webhook_url      VARCHAR(500) NOT NULL,
  secret           VARCHAR(255) NOT NULL DEFAULT '',
  is_enabled       SMALLINT     NOT NULL DEFAULT 1,
  last_push_time   TIMESTAMPTZ  NULL,
  last_push_result VARCHAR(255) NOT NULL DEFAULT '',
  is_deleted       SMALLINT     NOT NULL DEFAULT 0,
  create_time      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX idx_notify_channel_user ON tf_notify_channel (user_id);
COMMENT ON TABLE  tf_notify_channel                IS '机器人通知渠道表';
COMMENT ON COLUMN tf_notify_channel.channel_type   IS '类型：dingtalk=钉钉 feishu=飞书';
COMMENT ON COLUMN tf_notify_channel.secret         IS '加签密钥；空串=不加签';
COMMENT ON COLUMN tf_notify_channel.is_enabled     IS '启用：1=是 0=否';
COMMENT ON COLUMN tf_notify_channel.last_push_result IS '最近推送结果（成功/错误摘要）';

CREATE TABLE tf_push_subscription (
  id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT       NOT NULL,
  endpoint    VARCHAR(500) NOT NULL,
  p256dh      VARCHAR(255) NOT NULL,
  auth        VARCHAR(255) NOT NULL,
  is_deleted  SMALLINT     NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_push_sub_endpoint ON tf_push_subscription (endpoint);
CREATE INDEX idx_push_sub_user ON tf_push_subscription (user_id);
COMMENT ON TABLE  tf_push_subscription           IS '浏览器推送订阅表（Web Push / VAPID）';
COMMENT ON COLUMN tf_push_subscription.endpoint  IS '推送服务端点 URL，全局唯一';
COMMENT ON COLUMN tf_push_subscription.p256dh    IS '订阅密钥 p256dh';
COMMENT ON COLUMN tf_push_subscription.auth      IS '订阅密钥 auth';

-- ============ 日志域 ============

CREATE TABLE tl_api_log (
  id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT      NOT NULL,
  api_key_id  BIGINT      NOT NULL DEFAULT 0,
  key_hint    VARCHAR(32) NOT NULL DEFAULT '',
  action      VARCHAR(64) NOT NULL,
  target      VARCHAR(255) NOT NULL DEFAULT '',
  status_code SMALLINT    NOT NULL,
  cost_ms     INT         NOT NULL DEFAULT 0,
  client_ip   VARCHAR(45) NOT NULL,
  ip_region   VARCHAR(128) NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_log_key_time ON tl_api_log (api_key_id, create_time);
CREATE INDEX idx_log_user_time ON tl_api_log (user_id, create_time);
COMMENT ON TABLE  tl_api_log             IS 'API 审计日志表（不可变；保留期 LOG_RETENTION_DAYS）';
COMMENT ON COLUMN tl_api_log.api_key_id  IS '密钥 id；0=Web 会话';
COMMENT ON COLUMN tl_api_log.key_hint    IS '密钥提示冗余（密钥删除后日志仍可读）';
COMMENT ON COLUMN tl_api_log.action      IS '规范化操作名：路由模板+method 映射，如 todo.create / note.upsert / auth.failed';
COMMENT ON COLUMN tl_api_log.target      IS '操作对象：待办 id / 笔记 path 等';
COMMENT ON COLUMN tl_api_log.client_ip   IS '客户端 IP（最长 IPv6=45）';
COMMENT ON COLUMN tl_api_log.ip_region   IS 'IP 归属地（ip2region 离线解析，写入时冗余）';
COMMENT ON COLUMN tl_api_log.cost_ms     IS '请求耗时毫秒';
