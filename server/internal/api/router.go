// Package api —— HTTP 路由与中间件编排。
package api

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/config"
	"extbrain-server/internal/e2e"
	"extbrain-server/internal/mcp"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps 路由依赖集
type Deps struct {
	DB      *gorm.DB
	Cfg     *config.Config
	JWT     *auth.Manager
	Auth    *service.AuthService
	Keys    *service.APIKeyService
	Logs    *service.LogQueryService
	Todos   *service.TodoService
	Memos   *service.MemoService
	Dash    *service.DashboardService
	Notes   *service.NoteService
	Notify  *service.NotifyService
	CLIAuth *service.CLIAuthService
	Push    *service.PushService
	Account *service.AccountService
	Audit   *service.Audit
	Limiter *service.MemRateLimiter
	// 公开端点 per-IP 限流（独立低阈值实例）：注册/登录/发码从严，poll 宽（CLI 2s 轮询=30 次/分）
	PublicLimiter *service.MemRateLimiter // 10 次/分：register / login / cli-auth start
	PollLimiter   *service.MemRateLimiter // 60 次/分：cli-auth poll
	V1Limiter     *service.MemRateLimiter // 240 次/分：v1 组级兜底（401 洪泛刷审计的最后防线）
	Trash         *service.TrashService
	PushLog       *service.PushLogService
	Shares        *service.ShareService
	Export        *service.ExportService
	// 向量检索（迁移 0009）：平台配置 + 索引服务
	Sys   *service.SysConfig
	Index *service.IndexService
	// 运营看板（仅管理员）：平台运营数据聚合
	Ops *service.OpsService
	// 目录权限（迁移 0010）：AI Key 对知识库目录的可见性
	Perm *service.NotePermService
	// 知识库仓库（迁移 0012）：仓库 > 文件夹 > 笔记
	Repos *service.RepoService
	// MCP 接入（/mcp）
	Version string
}

func Router(d Deps) *gin.Engine {
	r := gin.New()
	// 访问日志对敏感 query 脱敏（share token / cli-auth code 不进 stdout 与反代日志）；healthz 不记
	r.Use(gin.Recovery(),
		gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
			if p.Path == "/healthz" {
				return ""
			}
			return fmt.Sprintf("[gin] %s %3d %13v %-7s %s\n",
				p.TimeStamp.Format("2006-01-02 15:04:05"), p.StatusCode, p.Latency, p.Method, redactSensitiveQuery(p.Path))
		}),
		BodyLimit(2<<20)) // 2MB 全局请求体上限
	// HEAD 按 GET 语义执行（net/http 自动省略响应体）——此前 HEAD 一律 404（NoRoute 显式拦非 GET），
	// curl -I / 健康探测全挂；/mcp 例外（其 GET 是长连接流，不可重写）
	r.Use(func(c *gin.Context) {
		if c.Request.Method == http.MethodHead && c.Request.URL.Path != "/mcp" {
			c.Request.Method = http.MethodGet
		}
		c.Next()
	})
	_ = r.SetTrustedProxies(d.Cfg.TrustedProxies)

	// 分发：一键安装脚本、操作手册与 CLI/Skill 包（游客可用）
	r.GET("/install.sh", InstallSh(d.Cfg.PublicBaseURL))
	r.GET("/guide.md", GuideMd(d.Cfg.PublicBaseURL))
	r.GET("/guide-mcp.md", GuideMcpMd(d.Cfg.PublicBaseURL))
	r.GET("/downloads/*filepath", Downloads(d.Cfg.DownloadsDir))
	r.HEAD("/downloads/*filepath", Downloads(d.Cfg.DownloadsDir))

	r.GET("/healthz", func(c *gin.Context) {
		dbOK := d.DB != nil
		if d.DB != nil {
			sqlDB, err := d.DB.DB()
			dbOK = err == nil && sqlDB.Ping() == nil
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": dbOK, "ts": time.Now().UTC().Format(time.RFC3339)})
	})

	// MCP 接入（Streamable HTTP，无状态；仅 API Key——鉴权在 MCPRoute 内完成并注入身份）
	if d.Cfg.MCPEnabled {
		mcpH := mcp.Handler(mcp.Deps{
			Todos: d.Todos, Memos: d.Memos, Notes: d.Notes, Repos: d.Repos, Perm: d.Perm, Account: d.Account,
			Audit: d.Audit, Version: d.Version,
		})
		r.Any("/mcp", MCPRoute(d, mcpH))
	}

	// E2E 洁净室信箱（仅三重门全开时注册；门不开=404）。无鉴权：洁净环境与外网隔离。
	if e2e.Enabled() {
		eh := &E2EHandler{DB: d.DB}
		r.GET("/api/v1/e2e/mailbox", eh.Mailbox)
		r.DELETE("/api/v1/e2e/mailbox", eh.MailboxClear)
	}

	// v1 组级宽松限流须挂在审计之前（组构造参数先于 Use）：被拦的 401 洪泛不产生审计写入
	v1 := r.Group("/api/v1", IPRateLimit(d.V1Limiter, "240 次/分钟"))
	// 审计中间件须先于路由注册挂载（包裹认证与 handler，Next 后读取 uid/key 记录）
	if d.Audit != nil {
		v1.Use(d.Audit.Middleware())
	}

	// 认证 + CLI 设备授权 start（公开；per-IP 限流防写放大灌库）
	pub := v1.Group("", IPRateLimit(d.PublicLimiter, "10 次/分钟"))
	authH := &AuthHandler{Auth: d.Auth, RegisterEnabled: d.Cfg.RegisterEnabled}
	pub.POST("/auth/register", authH.Register)
	pub.POST("/auth/login", authH.Login)

	// CLI 设备授权（start/poll 公开；approve 需登录）
	cliH := &CLIAuthHandler{Auth: d.CLIAuth, Base: func(c *gin.Context) string {
		scheme := "http"
		if c.GetHeader("X-Forwarded-Proto") == "https" || c.Request.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + c.Request.Host
	}}
	pub.POST("/cli-auth/start", cliH.Start)
	// poll 单独挂宽限流：CLI 2s 轮询=30 次/分，10/min 的 pub 组会打死正常流程
	v1.GET("/cli-auth/poll", IPRateLimit(d.PollLimiter, "60 次/分钟"), cliH.Poll)

	// 笔记分享公开读取（游客；挂公开限流防扫描刷计数）
	shareH := &ShareHandler{Shares: d.Shares, Repos: d.Repos, Perm: d.Perm}
	pub.GET("/public/share/:token", shareH.PublicGet)

	// 管理面（Web JWT）
	web := v1.Group("", JWTAuth(d.JWT, d.DB))
	keysH := &KeysHandler{Keys: d.Keys}
	web.GET("/keys", keysH.List)
	web.POST("/keys", keysH.Issue)
	web.PATCH("/keys/:id", keysH.Update)
	web.DELETE("/keys/:id", keysH.Revoke)
	logsH := &LogsHandler{Logs: d.Logs}
	web.GET("/logs", logsH.List)

	// 业务面（双通道：Web JWT 或 API Key；todo 域与 notes 域分组挂载）
	biz := v1.Group("", AuthAny(d.JWT, d.Keys, d.Limiter, model.KeyScopeTodo, d.DB))
	todoH := &TodoHandler{Todos: d.Todos}
	biz.POST("/todos", todoH.Create)
	biz.GET("/todos", todoH.List)
	biz.PATCH("/todos/:id", todoH.Update)
	biz.POST("/todos/:id/restore", todoH.Restore)
	biz.DELETE("/todos/:id", todoH.Delete)
	web.PUT("/todo-sort", todoH.SetSortPref) // 排序偏好=面板 UI 习惯，仅 Web JWT
	memoH := &MemoHandler{Memos: d.Memos}
	biz.POST("/memos", memoH.Create)
	biz.GET("/memos", memoH.List)
	biz.PATCH("/memos/:id", memoH.Update)
	biz.DELETE("/memos/:id", memoH.Delete)

	// 知识库（notes 域 scope）
	notesBiz := v1.Group("", AuthAny(d.JWT, d.Keys, d.Limiter, model.KeyScopeNotes, d.DB))
	noteH := &NoteHandler{Notes: d.Notes, Repos: d.Repos, Todos: d.Todos, Memos: d.Memos, Perm: d.Perm}
	notesBiz.GET("/notes", noteH.List)
	notesBiz.POST("/notes/move", noteH.Move)       // {from,to}；POST 树无 *path 通配，不与 PUT/GET 冲突
	notesBiz.POST("/notes/restore", noteH.Restore) // {path}
	notesBiz.GET("/notes/*path", noteH.Get)
	notesBiz.PUT("/notes/*path", noteH.Put)
	notesBiz.DELETE("/notes/*path", noteH.Delete)
	notesBiz.GET("/search", noteH.Search)

	// 仓库管理（Web JWT 专属：建/改/删仓库=owner 面板行为；AI 不可自建仓库）
	repoH := &RepoHandler{Repos: d.Repos}
	web.GET("/repos", repoH.List)
	web.POST("/repos", repoH.Create)
	web.PATCH("/repos/:id", repoH.Update)
	web.DELETE("/repos/:id", repoH.Delete)

	// 笔记分享管理（notes 域双通道；一篇笔记一个活跃分享）
	shareH.Repos = d.Repos
	notesBiz.POST("/shares", shareH.Create)
	notesBiz.GET("/shares", shareH.Get)
	notesBiz.DELETE("/shares/:token", shareH.Revoke)
	notesBiz.POST("/shares/:token/copy", shareH.Copy)

	// 目录权限（Web JWT 专属；AI 不可自改权限）
	permH := &NotePermHandler{Perm: d.Perm}
	web.GET("/note-perm", permH.Get)
	web.PUT("/note-perm", permH.Put)

	// 回收站（Web JWT 专属；scope=all 搜索在 notes 域路由上，待办/便签域仅 CLI 场景）
	web.GET("/trash", (&TrashHandler{Trash: d.Trash}).List)
	web.POST("/trash/restore", (&TrashHandler{Trash: d.Trash}).Restore)
	web.DELETE("/trash/:type/:id", (&TrashHandler{Trash: d.Trash}).PurgeItem)
	web.DELETE("/trash/purge", (&TrashHandler{Trash: d.Trash}).PurgeAll)

	// 仪表盘（Web JWT 专属）
	web.GET("/dashboard", (&DashboardHandler{Dash: d.Dash}).Summary)
	web.POST("/cli-auth/approve", cliH.Approve)

	// 账户与通知（Web JWT）
	accH := &AccountHandler{Account: d.Account, JWT: d.JWT}
	web.GET("/auth/me", accH.Me)
	web.POST("/auth/bind", accH.Bind)
	web.PATCH("/auth/password", accH.ChangePassword)
	web.PATCH("/auth/me", accH.UpdateProfile)
	web.GET("/export", (&ExportHandler{Export: d.Export}).All)
	nfH := &NotifyHandler{Notify: d.Notify, Push: d.Push, PushLog: d.PushLog}
	web.GET("/notify/channels", nfH.ListChannels)
	web.POST("/notify/channels", nfH.CreateChannel)
	web.PATCH("/notify/channels/:id", nfH.ToggleChannel)
	web.DELETE("/notify/channels/:id", nfH.DeleteChannel)
	web.POST("/notify/channels/:id/test", nfH.TestChannel)
	web.GET("/notify/logs", nfH.ListLogs)
	web.POST("/notify/logs/:id/retry", nfH.RetryLog)
	web.GET("/push/vapid", nfH.VapidKey)
	web.POST("/push/subscriptions", nfH.Subscribe)
	web.DELETE("/push/subscriptions", nfH.Unsubscribe)
	web.POST("/push/test", nfH.TestPush)

	// 平台管理（Web JWT + 仅管理员）：embedding 配置 + 索引状态/重建 + 运营看板
	admH := &AdminHandler{Sys: d.Sys, Index: d.Index, Ops: d.Ops}
	adm := v1.Group("/admin", JWTAuth(d.JWT, d.DB), AdminOnly(d.DB))
	adm.GET("/embedding", admH.GetEmbedding)
	adm.PUT("/embedding", admH.SaveEmbedding)
	adm.POST("/embedding/test", admH.TestEmbedding)
	adm.GET("/index/status", admH.IndexStatus)
	adm.POST("/index/rebuild", admH.Rebuild)
	adm.GET("/ops", admH.OpsSummary)

	// 内部端点：pg_cron + pg_net 定时触发到期扫描（X-Internal-Token）
	r.POST("/internal/push/due", func(c *gin.Context) {
		if d.Cfg.InternalToken == "" ||
			subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Token")), []byte(d.Cfg.InternalToken)) != 1 {
			fail(c, errUnauthorized("X-Internal-Token 不正确"))
			return
		}
		n, err := d.Push.DueScan(c.Request.Context())
		if err != nil {
			fail(c, errInternal("扫描失败: "+err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{"pushed_users": n})
	})

	return r
}

// redactSensitiveQuery 打码 URL query 中的凭据类参数（code/token）
var sensitiveQueryRe = regexp.MustCompile(`([?&](?:code|token)=)[^&\s]*`)

func redactSensitiveQuery(path string) string {
	return sensitiveQueryRe.ReplaceAllString(path, "${1}***")
}
