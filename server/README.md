# extbrain-server

[my-extbrain（我的外脑）](..) 的后端 + CLI：Go + Gin + GORM + golang-migrate，前端产物 `go:embed` 进单二进制。

## 快速开始

服务把前端 embed 进二进制（`//go:embed dist`），容器构建前需要先产出前端产物：

```bash
# 1) 构建 web dist 并拷入 embed 目录（仓库不含 dist）
pnpm --dir ../web install && pnpm --dir ../web build
rm -rf cmd/app/dist && cp -R ../web/dist cmd/app/dist

# 2) 起服务
cp env-template .env          # DATABASE_URL 必填；JWT_SECRET 必填（openssl rand -hex 32）
docker compose up -d --build  # app + pgvector/pg16，启动自动迁移
curl http://127.0.0.1:8080/healthz
```

连接 Supabase：`DATABASE_URL` 指向 Session Pooler（IPv4），单跑 app 容器即可。

## 公网部署安全基线

- **`JWT_SECRET` 必须随机且 ≥32 位**（占位值/短值服务会拒绝启动）——仓库示例值是公开的，泄露即可离线伪造任意账户会话。
- **部署后立即注册管理员账号**；公网实例注册完成后建议设 `REGISTER_ENABLED=false`（首个注册用户自动成为管理员，开放注册存在被抢注可能）。
- 建议置于反向代理（nginx）之后并配置 `client_max_body_size`；服务侧已有 2MB 请求体上限与全端点限流兜底。
- 配置 `PUBLIC_BASE_URL`（如 `https://your.domain`）：`/install.sh`、`/guide.md` 的下载地址随之固定，不依赖请求 Host 头。

## CLI 安装

```bash
# 方式一：从你部署好的实例一键安装（脚本按当前平台自动下载）
curl -fsSL https://<你的实例>/install.sh | sh

# 方式二：源码构建（本机有 Go）
go build -o /usr/local/bin/extbrain ./cmd/extbrain

# 配置：设备授权（推荐）——浏览器点一下「授权」即可，无需密钥
extbrain auth login --server https://<你的实例>
# 备选：面板「API 密钥」签发 Key 后 --key ak_live_...

extbrain todo add "试试记一条" && extbrain search "试试"
```

## MCP 接入（AI 客户端直连）

支持 MCP 的客户端（Claude Code / ZCode / Cursor 等）可不经 CLI 直接读写外脑，与 CLI/Skill 二选一：

```bash
claude mcp add --transport http extbrain https://<你的实例>/mcp --header "Authorization: Bearer ak_live_..."
```

- 端点：`/mcp`（Streamable HTTP，无状态；鉴权=仅 API Key，per-key 限流与 REST 共享配额）
- 16 个工具（search / todo×4 / memo×4 / note×6 / whoami），按 Key 的 scope 过滤可见工具；每次工具调用落审计日志（action 与 REST 同构）
- 完整手册：`GET /guide-mcp.md`（或网页 `/guide` 页「MCP 版」标签页；含仅支持 stdio 的客户端用 `mcp-remote` 垫片的兜底配置）

## 开发

```bash
go build ./... && go test ./...
go run ./cmd/app              # API 服务
go run ./cmd/extbrain --version  # CLI
```

## 工程规则（详见根目录 AGENTS.md）

- **枚举唯一真源**：`internal/model/enums.go`；CLI 同仓直接复用，web 侧 TS 同值对齐，禁止魔法值。
- **迁移**：`internal/migrate/migrations/`（golang-migrate SQL up/down，embed 进二进制，启动自动执行）；表结构按项目建表规范（前缀/三字段/注释/无物理外键）。
- API 一律 JSON + 字符串枚举；错误统一 `{"error":{"code","message"}}`。

## 目录

```
cmd/app            # API 服务入口（embed 前端）
cmd/extbrain       # CLI 入口
internal/config    # 环境变量配置
internal/model     # GORM 模型 + 枚举唯一真源
internal/api       # 路由与 handler
internal/migrate   # 内嵌迁移执行
skill/             # AI Skill 包（随 server 同 tag 发布）
```

MIT License.
