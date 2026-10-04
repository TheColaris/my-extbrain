package api

import (
	"net/http"
	"strings"
	"time"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	CtxUserID      = "uid"
	CtxKeyID       = "key_id"
	CtxKeyHint     = "key_hint"
	CtxKeyScope    = "key_scope"
	CtxAuthCh      = "auth_ch"      // web | cli（业务 source 字段与审计用）
	CtxAuditTarget = "audit_target" // handler 覆盖审计 Target（路由无 path 参数时，如 notes/move）
	bearerPrefix   = "Bearer "
	keyPrefix      = "ak_live_"
	jwtPrefix      = "eyJ"
)

// AuthAny 双通道认证：JWT（Web 面板）或 API Key（CLI/AI，校验 scope）。
// 业务接口（todo/memo/note/dashboard）挂本中间件；管理面（keys/logs/settings）仍用 JWTAuth。
func AuthAny(m *auth.Manager, keys *service.APIKeyService, limiter *service.MemRateLimiter, require model.KeyScope, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, bearerPrefix) {
			fail(c, errUnauthorized("缺少 Bearer 凭证（JWT 或 ak_live_ 开头的 API Key）"))
			return
		}
		cred := strings.TrimPrefix(h, bearerPrefix)
		switch {
		case strings.HasPrefix(cred, jwtPrefix):
			cl, err := m.Parse(cred)
			if err != nil {
				fail(c, errUnauthorized("登录已过期，请重新登录"))
				return
			}
			if !tokenVersionOK(c, db, cl.UserID, cl.Ver) {
				fail(c, errUnauthorized("登录已失效（密码或账号已变更），请重新登录"))
				return
			}
			c.Set(CtxUserID, cl.UserID)
			c.Set("nick", cl.NickName)
			c.Set(CtxAuthCh, "web")
			c.Next()
		case strings.HasPrefix(cred, keyPrefix):
			k, err := keys.VerifyByRawKey(c.Request.Context(), cred)
			if err != nil {
				fail(c, errUnauthorized("API Key 无效、已吊销或已过期"))
				return
			}
			if !k.Scope.Allows(require) {
				fail(c, errForbidden("该 Key 的 scope（"+string(k.Scope)+"）无权访问 "+string(require)+" 资源"))
				return
			}
			if limiter != nil && !limiter.Allow("k"+itoa(k.ID)) {
				fail(c, errTooMany("请求过于频繁（60 次/分钟），稍后再试"))
				return
			}
			c.Set(CtxUserID, k.UserID)
			c.Set(CtxKeyID, k.ID)
			c.Set(CtxKeyHint, k.KeyHint)
			c.Set(CtxKeyScope, string(k.Scope))
			c.Set(CtxAuthCh, "cli")
			c.Next()
			if k.LastUseTime == nil || time.Since(*k.LastUseTime) > time.Minute {
				keys.TouchLastUse(c.Request.Context(), k.ID)
			}
		default:
			fail(c, errUnauthorized("凭证格式不合法（JWT 或 ak_live_ 开头的 API Key）"))
		}
	}
}

// JWTAuth —— Web 会话认证（Authorization: Bearer <jwt>）
func JWTAuth(m *auth.Manager, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, bearerPrefix) {
			fail(c, errUnauthorized("缺少 Bearer JWT"))
			return
		}
		cl, err := m.Parse(strings.TrimPrefix(h, bearerPrefix))
		if err != nil {
			fail(c, errUnauthorized("登录已过期，请重新登录"))
			return
		}
		if !tokenVersionOK(c, db, cl.UserID, cl.Ver) {
			fail(c, errUnauthorized("登录已失效（密码或账号已变更），请重新登录"))
			return
		}
		c.Set(CtxUserID, cl.UserID)
		c.Set("nick", cl.NickName)
		c.Next()
	}
}

// tokenVersionOK 校验 JWT 版本与库内 token_version 一致（改密/换绑后旧 token 全部失效）。
// db 为 nil（单元测试轻装配）时跳过校验。
func tokenVersionOK(c *gin.Context, db *gorm.DB, uid int64, ver int) bool {
	if db == nil {
		return true
	}
	var vers []int
	if err := db.WithContext(c.Request.Context()).Model(&model.User{}).
		Where("id = ? AND is_deleted = 0", uid).Pluck("token_version", &vers).Error; err != nil {
		return false
	}
	return len(vers) == 1 && vers[0] == ver
}

// APIKeyAuth —— API Key 认证（含限流 + scope 校验）；认证通过节流更新 last_use_time。
func APIKeyAuth(keys *service.APIKeyService, limiter *service.MemRateLimiter, require model.KeyScope) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, bearerPrefix) {
			fail(c, errUnauthorized("缺少 Bearer API Key"))
			return
		}
		raw := strings.TrimPrefix(h, bearerPrefix)
		k, err := keys.VerifyByRawKey(c.Request.Context(), raw)
		if err != nil {
			fail(c, errUnauthorized("API Key 无效、已吊销或已过期"))
			return
		}
		if !k.Scope.Allows(require) {
			fail(c, errForbidden("该 Key 的 scope（"+string(k.Scope)+"）无权访问 "+string(require)+" 资源"))
			return
		}
		if limiter != nil && !limiter.Allow("k"+itoa(k.ID)) {
			fail(c, errTooMany("请求过于频繁（60 次/分钟），稍后再试"))
			return
		}
		c.Set(CtxUserID, k.UserID)
		c.Set(CtxKeyID, k.ID)
		c.Set(CtxKeyHint, k.KeyHint)
		c.Set(CtxKeyScope, string(k.Scope))
		c.Next()
		// 节流更新最近使用（>60s 才写，避免热点 Key 每请求一写）
		if k.LastUseTime == nil || time.Since(*k.LastUseTime) > time.Minute {
			keys.TouchLastUse(c.Request.Context(), k.ID)
		}
	}
}

// AdminOnly —— 仅平台管理员（tu_user.is_admin=1；挂在 JWTAuth 之后）。
func AdminOnly(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if db == nil {
			c.Next()
			return
		}
		var flags []int16
		err := db.WithContext(c.Request.Context()).Model(&model.User{}).
			Where("id = ? AND is_deleted = 0", uid(c)).Pluck("is_admin", &flags).Error
		if err != nil || len(flags) != 1 || flags[0] != 1 {
			fail(c, errForbidden("仅平台管理员可访问"))
			return
		}
		c.Next()
	}
}

// IPRateLimit —— 按 IP 限流（公开端点与 v1 组级兜底共用）。
// 防无认证写放大：伪造 token 打业务端点的 401、cli-auth/start 的 sys_cache 插行，
// 不限流都可灌库。须挂在审计中间件之前，被拦请求不产生审计写入。
func IPRateLimit(limiter *service.MemRateLimiter, limitHint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limiter != nil && !limiter.Allow("ip:"+c.ClientIP()) {
			fail(c, errTooMany("请求过于频繁（"+limitHint+"），稍后再试"))
			return
		}
		c.Next()
	}
}

// BodyLimit 请求体上限（防无认证大 body 内存放大 DoS；gin 默认不限制）
func BodyLimit(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		}
		c.Next()
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
