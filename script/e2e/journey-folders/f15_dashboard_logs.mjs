const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f15 仪表盘 + f16 审计日志（合并为面板只读域）
export const meta = '仪表盘与审计日志'
export const steps = [
  // 仪表盘：数字带/今日待办/14 天趋势
  { t: 'http', name: 'dash', path: '/api/v1/dashboard', auth: 'A', assert: (d) => {
      S.assert(Array.isArray(d.daily) && d.daily.length === 14, '应含 14 天趋势')
      S.assert(d.counts || d.stats || d.today_todos !== undefined, '应含数字带/今日待办') } },
  // 审计日志：旅程全程写操作应已落审计
  { t: 'http', name: 'logs', path: '/api/v1/logs?limit=50', auth: 'A', assert: (d) => {
      const items = d.items || d.logs || d
      S.assert(items.length >= 5, '审计日志应有数据')
      S.assert(items.some((x) => String(x.action).startsWith('todo.')), '应含 todo.* 动作')
      S.firstBefore = items[items.length - 1]?.id } },
  // 过滤：仅 Web 会话（key_id=-1）
  { t: 'http', name: 'logsWebOnly', path: '/api/v1/logs?limit=50&key_id=-1', auth: 'A', assert: (d) => {
      const items = d.items || d.logs || d
      S.assert(items.every((x) => (x.api_key_id ?? 0) === 0), 'key_id=-1 应只回 Web 会话行') } },
  // 分页：before_id 游标
  { t: 'http', name: 'logsPage2', path: () => '/api/v1/logs?limit=5&before_id=' + (S.firstBefore || 0), auth: 'A',
    assert: (d) => { const items = d.items || d.logs || d; S.assert(items.every((x) => x.id < S.firstBefore), '游标分页应只回更旧行') } },
  // 负向：B 看不到 A 的日志（跨用户隔离）
  { t: 'http', name: 'logsIsolated', path: '/api/v1/logs?limit=50', auth: 'B', assert: (d, S) => {
      const items = d.items || d.logs || d
      S.assert(items.every((x) => x.user_id === S.B.userID), 'B 只应看到自己的审计行') } },
]