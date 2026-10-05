const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f18 CLI 真二进制通道：auth login --key → me/todo/memo/note/search 全命令 → logout
// HOME 隔离在 .cli-home/cliA（config.json 不落真机家目录）
const H = 'cliA'
export const meta = 'CLI 真二进制'
export const steps = [
  { t: 'cli', name: 'cliLogin', home: H, cleanHome: true,
    args: (S) => ['auth', 'login', '--server', S.BASE, '--key', S.K3.rawKey],
    expectOut: ['✓ 配置完成'] },
  { t: 'cli', name: 'cliMe', home: H, args: ['me'], expectOut: ['✓ 配置有效'] },
  { t: 'cli', name: 'cliTodoAdd', home: H, args: ['todo', 'add', 'CLI 记的待办', '--due', '2026-12-01', '--tag', 'cli'], expectOut: ['✓'] },
  { t: 'cli', name: 'cliTodoList', home: H, args: ['todo', 'list'], expectOut: ['CLI 记的待办'] },
  { t: 'cli', name: 'cliMemoAdd', home: H, args: ['memo', 'add', 'CLI 记的便签'], expectOut: ['✓'] },
  { t: 'fn', name: 'prepNoteFile', fn: (S) => S.writeFile(S.noteFile, '# CLI 笔记\n\nCLI push 的内容。') },
  // 推到「旅程仓壹」（f07 后该仓无权限规则=开放；顺带验证 CLI --repo 跨仓库）
  { t: 'cli', name: 'cliNotePush', home: H, args: (S) => ['note', 'push', S.noteFile, '--path', 'cli/from-cli.md', '--repo', '旅程仓壹'], expectOut: ['✓'] },
  { t: 'cli', name: 'cliNoteLs', home: H, args: ['note', 'ls', '--repo', '旅程仓壹'], expectOut: ['from-cli.md'] },
  { t: 'cli', name: 'cliNoteCat', home: H, args: ['note', 'cat', 'cli/from-cli.md', '--repo', '旅程仓壹'], expectOut: ['CLI push 的内容'] },
  { t: 'cli', name: 'cliSearch', home: H, args: ['search', 'CLI 笔记'], expectOut: ['cli/from-cli.md'] },
  // 回读断言：CLI 写的待办在 Web 侧可见（跨端同源）
  { t: 'http', name: 'cliTodoWebVisible', path: '/api/v1/todos', auth: 'A', assert: (d) => {
      const list = d.todos || d.items || d
      S.assert(list.some((x) => x.title === 'CLI 记的待办' && x.source === 'cli'), 'CLI 待办应带 source=cli 出现在面板') } },
  { t: 'cli', name: 'cliAuthShow', home: H, args: ['auth', 'show'], expectOut: ['server:'] },
  { t: 'cli', name: 'cliLogout', home: H, args: ['auth', 'logout'], expectOut: ['✓ 已清除配置'] },
  { t: 'cli', name: 'cliMeAfterLogout', home: H, args: ['me'], expectCode: 1, expectOut: ['未配置'] },
]