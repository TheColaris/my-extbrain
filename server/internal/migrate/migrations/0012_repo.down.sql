-- 0012 回滚：摘除仓库层。笔记 path 未曾改动（仓库化是等价变换）。
-- 注意（本迁移只对「尚处于升级前后临界态」的库安全）：
--   * 权限：仅保留默认仓库的规则回填为全局前缀规则，其余仓库的规则删除；
--   * 笔记：若不同仓库存在同路径笔记，末尾重建 uk_note_user_path 会因唯一键冲突失败
--           （golang-migrate 每个迁移一个事务，失败整体回滚，不产生半回滚状态）。

DELETE FROM tf_note_perm p
USING tf_repo r
WHERE r.id = p.repo_id AND r.is_default = 0;

DROP INDEX IF EXISTS uk_note_perm_repo_folder;
ALTER TABLE tf_note_perm DROP COLUMN IF EXISTS repo_id;
ALTER TABLE tf_note_perm ADD CONSTRAINT uk_note_perm_folder UNIQUE (user_id, folder_path);

DROP INDEX IF EXISTS uk_note_repo_path;
ALTER TABLE tf_note DROP COLUMN IF EXISTS repo_id;
CREATE UNIQUE INDEX uk_note_user_path ON tf_note (user_id, path);

DROP INDEX IF EXISTS uk_repo_user_name;
DROP TABLE IF EXISTS tf_repo;
