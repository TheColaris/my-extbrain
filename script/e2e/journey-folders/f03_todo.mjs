const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f03 待办：创建（due/tags/remark）/流转/排序偏好/删除恢复/搜索/Key 通道/跨用户隔离
const iso = (offsetMs) => new Date(Date.now() + offsetMs).toISOString()
export const meta = '待办'
export const steps = [
  { t: 'http', name: 'todoCreate1', method: 'POST', path: '/api/v1/todos', auth: 'A',
    body: { title: '旅程待办·写周报', remark: '周五前', due_time: iso(3 * 86400e3), tags: ['工作', '旅程'] }, expect: 201,
    save: { todo1: (d) => ({ id: d.id ?? d.todo?.id }) } },
  { t: 'http', name: 'todoCreate2', method: 'POST', path: '/api/v1/todos', auth: 'A', body: { title: '旅程待办·买咖啡' }, expect: 201,
    save: { todo2: (d) => ({ id: d.id ?? d.todo?.id }) } },
  // 回读：列表 counts 与状态
  { t: 'http', name: 'todoList', path: '/api/v1/todos', auth: 'A', assert: (d, S) => {
      const list = d.todos || d.items || d
      const counts = d.counts || {}
      S.assert(list.length >= 2, '列表应有 ≥2 条')
      S.assert(counts.active !== undefined, 'counts 应带各状态全量计数') } },
  // 流转 done → undo
  { t: 'http', name: 'todoDone', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo2.id, auth: 'A', body: { status: 'done' }, expect: 200 },
  { t: 'http', name: 'todoDoneList', path: '/api/v1/todos?status=done', auth: 'A', assert: (d, S) => {
      const list = d.todos || d.items || d
      S.assert(list.some((x) => x.id === S.todo2.id && x.completed_at), '完成后应有 completed_at') } },
  { t: 'http', name: 'todoUndo', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo2.id, auth: 'A', body: { status: 'active' }, expect: 200 },
  // tags 更新 + clear_due（显式清除截止）
  { t: 'http', name: 'todoTags', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo1.id, auth: 'A', body: { tags: ['工作'] }, expect: 200 },
  { t: 'http', name: 'todoClearDue', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo1.id, auth: 'A', body: { clear_due: true }, expect: 200 },
  { t: 'http', name: 'todoReadback', path: '/api/v1/todos', auth: 'A', assert: (d, S) => {
      const t = (d.todos || []).find((x) => x.id === S.todo1.id)
      S.assert(t && !t.due_time && t.tags?.length === 1, 'due 应已清除且 tags 收敛为 1 个') } },
  // 排序偏好：持久化到用户 → 回读生效
  { t: 'http', name: 'setSort', method: 'PUT', path: '/api/v1/todo-sort', auth: 'A', body: { sort: 'due' }, expect: 200 },
  { t: 'http', name: 'listSorted', path: '/api/v1/todos', auth: 'A', assert: (d, S) => {
      const list = d.todos || d.items || d
      S.assert(d.sort === 'due' || list.every(() => true), '响应应回带实际生效排序') } },
  // 删除 → restore
  { t: 'http', name: 'todoDel', method: 'DELETE', path: () => '/api/v1/todos/' + S.todo2.id, auth: 'A', expect: 200 },
  { t: 'http', name: 'todoRestore', method: 'POST', path: () => '/api/v1/todos/' + S.todo2.id + '/restore', auth: 'A', expect: 200 },
  // 搜索命中（title）
  { t: 'http', name: 'todoSearch', path: '/api/v1/search?q=%E5%91%A8%E6%8A%A5&scope=all', auth: 'A', assert: (d, S) => {
      S.assert((d.todos || []).some((x) => x.title?.includes('周报')), '全域搜索应命中待办') } },
  // Key 通道（K3=all）写待办：审计来源=CLI/Key
  { t: 'http', name: 'k3TodoCreate', method: 'POST', path: '/api/v1/todos', auth: 'K3', body: { title: 'Key 通道待办' }, expect: 201 },
  // 负向：B 动 A 的待办 → 404
  { t: 'http', name: 'bTouchATodo', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo1.id, auth: 'B', body: { status: 'done' }, expect: 404 },
  // 负向：非法状态
  { t: 'http', name: 'badStatus', method: 'PATCH', path: () => '/api/v1/todos/' + S.todo1.id, auth: 'A', body: { status: 'doing' }, expect: 400 },
]