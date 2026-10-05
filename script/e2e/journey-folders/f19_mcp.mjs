const S = globalThis.J // runner 注入的接力环境（避免循环 import 死锁）
// f19 MCP 通道：tools/list 按 scope 过滤 / 工具调用 / 审计同构 / 越权与无效 Key
export const meta = 'MCP'
export const steps = [
  // K3=all → 16 工具
  { t: 'mcp', name: 'mcpToolsAll', key: 'K3', method: 'tools/list', expect: 200,
    assert: (d) => S.assert(d.result?.tools?.length === 16, `all Key 应 16 工具（实际 ${d.result?.tools?.length}）`) },
  // K2=notes → search + note×6 + whoami = 8
  { t: 'mcp', name: 'mcpToolsNotes', key: 'K2', method: 'tools/list', expect: 200,
    assert: (d) => {
      const names = (d.result?.tools || []).map((x) => x.name)
      S.assert(names.length === 8, `notes Key 应 8 工具（实际 ${names.length}）`)
      S.assert(!names.includes('todo_create'), 'notes Key 不得见 todo 工具') } },
  // whoami
  { t: 'mcp', name: 'mcpWhoami', key: 'K3', method: 'tools/call', params: { name: 'whoami', arguments: {} }, expect: 200,
    assert: (d) => S.assert(JSON.stringify(d.result).includes('旅程超管') || JSON.stringify(d.result).includes('user'), 'whoami 应返回身份') },
  // todo_create → REST 回读（跨通道同源）
  { t: 'mcp', name: 'mcpTodoCreate', key: 'K3', method: 'tools/call',
    params: { name: 'todo_create', arguments: { title: 'MCP 记的待办' } }, expect: 200,
    assert: (d) => S.assert(d.result?.isError !== true, 'todo_create 不应报错') },
  { t: 'http', name: 'mcpTodoReadback', path: '/api/v1/todos', auth: 'A', assert: (d) => {
      const list = d.todos || d.items || d
      S.assert(list.some((x) => x.title === 'MCP 记的待办' && x.source === 'cli'), 'MCP 待办应落库且 source=cli') } },
  // note_write / note_read / search
  { t: 'mcp', name: 'mcpNoteWrite', key: 'K3', method: 'tools/call',
    params: { name: 'note_write', arguments: { path: 'mcp/from-mcp.md', content: '# MCP 写入\n\nmcp 通道内容。', repo: '旅程仓壹' } }, expect: 200 },
  { t: 'mcp', name: 'mcpNoteRead', key: 'K3', method: 'tools/call',
    params: { name: 'note_read', arguments: { path: 'mcp/from-mcp.md', repo: '旅程仓壹' } }, expect: 200,
    assert: (d) => S.assert(JSON.stringify(d.result).includes('MCP 写入'), 'note_read 应回读内容') },
  { t: 'mcp', name: 'mcpSearch', key: 'K3', method: 'tools/call',
    params: { name: 'search', arguments: { q: 'MCP 写入' } }, expect: 200,
    assert: (d) => S.assert(JSON.stringify(d.result).includes('from-mcp.md'), 'search 应命中 MCP 笔记') },
  // 目录权限经 MCP 同样生效：P3 读受限目录 → isError 可执行文案
  { t: 'mcp', name: 'mcpPermDenied', key: 'P3', method: 'tools/call',
    params: { name: 'note_read', arguments: { path: 'research/综述.md' } }, expect: 200,
    assert: (d) => S.assert(d.result?.isError === true && JSON.stringify(d.result).includes('不可见'), '权限拦截应是 isError + 可执行文案') },
  // 负向：notes Key 调 todo 工具 → 工具不存在（协议错误）
  { t: 'mcp', name: 'mcpScopeDenied', key: 'K2', method: 'tools/call',
    params: { name: 'todo_create', arguments: { title: '越权' } }, expect: 200,
    assert: (d) => S.assert(d.error?.code === -32602, '越权工具调用应回 -32602 unknown tool（HTTP 200 包协议错误）') },
  // 负向：无效 Key → 401
  { t: 'mcp', name: 'mcpBadKey', key: 'ak_live_invalid000000000000', method: 'tools/list', expect: 401 },
]