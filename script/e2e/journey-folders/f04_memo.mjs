const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f04 便签：创建/置顶/列表/搜索/字数上限/删除
export const meta = '便签'
export const steps = [
  { t: 'http', name: 'memoCreate1', method: 'POST', path: '/api/v1/memos', auth: 'A', body: { content: 'GORM 写 PG text[] 必须用 pq.StringArray', tags: ['db'] }, expect: 201,
    save: { memo1: (d) => ({ id: d.id ?? d.memo?.id }) } },
  { t: 'http', name: 'memoCreate2', method: 'POST', path: '/api/v1/memos', auth: 'A', body: { content: '旅程便签·置顶我' }, expect: 201,
    save: { memo2: (d) => ({ id: d.id ?? d.memo?.id }) } },
  // 置顶 → 列表回读第一条
  { t: 'http', name: 'memoPin', method: 'PATCH', path: () => '/api/v1/memos/' + S.memo2.id, auth: 'A', body: { is_pinned: 1 }, expect: 200 },
  { t: 'http', name: 'memoList', path: '/api/v1/memos', auth: 'A', assert: (d, S) => {
      const list = d.memos || d.items || d
      S.assert(list.length >= 2, '列表应有 ≥2 条')
      S.assert(list[0].id === S.memo2.id && list[0].is_pinned === 1, `置顶应排最前（首条=${list[0].id} is_pinned=${list[0].is_pinned}）`) } },
  // 搜索
  { t: 'http', name: 'memoSearch', path: '/api/v1/search?q=pq.StringArray&scope=all', auth: 'A', assert: (d) => S.assert((d.memos || []).length >= 1, '全域搜索应命中便签') },
  // 负向：超 2000 字
  { t: 'http', name: 'memoTooLong', method: 'POST', path: '/api/v1/memos', auth: 'A', body: { content: '长'.repeat(2001) }, expect: 400 },
  // 删除（软删，f10 回收站接管）
  { t: 'http', name: 'memoDel', method: 'DELETE', path: () => '/api/v1/memos/' + S.memo1.id, auth: 'A', expect: 200 },
  // 负向：B 动 A 的便签
  { t: 'http', name: 'bTouchAMemo', method: 'DELETE', path: () => '/api/v1/memos/' + S.memo2.id, auth: 'B', expect: 404 },
]