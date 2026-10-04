package api

import (
	"net/http"
	"strings"
	"time"

	"extbrain-server/internal/geoip"
	"extbrain-server/internal/mcp"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

// MCPRoute —— /mcp（MCP Streamable HTTP）。仅 API Key 接入；鉴权在 gin 层完成，
// 身份注入 request context 后交给 MCP 处理器（无状态：每请求独立鉴权）。
func MCPRoute(d Deps, h http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := mcpAuth(c, d)
		if id == nil {
			return // 已写出 401/429
		}
		ctx := mcp.WithIdentity(c.Request.Context(), id)
		h.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	}
}

// mcpAuth 校验 Bearer API Key（不接受 JWT——MCP 场景用可吊销、可限权的 Key）。
// 限流与 REST 共享同一 per-key 配额（防换通道绕过）。
func mcpAuth(c *gin.Context, d Deps) *mcp.Identity {
	cred := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !strings.HasPrefix(cred, "ak_live_") {
		mcpAuthFail(d, c)
		fail(c, errUnauthorized("MCP 需要 API Key：Authorization: Bearer ak_live_…（网页面板「API 密钥」签发）"))
		return nil
	}
	k, err := d.Keys.VerifyByRawKey(c.Request.Context(), cred)
	if err != nil {
		mcpAuthFail(d, c)
		fail(c, errUnauthorized("API Key 无效、已吊销或已过期"))
		return nil
	}
	if d.Limiter != nil && !d.Limiter.Allow("k"+itoa(k.ID)) {
		fail(c, errTooMany("请求过于频繁（60 次/分钟），稍后再试"))
		return nil
	}
	if k.LastUseTime == nil || time.Since(*k.LastUseTime) > time.Minute {
		d.Keys.TouchLastUse(c.Request.Context(), k.ID)
	}
	return &mcp.Identity{
		UserID: k.UserID, KeyID: k.ID, KeyName: k.KeyName, KeyHint: k.KeyHint,
		Scope: k.Scope, ClientIP: c.ClientIP(),
	}
}

// mcpAuthFail 鉴权失败也落审计（与 REST 同口径：auth.failed + target=/mcp，可观测扫描）。
func mcpAuthFail(d Deps, c *gin.Context) {
	if d.Audit == nil {
		return
	}
	d.Audit.Record(model.APILog{
		Action:     service.ActionAuthFailed,
		Target:     "/mcp",
		StatusCode: http.StatusUnauthorized,
		ClientIP:   c.ClientIP(),
		IPRegion:   geoip.Search(c.ClientIP()),
	})
}
