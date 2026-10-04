-- my-extbrain 初始迁移（down）：按依赖逆序整表删除，幂等可重放
DROP TABLE IF EXISTS tl_api_log;
DROP TABLE IF EXISTS tf_push_subscription;
DROP TABLE IF EXISTS tf_notify_channel;
DROP TABLE IF EXISTS tf_note_content;
DROP TABLE IF EXISTS tf_note;
DROP TABLE IF EXISTS tf_memo;
DROP TABLE IF EXISTS tf_todo;
DROP TABLE IF EXISTS tu_api_key;
DROP TABLE IF EXISTS tu_user;
