const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f05 笔记：upsert/乐观锁 409/软删复活/原文读取/列表目录/移动重命名/非法路径
const enc = encodeURIComponent
export const meta = '笔记'
export const steps = [
  { t: 'http', name: 'notePut1', method: 'PUT', auth: 'A',
    path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md'),
    body: { content: '# 旅程第一篇\n\n限流策略用固定窗口。', title: '旅程第一篇', tags: ['ai'] }, expect: 200,
    save: { note1: (d) => ({ path: d.note?.path || d.path, hash: d.note?.content_hash || d.content_hash }) } },
  // 覆盖：带错 hash → 409
  { t: 'http', name: 'notePutConflict', method: 'PUT', auth: 'A',
    path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md'),
    body: () => ({ content: 'v2', expected_hash: 'deadbeef'.repeat(8) }), expect: 409 },
  // 覆盖：带对 hash → ok，回读 hash 变化
  { t: 'http', name: 'notePut2', method: 'PUT', auth: 'A',
    path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md'),
    body: () => ({ content: '# 旅程第一篇 v2\n\ndocker 容器自愈。', expected_hash: S.note1.hash }), expect: 200,
    save: { note1: (d, S) => ({ ...S.note1, hash: d.note?.content_hash || d.content_hash }) } },
  // GET 原文（format=md 纯文本）
  { t: 'http', name: 'noteGetMd', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md') + '?format=md',
    assert: (d, S, _st4, st) => S.assert(st.text.includes('# 旅程第一篇 v2'), '原文应含 v2 标题') },
  // GET JSON 元信息（hash 回读）
  { t: 'http', name: 'noteGetMeta', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md'),
    assert: (d, S) => S.assert((d.note?.content_hash || d.content_hash) === S.note1.hash, 'hash 回读应一致') },
  // 第二篇 + 目录列表聚合（一级目录=ai）
  { t: 'http', name: 'notePut2nd', method: 'PUT', auth: 'A', path: () => '/api/v1/notes/' + enc('dev/容器笔记.md'),
    body: { content: 'docker compose 起服务。' }, expect: 200 },
  { t: 'http', name: 'noteList', path: '/api/v1/notes?prefix=', auth: 'A', assert: (d) => {
      S.assert((d.dirs || []).includes('ai') && (d.dirs || []).includes('dev'), '目录聚合应含 ai/dev') } },
  { t: 'http', name: 'noteListPrefix', path: '/api/v1/notes?prefix=ai%2F', auth: 'A', assert: (d) => {
      S.assert((d.notes || d.items || []).some((n) => (n.path || '').includes('旅程第一篇')), '前缀列表应命中') } },
  // 移动重命名 → 回读旧路径 404 / 新路径 200
  { t: 'http', name: 'noteMove', method: 'POST', path: '/api/v1/notes/move', auth: 'A',
    body: () => ({ from: 'ai/旅程第一篇.md', to: 'ai/旅程改名.md' }), expect: 200 },
  { t: 'http', name: 'noteOldGone', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程第一篇.md'), expect: 404 },
  { t: 'http', name: 'noteNewHere', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程改名.md'), expect: 200 },
  // 删除 → restore（同路径复活语义）
  { t: 'http', name: 'noteDel', method: 'DELETE', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程改名.md'), expect: 200 },
  { t: 'http', name: 'noteRestore', method: 'POST', path: '/api/v1/notes/restore', auth: 'A', body: () => ({ path: 'ai/旅程改名.md' }), expect: 200 },
  { t: 'http', name: 'noteRevived', auth: 'A', path: () => '/api/v1/notes/' + enc('ai/旅程改名.md'), expect: 200 },
  // 负向：路径穿越
  { t: 'http', name: 'noteBadPath', method: 'PUT', auth: 'A', path: '/api/v1/notes/%2E%2E%2Fescape.md', body: { content: 'x' }, expect: 400 },
  // 负向：B 读 A 的笔记 → 404（跨用户隔离）
  { t: 'http', name: 'bReadANote', auth: 'B', path: () => '/api/v1/notes/' + enc('ai/旅程改名.md'), expect: 404 },
]