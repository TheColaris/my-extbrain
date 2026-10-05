const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f10 回收站：三实体软删统一视图/来源反查/恢复/彻底删除/清空
const enc = encodeURIComponent
export const meta = '回收站'
export const steps = [
  // 软删三实体（todo2 已在 f03 恢复；此处新删）
  { t: 'http', name: 'delTodo', method: 'POST', path: '/api/v1/todos', auth: 'A', body: { title: '回收站待办' }, expect: 201,
    save: { trTodo: (d) => ({ id: d.id ?? d.todo?.id }) } },
  { t: 'http', name: 'delTodoGo', method: 'DELETE', auth: 'A', path: () => '/api/v1/todos/' + S.trTodo.id, expect: 200 },
  { t: 'http', name: 'delMemoGo', method: 'DELETE', auth: 'A', path: () => '/api/v1/memos/' + S.memo1.id, expect: [200, 404] },
  { t: 'http', name: 'delNoteGo', method: 'DELETE', auth: 'A', path: (S) => '/api/v1/notes/' + enc('dev/容器笔记.md'), expect: 200 },
  // 列表：≥2 条（memo1 在 f04 已删，404 分支容忍重复删）+ 来源反查 + 记下 note 条目 id
  { t: 'wait', ms: 1500 }, // 来源反查依赖审计异步落库（1s 批量 flush），稍候再断言
  { t: 'http', name: 'trashList', path: '/api/v1/trash', auth: 'A', assert: (d, S) => {
      const items = d.items || []
      S.assert(items.length >= 2, '回收站应有 ≥2 条')
      const todoRow = items.find((x) => x.type === 'todo')
      S.assert(todoRow && /Web|CLI/.test(todoRow.source || ''), '应反查出删除来源')
      const noteRow = items.find((x) => x.type === 'note')
      S.assert(noteRow, '回收站应有笔记条目')
      S.trashNoteID = noteRow.id } },
  // 恢复 todo
  { t: 'http', name: 'trashRestore', method: 'POST', auth: 'A', path: '/api/v1/trash/restore', body: () => ({ type: 'todo', id: S.trTodo.id }), expect: 200 },
  { t: 'http', name: 'restoredBack', path: '/api/v1/todos?status=active', auth: 'A',
    assert: (d, S) => S.assert((d.todos || d.items || d).some((x) => x.id === S.trTodo.id), '恢复后应回到在途列表') },
  // 彻底删除一条（note；id=回收站条目 id，来自列表接力）
  { t: 'http', name: 'trashPurgeItem', method: 'DELETE', auth: 'A', path: () => '/api/v1/trash/note/' + S.trashNoteID, expect: 200 },
  // 清空
  { t: 'http', name: 'trashPurgeAll', method: 'DELETE', path: '/api/v1/trash/purge', auth: 'A', expect: 200 },
  { t: 'http', name: 'trashEmpty', path: '/api/v1/trash', auth: 'A', assert: (d) => {
      const items = d.items || d.trash || d
      S.assert(items.length === 0, '清空后应为 0 条') } },
]