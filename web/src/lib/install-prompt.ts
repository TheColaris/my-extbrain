// 「复制安装指令给 AI」提示词：首页三步卡与上手引导收尾共用（地址随当前域名生成，换域名无需改文档）
export function aiInstallPrompt(): string {
  const origin = location.origin
  return [
    `帮我装 ${origin} 这个「外脑」（我的个人信息库）Skill，并帮我配上我的 API 密钥：`,
    '',
    '1. 安装 CLI 与 Skill：',
    `   curl -fsSL ${origin}/install.sh | sh`,
    '2. 登录授权（会打开浏览器，我点一下「授权」即可）：',
    `   extbrain auth login --server ${origin}`,
    '3. 装好后：我说「记一下」就用 extbrain todo add；我说「存起来」就写 MD 后 extbrain note push；回答我问题前先 extbrain search 查我的库。',
  ].join('\n')
}
