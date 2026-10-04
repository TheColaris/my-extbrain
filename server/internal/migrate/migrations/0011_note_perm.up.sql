-- 0011 知识库目录权限：按目录（前缀）配置哪些 AI（API Key）可见。
-- 语义：未配置=开放；白名单制（mode=allow 落行，key_ids=可见 Key 集）；读写同权；
--              最深前缀规则优先（根 '' = 全库默认，子目录可再收窄）；只约束 AI（Key），Web 面板不受限。

CREATE TABLE tf_note_perm (
  id           BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id      BIGINT       NOT NULL,
  folder_path  VARCHAR(500) NOT NULL DEFAULT '',
  mode         VARCHAR(16)  NOT NULL DEFAULT 'allow',
  key_ids      TEXT[]       NOT NULL DEFAULT '{}',
  is_deleted   SMALLINT     NOT NULL DEFAULT 0,
  create_time  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT uk_note_perm_folder UNIQUE (user_id, folder_path)
);
CREATE INDEX idx_note_perm_user ON tf_note_perm (user_id);
COMMENT ON TABLE  tf_note_perm             IS '知识库目录权限表（AI Key 可见性；按目录前缀白名单）';
COMMENT ON COLUMN tf_note_perm.user_id     IS '归属用户（业务层约束，无物理外键）';
COMMENT ON COLUMN tf_note_perm.folder_path IS '目录前缀（归一化：无前导/尾随 /；空串=根目录=全库默认）';
COMMENT ON COLUMN tf_note_perm.mode        IS '规则模式：allow=白名单（仅 key_ids 可见）；open 不落行（无行=未配置=开放）';
COMMENT ON COLUMN tf_note_perm.key_ids     IS '白名单 Key id 列表（tu_api_key.id；mode=allow 时生效）';
COMMENT ON COLUMN tf_note_perm.is_deleted  IS '逻辑删除：0=正常 1=删除';
