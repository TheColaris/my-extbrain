const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f17 CLI 设备授权：start → approve（JWT）→ poll 一次性取 Key → 错码/重放负向
export const meta = 'CLI 设备授权'
export const steps = [
  // start：拿授权码与 auth_url
  { t: 'http', name: 'cliStart', method: 'POST', path: '/api/v1/cli-auth/start', expect: 201,
    save: { cliAuth: (d) => ({ code: d.code, authURL: d.auth_url }) },
    assert: (d) => S.assert(d.code && d.auth_url, 'start 应返回授权码与授权 URL') },
  // 负向：未授权 poll → pending（不发给 Key）
  { t: 'http', name: 'pollPending', path: () => '/api/v1/cli-auth/poll?code=' + S.cliAuth.code, expect: 200,
    assert: (d) => S.assert(d.status === 'pending', '未授权应 pending') },
  // 负向：错码 poll → 200 + status=expired（不区分存在性，防授权码枚举）
  { t: 'http', name: 'pollBadCode', path: '/api/v1/cli-auth/poll?code=WRONGCOD', expect: 200,
    assert: (d) => S.assert(d.status === 'expired', '错码应回 expired（防枚举）') },
  // A 在浏览器侧 approve（JWT）
  { t: 'http', name: 'approve', method: 'POST', path: '/api/v1/cli-auth/approve', auth: 'A', body: () => ({ code: S.cliAuth.code }), expect: 200 },
  // 负向：重复 approve（pending 已消费）
  { t: 'http', name: 'approveReplay', method: 'POST', path: '/api/v1/cli-auth/approve', auth: 'A', body: () => ({ code: S.cliAuth.code }), expect: [400, 404, 409] },
  // poll 一次性取走 Key
  { t: 'http', name: 'pollTake', path: () => '/api/v1/cli-auth/poll?code=' + S.cliAuth.code, expect: 200,
    save: { cliKey: (d) => ({ rawKey: d.key, authHeader: 'Bearer ' + d.key }) },
    assert: (d) => S.assert(d.status === 'approved' && (d.key || '').startsWith('ak_live_'), 'approved 应带一次性完整 Key') },
  // 二次 poll：已被取走 → 失效
  { t: 'http', name: 'pollReplay', path: () => '/api/v1/cli-auth/poll?code=' + S.cliAuth.code, expect: [200, 400, 404, 410],
    assert: (d) => S.assert(!d.key, '重放 poll 不得再拿到 Key') },
  // 授权来的 Key 真实可用（notes 域搜索）
  { t: 'http', name: 'cliKeyWorks', path: '/api/v1/search?q=docker', auth: 'cliKey', expect: 200 },
]