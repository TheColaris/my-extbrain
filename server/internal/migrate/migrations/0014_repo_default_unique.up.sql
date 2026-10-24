-- 0014 默认仓库唯一性：每用户至多一个活跃默认仓库。
-- 背景：DefaultID 懒建是「查不到再建」，并发请求可能建出两个默认仓库（无唯一约束兜底）。
-- 步骤：先保留每用户 id 最小的默认仓库，其余降级为普通仓库，再建部分唯一索引。
UPDATE tf_repo r SET is_default = 0
 WHERE r.is_default = 1 AND r.is_deleted = 0
   AND r.id <> (SELECT min(x.id) FROM tf_repo x
                 WHERE x.user_id = r.user_id AND x.is_default = 1 AND x.is_deleted = 0);

CREATE UNIQUE INDEX IF NOT EXISTS uk_repo_user_default
  ON tf_repo (user_id) WHERE is_default = 1 AND is_deleted = 0;

COMMENT ON INDEX uk_repo_user_default IS '每用户至多一个活跃默认仓库（部分唯一索引）';
