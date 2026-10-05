// Package model —— GORM 模型（对应 migrations/0001_init.up.sql，两处同步改）。
// 表名/字段按项目建表规范：
// 前缀 tu_/tf_/tl_、id/create_time/update_time 必备、is_xxx 布尔、无物理外键。
package model

import (
	"time"

	"github.com/lib/pq"
)

type User struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Phone        *string   `gorm:"column:phone" json:"phone,omitempty"`
	Email        *string   `gorm:"column:email" json:"email,omitempty"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	NickName     string    `gorm:"column:nick_name" json:"nick_name"`
	AvatarEmoji  string    `gorm:"column:avatar_emoji" json:"avatar_emoji"`
	AvatarBg     string    `gorm:"column:avatar_bg" json:"avatar_bg"`
	TokenVersion int       `gorm:"column:token_version" json:"-"`                                 // 会话版本（改密/换绑 +1）
	TodoSort     string    `gorm:"column:todo_sort;default:'created'" json:"todo_sort,omitempty"` // 零值时 INSERT 走 DB 默认，避免写空串覆盖列默认
	IsAdmin      int16     `gorm:"column:is_admin" json:"is_admin"`                               // 平台管理员（迁移 0009；首个注册用户自动为 1）
	IsDeleted    int16     `gorm:"column:is_deleted" json:"-"`
	CreateTime   time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime   time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (User) TableName() string { return "tu_user" }

type APIKey struct {
	ID          int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64      `gorm:"column:user_id;index:idx_api_key_user" json:"user_id"`
	KeyName     string     `gorm:"column:key_name" json:"key_name"`
	KeyHash     string     `gorm:"column:key_hash;uniqueIndex:uk_api_key_hash" json:"-"`
	KeyHint     string     `gorm:"column:key_hint" json:"key_hint"`
	Scope       KeyScope   `gorm:"column:scope" json:"scope"`
	ExpireTime  *time.Time `gorm:"column:expire_time" json:"expire_time,omitempty"`
	LastUseTime *time.Time `gorm:"column:last_use_time" json:"last_use_time,omitempty"`
	IsRevoked   int16      `gorm:"column:is_revoked" json:"-"`
	IsDeleted   int16      `gorm:"column:is_deleted" json:"-"`
	CreateTime  time.Time  `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time  `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (APIKey) TableName() string { return "tu_api_key" }

type Todo struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64          `gorm:"column:user_id;index:idx_todo_user_status,priority:1" json:"user_id"`
	Title       string         `gorm:"column:title" json:"title"`
	Remark      string         `gorm:"column:remark" json:"remark"`
	Status      TodoStatus     `gorm:"column:status;index:idx_todo_user_status,priority:2" json:"status"`
	DueTime     *time.Time     `gorm:"column:due_time" json:"due_time,omitempty"`
	Tags        pq.StringArray `gorm:"column:tags;type:text[]" json:"tags"`
	Source      TodoSource     `gorm:"column:source" json:"source"`
	APIKeyID    *int64         `gorm:"column:api_key_id" json:"api_key_id,omitempty"`
	CompletedAt *time.Time     `gorm:"column:completed_at" json:"completed_at,omitempty"`
	IsDeleted   int16          `gorm:"column:is_deleted" json:"-"`
	CreateTime  time.Time      `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time      `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (Todo) TableName() string { return "tf_todo" }

type Memo struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64          `gorm:"column:user_id;index:idx_memo_user_time,priority:1" json:"user_id"`
	Content    string         `gorm:"column:content" json:"content"`
	Tags       pq.StringArray `gorm:"column:tags;type:text[]" json:"tags"`
	IsPinned   int16          `gorm:"column:is_pinned" json:"is_pinned"`
	IsDeleted  int16          `gorm:"column:is_deleted" json:"-"`
	CreateTime time.Time      `gorm:"column:create_time;autoCreateTime;index:idx_memo_user_time,priority:2" json:"create_time"`
	UpdateTime time.Time      `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (Memo) TableName() string { return "tf_memo" }

type Note struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64          `gorm:"column:user_id;uniqueIndex:uk_note_repo_path,priority:1" json:"user_id"`
	RepoID     int64          `gorm:"column:repo_id;uniqueIndex:uk_note_repo_path,priority:2" json:"repo_id"` // 所属仓库；path 为仓库内相对路径
	RepoName   string         `gorm:"-" json:"repo_name,omitempty"`                                           // 仓库名（返回面注记，由服务层填；不落库）
	Path       string         `gorm:"column:path;uniqueIndex:uk_note_repo_path,priority:3" json:"path"`
	Title      string         `gorm:"column:title" json:"title"`
	Tags       pq.StringArray `gorm:"column:tags;type:text[]" json:"tags"`
	IsDeleted  int16          `gorm:"column:is_deleted" json:"-"`
	CreateTime time.Time      `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime time.Time      `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (Note) TableName() string { return "tf_note" }

// Repo 知识库仓库（迁移 0012；一等实体：仓库 > 文件夹 > 笔记）。
// 每用户恰一个默认仓库（IsDefault=1，不可删除）；裸路径寻址=默认仓库。
type Repo struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64     `gorm:"column:user_id;index:idx_repo_user" json:"user_id"`
	Name        string    `gorm:"column:name" json:"name"`
	Description string    `gorm:"column:description" json:"description"`
	IsDefault   int16     `gorm:"column:is_default" json:"is_default"`
	Sort        int       `gorm:"column:sort" json:"sort"`
	IsDeleted   int16     `gorm:"column:is_deleted" json:"-"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (Repo) TableName() string { return "tf_repo" }

// NoteContent 笔记正文（规范：超 5000 → text 独立表，1:1 以主键对应）
type NoteContent struct {
	NoteID      int64     `gorm:"primaryKey;column:note_id" json:"note_id"`
	Content     string    `gorm:"column:content" json:"content"`
	ContentHash string    `gorm:"column:content_hash" json:"content_hash"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (NoteContent) TableName() string { return "tf_note_content" }

type NotifyChannel struct {
	ID             int64       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         int64       `gorm:"column:user_id;index:idx_notify_channel_user" json:"user_id"`
	ChannelType    ChannelType `gorm:"column:channel_type" json:"channel_type"`
	WebhookURL     string      `gorm:"column:webhook_url" json:"webhook_url"`
	Secret         string      `gorm:"column:secret" json:"-"`
	IsEnabled      int16       `gorm:"column:is_enabled" json:"is_enabled"`
	LastPushTime   *time.Time  `gorm:"column:last_push_time" json:"last_push_time,omitempty"`
	LastPushResult string      `gorm:"column:last_push_result" json:"last_push_result"`
	IsDeleted      int16       `gorm:"column:is_deleted" json:"-"`
	CreateTime     time.Time   `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime     time.Time   `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (NotifyChannel) TableName() string { return "tf_notify_channel" }

type PushSubscription struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64     `gorm:"column:user_id;index:idx_push_sub_user" json:"user_id"`
	Endpoint   string    `gorm:"column:endpoint;uniqueIndex:uk_push_sub_endpoint" json:"endpoint"`
	P256DH     string    `gorm:"column:p256dh" json:"-"`
	Auth       string    `gorm:"column:auth" json:"-"`
	IsDeleted  int16     `gorm:"column:is_deleted" json:"-"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (PushSubscription) TableName() string { return "tf_push_subscription" }

type APILog struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64     `gorm:"column:user_id;index:idx_log_user_time,priority:1" json:"user_id"`
	APIKeyID   int64     `gorm:"column:api_key_id;index:idx_log_key_time,priority:1" json:"api_key_id"`
	KeyHint    string    `gorm:"column:key_hint" json:"key_hint"`
	Action     string    `gorm:"column:action" json:"action"`
	Target     string    `gorm:"column:target" json:"target"`
	StatusCode int16     `gorm:"column:status_code" json:"status_code"`
	CostMs     int32     `gorm:"column:cost_ms" json:"cost_ms"`
	ClientIP   string    `gorm:"column:client_ip" json:"client_ip"`
	IPRegion   string    `gorm:"column:ip_region" json:"ip_region"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime;index:idx_log_key_time,priority:2;index:idx_log_user_time,priority:2" json:"create_time"`
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"-"`
}

func (APILog) TableName() string { return "tl_api_log" }

// PushLog 通知推送记录（迁移 0005；每次投递一行，重推原地更新）
type PushLog struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64     `gorm:"column:user_id;index:idx_push_log_user_time,priority:1" json:"user_id"`
	EventType   string    `gorm:"column:event_type" json:"event_type"`
	EventKey    string    `gorm:"column:event_key" json:"event_key"`
	ChannelType string    `gorm:"column:channel_type" json:"channel_type"`
	ChannelID   int64     `gorm:"column:channel_id" json:"channel_id"`
	Title       string    `gorm:"column:title" json:"title"`
	Body        string    `gorm:"column:body" json:"body"`
	Status      string    `gorm:"column:status" json:"status"`
	Error       string    `gorm:"column:error" json:"error"`
	CostMs      int32     `gorm:"column:cost_ms" json:"cost_ms"`
	RetryCount  int       `gorm:"column:retry_count" json:"retry_count"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime;index:idx_push_log_user_time,priority:2" json:"create_time"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"-"`
}

func (PushLog) TableName() string { return "tl_push_log" }

// NoteShare 笔记分享（迁移 0007）：一篇笔记一个活跃分享（部分唯一索引），撤销=终态行保留
type NoteShare struct {
	ID          int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64          `gorm:"column:user_id;index:idx_note_share_user" json:"-"`
	NoteID      int64          `gorm:"column:note_id" json:"-"`
	Token       string         `gorm:"column:token;uniqueIndex:uk_note_share_token" json:"token"`
	Title       string         `gorm:"column:title" json:"title"`
	Content     string         `gorm:"column:content" json:"-"`
	ContentHash string         `gorm:"column:content_hash" json:"-"`
	Tags        pq.StringArray `gorm:"column:tags;type:text[]" json:"tags"`
	Mode        ShareMode      `gorm:"column:mode" json:"mode"`
	ExpireTime  *time.Time     `gorm:"column:expire_time" json:"expire_time,omitempty"`
	ViewCount   int64          `gorm:"column:view_count" json:"view_count"`
	IsRevoked   int16          `gorm:"column:is_revoked" json:"-"`
	CreateTime  time.Time      `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time      `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (NoteShare) TableName() string { return "tf_note_share" }

// CacheEntry 系统缓存（sys_cache，UNLOGGED）：键值+TTL 惰性清理
type CacheEntry struct {
	CacheKey   string    `gorm:"primaryKey;column:cache_key" json:"cache_key"`
	CacheValue string    `gorm:"column:cache_value" json:"cache_value"`
	ExpireTime time.Time `gorm:"column:expire_time" json:"expire_time"`
}

func (CacheEntry) TableName() string { return "sys_cache" }

// SystemConfig 平台参数配置（迁移 0009；KV，站点级，仅管理员可改）
type SystemConfig struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ConfigKey   string    `gorm:"column:config_key;uniqueIndex:uk_system_config_key" json:"config_key"`
	ConfigValue string    `gorm:"column:config_value" json:"config_value"`
	IsSecret    int16     `gorm:"column:is_secret" json:"is_secret"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (SystemConfig) TableName() string { return "tp_system_config" }

// E2EMailbox E2E 洁净室信箱（迁移 0013；三重门全开时截获外发内容落此表，生产恒空表）
type E2EMailbox struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Channel    string    `gorm:"column:channel" json:"channel"`
	Target     string    `gorm:"column:target" json:"target"`
	Title      string    `gorm:"column:title" json:"title"`
	Body       string    `gorm:"column:body" json:"body"`
	Payload    string    `gorm:"column:payload" json:"payload"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
}

func (E2EMailbox) TableName() string { return "tl_e2e_mailbox" }

// NoteChunk 笔记切片（迁移 0009；向量检索。embedding 列不走 GORM（vector 类型），
// 读写一律 raw SQL + `?::vector` 显式转型）
type NoteChunk struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	NoteID      int64     `gorm:"column:note_id;index:idx_note_chunk_note" json:"note_id"`
	UserID      int64     `gorm:"column:user_id;index:idx_note_chunk_user" json:"user_id"`
	ChunkSeq    int       `gorm:"column:chunk_seq" json:"chunk_seq"`
	ChunkText   string    `gorm:"column:chunk_text" json:"chunk_text"`
	ContentHash string    `gorm:"column:content_hash" json:"content_hash"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (NoteChunk) TableName() string { return "tf_note_chunk" }

// NotePerm 知识库目录权限（AI Key 可见性；按仓库内目录前缀白名单）。
// 语义：无行=未配置=开放；mode=allow 时仅 key_ids 内的 Key 可见（读写同权）；最深前缀规则优先。
// folder_path=” = 仓库级默认规则；folder_path 为仓库内相对前缀。
type NotePerm struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     int64          `gorm:"column:user_id;uniqueIndex:uk_note_perm_repo_folder,priority:1" json:"user_id"`
	RepoID     int64          `gorm:"column:repo_id;uniqueIndex:uk_note_perm_repo_folder,priority:2" json:"repo_id"`
	FolderPath string         `gorm:"column:folder_path;uniqueIndex:uk_note_perm_repo_folder,priority:3" json:"folder_path"`
	Mode       string         `gorm:"column:mode" json:"mode"`
	KeyIDs     pq.StringArray `gorm:"column:key_ids;type:text[]" json:"key_ids"`
	IsDeleted  int16          `gorm:"column:is_deleted" json:"-"`
	CreateTime time.Time      `gorm:"column:create_time;autoCreateTime" json:"create_time"`
	UpdateTime time.Time      `gorm:"column:update_time;autoUpdateTime" json:"update_time"`
}

func (NotePerm) TableName() string { return "tf_note_perm" }
