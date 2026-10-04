// 枚举对齐层 —— 与 extbrain-server internal/model/enums.go 一一对应（同字符串值）。
// 唯一真源在 Go 侧；本文件手工对齐，禁止出现此表之外的魔法值。
// 对齐方：server/internal/model/enums.go（同字符串值）。

export const TODO_STATUS = {
  active: 'active',
  done: 'done',
} as const
export type TodoStatus = (typeof TODO_STATUS)[keyof typeof TODO_STATUS]

/** 待办列表排序偏好（tu_user.todo_sort，按用户持久化） */
export const TODO_SORT = {
  created: 'created',
  due: 'due',
} as const
export type TodoSort = (typeof TODO_SORT)[keyof typeof TODO_SORT]

export const TODO_SOURCE = {
  web: 'web',
  cli: 'cli',
} as const
export type TodoSource = (typeof TODO_SOURCE)[keyof typeof TODO_SOURCE]

export const KEY_SCOPE = {
  all: 'all',
  todo: 'todo',
  notes: 'notes',
} as const
export type KeyScope = (typeof KEY_SCOPE)[keyof typeof KEY_SCOPE]

/** 检索模式（/search?mode=；缺省=自动：provider 可用则混合，否则纯关键词。Web 不暴露切换入口） */
export const SEARCH_MODE = {
  keyword: 'keyword',
  vector: 'vector',
  hybrid: 'hybrid',
} as const
export type SearchMode = (typeof SEARCH_MODE)[keyof typeof SEARCH_MODE]

/** 命中来源（/search 响应 hits[].source；「语义」徽章依据：semantic/both 显示徽章） */
export const SEARCH_SOURCE = {
  keyword: 'keyword',
  semantic: 'semantic',
  both: 'both',
} as const
export type SearchSource = (typeof SEARCH_SOURCE)[keyof typeof SEARCH_SOURCE]

export const CHANNEL_TYPE = {
  dingtalk: 'dingtalk',
  feishu: 'feishu',
} as const
export type ChannelType = (typeof CHANNEL_TYPE)[keyof typeof CHANNEL_TYPE]

/** 推送投递渠道（tl_push_log.channel_type；web=浏览器订阅） */
export const PUSH_CHANNEL = {
  web: 'web',
  dingtalk: 'dingtalk',
  feishu: 'feishu',
} as const
export type PushChannel = (typeof PUSH_CHANNEL)[keyof typeof PUSH_CHANNEL]

/** 推送事件类型（tl_push_log.event_type） */
export const NOTIFY_EVENT_TYPE = {
  due_today: 'due_today',
  overdue: 'overdue',
  test: 'test',
} as const
export type NotifyEventType = (typeof NOTIFY_EVENT_TYPE)[keyof typeof NOTIFY_EVENT_TYPE]

/** 推送投递结果（tl_push_log.status） */
export const PUSH_STATUS = {
  ok: 'ok',
  fail: 'fail',
} as const
export type PushStatus = (typeof PUSH_STATUS)[keyof typeof PUSH_STATUS]

/** 回收站条目类型 */
export const TRASH_TYPE = {
  todo: 'todo',
  memo: 'memo',
  note: 'note',
} as const
export type TrashType = (typeof TRASH_TYPE)[keyof typeof TRASH_TYPE]

export const BIND_CHANNEL = {
  phone: 'phone',
  email: 'email',
} as const
export type BindChannel = (typeof BIND_CHANNEL)[keyof typeof BIND_CHANNEL]

/** 头像底色（tu_user.avatar_bg；空=白）——值对齐 --neon-<name> 主题变量后缀 */
export const AVATAR_BG = {
  yellow: 'yellow',
  pink: 'pink',
  green: 'green',
  blue: 'blue',
  orange: 'orange',
  purple: 'purple',
  red: 'red',
} as const
export type AvatarBg = (typeof AVATAR_BG)[keyof typeof AVATAR_BG]
/** 头像底色选项（含「无底色」） */
export const AVATAR_BG_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: '默认白' },
  ...Object.values(AVATAR_BG).map((v) => ({ value: v, label: v })),
]

/** 笔记分享模式（tf_note_share.mode） */
export const SHARE_MODE = {
  snapshot: 'snapshot',
  live: 'live',
} as const
export type ShareMode = (typeof SHARE_MODE)[keyof typeof SHARE_MODE]

export function isTodoStatus(v: string): v is TodoStatus {
  return v === TODO_STATUS.active || v === TODO_STATUS.done
}

/* ===== 审计动作（对齐 server ActionType，枚举真源在 Go）===== */
export const ACTION = {
  todoCreate: 'todo.create',
  todoUpdate: 'todo.update',
  todoDelete: 'todo.delete',
  todoRestore: 'todo.restore',
  todoRead: 'todo.read',
  memoCreate: 'memo.create',
  memoUpdate: 'memo.update',
  memoDelete: 'memo.delete',
  memoRead: 'memo.read',
  noteUpsert: 'note.upsert',
  noteRead: 'note.read',
  noteDelete: 'note.delete',
  noteMove: 'note.move',
  noteRestore: 'note.restore',
  notePerm: 'note.perm',
  searchQuery: 'search.query',
  keyCreate: 'key.create',
  keyUpdate: 'key.update',
  keyDelete: 'key.delete',
  keyRead: 'key.read',
  webLogin: 'web.login',
  authRegister: 'auth.register',
  authFailed: 'auth.failed',
  notifyRead: 'notify.read',
  notifyRetry: 'notify.retry',
  trashRead: 'trash.read',
  trashDelete: 'trash.delete',
  trashRestore: 'trash.restore',
  trashPurge: 'trash.purge',
  shareUpsert: 'share.upsert',
  shareRead: 'share.read',
  shareDelete: 'share.delete',
  shareCopy: 'share.copy',
  exportRead: 'export.read',
  // 平台管理（仅管理员）
  configRead: 'config.read',
  configUpdate: 'config.update',
  configTest: 'config.test',
  indexRead: 'index.read',
  indexRebuild: 'index.rebuild',
  opsRead: 'ops.read',
} as const
export type ActionType = (typeof ACTION)[keyof typeof ACTION]

// 资源/动词中文映射（显示层）
const ACTION_RES: Record<string, string> = {
  todo: '待办', memo: '便签', note: '笔记', key: '密钥',
  auth: '账户', web: '面板', search: '知识检索', api: '接口',
  trash: '回收站', notify: '通知', share: '分享', export: '数据导出',
  config: '平台配置', index: '索引', ops: '运营看板',
}
const ACTION_VERB: Record<string, string> = {
  create: '新建', update: '修改', delete: '删除', read: '查看',
  restore: '恢复', upsert: '保存', failed: '失败', login: '登录', register: '注册', query: '检索', move: '移动',
  purge: '清空', retry: '重推', copy: '复制', test: '测试', rebuild: '重建',
}

// 整体特判（语义优先于拆词）
const ACTION_WHOLE: Partial<Record<ActionType, string>> = {
  [ACTION.webLogin]: '面板登录',
  [ACTION.authRegister]: '注册账号',
  [ACTION.authFailed]: '认证失败',
  [ACTION.searchQuery]: '知识检索',
  [ACTION.noteUpsert]: '笔记 · 保存',
  [ACTION.notePerm]: '知识库 · 目录权限',
  [ACTION.shareUpsert]: '分享 · 生成/更新',
  [ACTION.shareDelete]: '分享 · 停止',
  [ACTION.configUpdate]: '平台配置 · 保存',
  [ACTION.indexRebuild]: '索引 · 重建',
  [ACTION.opsRead]: '运营看板 · 查看',
}

export function actionLabel(a: string): string {
  const whole = ACTION_WHOLE[a as ActionType]
  if (whole) return whole
  const [res, verb] = a.split('.')
  const r = ACTION_RES[res]
  const v = ACTION_VERB[verb]
  if (!r || !v) return a // 未知动作回退原样（前向兼容）
  return `${r} · ${v}`
}
