const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f00 环境与分发面：健康/手册/安装脚本/404/信箱可用性
export const meta = '环境与分发面'
export const steps = [
  { t: 'http', name: 'healthz', path: '/healthz', assert: (d, S) => S.assert(d.status === 'ok' && d.db === true, 'healthz 应 ok 且 db 通') },
  { t: 'http', name: 'guideMd', path: '/guide.md', assert: (d, S, st4, st) => S.assert(st.text.includes('操作手册') && st.text.includes(S.BASE), 'guide.md 应含标题与注入的实例地址') },
  { t: 'http', name: 'guideMcpMd', path: '/guide-mcp.md', assert: (d, S, st4, st) => S.assert(st.text.includes('MCP') && st.text.includes(S.BASE), 'guide-mcp.md 应注入实例地址') },
  { t: 'http', name: 'installSh', path: '/install.sh', assert: (d, S, st4, st) => S.assert(st.text.includes('__BASE__') === false && st.text.includes(S.BASE), 'install.sh 的 __BASE__ 应被注入为实例地址') },
  { t: 'http', name: 'notFoundApi', method: 'DELETE', path: '/api/v1/definitely-not-exist', expect: 404, assert: (d) => S.assert(d?.error?.code === 'not_found', '未知 API（非 GET）应回统一 404 信封') },
  { t: 'http', name: 'mailboxReachable', path: '/api/v1/e2e/mailbox', assert: (d) => S.assert(Array.isArray(d.entries), '洁净室信箱应可达（三重门全开）') },
]