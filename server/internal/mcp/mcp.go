// Package mcp —— my-extbrain 的 MCP（Model Context Protocol）接入层。
//
// 形态：服务端原生 Streamable HTTP（无状态）挂在 /mcp；工具定义唯一真源在本包，
// CLI 的 `extbrain mcp` stdio 桥从 /mcp 动态镜像工具，不在 CLI 重复定义。
// 鉴权：仅 API Key（Bearer ak_live_…）——由 HTTP 层（gin 路由）完成校验并把
// Identity 注入 request context；工具集按 Key scope 过滤（todo 域 / notes 域）。
package mcp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"extbrain-server/internal/geoip"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Identity —— HTTP 层鉴权产物（每请求注入 context）。
type Identity struct {
	UserID   int64
	KeyID    int64
	KeyName  string
	KeyHint  string
	Scope    model.KeyScope
	ClientIP string
}

type identityKey struct{}

// WithIdentity 把鉴权身份注入 request context（api 层路由调用）。
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func identityFrom(ctx context.Context) *Identity {
	id, _ := ctx.Value(identityKey{}).(*Identity)
	return id
}

// Deps 工具层依赖（api.Deps 的子集）。
type Deps struct {
	Todos   *service.TodoService
	Memos   *service.MemoService
	Notes   *service.NoteService
	Repos   *service.RepoService     // 仓库解析（可选 repo 字段；迁移 0012）
	Perm    *service.NotePermService // 目录权限（AI Key 可见性）
	Account *service.AccountService
	Audit   *service.Audit
	Version string
}

// Handler 返回 /mcp 的 Streamable HTTP 处理器（无状态：每请求独立鉴权，无会话面）。
// getServer 回调按请求构建服务器实例——工具 handler 闭包捕获当次身份，天然按请求隔离。
func Handler(d Deps) http.Handler {
	return sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		id := identityFrom(r.Context())
		if id == nil {
			return nil // 400：正常路径不会发生（gin 路由已鉴权并注入身份）
		}
		return buildServer(d, id)
	}, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
}

// buildServer 按身份构建服务器实例：Instructions + 按 scope 过滤的工具集。
func buildServer(d Deps, id *Identity) *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{
		Name:    "extbrain",
		Title:   "我的外脑",
		Version: d.Version,
	}, &sdk.ServerOptions{Instructions: instructionsText})
	e := &env{srv: srv, d: d, id: id}
	registerTools(e)
	return srv
}

// instructionsText 服务端级说明（等价于把 Skill 的四模式纪律内嵌到协议层）。
const instructionsText = `你是「我的外脑」（my-extbrain）的接入端：待办、便签、Markdown 知识库都在这里，通过工具读写。

行为约定：
1. 用户说「记一下 / 别忘了 / 提醒我」→ todo_create（说了明确时间就带 due）。
2. 一句话速记、结论、灵感 → memo_create（≤2000 字）；成篇内容走 note_write。
3. 存知识前先 search 查重；重复主题更新已有路径（note_write 是覆盖式），不要新建冗余文件。
4. 回答用户问题前先 search 查库里有没有相关内容；查到就引用并给出笔记路径。
5. 删除或覆盖用户数据前，先向用户确认。
6. 不要向用户展示任何密钥；输出用中文。
7. 部分目录可能对你的 Key 不可见（目录权限）：遇到「不可见」提示属正常，直接告知用户，不要反复尝试。
8. 知识库分仓库（仓库 > 文件夹 > 笔记）：note_* 工具不带 repo 参数=默认仓库；用户指定仓库时带 repo 字段（仓库名）。`

type env struct {
	srv *sdk.Server
	d   Deps
	id  *Identity
}

// permView 解析当前 Key 的目录权限视图（nil=不受限）。
func (e *env) permView(ctx context.Context) (*service.PermView, error) {
	if e.d.Perm == nil {
		return nil, nil
	}
	return e.d.Perm.View(ctx, e.id.UserID, e.id.KeyID)
}

// record 记一条审计（与 REST 同构：action=资源.动词；来源靠 Key 名/hint 区分）。
func (e *env) record(action, target string, status int, start time.Time) {
	if e.d.Audit == nil {
		return
	}
	e.d.Audit.Record(model.APILog{
		UserID:     e.id.UserID,
		APIKeyID:   e.id.KeyID,
		KeyHint:    e.id.KeyHint,
		Action:     action,
		Target:     target,
		StatusCode: int16(status),
		CostMs:     int32(time.Since(start).Milliseconds()),
		ClientIP:   e.id.ClientIP,
		IPRegion:   geoip.Search(e.id.ClientIP),
	})
}

// statusOf 业务错误 → 审计 status_code（与 REST 语义对齐）。
func statusOf(err error) int {
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, service.ErrTodoNotFound), errors.Is(err, service.ErrMemoNotFound),
		errors.Is(err, service.ErrNoteNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrHashConflict), errors.Is(err, service.ErrPathConflict),
		errors.Is(err, service.ErrPathConflictDeleted):
		return http.StatusConflict
	}
	var ue *service.UserError
	if errors.As(err, &ue) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// addTool 注册工具：统一包审计 + 结果包装。
// fn 返回的 error 由 SDK 的 ToolHandlerFor 自动打包为 isError 结果（AI 可见可自纠，不走协议错误）。
func addTool[In any](e *env, t *sdk.Tool, action string, target func(In) string, fn func(ctx context.Context, in In) (string, error)) {
	sdk.AddTool(e.srv, t, func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		start := time.Now()
		text, err := fn(ctx, in)
		tgt := ""
		if target != nil {
			tgt = target(in)
		}
		e.record(action, tgt, statusOf(err), start)
		if err != nil {
			return nil, nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, nil, nil
	})
}

func boolPtr(b bool) *bool { return &b }

// 注解三态（closed world：只操作本实例内数据）
func ro() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
}
func rw() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), OpenWorldHint: boolPtr(false)}
}
func del() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(false)}
}
func upsert() *sdk.ToolAnnotations {
	return &sdk.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)}
}
