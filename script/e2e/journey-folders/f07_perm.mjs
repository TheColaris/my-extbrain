const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f07 目录权限矩阵：白名单/最深前缀优先/读写同权/跨仓库隔离/列表与搜索不泄露/Web JWT 不受限
const enc = encodeURIComponent
const put = (name, path, body, auth, expect, extra = {}) => ({ t: 'http', name, method: 'PUT', path, body, auth, expect, ...extra })
export const meta = '目录权限'
export const steps = [
  // 三把权限测试 Key（A 名下）
  { t: 'http', name: 'p1', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '权限P1' },
    save: { P1: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) }, expect: 201 },
  { t: 'http', name: 'p2', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '权限P2' },
    save: { P2: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) }, expect: 201 },
  { t: 'http', name: 'p3', method: 'POST', path: '/api/v1/keys', auth: 'A', body: { name: '权限P3' },
    save: { P3: (d) => ({ rawKey: d.full_key, id: d.key.id, authHeader: 'Bearer ' + d.full_key }) }, expect: 201 },
  // PATCH 收窄 scope（签发默认 all；旅程要求 notes 即可，all 亦可——收窄验证 PATCH 生效）
  { t: 'http', name: 'p1Scope', method: 'PATCH', path: () => '/api/v1/keys/' + S.P1.id, auth: 'A', body: { scope: 'notes' }, expect: 200 },
  { t: 'http', name: 'p2Scope', method: 'PATCH', path: () => '/api/v1/keys/' + S.P2.id, auth: 'A', body: { scope: 'notes' }, expect: 200 },
  { t: 'http', name: 'p3Scope', method: 'PATCH', path: () => '/api/v1/keys/' + S.P3.id, auth: 'A', body: { scope: 'notes' }, expect: 200 },
  // 种笔记：根级 / research/ / finance/
  put('permRootNote', (S) => '/api/v1/notes/' + enc('根级公开.md'), { content: '根级内容 keywordRoot' }, 'A', 200),
  put('permResNote', (S) => '/api/v1/notes/' + enc('research/综述.md'), { content: '研究综述 keywordRes' }, 'A', 200),
  put('permFinNote', (S) => '/api/v1/notes/' + enc('finance/预算.md'), { content: '预算机密 keywordFin' }, 'A', 200),
  // 规则：仓库级''=[P1]；research/=[P1,P2]；finance/=[]（完全封闭）——最深前缀优先
  { t: 'http', name: 'permRepoLevel', method: 'PUT', path: '/api/v1/note-perm', auth: 'A',
    body: () => ({ repo_id: S.A.defaultRepoID, folder_path: '', mode: 'allow', key_ids: [S.P1.id] }), expect: 200 },
  { t: 'http', name: 'permResearch', method: 'PUT', path: '/api/v1/note-perm', auth: 'A',
    body: () => ({ repo_id: S.A.defaultRepoID, folder_path: 'research', mode: 'allow', key_ids: [S.P1.id, S.P2.id] }), expect: 200 },
  { t: 'http', name: 'permFinance', method: 'PUT', path: '/api/v1/note-perm', auth: 'A',
    body: () => ({ repo_id: S.A.defaultRepoID, folder_path: 'finance', mode: 'allow', key_ids: [] }), expect: 200 },
  // 规则回读
  { t: 'http', name: 'permList', path: '/api/v1/note-perm', auth: 'A', assert: (d, S) => {
      const rules = d.rules || d
      S.assert(rules.length >= 3, '应有 ≥3 条规则') } },
  // 矩阵：P1 全可见（除 finance）；P2 仅 research；P3 全不可见
  { t: 'http', name: 'p1RootOk', auth: 'P1', path: (S) => '/api/v1/notes/' + enc('根级公开.md'), expect: 200 },
  { t: 'http', name: 'p1ResOk', auth: 'P1', path: (S) => '/api/v1/notes/' + enc('research/综述.md'), expect: 200 },
  { t: 'http', name: 'p1FinDenied', auth: 'P1', path: (S) => '/api/v1/notes/' + enc('finance/预算.md'), expect: 403 },
  { t: 'http', name: 'p2RootDenied', auth: 'P2', path: (S) => '/api/v1/notes/' + enc('根级公开.md'), expect: 403 },
  { t: 'http', name: 'p2ResOk', auth: 'P2', path: (S) => '/api/v1/notes/' + enc('research/综述.md'), expect: 200 },
  { t: 'http', name: 'p3ResDenied', auth: 'P3', path: (S) => '/api/v1/notes/' + enc('research/综述.md'), expect: 403 },
  // 读写同权：不可见即写不进
  { t: 'http', name: 'p2WriteResOk', method: 'PUT', auth: 'P2', path: (S) => '/api/v1/notes/' + enc('research/新论文.md'), body: { content: 'P2 写入' }, expect: 200 },
  { t: 'http', name: 'p3WriteDenied', method: 'PUT', auth: 'P3', path: (S) => '/api/v1/notes/' + enc('research/越权.md'), body: { content: 'x' }, expect: 403 },
  // 列表过滤：P2 的列表只见 research（根级不可见即目录名也不泄露）
  { t: 'http', name: 'p2ListFiltered', auth: 'P2', path: '/api/v1/notes?prefix=', assert: (d, S) => {
      const dirs = d.dirs || []
      S.assert(!dirs.includes('finance') && !dirs.includes('根级公开.md'), '受限目录/文件不应出现在列表') } },
  // 搜索不泄露：P2 搜 finance 内容 → 0 命中；P1 搜 finance → 0（finance 全封闭）
  { t: 'http', name: 'p2SearchNoLeak', auth: 'P2', path: '/api/v1/search?q=keywordFin', assert: (d) => S.assert(!(d.notes || []).some((h) => (h.path || '').includes('finance')), 'finance 内容不得出现在 P2 搜索结果') },
  { t: 'http', name: 'p1SearchNoLeak', auth: 'P1', path: '/api/v1/search?q=keywordFin', assert: (d) => S.assert(!(d.notes || []).some((h) => (h.path || '').includes('finance')), 'finance 内容不得出现在 P1 搜索结果') },
  // 跨仓库隔离：默认仓库的规则不影响其他仓库——P3 先写旅程仓壹（无规则=开放）再读回
  { t: 'http', name: 'isolatedWrite', method: 'PUT', auth: 'P3',
    path: (S) => '/api/v1/notes/' + enc('隔离验证.md') + '?repo=' + enc('旅程仓壹'), body: { content: 'P3 在无规则仓库写入' }, expect: 200 },
  { t: 'http', name: 'isolatedRepoOpen', auth: 'P3',
    path: (S) => '/api/v1/notes/' + enc('隔离验证.md') + '?repo=' + enc('旅程仓壹'), expect: 200 },
  // Web JWT 不受限（A 本人读 finance ok）
  { t: 'http', name: 'jwtUnrestricted', auth: 'A', path: (S) => '/api/v1/notes/' + enc('finance/预算.md'), expect: 200 },
  // 负向：把 B 名下的 Key 配进 A 的规则 → 拒绝（Key 归属校验）
  { t: 'http', name: 'bKeyIssue', method: 'POST', path: '/api/v1/keys', auth: 'B', body: { name: 'B 的key' },
    save: { BKey: (d) => ({ id: d.key?.id }) }, expect: 201 },
  { t: 'http', name: 'foreignKeyDenied', method: 'PUT', path: '/api/v1/note-perm', auth: 'A',
    body: () => ({ repo_id: S.A.defaultRepoID, folder_path: 'steal', mode: 'allow', key_ids: [S.BKey.id] }), expect: 400 },
]