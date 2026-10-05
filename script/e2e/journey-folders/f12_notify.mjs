const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f12 通知渠道与 Web Push：钉钉/飞书建渠道即测试（洁净捕获断言信箱）/启停/删除/VAPID/订阅冲突
export const meta = '通知渠道与推送'
const hookDing = 'https://dingtalk-e2e.invalid/robot?access_token=e2e'
const hookFeishu = 'https://feishu-e2e.invalid/open-apis/bot/v2/hook/e2e'
export const steps = [
  // 建钉钉渠道：创建即测试 → 信箱应收到截获的 webhook
  { t: 'http', name: 'createDing', method: 'POST', path: '/api/v1/notify/channels', auth: 'A',
    body: { channel_type: 'dingtalk', webhook_url: hookDing }, expect: 201,
    save: { ding: (d) => ({ id: d.channel?.id ?? d.id }) } },
  { t: 'http', name: 'dingMailbox', path: '/api/v1/e2e/mailbox?channel=webhook&target=' + encodeURIComponent(hookDing),
    assert: (d) => {
      S.assert(d.count >= 1, '钉钉 webhook 应被截获落信箱')
      const e = d.entries[0]
      S.assert((e.payload || '').includes('markdown'), 'payload 应为钉钉消息体 JSON') } },
  // 飞书加签渠道：payload 应含 sign/timestamp
  { t: 'http', name: 'createFeishu', method: 'POST', path: '/api/v1/notify/channels', auth: 'A',
    body: { channel_type: 'feishu', webhook_url: hookFeishu, secret: 'e2e-sign-secret' }, expect: 201,
    save: { feishu: (d) => ({ id: d.channel?.id ?? d.id }) } },
  { t: 'http', name: 'feishuSigned', path: '/api/v1/e2e/mailbox?channel=webhook&target=' + encodeURIComponent(hookFeishu),
    assert: (d) => {
      S.assert(d.count >= 1, '飞书 webhook 应被截获')
      const p = JSON.parse(d.entries[0].payload)
      S.assert(p.timestamp && p.sign, '加签渠道 payload 应含 timestamp/sign') } },
  // 负向：http（非 https）webhook 拒绝
  { t: 'http', name: 'badHook', method: 'POST', path: '/api/v1/notify/channels', auth: 'A',
    body: { channel_type: 'dingtalk', webhook_url: 'http://insecure.e2e.invalid/hook' }, expect: 400 },
  // 渠道列表 / 停用 / 启用
  { t: 'http', name: 'chList', path: '/api/v1/notify/channels', auth: 'A',
    assert: (d, S) => S.assert((d.channels || d).length >= 2, '应有 ≥2 个渠道') },
  { t: 'http', name: 'chDisable', method: 'PATCH', auth: 'A', path: () => '/api/v1/notify/channels/' + S.feishu.id, body: { is_enabled: false }, expect: 200 },
  { t: 'http', name: 'chEnable', method: 'PATCH', auth: 'A', path: () => '/api/v1/notify/channels/' + S.feishu.id, body: { is_enabled: true }, expect: 200 },
  // VAPID：洁净配方注入密钥 → enabled
  { t: 'http', name: 'vapid', path: '/api/v1/push/vapid', auth: 'A',
    assert: (d, S) => { S.assert(d.enabled === true, '洁净环境应已配 VAPID'); S.vapidPub = d.public_key } },
  // A 订阅浏览器推送（伪造 endpoint；E2E 下不会真发）
  { t: 'http', name: 'subA', method: 'POST', path: '/api/v1/push/subscriptions', auth: 'A',
    body: { endpoint: 'https://fcm-e2e.invalid/sub/' + S.runId8, keys: { p256dh: 'e2e-p256dh', auth: 'e2e-auth' } }, expect: [200, 201] },
  // 负向：B 接管同 endpoint → 拒绝
  { t: 'http', name: 'subConflict', method: 'POST', path: '/api/v1/push/subscriptions', auth: 'B',
    body: () => ({ endpoint: 'https://fcm-e2e.invalid/sub/' + S.runId8, keys: { p256dh: 'x', auth: 'y' } }), expect: 400 },
  // 测试推送：信箱 webpush 应收到 + 推送记录留痕
  { t: 'http', name: 'pushTest', method: 'POST', path: '/api/v1/push/test', auth: 'A', expect: 200 },
  { t: 'http', name: 'webpushMailbox', path: '/api/v1/e2e/mailbox?channel=webpush',
    assert: (d) => {
      S.assert(d.count >= 1, 'webpush 应被截获落信箱')
      const p = JSON.parse(d.entries[0].payload)
      S.assert(p.title && p.body, '推送 payload 应含 title/body') } },
  { t: 'http', name: 'pushLogs', path: '/api/v1/notify/logs?days=7', auth: 'A',
    assert: (d, S) => {
      const events = d.events || d
      S.assert(events.length >= 1, '通知记录应有事件')
      S.lastLogID = (events[0]?.channels?.[0]?.id) || events[0]?.id } },
  // 重推（对一条投递重试）
  { t: 'http', name: 'retryLog', method: 'POST', auth: 'A', path: () => '/api/v1/notify/logs/' + S.lastLogID + '/retry', expect: [200, 404] },
  // 删除渠道 + 退订
  { t: 'http', name: 'chDelete', method: 'DELETE', auth: 'A', path: () => '/api/v1/notify/channels/' + S.feishu.id, expect: 200 },
  { t: 'http', name: 'unsub', method: 'DELETE', auth: 'A',
    path: () => '/api/v1/push/subscriptions?endpoint=' + encodeURIComponent('https://fcm-e2e.invalid/sub/' + S.runId8), expect: 200 },
]