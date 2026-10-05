const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f09 分享：快照冻结/live 跟随/重冻/撤销/公开读取计数/复制到他人库/失效三分
const enc = encodeURIComponent
export const meta = '笔记分享'
export const steps = [
  // 快照分享（默认）
  { t: 'http', name: 'shareCreate', method: 'POST', path: '/api/v1/shares', auth: 'A',
    body: () => ({ path: 'dev/容器笔记.md', mode: 'snapshot', expire_days: 0 }), expect: 200,
    save: { share1: (d) => ({ token: d.share?.token || d.token }) } },
  // 公开读取（无鉴权）：内容=快照 + 计数
  { t: 'http', name: 'sharePublic1', path: () => '/api/v1/public/share/' + S.share1.token,
    assert: (d, S) => { const s = d.share || d; S.assert((s.content || s.note?.content || '').includes('docker'), '公开读应返回快照内容') } },
  { t: 'http', name: 'sharePublic2', path: () => '/api/v1/public/share/' + S.share1.token, expect: 200 },
  // 计数只能从 owner 视角回读（公开响应不含 view_count）；两次公开读后应 ≥2
  { t: 'http', name: 'shareCountGrow', auth: 'A', path: () => '/api/v1/shares?path=' + encodeURIComponent('dev/容器笔记.md'),
    assert: (d) => { const s = d.share || d; S.assert((s.view_count ?? 0) >= 2, `view_count 应 ≥2（实际 ${s.view_count}）`) } },
  // A 更新源笔记 → 快照不变
  { t: 'http', name: 'srcUpdate', method: 'PUT', auth: 'A', path: (S) => '/api/v1/notes/' + enc('dev/容器笔记.md'),
    body: { content: 'docker compose v2 内容。' }, expect: 200 },
  { t: 'http', name: 'snapshotFrozen', path: () => '/api/v1/public/share/' + S.share1.token,
    assert: (d) => { const s = d.share || d; S.assert(!(s.content || '').includes('v2'), '快照不应跟随更新') } },
  // 再分享=upsert：保留 token 重冻快照 → 内容变
  { t: 'http', name: 'shareUpsert', method: 'POST', path: '/api/v1/shares', auth: 'A',
    body: () => ({ path: 'dev/容器笔记.md', mode: 'snapshot', expire_days: 0 }), expect: 200,
    save: { share1b: (d, S) => ({ token: d.share?.token || d.token }) },
    assert: (d, S) => S.assert((d.share?.token || d.token) === S.share1.token, '重分享应保留同一 token') },
  { t: 'http', name: 'snapshotRefrozen', path: () => '/api/v1/public/share/' + S.share1.token,
    assert: (d) => { const s = d.share || d; S.assert((s.content || '').includes('v2'), '重冻后应含新内容') } },
  // live 分享：跟随更新
  { t: 'http', name: 'shareLive', method: 'POST', path: '/api/v1/shares', auth: 'A',
    body: () => ({ path: 'ai/旅程改名.md', mode: 'live', expire_days: 0 }), expect: 200,
    save: { shareLive: (d) => ({ token: d.share?.token || d.token }) } },
  { t: 'http', name: 'liveFollow', path: () => '/api/v1/public/share/' + S.shareLive.token,
    assert: (d) => { const s = d.share || d; S.assert((s.content || '').includes('v2'), 'live 应返回实时内容') } },
  // live 源删除 → 失效三分 source_deleted
  { t: 'http', name: 'liveSrcDel', method: 'DELETE', auth: 'A', path: (S) => '/api/v1/notes/' + enc('ai/旅程改名.md'), expect: 200 },
  { t: 'http', name: 'liveSourceDeleted', path: () => '/api/v1/public/share/' + S.shareLive.token, expect: 404,
    assert: (d) => S.assert(d?.error?.code === 'source_deleted', 'live 源删应回 source_deleted') },
  // 撤销快照分享 → 公开 404（not_found/revoked 语义）
  { t: 'http', name: 'shareRevoke', method: 'DELETE', auth: 'A', path: () => '/api/v1/shares/' + S.share1.token, expect: 200 },
  { t: 'http', name: 'revoked404', path: () => '/api/v1/public/share/' + S.share1.token, expect: 404 },
  // B 复制到自己库 → 回读；自复制同名 → -2 后缀
  { t: 'http', name: 'shareCopyB', method: 'POST', auth: 'B', path: () => '/api/v1/shares/' + S.shareLive.token + '/copy', expect: [200, 404] },
  // 用仍有效的快照分享测复制（share1 已撤销 → 重建一个）
  { t: 'http', name: 'shareForCopy', method: 'POST', path: '/api/v1/shares', auth: 'A',
    body: () => ({ path: 'dev/容器笔记.md', mode: 'snapshot', expire_days: 0 }), expect: 200,
    save: { shareCopy: (d) => ({ token: d.share?.token || d.token }) } },
  { t: 'http', name: 'copyToB', method: 'POST', auth: 'B', path: () => '/api/v1/shares/' + S.shareCopy.token + '/copy', expect: 200,
    save: { copied: (d) => ({ path: d.note?.path || d.path }) } },
  { t: 'http', name: 'copiedReadback', auth: 'B', path: (S) => '/api/v1/notes/' + enc(S.copied.path),
    assert: (d, S) => { const n = d.note || d; S.assert((n.content || '').includes('v2'), 'B 库应有复制内容') } },
  { t: 'http', name: 'copyAgain', method: 'POST', auth: 'B', path: () => '/api/v1/shares/' + S.shareCopy.token + '/copy', expect: 200,
    assert: (d) => S.assert((d.note?.path || d.path || '').includes('-2'), '自复制应自动 -2 后缀') },
  // 负向：不存在的 token
  { t: 'http', name: 'share404', path: '/api/v1/public/share/ffffffffffffffffffffffffffffffff', expect: 404 },
]