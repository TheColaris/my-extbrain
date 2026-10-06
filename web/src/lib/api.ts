// API 客户端：baseURL /api/v1（dev 由 vite proxy 转 8080，生产同源 embed）
import type { SearchSource } from '@/lib/enums'

const BASE = '/api/v1'
const TOKEN_KEY = 'extbrain_token'

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY)
}
export function setToken(t: string | null) {
  if (t) localStorage.setItem(TOKEN_KEY, t)
  else localStorage.removeItem(TOKEN_KEY)
}

/** 会话失效统一出口：清凭据回首页（登录页路由守卫会再引导）；api() 与旁路 fetch 共用 */
function authExpiredRedirect() {
  setToken(null)
  localStorage.removeItem('extbrain_user')
  window.location.href = '/'
}

export class ApiError extends Error {
  code: string
  status: number
  constructor(code: string, message: string, status: number) {
    super(message)
    this.code = code
    this.status = status
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(BASE + path, { ...init, headers })
  const body = await res.json().catch(() => null)
  if (!res.ok) {
    // 401 = 会话失效：清凭据回登录页（登录/注册请求除外，避免循环）
    if (res.status === 401 && !path.startsWith('/auth/')) authExpiredRedirect()
    const e = body?.error
    throw new ApiError(e?.code ?? 'unknown', e?.message ?? `请求失败（${res.status}）`, res.status)
  }
  return body as T
}

export interface User {
  id: number
  phone?: string
  email?: string
  nick_name: string
  avatar_emoji?: string
  avatar_bg?: string
  is_admin?: boolean
}

/** 账户信息（/auth/me；含注册时间与管理员标记） */
export interface MeInfo extends User {
  create_time: string
  is_admin: boolean
}

export interface AuthResp {
  token: string
  user: User
}

export const authApi = {
  register: (account: string, password: string, emailCode: string) =>
    api<AuthResp>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ account, password, email_code: emailCode }),
    }),
  login: (account: string, password: string) =>
    api<AuthResp>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ account, password }),
    }),
  sendEmailCode: (email: string) =>
    api<{ sent: boolean }>('/auth/send-email-code', {
      method: 'POST',
      body: JSON.stringify({ email }),
    }),
}

export interface APIKeyItem {
  id: number
  key_name: string
  key_hint: string
  scope: 'all' | 'todo' | 'notes'
  expire_time?: string
  last_use_time?: string
  create_time: string
}

export const keysApi = {
  list: () => api<{ keys: APIKeyItem[] }>('/keys'),
}

export interface IssueResp {
  key: APIKeyItem
  full_key: string
  cli_command: string
}

export const keysApiFull = {
  issue: (name: string, expire_days = 0) =>
    api<IssueResp>('/keys', { method: 'POST', body: JSON.stringify({ name, expire_days }) }),
  update: (id: number, body: { name?: string; scope?: APIKeyItem['scope'] }) =>
    api<{ key: APIKeyItem }>(`/keys/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  revoke: (id: number) =>
    api<{ revoked: boolean }>(`/keys/${id}`, { method: 'DELETE' }),
}

export interface LogItem {
  id: number
  api_key_id: number
  key_hint: string
  action: string
  target: string
  repo_name?: string // 笔记类 action 反解的所属仓库名（跳转按仓库寻址）
  status_code: number
  cost_ms: number
  client_ip: string
  ip_region: string
  create_time: string
}

export interface Todo {
  id: number
  title: string
  remark: string
  status: 'active' | 'done'
  due_time?: string
  tags: string[]
  source: 'web' | 'cli'
  completed_at?: string
  create_time: string
  update_time: string
}

/** 各状态全量计数（与筛选无关，Tab 计数用） */
export interface TodoCounts {
  active: number
  done: number
}

export interface Memo {
  id: number
  content: string
  tags: string[]
  is_pinned: number
  create_time: string
  update_time: string
}

export interface DayCount {
  date: string
  todos: number
  memos: number
}

export interface DashboardData {
  todo_count: number
  memo_count: number
  note_count: number
  due_today: number
  today_todos: Todo[]
  recent_memos: Memo[]
  daily: DayCount[]
}

export const todosApi = {
  create: (body: { title: string; remark?: string; due_time?: string; tags?: string[] }) =>
    api<Todo>('/todos', { method: 'POST', body: JSON.stringify(body) }),
  list: (params?: { status?: string; sort?: string; limit?: number; before_id?: number }) => {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.sort) q.set('sort', params.sort)
    if (params?.limit) q.set('limit', String(params.limit))
    if (params?.before_id) q.set('before_id', String(params.before_id))
    // sort 省略时服务端取用户偏好，响应带实际生效的 sort
    return api<{ todos: Todo[]; counts: TodoCounts; sort?: string }>(`/todos?${q}`)
  },
  update: (
    id: number,
    body: Partial<{ title: string; remark: string; status: string; due_time: string; clear_due: boolean; tags: string[] }>
  ) => api<Todo>(`/todos/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  del: (id: number) => api<{ deleted: boolean }>(`/todos/${id}`, { method: 'DELETE' }),
  /** 撤销删除（软删除恢复） */
  restore: (id: number) => api<Todo>(`/todos/${id}/restore`, { method: 'POST' }),
  /** 排序偏好按用户持久化（Web JWT） */
  setSortPref: (sort: string) =>
    api<{ todo_sort: string }>('/todo-sort', { method: 'PUT', body: JSON.stringify({ sort }) }),
}

export const memosApi = {
  create: (body: { content: string; tags?: string[] }) =>
    api<Memo>('/memos', { method: 'POST', body: JSON.stringify(body) }),
  list: (limit = 50) => api<{ memos: Memo[] }>(`/memos?limit=${limit}`),
  update: (id: number, body: Partial<{ content: string; tags: string[]; is_pinned: number }>) =>
    api<Memo>(`/memos/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  del: (id: number) => api<{ deleted: boolean }>(`/memos/${id}`, { method: 'DELETE' }),
}

export const dashboardApi = {
  summary: () => api<DashboardData>('/dashboard'),
}

export interface NotifyChannel {
  id: number
  channel_type: 'dingtalk' | 'feishu'
  webhook_url: string
  is_enabled: number
  last_push_time?: string
  last_push_result: string
  create_time: string
}

export const accountApi = {
  me: () => api<MeInfo>('/auth/me'),
  /** 绑定/换绑邮箱（安全操作：服务端会 +1 会话版本并顺发新 token → 替换本地） */
  bind: (type: 'email', value: string, password: string) =>
    api<MeInfo & { token: string }>('/auth/bind', {
      method: 'POST', body: JSON.stringify({ type, value, password }),
    }),
  /** 改密（同上顺发新 token） */
  changePassword: (oldPwd: string, newPwd: string) =>
    api<{ ok: boolean; token: string }>('/auth/password', { method: 'PATCH', body: JSON.stringify({ old: oldPwd, new: newPwd }) }),
  /** 部分更新：昵称 / 头像表情 / 头像底色 */
  updateProfile: (body: { nick_name?: string; avatar_emoji?: string; avatar_bg?: string }) =>
    api<MeInfo>('/auth/me', { method: 'PATCH', body: JSON.stringify(body) }),
  /** 导出全部数据（zip 下载；文件名取服务端 Content-Disposition） */
  exportAll: async (): Promise<void> => {
    const res = await fetch('/api/v1/export', { headers: { Authorization: `Bearer ${getToken()}` } })
    if (res.status === 401) {
      authExpiredRedirect()
      throw new ApiError('unauthorized', '登录已失效，请重新登录', 401)
    }
    if (!res.ok) {
      const body = await res.json().catch(() => null)
      throw new ApiError(body?.error?.code ?? 'unknown', body?.error?.message ?? `导出失败（${res.status}）`, res.status)
    }
    const blob = await res.blob()
    const name = res.headers.get('Content-Disposition')?.match(/filename="([^"]+)"/)?.[1] ?? 'extbrain-export.zip'
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = name
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  },
}

export const notifyApi = {
  list: () => api<{ channels: NotifyChannel[] }>('/notify/channels'),
  create: (body: { channel_type: string; webhook_url: string; secret?: string }) =>
    api<NotifyChannel>('/notify/channels', { method: 'POST', body: JSON.stringify(body) }),
  toggle: (id: number, is_enabled: boolean) =>
    api<{ ok: boolean }>(`/notify/channels/${id}`, { method: 'PATCH', body: JSON.stringify({ is_enabled }) }),
  del: (id: number) => api(`/notify/channels/${id}`, { method: 'DELETE' }),
  test: (id: number) => api<{ ok: boolean; message: string }>(`/notify/channels/${id}/test`, { method: 'POST' }),
}

export const pushApi = {
  vapid: () => api<{ enabled: boolean; public_key: string }>('/push/vapid'),
  subscribe: (sub: { endpoint: string; keys: { p256dh: string; auth: string } }) =>
    api('/push/subscriptions', { method: 'POST', body: JSON.stringify(sub) }),
  unsubscribe: (endpoint: string) => api(`/push/subscriptions?endpoint=${encodeURIComponent(endpoint)}`, { method: 'DELETE' }),
  test: () => api<{ message: string }>('/push/test', { method: 'POST' }),
}

export interface NoteMeta {
  id: number
  path: string
  title: string
  tags: string[]
  update_time: string
}

export interface NoteFull extends NoteMeta {
  content: string
  content_hash: string
}

/* ================= 知识库仓库（仓库 > 文件夹 > 笔记；Web JWT 专属） ================= */
export interface RepoItem {
  id: number
  name: string
  description: string
  is_default: number
  sort: number
  note_count: number
}

export const reposApi = {
  list: () => api<{ repos: RepoItem[] }>('/repos'),
  create: (name: string, description = '') =>
    api<{ repo: RepoItem }>('/repos', { method: 'POST', body: JSON.stringify({ name, description }) }),
  update: (id: number, body: { name?: string; description?: string }) =>
    api<{ repo: RepoItem }>(`/repos/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  remove: (id: number) => api(`/repos/${id}`, { method: 'DELETE' }),
}

export const notesApi = {
  list: (prefix?: string, repo?: string) => {
    const q = qs({ prefix, repo })
    return api<{ notes: NoteMeta[]; dirs: string[] }>(`/notes${q}`)
  },
  get: (path: string, formatMd = false, repo?: string) => {
    const q = qs({ format: formatMd ? 'md' : '', repo })
    return api<NoteFull>(`/notes/${encodePath(path)}${q}`)
  },
  getMd: async (path: string): Promise<string> => {
    const res = await fetch(`/api/v1/notes/${encodePath(path)}?format=md`, {
      headers: { Authorization: `Bearer ${getToken()}` },
    })
    if (res.status === 401) authExpiredRedirect() // 与 api() 主通道同口径
    if (!res.ok) throw new Error('取原文失败')
    return res.text()
  },
  put: (path: string, body: { content: string; title?: string; tags?: string[]; expected_hash?: string }, repo?: string) =>
    api<{ note: NoteMeta; content_hash: string }>(`/notes/${encodePath(path)}${qs({ repo })}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),
  del: (path: string, repo?: string) => api(`/notes/${encodePath(path)}${qs({ repo })}`, { method: 'DELETE' }),
  /** 重命名 / 移动（原子；目标已存在 → 409 path_conflict；to 支持「仓库名:路径」跨仓库） */
  move: (from: string, to: string, repo?: string) =>
    api<{ note: NoteMeta }>('/notes/move', { method: 'POST', body: JSON.stringify({ from, to, repo }) }),
  /** 撤销删除（软删恢复） */
  restore: (path: string, repo?: string) =>
    api<{ note: NoteMeta }>('/notes/restore', { method: 'POST', body: JSON.stringify({ path, repo }) }),
  search: (q: string) =>
    api<{ hits: { repo_name?: string; path: string; title: string; snippet: string }[] }>(`/search?q=${encodeURIComponent(q)}`),
}

/** 查询串组装（空值跳过；问号仅在非空时出现） */
function qs(params: Record<string, string | undefined>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v) sp.set(k, v)
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

/* ================= 目录权限（AI Key 可见性；Web JWT 专属） ================= */
export interface NotePermRule {
  id: number
  repo_id: number
  folder_path: string // '' = 仓库级默认规则
  mode: 'open' | 'allow'
  key_ids: string[]
}

export const notePermApi = {
  list: () => api<{ rules: NotePermRule[] }>('/note-perm'),
  put: (body: { repo_id: number; folder_path: string; mode: 'open' | 'allow'; key_ids: number[] }) =>
    api<{ saved: boolean; rules: NotePermRule[] }>('/note-perm', { method: 'PUT', body: JSON.stringify(body) }),
}

function encodePath(p: string): string {
  return p.split('/').map(encodeURIComponent).join('/')
}

/* ===== 笔记分享（一篇笔记一个活跃分享；快照 token + 公开只读） ===== */
export interface ShareItem {
  token: string
  title: string
  tags?: string[]
  mode: 'snapshot' | 'live'
  expire_time?: string
  view_count: number
  create_time: string
  update_time: string
}

/** 公开分享视图（/share/:token 页拉取；游客无凭据） */
export interface ShareView {
  title: string
  content: string
  tags?: string[]
  mode: 'snapshot' | 'live'
  create_time: string
  update_time: string
  expire_time?: string
}

export const sharesApi = {
  /** 查该笔记的活跃分享（无则 share=null） */
  get: (path: string) => api<{ share: ShareItem | null }>(`/shares?path=${encodeURIComponent(path)}`),
  /** 创建/更新分享（保留 token；expire_days 0=永久） */
  create: (path: string, mode: ShareItem['mode'], expireDays: number) =>
    api<{ share: ShareItem }>('/shares', {
      method: 'POST',
      body: JSON.stringify({ path, mode, expire_days: expireDays }),
    }),
  /** 停止分享（再分享会生成新 token） */
  revoke: (token: string) => api(`/shares/${encodeURIComponent(token)}`, { method: 'DELETE' }),
  /** 公开读取（游客） */
  publicGet: (token: string) => api<ShareView>(`/public/share/${encodeURIComponent(token)}`),
  /** 复制到自己的知识库（需登录；复制=新建，同名自动后缀） */
  copy: (token: string) => api<{ note: NoteMeta }>(`/shares/${encodeURIComponent(token)}/copy`, { method: 'POST' }),
}

/* ===== 全局搜索（scope=all：三域分组；不带 scope 走旧 hits 形态=CLI 兼容） ===== */
export interface TodoSearchHit {
  id: number
  title: string
  snippet: string
  status: 'active' | 'done'
  due_time?: string
  tags: string[]
  source: 'web' | 'cli'
  api_key_id?: number
  create_time: string
}
export interface MemoSearchHit {
  id: number
  snippet: string
  tags: string[]
  create_time: string
}
export interface NoteSearchHit {
  repo_id: number
  repo_name: string
  path: string
  title: string
  snippet: string
  size_bytes: number
  updated_at: string
  source?: SearchSource // keyword=纯关键词 / semantic=纯语义 / both=双命中（「语义」徽章依据）
}
export const searchApi = {
  all: (q: string, signal?: AbortSignal) => {
    const headers: Record<string, string> = { Authorization: `Bearer ${getToken()}` }
    return fetch(`${BASE}/search?scope=all&q=${encodeURIComponent(q)}`, { headers, signal }).then(async (res) => {
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        if (res.status === 401) authExpiredRedirect() // 与 api() 主通道同口径
        const e = body?.error
        throw new ApiError(e?.code ?? 'unknown', e?.message ?? `请求失败（${res.status}）`, res.status)
      }
      return body as { todos: TodoSearchHit[]; memos: MemoSearchHit[]; notes: NoteSearchHit[] }
    })
  },
}

/* ===== 回收站 ===== */
export interface TrashItem {
  type: 'todo' | 'memo' | 'note'
  id: number
  title: string
  subtitle?: string
  tags: string[]
  repo_name?: string
  deleted_at: string
  left_days: number
  source: string
}
export const trashApi = {
  list: () => api<{ items: TrashItem[]; counts: Record<string, number> }>('/trash'),
  restore: (type: string, id: number) =>
    api<{ ok: boolean }>('/trash/restore', { method: 'POST', body: JSON.stringify({ type, id }) }),
  purgeItem: (type: string, id: number) => api<{ purged: boolean }>(`/trash/${type}/${id}`, { method: 'DELETE' }),
  purgeAll: () => api<{ purged: Record<string, number> }>('/trash/purge', { method: 'DELETE' }),
}

/* ===== 通知记录（事件聚合） ===== */
export interface PushChannelHit {
  id: number
  channel_type: 'web' | 'dingtalk' | 'feishu'
  channel_id: number
  status: 'ok' | 'fail'
  error: string
  cost_ms: number
  retry_count: number
  create_time: string
}
export interface PushEvent {
  event_key: string
  event_type: 'due_today' | 'overdue' | 'test'
  title: string
  body: string
  create_time: string
  channels: PushChannelHit[]
}
export interface PushStats {
  events: number
  deliveries: number
  fails: number
}
export const notifyLogApi = {
  list: (f: { channel?: string; result?: string; type?: string; days?: number } = {}) => {
    const p = new URLSearchParams()
    if (f.channel) p.set('channel', f.channel)
    if (f.result) p.set('result', f.result)
    if (f.type) p.set('type', f.type)
    if (f.days) p.set('days', String(f.days))
    const q = p.toString() ? `?${p.toString()}` : ''
    return api<{ events: PushEvent[]; stats: PushStats }>(`/notify/logs${q}`)
  },
  retry: (id: number) =>
    api<{ ok: boolean; message: string }>(`/notify/logs/${id}/retry`, { method: 'POST' }),
}

/* ===== 平台管理（仅管理员）：embedding 配置 + 索引状态/重建 ===== */
export interface IndexStatus {
  enabled: boolean
  notes: number // 已索引笔记数
  chunks: number // 切片总数
  pending: number // 待索引
  failed: number
  failed_items: { note_id: number; path: string; error: string }[]
  last_index_at?: string | null
  index_dim: number // 库内列维度
  config_dim: number // 平台配置维度（不一致=需重建索引）
  progress: { running: boolean; done: number; total: number; failed: number }
}

export interface EmbeddingTestResult {
  ok: boolean
  ms: number
  dim: number
  error: string
  at?: string
}

export interface EmbeddingConfigOut {
  enabled: boolean
  base_url: string
  model: string
  dim: number
  api_key_hint: string
  api_key_set: boolean
  defaults: { base_url: string; model: string; dim: number }
  last_test?: EmbeddingTestResult | null
  index: IndexStatus
}

export interface EmbeddingSaveInput {
  enabled: boolean
  base_url: string
  model: string
  dim: number
  api_key?: string // 空=保持不变（「更换」=清空后填新值）
}

/* ===== 运营看板（仅管理员）：平台运营数据聚合（一次请求返回全部分区）===== */
export interface OpsOverview {
  users_total: number
  users_today: number
  active_today: number
  active_7d: number
  notes_total: number
  notes_today: number
  todos_total: number // 未完成待办
  memos_total: number
  memos_today: number
  calls_today: number
  calls_7d_avg: number
  errors_today: number
  error_rate_today: number
  push_7d_total: number
  push_7d_ok: number
}

export interface OpsDay {
  date: string // MM-DD
  registers: number
  actives: number
  calls_ai: number
  calls_web: number
  notes: number
  todos: number
  memos: number
  err4xx: number
  err5xx: number
}

export interface OpsSummary {
  overview: OpsOverview
  daily: OpsDay[]
  top_actions: { action: string; count: number }[]
  top_errors: { status: number; count: number }[]
  top_users: {
    user_id: number
    nick_name: string
    calls: number
    notes: number
    todos: number
    memos: number
    last_active: string
  }[]
}

/* ===== 邮件服务（Resend，注册验证码发信）===== */
export interface EmailConfigOut {
  enabled: boolean
  from_address: string
  from_name: string
  api_key_hint: string
  api_key_set: boolean
  last_test: { ok: boolean; ms: number; to: string; error: string; at: string } | null
}

export interface EmailSaveInput {
  enabled: boolean
  from_address: string
  from_name: string
  api_key?: string
}

export interface EmailTestResult {
  ok: boolean
  ms: number
  to: string
  error: string
}

export const adminApi = {
  getEmbedding: () => api<EmbeddingConfigOut>('/admin/embedding'),
  saveEmbedding: (b: EmbeddingSaveInput) =>
    api<{ ok: boolean }>('/admin/embedding', { method: 'PUT', body: JSON.stringify(b) }),
  testEmbedding: (b: { base_url?: string; api_key?: string; model?: string; dim?: number }) =>
    api<EmbeddingTestResult>('/admin/embedding/test', { method: 'POST', body: JSON.stringify(b) }),
  indexStatus: () => api<IndexStatus>('/admin/index/status'),
  rebuild: () => api<{ ok: boolean }>('/admin/index/rebuild', { method: 'POST' }),
  ops: (days: 14 | 30) => api<OpsSummary>(`/admin/ops?days=${days}`),
  getEmail: () => api<EmailConfigOut>('/admin/email'),
  saveEmail: (b: EmailSaveInput) => api<{ ok: boolean }>('/admin/email', { method: 'PUT', body: JSON.stringify(b) }),
  testEmail: () => api<EmailTestResult>('/admin/email/test', { method: 'POST' }),
}
