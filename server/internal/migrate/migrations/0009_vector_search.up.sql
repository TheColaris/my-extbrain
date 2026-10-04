-- 0009 向量检索（v2 预留设计落地）+ 平台配置表 + 管理员标记
-- 三部分：①平台参数配置 KV（embedding provider 等，仅管理员可改）
--         ②tu_user.is_admin（平台管理员；首个注册用户自动为 1）
--         ③tf_note_chunk（笔记切片 + 向量；HNSW cosine 索引）

-- ① 平台参数配置（KV；tp_=参数配置，见项目建表规范）
CREATE TABLE tp_system_config (
  id           BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  config_key   VARCHAR(64)  NOT NULL,
  config_value TEXT         NOT NULL DEFAULT '',
  is_secret    SMALLINT     NOT NULL DEFAULT 0,
  create_time  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uk_system_config_key ON tp_system_config (config_key);
COMMENT ON TABLE  tp_system_config              IS '平台参数配置表（KV；站点级，仅管理员可读写）';
COMMENT ON COLUMN tp_system_config.config_key   IS '配置键，如 embedding.base_url';
COMMENT ON COLUMN tp_system_config.config_value IS '配置值（文本/JSON 字面量）';
COMMENT ON COLUMN tp_system_config.is_secret    IS '敏感值：1=是（接口出参只回打码）0=否';

-- ② 平台管理员（首个注册用户自动为 1；迁移时给现存最早用户补齐）
ALTER TABLE tu_user ADD COLUMN IF NOT EXISTS is_admin SMALLINT NOT NULL DEFAULT 0;
COMMENT ON COLUMN tu_user.is_admin IS '平台管理员：1=是 0=否（可见「平台管理」；首个注册用户自动为 1）';
UPDATE tu_user SET is_admin = 1
 WHERE is_deleted = 0
   AND id = (SELECT MIN(id) FROM tu_user WHERE is_deleted = 0)
   AND NOT EXISTS (SELECT 1 FROM tu_user WHERE is_admin = 1);

-- ③ 向量检索：笔记切片表 + HNSW 索引（预留设计：chunk_text varchar(2000) + vector(1024) cosine）
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE tf_note_chunk (
  id           BIGINT        GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  note_id      BIGINT        NOT NULL,
  user_id      BIGINT        NOT NULL,
  chunk_seq    INT           NOT NULL,
  chunk_text   VARCHAR(2000) NOT NULL,
  content_hash CHAR(64)      NOT NULL,
  embedding    vector(1024),
  create_time  TIMESTAMPTZ   NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX idx_note_chunk_note ON tf_note_chunk (note_id);
CREATE INDEX idx_note_chunk_user ON tf_note_chunk (user_id);
CREATE INDEX idx_note_chunk_hnsw ON tf_note_chunk USING hnsw (embedding vector_cosine_ops);
COMMENT ON TABLE  tf_note_chunk              IS '笔记切片表（向量检索：按标题分节切分，embedding=1024 维）';
COMMENT ON COLUMN tf_note_chunk.note_id      IS '对应 tf_note.id（业务层约束，无物理外键）';
COMMENT ON COLUMN tf_note_chunk.user_id      IS '冗余用户（ANN 查询按用户过滤，免 join）';
COMMENT ON COLUMN tf_note_chunk.chunk_seq    IS '切片序号（同一笔记内从 0 递增）';
COMMENT ON COLUMN tf_note_chunk.chunk_text   IS '切片原文（语义命中的摘录来源）';
COMMENT ON COLUMN tf_note_chunk.content_hash IS '来源正文 SHA-256（变更检测：hash 一致跳过重建）';
COMMENT ON COLUMN tf_note_chunk.embedding    IS '切片向量（cosine；维度须与平台配置一致）';
