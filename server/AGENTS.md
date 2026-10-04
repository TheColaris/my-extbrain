# extbrain-server AGENTS

Go 后端 + CLI 子仓。硬规则（真源=根目录 AGENTS.md）：

1. 枚举：唯一真源 `internal/model/enums.go`，线格式测试 `enums_test.go` 锁死契约；新增枚举同步 web 侧（`src/lib/enums.ts`）同值对齐。
2. 迁移：`internal/migrate/migrations/NNNN_name.{up,down}.sql`，建表按项目建表规范（td_/tf_/tp_/tr_/tu_/tl_/ts_ 前缀、id/create_time/update_time、pk_/uk_/idx_、COMMENT 全列、无物理外键、超 5000 text 独立表；PG：timestamptz/smallint/text[]）。
3. GORM：模型与 DDL 同步改；禁 N+1（批量 IN）；并发敏感用原子 UPDATE/乐观锁（content_hash）。
4. API：错误统一 `{"error":{"code","message"}}`，message 可执行（说清缺什么）。
5. 审计日志异步落库（channel+goroutine），永不阻塞主请求。
6. 测试：Service 公共方法单测覆盖 ≥80%；对齐用例（枚举线格式）不许跳过。
