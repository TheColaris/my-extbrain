const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f06 仓库：默认仓库/建仓配额/改名/删除守卫/跨仓库寻址与移动
const enc = encodeURIComponent
export const meta = '仓库'
export const steps = [
  // 默认仓库懒建即存在（首个注册用户已带）
  { t: 'http', name: 'repoList', path: '/api/v1/repos', auth: 'A', assert: (d, S) => {
      const list = d.repos || d
      S.assert(list.length >= 1 && list.some((r) => r.is_default), '应有默认仓库')
      const def = list.find((r) => r.is_default)
      S.A = { ...S.A, defaultRepoID: def.id } } },
  // 建仓（配额默认 3，不含默认仓库）
  { t: 'http', name: 'repo1', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: '旅程仓一', description: '跨仓库测试' }, expect: 200,
    save: { repo1: (d) => ({ id: d.repo?.id ?? d.id }) } },
  { t: 'http', name: 'repo2', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: '旅程仓二' }, expect: 200,
    save: { repo2: (d) => ({ id: d.repo?.id ?? d.id }) } },
  { t: 'http', name: 'repo3', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: '旅程仓三' }, expect: 200,
    save: { repo3: (d) => ({ id: d.repo?.id ?? d.id }) } },
  // 负向：第 4 个 → 配额拒绝
  { t: 'http', name: 'repoQuota', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: '旅程仓四' }, expect: [400, 403] },
  // 负向：重名（含软删释放前）
  { t: 'http', name: 'repoDup', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: '旅程仓一' }, expect: [400, 409] },
  // 改名（默认仓库也可改名）
  { t: 'http', name: 'repoRename', method: 'PATCH', auth: 'A', path: () => '/api/v1/repos/' + S.repo1.id, body: { name: '旅程仓壹' }, expect: 200 },
  // 跨仓库寻址：显式 repo 参数写「旅程仓壹」
  { t: 'http', name: 'noteIntoRepo1', method: 'PUT', auth: 'A',
    path: () => '/api/v1/notes/' + enc('需求/仓库化.md') + '?repo=' + enc('旅程仓壹'),
    body: { content: '仓库化设计。' }, expect: 200 },
  { t: 'http', name: 'noteInRepo1Read', auth: 'A',
    path: () => '/api/v1/notes/' + enc('需求/仓库化.md') + '?repo=' + enc('旅程仓壹'), expect: 200 },
  // 跨仓库移动：to 用「仓库名:路径」形态（前缀=目标仓库），from+repo 指定源
  { t: 'http', name: 'noteCrossMove', method: 'POST', path: '/api/v1/notes/move', auth: 'A',
    body: () => ({ from: '需求/仓库化.md', repo: '旅程仓壹', to: '旅程仓二:需求/挪入仓二.md' }), expect: 200 },
  { t: 'http', name: 'crossMovedRead', auth: 'A',
    path: () => '/api/v1/notes/' + enc('需求/挪入仓二.md') + '?repo=' + enc('旅程仓二'), expect: 200 },
  // 删除守卫：非空仓库禁删 → 清空后可删
  { t: 'http', name: 'repoDelNonEmpty', method: 'DELETE', auth: 'A', path: () => '/api/v1/repos/' + S.repo2.id, expect: [400, 403] },
  { t: 'http', name: 'emptyRepo2Note', method: 'DELETE', auth: 'A',
    path: () => '/api/v1/notes/' + enc('需求/挪入仓二.md') + '?repo=' + enc('旅程仓二'), expect: 200 },
  { t: 'http', name: 'repoDelEmpty', method: 'DELETE', auth: 'A', path: () => '/api/v1/repos/' + S.repo2.id, expect: 200 },
  // 负向：非法仓库名（路径分隔符）
  { t: 'http', name: 'repoBadName', method: 'POST', path: '/api/v1/repos', auth: 'A', body: { name: 'a/b' }, expect: 400 },
  // 负向：不存在的仓库
  { t: 'http', name: 'repo404', auth: 'A', path: '/api/v1/notes?repo=' + enc('不存在的仓'), expect: 404 },
]