package service

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"extbrain-server/internal/geoip"
	"extbrain-server/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Audit —— API 审计：channel 缓冲 + 后台 goroutine 批量落 tl_api_log。
// 日志永不阻塞主请求：缓冲满即丢弃并计数（丢弃量在日志可见）。
type Audit struct {
	db    *gorm.DB
	ch    chan model.APILog
	drops atomic.Int64
	// failSample 匿名 auth.failed 采样器：每 IP 每分钟最多记 10 条——
	// 401 洪泛（伪造 token 刷审计行）即使过了路由限流，写放大也被封顶
	failSample *MemRateLimiter
}

const (
	auditBufSize    = 1024
	auditBatch      = 50
	auditFlushEvery = time.Second
	// 认证通道动作（引用枚举真源，防散落拼接）
	ActionAuthFailed = string(model.ActionAuthFailed)
	ActionWebLogin   = string(model.ActionWebLogin)
)

func NewAudit(db *gorm.DB) *Audit {
	a := &Audit{db: db, ch: make(chan model.APILog, auditBufSize), failSample: NewMemRateLimiter(10, time.Minute)}
	go a.loop()
	return a
}

// Record 投递一条（非阻塞，满则丢弃）。target 按 rune 截断到列宽：
// 一行超长会让整批（≤50 条）INSERT 失败、静默陪葬。
func (a *Audit) Record(e model.APILog) {
	e.Target = cutRunes(e.Target, auditTargetMax)
	select {
	case a.ch <- e:
	default:
		n := a.drops.Add(1)
		if n%100 == 1 {
			gin.DefaultErrorWriter.Write([]byte("[audit] 缓冲满丢弃，累计 " + time.Now().Format(time.RFC3339) + " " + itoa(n) + "\n"))
		}
	}
}

const auditTargetMax = 1200 // 对齐迁移 0006 的列宽 varchar(1200)

// cutRunes 按 rune 截断（按字节切会碎 UTF-8，PG 拒收整行）
func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (a *Audit) loop() {
	t := time.NewTicker(auditFlushEvery)
	defer t.Stop()
	buf := make([]model.APILog, 0, auditBatch)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if err := a.db.Create(&buf).Error; err != nil {
			gin.DefaultErrorWriter.Write([]byte("[audit] 批量落库失败: " + err.Error() + "\n"))
		}
		buf = buf[:0]
	}
	for {
		select {
		case e := <-a.ch:
			buf = append(buf, e)
			if len(buf) >= auditBatch {
				flush()
			}
		case <-t.C:
			flush()
		}
	}
}

// Middleware Gin 审计中间件：须挂在认证中间件之后（读取 uid/key 信息）。
// action = 路由模板资源段 + method 动词映射，如 PATCH /api/v1/todos/:id → todo.update。
func (a *Audit) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		uid, _ := c.Get("uid")
		keyID, _ := c.Get("key_id")
		keyHint, _ := c.Get("key_hint")
		u64 := toInt64(uid)
		k64 := toInt64(keyID)
		if u64 == 0 { // 公开路由（登录/注册/分享读取）或未认证失败
			st := c.Writer.Status()
			action, skip := anonAction(c.FullPath(), st)
			if skip {
				return // 高频/无审计价值的匿名成功（cli-auth 轮询、vapid 拉公钥等）：不记
			}
			if action == ActionAuthFailed && !a.failSample.Allow("ip:"+c.ClientIP()) {
				return // 同 IP 每分钟只记前 10 条失败：保留探测信号，写放大封顶
			}
			a.Record(model.APILog{
				UserID: u64, APIKeyID: 0, Action: action,
				Target: c.FullPath(), StatusCode: int16(st),
				CostMs: int32(time.Since(start).Milliseconds()), ClientIP: c.ClientIP(),
				IPRegion: geoip.Search(c.ClientIP()),
			})
			return
		}
		hint, _ := keyHint.(string)
		fp := c.FullPath()
		action := string(model.ActionOf(resOf(fp), verbOf(c)))
		if strings.Contains(fp, "/public/share") {
			action = string(model.ActionShareRead) // 登录态访问公开分享端点也归一（否则 resOf 会记成 public.read）
		}
		if strings.HasPrefix(fp, "/api/v1/admin/") {
			action = adminAction(c) // 平台管理端点精确映射（resOf 会粗归成 admin.read/create）
		}
		st := c.Writer.Status()
		if skipWebRead(action, int64(st), k64) {
			return // Web 面板读类成功不记（只砍 Web，AI/Key 来源全量照记）
		}
		a.Record(model.APILog{
			UserID: u64, APIKeyID: k64, KeyHint: hint,
			Action:     action,
			Target:     actionTarget(c),
			StatusCode: int16(st),
			CostMs:     int32(time.Since(start).Milliseconds()),
			ClientIP:   c.ClientIP(),
			IPRegion:   geoip.Search(c.ClientIP()),
		})
	}
}

// resOf 路由模板 → 资源段（todos→todo；note-perm 归 note）
func resOf(fullPath string) string {
	parts := strings.Split(strings.Trim(fullPath, "/"), "/")
	if len(parts) >= 3 {
		if parts[2] == "note-perm" {
			return "note" // 目录权限配置 → note.perm
		}
		return strings.TrimSuffix(parts[2], "s")
	}
	return "api"
}

// adminAction 平台管理端点 → 精确动作（配置读写/测试连接/索引状态与重建）
func adminAction(c *gin.Context) string {
	fp := c.FullPath()
	switch {
	case strings.HasSuffix(fp, "/admin/embedding/test"):
		return string(model.ActionConfigTest)
	case strings.HasSuffix(fp, "/admin/embedding"):
		if c.Request.Method == http.MethodGet {
			return string(model.ActionConfigRead)
		}
		return string(model.ActionConfigUpdate)
	case strings.HasSuffix(fp, "/admin/index/rebuild"):
		return string(model.ActionIndexRebuild)
	case strings.HasSuffix(fp, "/admin/index/status"):
		return string(model.ActionIndexRead)
	case strings.HasSuffix(fp, "/admin/ops"):
		return string(model.ActionOpsRead)
	}
	return string(model.ActionConfigRead)
}

// skipWebRead Web 面板读类成功不记：只砍 Web（api_key_id=0）来源的
// `.read` 成功（<400）；AI/Key 来源全量照记；export.read（导出全量数据）与 share.read
// （游客/owner 查分享）为白名单例外，照记。失败（>=400）永不跳过——探测/越权读是安全信号。
func skipWebRead(action string, status, keyID int64) bool {
	return keyID == 0 && status < 400 &&
		strings.HasSuffix(action, ".read") &&
		action != string(model.ActionExportRead) &&
		action != string(model.ActionShareRead)
}

// anonAction 匿名（uid=0）请求的审计归类。返回 skip=true = 不记。
// 语义修复：cli-auth start/poll、push/vapid 此前被兜底记成 auth.failed（poll 2s 轮询=纯噪音）；
// 未认证打受保护接口（>=400）仍记 auth.failed（探测安全信号）。
func anonAction(fullPath string, status int) (action string, skip bool) {
	switch {
	case strings.Contains(fullPath, "/public/share"):
		return string(model.ActionShareRead), false // 含失效(404)/限流(429)：流量可观测，status_code 已区分
	case strings.HasSuffix(fullPath, "/cli-auth/poll"),
		strings.HasSuffix(fullPath, "/cli-auth/start"),
		strings.HasSuffix(fullPath, "/push/vapid"):
		return "", true
	case status < 400 && strings.HasSuffix(fullPath, "/auth/login"):
		return ActionWebLogin, false
	case status < 400 && strings.HasSuffix(fullPath, "/auth/register"):
		return string(model.ActionRegister), false
	case status == 429:
		return "", true // 被限流的匿名流量=攻击面：不记，否则限流挡不住审计行自身的写放大
	case status >= 400:
		return ActionAuthFailed, false
	}
	return "", true // 其余匿名成功：不记
}

// verbOf method → 动词段；POST /…/:id/restore 归为 restore、POST /notes/move 归为 move（否则会误记为 create）
func verbOf(c *gin.Context) string {
	if c.Request.Method == "POST" {
		switch {
		case strings.HasSuffix(c.FullPath(), "/restore"):
			return "restore"
		case strings.HasSuffix(c.FullPath(), "/move"):
			return "move"
		case strings.HasSuffix(c.FullPath(), "/retry"):
			return "retry"
		case strings.HasSuffix(c.FullPath(), "/copy"):
			return "copy"
		case c.FullPath() == "/api/v1/shares":
			return "upsert" // 创建/更新分享共用（保留 token），对齐 PUT 语义
		}
	}
	if c.Request.Method == "DELETE" && strings.HasSuffix(c.FullPath(), "/purge") {
		return "purge"
	}
	if c.Request.Method == "PUT" && strings.HasSuffix(c.FullPath(), "/note-perm") {
		return "perm" // 目录权限保存（note.perm）
	}
	return map[string]string{
		"GET": "read", "POST": "create", "PUT": "upsert", "PATCH": "update", "DELETE": "delete",
	}[c.Request.Method]
}

func actionTarget(c *gin.Context) string {
	// 键名对齐 api.CtxAuditTarget（service 不能反向 import api，故用字面量）
	if v, ok := c.Get("audit_target"); ok {
		if s, _ := v.(string); s != "" {
			return s
		}
	}
	if p := c.Param("id"); p != "" {
		return p
	}
	if p := c.Param("path"); p != "" {
		return p
	}
	return ""
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

var _ = context.Background // 保留 context 依赖位（未来 flush 可取消）
