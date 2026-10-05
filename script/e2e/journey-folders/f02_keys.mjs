const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f02 API 密钥：签发（默认 all）/PATCH 设 scope/列表打码/编辑/吊销/scope 越权/他人 Key 隔离
export const meta = 'API 密钥'
export const steps = [
  // 签发（接口契约 {name, expire_days}；scope 默认 all，签发后 PATCH 收窄）
  { t: 'http', name: 'issueK1', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '旅程todoKey' }, expect: 201,
    save: { K1: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) } },
  { t: 'http', name: 'issueK2', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '旅程notesKey' }, expect: 201,
    save: { K2: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) } },
  { t: 'http', name: 'issueK3', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '旅程allKey', expire_days: 7 }, expect: 201,
    save: { K3: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) },
    assert: (d) => S.assert(!!d.full_key && d.full_key.startsWith('ak_live_'), '应返回一次性完整 Key（ak_live_ 前缀）') },
  // PATCH 收窄 scope
  { t: 'http', name: 'k1Todo', method: 'PATCH', path: () => '/api/v1/keys/' + S.K1.id, auth: 'A', body: { scope: 'todo' }, expect: 200 },
  { t: 'http', name: 'k2Notes', method: 'PATCH', path: () => '/api/v1/keys/' + S.K2.id, auth: 'A', body: { scope: 'notes' }, expect: 200 },
  // 回读：列表含三把；hint 打码不出现完整 key；K3 带过期时间
  { t: 'http', name: 'keysList', path: '/api/v1/keys', auth: 'A', assert: (d, S) => {
      const list = d.keys
      S.assert(list.length >= 3, '列表应有 ≥3 把 Key')
      S.assert(list.every((k) => !String(k.key_hint || '').includes(S.K3.rawKey)), 'hint 必须打码，不得含完整 Key')
      S.assert(list.some((k) => k.id === S.K3.id && k.expire_time), 'K3 应带过期时间')
      S.assert(list.find((k) => k.id === S.K1.id)?.scope === 'todo', 'K1 scope 应为 todo') } },
  // 编辑：改名
  { t: 'http', name: 'renameK3', method: 'PATCH', path: () => '/api/v1/keys/' + S.K3.id, auth: 'A', body: { name: '旅程allKey改' }, expect: 200 },
  // 负向：非法 scope（PATCH 层枚举校验）
  { t: 'http', name: 'badScope', method: 'PATCH', path: () => '/api/v1/keys/' + S.K3.id, auth: 'A', body: { scope: 'root' }, expect: 400 },
  // K1（todo scope）打待办 ok
  { t: 'http', name: 'k1TodoOk', path: '/api/v1/todos?limit=1', auth: 'K1', expect: 200 },
  // K1 打知识库 → 403（scope 越权）
  { t: 'http', name: 'k1NotesDenied', path: '/api/v1/notes?prefix=', auth: 'K1', expect: 403 },
  // K2（notes scope）打待办 → 403
  { t: 'http', name: 'k2TodoDenied', path: '/api/v1/todos?limit=1', auth: 'K2', expect: 403 },
  // B 的 JWT 动 A 的 Key → 404（他人 key 不可操作）
  { t: 'http', name: 'bTouchAKey', method: 'PATCH', path: () => '/api/v1/keys/' + S.K1.id, auth: 'B', body: { name: '偷改' }, expect: 404 },
  // 无效 Key → 401
  { t: 'http', name: 'badKey', path: '/api/v1/todos?limit=1', headers: { authorization: 'Bearer ak_live_deadbeefdeadbeef' }, expect: 401 },
  // 吊销 K1 → 立即 401
  { t: 'http', name: 'revokeK1', method: 'DELETE', path: () => '/api/v1/keys/' + S.K1.id, auth: 'A', expect: 200 },
  { t: 'http', name: 'k1DeadAfterRevoke', path: '/api/v1/todos?limit=1', auth: 'K1', expect: 401 },
]
