# extbrain MCP 接入（AI 执行手册）

替用户把「我的外脑」（`{{BASE}}`）接成当前宿主的 MCP 服务器：写配置 → 向用户要 Key → 验证连通。能直接执行的直接执行，唯一要用户亲手做的只有「要 Key」。

## 前置检查

- 调用 `whoami` 工具：有返回 = 已接入，直接按「使用规则」干活，不用往下
- 宿主判断：有 `claude` 命令 = Claude Code；有 `~/.cursor/` = Cursor；有 `~/.zcode/` = ZCode；有 `~/Library/Application Support/Claude/` = Claude Desktop（需 Node.js，`node -v` 检查，缺则 `brew install node`）

## 三步接入

### 1. 写配置（Key 先占位 `<KEY>`）

按宿主执行对应一种，端点都是 `{{BASE}}/mcp`：

Claude Code：

```bash
claude mcp add --transport http extbrain {{BASE}}/mcp --header "Authorization: Bearer <KEY>"
```

Cursor——写 `~/.cursor/mcp.json`（已有则只合并 extbrain）：

```json
{ "mcpServers": { "extbrain": { "url": "{{BASE}}/mcp", "headers": { "Authorization": "Bearer <KEY>" } } } }
```

ZCode——写 `~/.zcode/cli/config.json` 的 `mcp.servers`：

```json
{ "extbrain": { "type": "http", "url": "{{BASE}}/mcp", "headers": { "Authorization": "Bearer <KEY>" }, "enabled": true } }
```

Claude Desktop——写 `~/Library/Application Support/Claude/claude_desktop_config.json`（Windows `%APPDATA%\Claude`；已有则合并）：

```json
{ "mcpServers": { "extbrain": { "command": "npx", "args": ["-y", "mcp-remote", "{{BASE}}/mcp", "--header", "Authorization: Bearer <KEY>"] } } }
```

### 2. 向用户要 Key（唯一要用户亲手做的）

对用户说：

> 请打开 {{BASE}} → 登录 → 「API 密钥」→ 签发一把 Key（名称建议填 MCP）→ 把 `ak_live_` 开头的完整 Key 粘贴给我

拿到后：配置里的 `<KEY>` 全部替换成真实 Key。Key 只进配置文件，不进对话、不进日志。

### 3. 验证

让用户重启客户端（Claude Code 即时生效，其余要重启）→ 调 `whoami`：返回用户与权限 = 接入成功 → 调 `todo_create` 记一条测试待办，让用户到面板确认 → 汇报结果。

失败排查：报 401 = Key 没替换干净；连不上 = 地址或网络问题。

## 使用规则（接入后长期遵守）

### 判断规则

- 用户说「记一下 / 别忘了 / 提醒我」→ `todo_create`（说了时间带 due，支持 `2026-10-09` 或 `2026-10-09 18:00`）
- 一句话速记、结论、灵感 → `memo_create`（≤2000 字）；成篇内容 → `note_write`
- 回答用户问题前 → 先 `search` 查库，查到就引用并给笔记路径
- 存知识前 → 先 `search` 查重，重复主题**更新已有路径**，不新建冗余文件

### 纪律

- 删除 / 覆盖前先向用户确认
- 任何情况不向用户展示 API Key
- 「该目录对你的 Key 不可见」= 目录权限，正常现象，直接告知用户，不要反复尝试
- 知识库分仓库（仓库 > 文件夹 > 笔记）：`note_*` 工具不带 `repo` = 默认仓库；用户指定仓库时带 `repo` 字段（`note_list` 不带参数会先列仓库清单）
