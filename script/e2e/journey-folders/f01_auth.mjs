const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f01 认证与账户：注册（首个=超管）/登录/锁定/改密/换绑/资料 —— token_version 踢会话全程回读
const pass = 'e2e-pass-123'
export const meta = '认证与账户'
export const steps = [
  // 注册 A：洁净库首个用户自动成为平台管理员（产品机制即旅程起点）
  { t: 'http', name: 'regA', method: 'POST', path: '/api/v1/auth/register', body: { account: '138' + S.runId8, password: pass },
    save: { A: (d) => ({ authHeader: 'Bearer ' + d.token, phone: '138' + S.runId8, password: pass, userID: d.user.id }) },
    assert: (d) => S.assert(!!d.token, 'A 注册应发 token') },
  // 负向：重复注册
  { t: 'http', name: 'regDup', method: 'POST', path: '/api/v1/auth/register', body: { account: '138' + S.runId8, password: pass }, expect: [400, 409] },
  // 负向：非法手机号 / 短密码
  { t: 'http', name: 'regBadPhone', method: 'POST', path: '/api/v1/auth/register', body: { account: '12345', password: pass }, expect: 400 },
  { t: 'http', name: 'regShortPwd', method: 'POST', path: '/api/v1/auth/register', body: { account: '136' + S.runId8, password: 'short' }, expect: 400 },
  // B：普通用户（非管理员）
  { t: 'http', name: 'regB', method: 'POST', path: '/api/v1/auth/register', body: { account: '139' + S.runId8, password: pass },
    save: { B: (d) => ({ authHeader: 'Bearer ' + d.token, phone: '139' + S.runId8, password: pass, userID: d.user.id }) },
    assert: (d) => S.assert(!!d.token, 'B 注册应发 token') },
  // me 回读：A=超管（首个注册用户自动 is_admin=1）；B=普通用户
  { t: 'http', name: 'meA', path: '/api/v1/auth/me', auth: 'A', assert: (d, S) => {
      const u = d.user || d
      S.assert(u.id === S.A.userID && u.is_admin === true, `首个注册用户应 is_admin=1（响应=${JSON.stringify(u).slice(0,200)}）`) } },
  { t: 'http', name: 'meB', path: '/api/v1/auth/me', auth: 'B', assert: (d) => {
      const u = d.user || d
      S.assert(u.is_admin === false, '第二个用户不应是管理员') } },
  // 登录：正确/错误密码
  { t: 'http', name: 'loginA', method: 'POST', path: '/api/v1/auth/login', body: () => ({ account: S.A.phone, password: pass }), expect: 200 },
  { t: 'http', name: 'loginWrong', method: 'POST', path: '/api/v1/auth/login', body: () => ({ account: S.A.phone, password: 'wrong-pass-9' }), expect: [400, 401] },
  // 资料更新（部分更新：只改昵称与头像）→ 回读
  { t: 'http', name: 'patchMe', method: 'PATCH', path: '/api/v1/auth/me', auth: 'A', body: { nick_name: '旅程超管', avatar_emoji: '🧠' }, expect: 200 },
  { t: 'http', name: 'meAfterPatch', path: '/api/v1/auth/me', auth: 'A', assert: (d, S) => {
      const u = d.user || d
      S.assert(u.nick_name === '旅程超管' && u.avatar_emoji === '🧠', '昵称/头像应已更新') } },
  // C：登录锁定专用（连续 5 次错误 → 第 6 次正确也拒绝；不污染 A/B）
  { t: 'http', name: 'regC', method: 'POST', path: '/api/v1/auth/register', body: { account: '137' + S.runId8, password: pass },
    save: { C: (d) => ({ phone: '137' + S.runId8 }) } },
  { t: 'fn', name: 'lockC', fn: async (S) => {
      for (let i = 0; i < 5; i++) {
        const r = await fetch(S.BASE + '/api/v1/auth/login', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ account: S.C.phone, password: 'wrong-' + i }) })
        S.assert(r.status === 401 || r.status === 400 || r.status === 429, `第 ${i + 1} 次错误密码应 401`)
      }
      const r6 = await fetch(S.BASE + '/api/v1/auth/login', { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ account: S.C.phone, password: 'e2e-pass-123' }) })
      S.assert(r6.status !== 200, '锁定窗口内正确密码也不应登录成功')
  } },
  // 改密：旧 token 全失效（token_version +1）→ 新 token 有效
  { t: 'http', name: 'changePwd', method: 'PATCH', path: '/api/v1/auth/password', auth: 'B', body: () => ({ old: S.B.password, new: 'e2e-pass-456' }),
    save: { B2: (d, S, res) => { const t = res.headers.get('authorization') || d.token; return { ...S.B, authHeader: 'Bearer ' + t } } }, expect: 200 },
  { t: 'http', name: 'oldTokenDead', path: '/api/v1/auth/me', auth: 'B', expect: 401, assert: (d, S) => { S.B = S.B2; } },
  { t: 'http', name: 'newTokenLive', path: '/api/v1/auth/me', auth: 'B', expect: 200 },
  // 换绑邮箱（密码确认）→ 再 +1 token_version（A 换，B 保留做跨用户负向）
  { t: 'http', name: 'bindEmail', method: 'POST', path: '/api/v1/auth/bind', auth: 'A', body: () => ({ type: 'email', value: 'a' + S.runId8 + '@e2e.test', password: S.A.password }),
    save: { A2: (d, S, res) => { const t = res.headers.get('authorization') || d.token; return { ...S.A, authHeader: 'Bearer ' + t, email: 'a' + S.runId8 + '@e2e.test' } } }, expect: 200 },
  { t: 'http', name: 'aOldTokenDead', path: '/api/v1/auth/me', auth: 'A', expect: 401, assert: (d, S) => { S.A = S.A2 } },
  { t: 'http', name: 'aNewTokenLive', path: '/api/v1/auth/me', auth: 'A', expect: 200 },
]