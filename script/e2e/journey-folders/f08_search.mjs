const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f08 检索：关键词三字段/scope=all 三域/mock 向量语义/hybrid/权限后过滤
const enc = encodeURIComponent
export const meta = '检索（关键词+向量混合）'
export const steps = [
  // 关键词：标题命中
  { t: 'http', name: 'kwTitle', path: '/api/v1/search?q=%E6%94%B9%E5%90%8D', auth: 'A',
    assert: (d) => S.assert((d.notes || []).some((h) => (h.path || '').includes('旅程改名')), '标题关键词应命中') },
  // 关键词：正文命中（带摘录高亮）
  { t: 'http', name: 'kwContent', path: '/api/v1/search?q=docker', auth: 'A', assert: (d) => {
      const hit = (d.notes || []).find((h) => (h.path || '').includes('容器笔记'))
      S.assert(hit && (hit.excerpt || hit.snippet || '').includes('docker'), '正文命中应带摘录') } },
  // scope=all 三域一次搜
  { t: 'http', name: 'scopeAll', path: '/api/v1/search?q=docker&scope=all&limit=5', auth: 'A',
    assert: (d) => S.assert('todos' in d && 'memos' in d && 'notes' in d, 'scope=all 应返回三域结构') },
  // 负向：notes scope 的 Key 借道 scope=all → 不得带出待办/便签
  { t: 'http', name: 'k2ScopeAllNoTodo', auth: 'K2', path: '/api/v1/search?q=&scope=all', assert: (d) => {
      S.assert((d.todos || []).length === 0 && (d.memos || []).length === 0, 'notes Key 不得经 scope=all 带出待办/便签') } },
  // 管理端配 mock embedding（旅程 mock 服务已由 runner 起 在 127.0.0.1:8199）
  { t: 'http', name: 'cfgEmbed', method: 'PUT', path: '/api/v1/admin/embedding', auth: 'A',
    body: { enabled: true, base_url: 'http://127.0.0.1:8199/v1', model: 'mock-embed', dim: 1024, api_key: 'sk-e2e-mock' }, expect: 200 },
  { t: 'http', name: 'testEmbed', method: 'POST', path: '/api/v1/admin/embedding/test', auth: 'A', expect: 200,
    assert: (d) => S.assert(d.ok === true || d.success === true, 'embedding 测试连接应 ok') },
  // 种一篇「限流族」笔记（mock 向量簇 0：与关键词不中的语义查询配对）
  { t: 'http', name: 'seedRateLimit', method: 'PUT', auth: 'A',
    path: () => '/api/v1/notes/' + encodeURIComponent('ops/限流治理.md'),
    body: { content: '# 限流治理\n\n线上限流用固定窗口，超限直接 429。' }, expect: 200 },
  // 重建索引（把已有笔记全部向量化）→ 轮询到 pending=0
  { t: 'http', name: 'rebuild', method: 'POST', path: '/api/v1/admin/index/rebuild', auth: 'A', expect: 202 },
  { t: 'fn', name: 'waitRebuild', fn: async (S) => {
      for (let i = 0; i < 30; i++) {
        const r = await fetch(S.BASE + '/api/v1/admin/index/status', { headers: { authorization: S.A.authHeader } })
        const j = await r.json()
        if ((j.status?.pending ?? j.pending ?? 1) === 0) return
        await new Promise((res) => setTimeout(res, 1000))
      }
      throw new Error('重建索引 30s 未收敛') } },
  { t: 'http', name: 'indexStatus', path: '/api/v1/admin/index/status', auth: 'A',
    assert: (d, S) => { const st = d.status || d; S.assert((st.chunks ?? st.notes) > 0, '索引应有切片/笔记') } },
  // 语义命中（关键词不中）：「限流」簇笔记已存在，搜「ratelimit 最佳实践」语义路命中
  { t: 'http', name: 'vecSearch', path: '/api/v1/search?q=ratelimit%20best%20practice&mode=vector', auth: 'A',
    assert: (d, S) => {
      
      S.assert((d.notes || []).some((h) => (h.path || '').includes('限流治理')), '语义路应命中限流簇笔记（ops/限流治理.md）') } },
  // hybrid：双命中排前
  { t: 'http', name: 'hybridSearch', path: '/api/v1/search?q=docker&mode=hybrid', auth: 'A',
    assert: (d) => S.assert((d.notes || []).length >= 1, 'hybrid 应有命中') },
  // 负向：无命中词（空簇 quantumflux 语义路为空 + 关键词不中）
  { t: 'http', name: 'noHit', path: '/api/v1/search?q=quantumfluxxyz', auth: 'A', assert: (d) => S.assert((d.notes || []).length === 0, '无词应零命中') },
  // 权限后过滤：P2 搜根级关键词 → 0（根级对 P2 不可见）
  { t: 'http', name: 'permFilteredSearch', auth: 'P2', path: '/api/v1/search?q=keywordRoot', assert: (d) => S.assert(!(d.notes || []).some((h) => (h.path || '').includes('根级公开')), '根级命中应被权限过滤（其余合法命中允许）') },
]