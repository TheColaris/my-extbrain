# my-extbrain · 我的外脑

**开源自托管 AI 外脑：待办 + 便签 + 纯 Markdown 知识库。**

![License](https://img.shields.io/badge/License-MIT-blue.svg)
![Backend](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![Frontend](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)

AI 通过 CLI 或 MCP 直接读写（API Key 鉴权、目录级权限、全程审计），人通过 Web 面板管理——一切为「人机共用一个本子」和「可复制」设计：任何笔记都能整篇复制原文，随时喂给别的 AI。

- **单二进制部署**：Go 编译时把前端 embed 进一个可执行文件，Docker Compose 一条命令起服务
- **三种接入，数据同源**：CLI / MCP / Web 面板，改一处三端可见
- **纯 Markdown 存储**：笔记就是 `.md` 原文，支持整体导出，没有私有格式锁定

## 三分钟理解这个项目

1. 对话里说「帮我记一下」→ AI 执行 `extbrain todo add "交报告"` → 面板出现待办，状态可流转
2. AI 调研完 → 整理成 Markdown → `extbrain note push 笔记.md` → 云端知识库多一篇可全文复制的笔记
3. 任何设备打开面板：复制 API Key 的配置命令、复制整篇 MD 原文喂给别的 AI

## 功能特性

| 模块 | 能力 |
|---|---|
| **待办** | 状态流转、到期时间、Web Push / 钉钉 / 飞书提醒 |
| **便签** | 一句话速记，随手记不丢 |
| **知识库** | 仓库 > 文件夹 > 笔记三层组织；**目录级 AI 权限**（哪些目录允许 AI 读/写，按 Key 生效）；关键词 + 向量混合检索；分享链接三形态、随时收回；回收站；整体导出 zip |
| **多 AI 接入** | CLI（设备授权，浏览器点一下即完成）/ MCP（16 个工具）/ 面板直用 |
| **安全与审计** | API Key 分 scope、目录白名单、操作日志全量落库（含 IP 归属地）、全端点限流、注册开关 |

## 快速开始（自托管）

前置：Docker、Node 20+ 与 pnpm（构建前端用）。

```bash
git clone https://github.com/TheColaris/my-extbrain.git
cd my-extbrain

# 1) 构建前端产物并放入 embed 目录（仓库不含构建产物）
pnpm --dir web install && pnpm --dir web build
rm -rf server/cmd/app/dist && cp -R web/dist server/cmd/app/dist

# 2) 配置并启动（compose 起 app + pgvector/pg16，启动时自动执行数据库迁移）
cp server/env-template server/.env   # 至少填写 DATABASE_URL、JWT_SECRET（openssl rand -hex 32）
docker compose -f server/docker-compose.yaml up -d --build

# 3) 验证
curl http://127.0.0.1:8080/healthz
```

浏览器打开 `http://127.0.0.1:8080` 注册账号——**首个注册用户自动成为管理员**。数据库也可换用 Supabase（`DATABASE_URL` 指向其 Session Pooler 即可），细节见 [server/README.md](server/README.md)。

### 公网部署安全基线

- `JWT_SECRET` 必须随机且 ≥32 位（占位值服务会拒绝启动）
- 部署后立即注册管理员，然后建议设 `REGISTER_ENABLED=false` 防抢注
- 建议置于反向代理之后，配置 `PUBLIC_BASE_URL` 固定下载/回调地址

## CLI（给 AI / 终端用）

```bash
# 方式一：从你部署好的实例一键安装（脚本按当前平台自动下载）
curl -fsSL https://<你的实例>/install.sh | sh

# 方式二：源码构建
cd server && go build -o /usr/local/bin/extbrain ./cmd/extbrain

# 配置：设备授权（推荐）——浏览器点一下「授权」即可，无需手动配密钥
extbrain auth login --server https://<你的实例>
# 备选：面板「API 密钥」签发 Key 后 --key ak_live_...

extbrain todo add "试试记一条" --due 2026-01-01
extbrain memo add "一句话也值得记"
printf '# 第一篇\n内容' > n.md && extbrain note push n.md --path ai/第一篇.md
extbrain search "第一篇"
```

### AI Skill

[`server/skill/`](server/skill) 内置 Skill 包：装进支持 skills 的 AI CLI 后，AI 自动学会「记一下 → todo add / 存起来 → note push / 答前先 search」的行为约定，无需在系统提示词里手写说明书。

## MCP 接入（AI 客户端直连）

支持 MCP 的客户端（Claude Code / ZCode / Cursor 等）可不经 CLI 直接读写，与 CLI 二选一：

```bash
claude mcp add --transport http extbrain https://<你的实例>/mcp --header "Authorization: Bearer ak_live_..."
```

- 端点 `/mcp`（Streamable HTTP，无状态），鉴权仅 API Key，与 REST 共享限流配额
- 16 个工具（search / todo×4 / memo×4 / note×6 / whoami），按 Key 的 scope 过滤可见工具，每次调用落审计日志
- 完整手册：部署后访问 `<你的实例>/guide-mcp.md`，或网页 `/guide` 页「MCP 版」标签页

## 目录结构

```
server/    Go 后端 + CLI + AI Skill 包（前端 embed 进单二进制）
web/       React 前端（BoldKit / neubrutalism 风格）
```

## 技术栈

- **后端**：Go · Gin · GORM · golang-migrate（SQL 迁移 embed 进二进制，启动自动执行）
- **前端**：React 19 · Vite · TypeScript · Tailwind CSS v4 · BoldKit
- **数据库**：Postgres（pgvector 向量检索；Supabase 或自托管均可）

## License

[MIT](LICENSE)
