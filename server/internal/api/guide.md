# 我的外脑 · 操作手册

> **本文件可直接交给 AI 使用。** 请把本手册当作你的操作指南，代替用户操作他的「我的外脑」（地址：`{{BASE}}`）：
>
> - 用户说「记一下 / 别忘了 / 待会要做 / 提醒我」→ 按 §3.1 记待办
> - 用户说「存起来 / 整理好了」→ 按 §3.3 存知识库
> - 回答用户问题前 → 按 §3.4 先搜外脑
> - 若 `extbrain` 命令不存在 → 先按 §1 完成接入

> **接入方式二选一**：本手册 = Skill/CLI 方式（任何 AI 宿主可用）。若你的客户端支持 MCP（Claude Code / ZCode / Cursor / Claude Desktop 等），可改用 MCP 接入手册：`{{BASE}}/guide-mcp.md`（或 /guide 页「MCP 版」标签页）。两种方式能力相同，不必同时配置。

## 1. 快速接入（一次性，约 1 分钟）

**① 安装 CLI 与 Skill**（脚本自动识别系统与架构）：

```bash
curl -fsSL {{BASE}}/install.sh | sh
```

**② 登录授权**（自动打开浏览器，点一下「授权」即可，无需密钥）：

```bash
extbrain auth login --server {{BASE}}
```

**③ 验证**：`extbrain me` 输出 `✓ 配置有效` 即接入完成。

## 2. 能力总览

| 能力 | 是什么 |
|---|---|
| 待办 | 要做的事，可带截止时间与标签；AI 记，人勾 |
| 便签 | 一句话速记、结论、灵感（≤2000 字） |
| 知识库 | 成篇资料，Markdown 按目录归档、全文可搜 |
| 搜索 | 待办 / 便签 / 笔记，一处搜全部 |
| 网页面板 | 浏览管理全部内容、分享笔记、签发密钥、看日志 |

## 3. 操作规则

### 3.1 记待办 —— 触发：「记一下 / 别忘了 / 待会要做 / 提醒我」

```bash
extbrain todo add "周五前交周报" --due 2026-10-09 --tag 工作
```

- 用户说了明确时间 → 带 `--due`（支持 `2026-10-09` 或 `2026-10-09 18:00`）；没说就省略
- 完成 / 恢复 / 删除：`extbrain todo done <id>` · `todo undo <id>` · `todo rm <id>`

### 3.2 记便签 —— 一句话速记、结论、灵感

```bash
extbrain memo add "GORM 写 PG text[] 必须用 pq.StringArray"
```

便签 ≤2000 字；更长的内容走 3.3。

### 3.3 存知识库 —— 调研 / 整理 / 总结完成，产物是成篇内容

**先查重**：`extbrain search "<主题>"` → 无重复才写文件并推送：

```bash
extbrain note push 调研笔记.md --path ai/glm-使用笔记.md
```

- 路径规划：`ai/`（AI 与模型）、`dev/`（开发）、`ops/`（运维）、`读后感/`；不确定先 `extbrain note ls` 看既有结构
- 重复主题 → 更新已有路径（覆盖式写入），不要新建冗余文件
- **仓库**：知识库分仓库（仓库 > 文件夹 > 笔记）；不带 `--repo` = 默认仓库。用户指定仓库时加 `--repo 仓库名`：
  ```bash
  extbrain note push 笔记.md --path 需求/仓库化.md --repo 工作库
  ```
  不确定有哪些仓库先 `extbrain note ls`（输出首行带仓库清单）

### 3.4 先查再答 —— 用户问到可能有存档的事

```bash
extbrain search "<关键词>"        # 摘要
extbrain note cat ai/<主题>.md    # 读全文
extbrain note pull ai/ --stdout   # 整目录拼成 MD bundle（资料多时）
```

查到 → 引用库里内容回答，并给出笔记路径；查不到 → 正常回答，可建议「要不要存进外脑」。

## 4. 命令速查

| 命令 | 用途 |
|---|---|
| `extbrain todo add / list / done / undo / rm` | 待办（`--status active\|done` 筛选，`--json` 程序化） |
| `extbrain memo add / list / pin / rm` | 便签（`pin` 置顶） |
| `extbrain note push / ls / cat / pull / rm` | 知识库（push 写、cat 读、pull 批量；均可 `--repo` 指定仓库） |
| `extbrain search <词>` | 全文检索（标题 / 路径 / 正文） |
| `extbrain me` · `auth show / logout` | 配置检查与管理 |

## 5. 纪律

1. **写前先查**：search / note ls 防重复；重复主题更新而非新建。
2. **不展示密钥**：任何情况下不要把 API Key 写进回答或文件。
3. 输出纯文本给用户看；`--json` 仅用于你自己程序化处理。
4. 删改用户数据（`rm` / 覆盖）前先向用户确认。

## 6. 网页面板

浏览器打开 {{BASE}}：待办 / 便签 / 知识库管理、分享笔记、签发密钥、查看操作日志与通知记录。
