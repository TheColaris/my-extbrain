-- 0012 知识库仓库（一等实体）：仓库 > 文件夹 > 笔记。
-- 语义：每用户一个默认仓库（is_default=1，不可删）；存量笔记全部移入默认仓库，
--              path 原值不变（仓库内相对路径语义与原全局路径前缀完全等价）；
--              唯一键改为 (user_id, repo_id, path)；目录权限挂仓库（folder_path=仓库内相对前缀，
--              '' = 仓库级默认规则；原全库根规则迁移为默认仓库的仓库级规则，语义等价）。

CREATE TABLE tf_repo (
  id          BIGINT       GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id     BIGINT       NOT NULL,
  name        VARCHAR(255) NOT NULL,
  description VARCHAR(500) NOT NULL DEFAULT '',
  is_default  SMALLINT     NOT NULL DEFAULT 0,
  sort        INT          NOT NULL DEFAULT 0,
  is_deleted  SMALLINT     NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ  NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- 部分唯一索引：软删行释放仓库名（删除后可重建同名仓库）
CREATE UNIQUE INDEX uk_repo_user_name ON tf_repo (user_id, name) WHERE is_deleted = 0;
CREATE INDEX idx_repo_user ON tf_repo (user_id);
COMMENT ON TABLE  tf_repo             IS '知识库仓库表（一等实体；仓库 > 文件夹 > 笔记）';
COMMENT ON COLUMN tf_repo.user_id     IS '归属用户（业务层约束，无物理外键）';
COMMENT ON COLUMN tf_repo.name        IS '仓库名（账号内唯一；禁止冒号——冒号是跨仓库寻址分隔符）';
COMMENT ON COLUMN tf_repo.description IS '描述（可选）';
COMMENT ON COLUMN tf_repo.is_default  IS '1=默认仓库（每用户至多一个；不可删除；裸路径寻址=默认仓库）';
COMMENT ON COLUMN tf_repo.sort        IS '展示排序（小在前）';
COMMENT ON COLUMN tf_repo.is_deleted  IS '逻辑删除：0=正常 1=删除（删除只删仓库行，不删笔记）';

-- 1) 每个存量用户建默认仓库
INSERT INTO tf_repo (user_id, name, description, is_default, sort)
SELECT u.id, '默认仓库', '开启仓库功能时自动创建；存量笔记都在这里', 1, 0
FROM tu_user u;

-- 2) 笔记归属默认仓库（含软删行；path 原值不变）
ALTER TABLE tf_note ADD COLUMN repo_id BIGINT NOT NULL DEFAULT 0;
UPDATE tf_note n SET repo_id = r.id FROM tf_repo r WHERE r.user_id = n.user_id AND r.is_default = 1;
DROP INDEX uk_note_user_path;
CREATE UNIQUE INDEX uk_note_repo_path ON tf_note (user_id, repo_id, path);
COMMENT ON COLUMN tf_note.repo_id IS '所属仓库（tf_repo.id；path 为仓库内相对路径）';

-- 3) 目录权限挂仓库：folder_path 变为仓库内相对前缀（'' = 仓库级默认规则）
ALTER TABLE tf_note_perm ADD COLUMN repo_id BIGINT NOT NULL DEFAULT 0;
UPDATE tf_note_perm p SET repo_id = r.id FROM tf_repo r WHERE r.user_id = p.user_id AND r.is_default = 1;
ALTER TABLE tf_note_perm DROP CONSTRAINT uk_note_perm_folder;
CREATE UNIQUE INDEX uk_note_perm_repo_folder ON tf_note_perm (user_id, repo_id, folder_path);
COMMENT ON COLUMN tf_note_perm.repo_id     IS '规则归属仓库（tf_repo.id）';
COMMENT ON COLUMN tf_note_perm.folder_path IS '目录前缀（仓库内相对；归一化：无前导/尾随 /；空串=仓库级默认规则）';
