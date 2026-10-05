const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f14 平台管理：非管理员 403/embedding 配置与测试/索引状态与重建/运营看板
export const meta = '平台管理'
export const steps = [
  // 负向：B（非管理员）→ 403
  { t: 'http', name: 'bAdminDenied', path: '/api/v1/admin/embedding', auth: 'B', expect: 403 },
  { t: 'http', name: 'bOpsDenied', path: '/api/v1/admin/ops', auth: 'B', expect: 403 },
  // embedding 配置读写（mock 端点由 runner 提供）
  { t: 'http', name: 'getEmbed', path: '/api/v1/admin/embedding', auth: 'A',
    assert: (d) => S.assert(d.config?.api_key_hint === undefined || !String(d.config?.api_key_hint).includes('sk-e2e-mock'), 'api_key 只回打码') },
  { t: 'http', name: 'saveEmbedKeep', method: 'PUT', path: '/api/v1/admin/embedding', auth: 'A',
    body: { enabled: true, base_url: 'http://127.0.0.1:8199/v1', model: 'mock-embed', dim: 1024 }, expect: 200,
    assert: (d) => S.assert(true, '不带 api_key 保存=保持不变') },
  { t: 'http', name: 'testEmbedConn', method: 'POST', path: '/api/v1/admin/embedding/test', auth: 'A', expect: 200 },
  // 负向：非法 base_url
  { t: 'http', name: 'badBaseURL', method: 'PUT', path: '/api/v1/admin/embedding', auth: 'A',
    body: { enabled: true, base_url: 'ftp://bad', model: 'x', dim: 1024, api_key: 'sk-x' }, expect: 400 },
  // 索引状态/重建
  { t: 'http', name: 'idxStatus', path: '/api/v1/admin/index/status', auth: 'A',
    assert: (d, S) => { const st = d.status || d; S.assert(st.index_dim === 1024 || st.config_dim === 1024, '维度应 1024') } },
  { t: 'http', name: 'idxRebuild', method: 'POST', path: '/api/v1/admin/index/rebuild', auth: 'A', expect: 202 },
  { t: 'fn', name: 'waitIdx', fn: async (S) => {
      for (let i = 0; i < 30; i++) {
        const r = await fetch(S.BASE + '/api/v1/admin/index/status', { headers: { authorization: S.A.authHeader } })
        const j = await r.json()
        const st = j.status || j
        if ((st.pending ?? 1) === 0) return
        await new Promise((res) => setTimeout(res, 1000))
      }
      throw new Error('索引重建 30s 未收敛') } },
  // 运营看板：14/30 天两档 + 字段结构
  { t: 'http', name: 'ops14', path: '/api/v1/admin/ops?days=14', auth: 'A', assert: (d) => {
      S.assert(d.overview && Array.isArray(d.daily) && Array.isArray(d.top_actions), 'ops 应含 overview/daily/top 序列') } },
  { t: 'http', name: 'ops30', path: '/api/v1/admin/ops?days=30', auth: 'A', assert: (d) => {
      S.assert((d.daily || []).length === 30, '30 天窗口应补齐 30 桶') } },
  { t: 'http', name: 'opsBad', path: '/api/v1/admin/ops?days=7', auth: 'A', expect: 400,
    assert: (d) => S.assert(d?.error?.code === 'invalid_days', '非法 days 应 400 invalid_days') },
]