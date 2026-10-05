const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f13 到期扫描推送：造逾期待办 → 直调内部端点触发 DueScan → 断言信箱收到 → 防重 → 错 token 拒绝
export const meta = '到期扫描推送'
export const steps = [
  // A 造一条昨日到期的待办（逾期）
  { t: 'http', name: 'overdueTodo', method: 'POST', path: '/api/v1/todos', auth: 'A',
    body: () => ({ title: '逾期提醒目标', due_time: new Date(Date.now() - 86400e3).toISOString() }), expect: 201,
    save: { od: (d) => ({ id: d.id ?? d.todo?.id }) } },
  // 信箱先清空（隔离 f12 的消息）
  { t: 'http', name: 'clearMailbox', method: 'DELETE', path: '/api/v1/e2e/mailbox', expect: 200 },
  // 触发扫描（内部端点；X-Internal-Token=洁净配方注入值）
  { t: 'fn', name: 'dueScan', fn: async (S) => {
      const token = process.env.INTERNAL_TOKEN || 'e2e-internal-token'
      const r = await fetch(S.BASE + '/internal/push/due', { method: 'POST', headers: { 'x-internal-token': token } })
      const j = await r.json()
      S.assert(r.status === 200, 'DueScan 应 200')
      S.assert((j.pushed_users ?? 0) >= 1, '应推送 ≥1 位到期用户')
  } },
  // 信箱应收到推送（webhook 钉钉渠道 + webpush 双路；A 有启用钉钉渠道与订阅——订阅在 f12 已退订，此处至少 webhook 一路）
  { t: 'http', name: 'dueMailbox', path: '/api/v1/e2e/mailbox',
    assert: (d) => {
      S.assert(d.count >= 1, '到期推送应落信箱')
      const e = d.entries[0]
      S.assert(/逾期|今日待办/.test(e.title + e.body), `推送标题应含逾期/今日待办语义（实际：${e.title}）`) } },
  // 防重：立即再扫 → 0 人（digest/overdue 键 24h）
  { t: 'fn', name: 'dueScanAgain', fn: async (S) => {
      const token = process.env.INTERNAL_TOKEN || 'e2e-internal-token'
      const r = await fetch(S.BASE + '/internal/push/due', { method: 'POST', headers: { 'x-internal-token': token } })
      const j = await r.json()
      S.assert(r.status === 200 && (j.pushed_users ?? 0) === 0, '同日重复扫描应防重（0 人）') } },
  // 清理：完成该待办（不影响其他域断言）
  { t: 'http', name: 'odDone', method: 'PATCH', auth: 'A', path: () => '/api/v1/todos/' + S.od.id, body: { status: 'done' }, expect: 200 },
  // 负向：错 token → 401
  { t: 'http', name: 'badInternalToken', method: 'POST', path: '/internal/push/due', headers: { 'x-internal-token': 'wrong' }, expect: 401 },
]