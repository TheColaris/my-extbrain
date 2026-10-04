// Package model —— 枚举唯一真源。
//
// 全项目（server / CLI / web）一切状态、类型、来源类取值以本文件为准，禁止散落魔法值：
//   - Go 侧（server + cmd/extbrain CLI，同仓）：直接引用本文件常量；
//   - TS 侧（web/）：在 src/lib/enums.ts 用同一组字符串值手工对齐；
//   - 对齐方：web/src/lib/enums.ts（同字符串值）。
//
// 新增/修改枚举必须两处同步（Go/TS）；枚举一致性单测遍历本文件输出 JSON 与 TS 常量 diff。
package model

// TodoStatus 待办状态（tf_todo.status）
type TodoStatus string

const (
	TodoStatusActive TodoStatus = "active"
	TodoStatusDone   TodoStatus = "done"
)

func (s TodoStatus) Valid() bool {
	return s == TodoStatusActive || s == TodoStatusDone
}

// TodoSort 待办列表排序偏好（tu_user.todo_sort；Web 面板按用户持久化）
type TodoSort string

const (
	TodoSortCreated TodoSort = "created" // 创建时间倒序（默认）
	TodoSortDue     TodoSort = "due"     // 截止升序、无截止在后
)

func (s TodoSort) Valid() bool {
	return s == TodoSortCreated || s == TodoSortDue
}

// TodoSource 待办来源（tf_todo.source）
type TodoSource string

const (
	TodoSourceWeb TodoSource = "web"
	TodoSourceCLI TodoSource = "cli"
)

// KeyScope API 密钥权限（tu_api_key.scope；todo 含便签）
type KeyScope string

const (
	KeyScopeAll   KeyScope = "all"
	KeyScopeTodo  KeyScope = "todo"
	KeyScopeNotes KeyScope = "notes"
)

func (s KeyScope) Valid() bool {
	return s == KeyScopeAll || s == KeyScopeTodo || s == KeyScopeNotes
}

// Allows 判断 scope 是否覆盖目标资源域
func (s KeyScope) Allows(target KeyScope) bool {
	return s == KeyScopeAll || s == target
}

// ChannelType 机器人通知渠道（tf_notify_channel.channel_type）
type ChannelType string

const (
	ChannelTypeDingTalk ChannelType = "dingtalk"
	ChannelTypeFeishu   ChannelType = "feishu"
)

// PushChannel 推送投递渠道（tl_push_log.channel_type；web=浏览器订阅，其余=机器人渠道）
type PushChannel string

const (
	PushChannelWeb      PushChannel = "web"
	PushChannelDingTalk PushChannel = "dingtalk"
	PushChannelFeishu   PushChannel = "feishu"
)

func (c PushChannel) Valid() bool {
	return c == PushChannelWeb || c == PushChannelDingTalk || c == PushChannelFeishu
}

// NotifyEventType 推送事件类型（tl_push_log.event_type）
type NotifyEventType string

const (
	NotifyEventDueToday NotifyEventType = "due_today"
	NotifyEventOverdue  NotifyEventType = "overdue"
	NotifyEventTest     NotifyEventType = "test"
)

func (t NotifyEventType) Valid() bool {
	return t == NotifyEventDueToday || t == NotifyEventOverdue || t == NotifyEventTest
}

// PushStatus 推送投递结果（tl_push_log.status）
type PushStatus string

const (
	PushStatusOK   PushStatus = "ok"
	PushStatusFail PushStatus = "fail"
)

// TrashType 回收站条目类型（API 请求/响应字段 type）
type TrashType string

const (
	TrashTodo TrashType = "todo"
	TrashMemo TrashType = "memo"
	TrashNote TrashType = "note"
)

func (t TrashType) Valid() bool {
	return t == TrashTodo || t == TrashMemo || t == TrashNote
}

// BindChannel 账户绑定通道（API 请求字段 type）
type BindChannel string

const (
	BindChannelPhone BindChannel = "phone"
	BindChannelEmail BindChannel = "email"
)

// AvatarBg 头像底色（tu_user.avatar_bg；空=白）
type AvatarBg string

const (
	AvatarBgNone   AvatarBg = ""
	AvatarBgYellow AvatarBg = "yellow" // 对齐前端 --neon-yellow 变量后缀
	AvatarBgPink   AvatarBg = "pink"
	AvatarBgGreen  AvatarBg = "green"
	AvatarBgBlue   AvatarBg = "blue"
	AvatarBgOrange AvatarBg = "orange"
	AvatarBgPurple AvatarBg = "purple"
	AvatarBgRed    AvatarBg = "red"
)

func (b AvatarBg) Valid() bool {
	switch b {
	case AvatarBgNone, AvatarBgYellow, AvatarBgPink, AvatarBgGreen, AvatarBgBlue, AvatarBgOrange, AvatarBgPurple, AvatarBgRed:
		return true
	}
	return false
}

// ShareMode 笔记分享模式（tf_note_share.mode）
type ShareMode string

const (
	ShareModeSnapshot ShareMode = "snapshot" // 快照冻结（默认）：创建/更新分享时冻结内容
	ShareModeLive     ShareMode = "live"     // 跟随更新：公开读取实时取原笔记
)

func (m ShareMode) Valid() bool { return m == ShareModeSnapshot || m == ShareModeLive }

// ShareExpire 分享有效期选项（API 请求字段 expire_days；0=永久）
const ShareExpireForever = 0

// SearchMode 检索模式（/search?mode=；""=auto：provider 可用则混合，否则纯关键词）
type SearchMode string

const (
	SearchModeKeyword SearchMode = "keyword"
	SearchModeVector  SearchMode = "vector"
	SearchModeHybrid  SearchMode = "hybrid"
)

func (m SearchMode) Valid() bool {
	return m == "" || m == SearchModeKeyword || m == SearchModeVector || m == SearchModeHybrid
}

// SearchSource 命中来源（/search 响应 hits[].source；前端「语义」徽章依据）
type SearchSource string

const (
	SearchSourceKeyword  SearchSource = "keyword"  // 纯关键词命中
	SearchSourceSemantic SearchSource = "semantic" // 纯向量召回（无关键词高亮）
	SearchSourceBoth     SearchSource = "both"     // 双命中（融合排名天然靠前）
)

// ActionType 审计动作（tl_api_log.action）——「资源.动词」线格式，前后端对齐
// （web enums.ts ACTION_* 同值；显示文案在前端映射，存储与筛选恒用本枚举值）。
type ActionType string

const (
	ActionTodoCreate  ActionType = "todo.create"
	ActionTodoUpdate  ActionType = "todo.update"
	ActionTodoDelete  ActionType = "todo.delete"
	ActionTodoRestore ActionType = "todo.restore"
	ActionTodoRead    ActionType = "todo.read"
	ActionMemoCreate  ActionType = "memo.create"
	ActionMemoUpdate  ActionType = "memo.update"
	ActionMemoDelete  ActionType = "memo.delete"
	ActionMemoRead    ActionType = "memo.read"
	ActionNoteUpsert  ActionType = "note.upsert"
	ActionNoteRead    ActionType = "note.read"
	ActionNoteDelete  ActionType = "note.delete"
	ActionNoteMove    ActionType = "note.move"
	ActionNoteRestore ActionType = "note.restore"
	ActionNotePerm    ActionType = "note.perm" // 目录权限配置（AI Key 可见性白名单；Web JWT 专属）
	ActionSearch      ActionType = "search.query"
	ActionKeyCreate   ActionType = "key.create"
	ActionKeyUpdate   ActionType = "key.update"
	ActionKeyDelete   ActionType = "key.delete"
	ActionKeyRead     ActionType = "key.read"
	ActionWebLogin    ActionType = "web.login"
	ActionRegister    ActionType = "auth.register"
	ActionAuthFailed  ActionType = "auth.failed"
	ActionNotifyRead  ActionType = "notify.read"
	ActionNotifyRetry ActionType = "notify.retry"
	ActionTrashRead   ActionType = "trash.read"
	ActionTrashPurge  ActionType = "trash.purge"
	ActionShareUpsert ActionType = "share.upsert"
	ActionShareRead   ActionType = "share.read" // 公开读取（游客 uid=0）/ owner 查询共用
	ActionShareDelete ActionType = "share.delete"
	ActionShareCopy   ActionType = "share.copy"  // 分享页复制到自己的知识库
	ActionExportRead  ActionType = "export.read" // 导出全部数据（zip）
	// 知识库仓库（迁移 0012；仓库管理=owner 面板行为，Web JWT 专属）
	ActionRepoCreate ActionType = "repo.create"
	ActionRepoUpdate ActionType = "repo.update"
	ActionRepoDelete ActionType = "repo.delete"
	ActionRepoRead   ActionType = "repo.read"
	// 平台管理（仅管理员；平台参数配置与索引重建）
	ActionConfigRead   ActionType = "config.read"   // 读取 embedding 配置
	ActionConfigUpdate ActionType = "config.update" // 保存 embedding 配置
	ActionConfigTest   ActionType = "config.test"   // 测试连接
	ActionIndexRead    ActionType = "index.read"    // 索引状态
	ActionIndexRebuild ActionType = "index.rebuild" // 重建全部索引
	ActionOpsRead      ActionType = "ops.read"      // 运营看板数据
)

// ActionOf 组装线格式（resource + verb，中间件唯一出口）
func ActionOf(resource, verb string) ActionType {
	return ActionType(resource + "." + verb)
}
