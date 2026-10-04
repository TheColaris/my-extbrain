---
name: extbrain
description: 我的外脑（my-extbrain）操作技能——用 extbrain CLI 帮用户记待办、记便签、把整理好的资料存进云端知识库，并在回答前先查库。适用于已部署 my-extbrain 且完成 CLI 配置的环境。
---

# extbrain（我的外脑）

帮用户把「记不住的事」和「整理好的知识」放进他的云端外脑；回答问题前先查他已有的库。

## 前置检查

- 运行 `extbrain me`：输出 `✓ 配置有效` 才可用；报「未配置」时帮用户装好 CLI 后执行设备授权（浏览器点一下「授权」即可，无需密钥）：
  ```bash
  extbrain auth login --server https://<实例地址>
  ```
  （备选：面板「API 密钥」签发 Key 后 `--key ak_live_...`）

## 四模式（判断规则）

### 1. 记待办 —— 用户说「记一下 / 别忘了 / 待会要做 / 提醒我」
```bash
extbrain todo add "周五前交周报" --due 2026-10-09 --tag 工作
```
用户说了明确时间 → `--due`（支持 `2026-10-09` 或 `2026-10-09 18:00`）；没说就省略。

### 2. 记便签 —— 一句话速记、结论、灵感（用户说「记一下这个」且不是任务）
```bash
extbrain memo add "GORM 写 PG text[] 必须用 pq.StringArray"
```
便签 ≤2000 字；更长的内容走模式 3。

### 3. 存知识 —— 调研/整理/总结完成，产物是成篇内容
先查重：`extbrain search "<主题>"` → 无重复才写 MD 文件并推送：
```bash
extbrain note push 调研笔记.md --path ai/<主题>.md
```
- 路径规划：`ai/`（AI 与模型）、`dev/`（开发）、`ops/`（运维）、`读后感/`、或按用户既有目录结构（`extbrain note ls` 先看）
- 仓库：知识库分仓库（仓库 > 文件夹 > 笔记）；不带 `--repo` = 默认仓库，用户指定仓库时加 `--repo 仓库名`
- 内容规范：顶部一级标题=主题；分节讲清「结论先行」；可复制代码块
- 重复主题 → 更新已有路径（PUT 是覆盖式），不要新建冗余文件

### 4. 先查再答 —— 用户问到知识库里可能有的事
```bash
extbrain search "<关键词>"          # 命中摘要
extbrain note cat ai/<主题>.md      # 取全文原文
extbrain note pull ai/ --stdout     # 整个目录拼成 MD bundle（资料多时）
```
查到 → 引用库里的内容回答；查不到 → 正常回答后可建议「要不要存进知识库」。

## 命令速查

| 命令 | 用途 |
|---|---|
| `extbrain todo add/list/done/undo/rm` | 待办（二态 active/done；list 支持 `--status active|done` `--json`） |
| `extbrain memo add/list/pin/rm` | 便签 |
| `extbrain note push/ls/cat/pull/rm` | 知识库（`pull --stdout` 输出 bundle） |
| `extbrain search <词>` | 全文检索（标题/路径/正文） |
| `extbrain me` / `auth show/logout` | 配置管理 |

## 纪律

- 写操作前能查就查（search/ls），防重复
- 不展示用户的 API Key（`auth show` 只显示打码 hint）
- 输出是给 AI/人看的纯文本，直接引用即可；`--json` 用于程序化处理

## MCP 客户端（可选，与上面二选一）

若你的宿主支持 MCP 且已接入外脑 MCP 服务器（`<实例>/mcp`，远程 HTTP 直连），直接用 MCP 工具即可——工具描述自带使用时机与纪律，效果与本 Skill 相同，**不必两者都配**。接入配置见 `<实例>/guide-mcp.md`。
