const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f20 E2E 信箱自测 + 全域收尾对账：审计总量/信箱过滤与清空
export const meta = '信箱与收尾对账'
export const steps = [
  // 信箱过滤：channel 维度
  { t: 'http', name: 'mbByChannel', path: '/api/v1/e2e/mailbox?channel=webhook',
    save: { mbByChannel: (d) => ({ count: d.count }) },
    assert: (d) => S.assert(d.entries.every((e) => e.channel === 'webhook'), 'channel 过滤应精确') },
  // 全量信箱 ≥ webhook 过滤量（f13 清箱后 f19 之前无新消息时两者可相等，取 ≥）
  { t: 'http', name: 'mbAll', path: '/api/v1/e2e/mailbox',
    assert: (d, S) => S.assert(d.count >= S.mbByChannel.count, `全量(${d.count}) 应 ≥ 单通道(${S.mbByChannel.count})`) },
  // 清一箱（webpush）
  { t: 'http', name: 'mbClearWebpush', method: 'DELETE', path: '/api/v1/e2e/mailbox?channel=webpush', expect: 200 },
  { t: 'http', name: 'mbWebpushGone', path: '/api/v1/e2e/mailbox?channel=webpush',
    assert: (d) => S.assert(d.count === 0, '清箱后该通道应为 0') },
  // 清全部
  { t: 'http', name: 'mbClearAll', method: 'DELETE', path: '/api/v1/e2e/mailbox', expect: 200 },
  { t: 'http', name: 'mbEmpty', path: '/api/v1/e2e/mailbox', assert: (d) => S.assert(d.count === 0, '全清后应为 0') },
  // 收尾对账：审计里应能找到 CLI/MCP/REST 三种来源痕迹（全程同构落库）
  { t: 'http', name: 'auditFinal', path: '/api/v1/logs?limit=100', auth: 'A', assert: (d) => {
      const items = d.items || d.logs || d
      const actions = new Set(items.map((x) => x.action))
      S.assert(actions.has('todo.create'), '审计应含 todo.create')
      S.assert([...actions].some((a) => a.startsWith('note.')), '审计应含 note.*')
      S.assert([...actions].some((a) => a.startsWith('share.')), '审计应含 share.*') } },
]