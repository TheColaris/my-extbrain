package api

import (
	"errors"
	"net/http"
	"os"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type NotifyHandler struct {
	Notify  *service.NotifyService
	Push    *service.PushService
	PushLog *service.PushLogService
}

type AccountHandler struct {
	Account *service.AccountService
	JWT     *auth.Manager
}

// GET /api/v1/notify/channels（JWT）
func (h *NotifyHandler) ListChannels(c *gin.Context) {
	cs, err := h.Notify.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"channels": cs})
}

// POST /api/v1/notify/channels（JWT，创建即测试）
func (h *NotifyHandler) CreateChannel(c *gin.Context) {
	var in service.ChannelInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {channel_type, webhook_url, secret?}"))
		return
	}
	ch, err := h.Notify.Create(c.Request.Context(), uid(c), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, ch)
}

// PATCH /api/v1/notify/channels/:id（启用/停用）
func (h *NotifyHandler) ToggleChannel(c *gin.Context) {
	id, err := paramID(c)
	if err != nil {
		return
	}
	var in struct {
		IsEnabled *bool `json:"is_enabled"`
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.IsEnabled == nil {
		fail(c, errBadRequest("invalid_body", "body 是 {is_enabled: true|false}"))
		return
	}
	if err := h.Notify.Toggle(c.Request.Context(), uid(c), id, *in.IsEnabled); err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			fail(c, errNotFound("渠道不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DELETE /api/v1/notify/channels/:id
func (h *NotifyHandler) DeleteChannel(c *gin.Context) {
	id, err := paramID(c)
	if err != nil {
		return
	}
	if err := h.Notify.Delete(c.Request.Context(), uid(c), id); err != nil {
		if errors.Is(err, service.ErrChannelNotFound) {
			fail(c, errNotFound("渠道不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// POST /api/v1/notify/channels/:id/test
func (h *NotifyHandler) TestChannel(c *gin.Context) {
	id, err := paramID(c)
	if err != nil {
		return
	}
	if err := h.Notify.Test(c.Request.Context(), uid(c), id); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "测试消息已发送"})
}

// GET /api/v1/push/vapid（JWT；未配置返回 enabled=false）
func (h *NotifyHandler) VapidKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": h.Push.Enabled(), "public_key": h.Push.VapidPublic})
}

// POST /api/v1/push/subscriptions（JWT）
func (h *NotifyHandler) Subscribe(c *gin.Context) {
	var in service.SubInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是浏览器 PushManager.subscribe() 的原始 subscription JSON"))
		return
	}
	if err := h.Push.Subscribe(c.Request.Context(), uid(c), in); err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

// DELETE /api/v1/push/subscriptions?endpoint=（JWT）
func (h *NotifyHandler) Unsubscribe(c *gin.Context) {
	ep := c.Query("endpoint")
	if ep == "" {
		fail(c, errBadRequest("invalid_endpoint", "缺少 endpoint 查询参数"))
		return
	}
	_ = h.Push.Unsubscribe(c.Request.Context(), uid(c), ep)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/push/test（JWT，给自己全部渠道发一条）
func (h *NotifyHandler) TestPush(c *gin.Context) {
	h.Push.SendToUserForTest(c.Request.Context(), uid(c))
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已向全部启用渠道发送测试"})
}

// GET /api/v1/notify/logs?channel=&result=&type=&days=（JWT，事件聚合）
func (h *NotifyHandler) ListLogs(c *gin.Context) {
	days, _ := atoiDefault(c.Query("days"), 7)
	events, err := h.PushLog.List(c.Request.Context(), uid(c), service.PushLogFilter{
		Channel: c.Query("channel"),
		Result:  c.Query("result"),
		Type:    c.Query("type"),
		Days:    days,
	})
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	stats, err := h.PushLog.StatsToday(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events, "stats": stats})
}

// POST /api/v1/notify/logs/:id/retry（JWT，对一条投递重推；结果原地更新）
func (h *NotifyHandler) RetryLog(c *gin.Context) {
	id, err := paramID(c)
	if err != nil {
		return
	}
	if err := h.Push.RetryChannel(c.Request.Context(), uid(c), id); err != nil {
		if errors.Is(err, service.ErrPushLogNotFound) {
			fail(c, errNotFound("推送记录不存在"))
			return
		}
		// 重推失败也是有效结果（记录已原地更新）：200 + ok:false + 原因
		c.JSON(http.StatusOK, gin.H{"ok": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "重推成功"})
}

// GET /api/v1/auth/me（JWT）
func (h *AccountHandler) Me(c *gin.Context) {
	u, err := h.Account.Me(c.Request.Context(), uid(c))
	if err != nil {
		fail(c, errNotFound("用户不存在"))
		return
	}
	c.JSON(http.StatusOK, accountOut(u))
}

// accountOut 账户响应统一出口（含头像、注册时间与管理员标记）
func accountOut(u *model.User) gin.H {
	return gin.H{
		"id": u.ID, "phone": u.Phone, "email": u.Email, "nick_name": u.NickName,
		"avatar_emoji": u.AvatarEmoji, "avatar_bg": u.AvatarBg,
		"is_admin":    u.IsAdmin == 1,
		"create_time": u.CreateTime,
	}
}

// POST /api/v1/auth/bind（JWT，{type, value, password}）
func (h *AccountHandler) Bind(c *gin.Context) {
	var in struct {
		Type     string `json:"type" binding:"required"`
		Value    string `json:"value" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {type: phone|email, value, password}"))
		return
	}
	u, err := h.Account.Bind(c.Request.Context(), uid(c), model.BindChannel(in.Type), in.Value, in.Password)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	// 换绑已 +1 token_version（其他设备失效）→ 给当前设备顺发新 token
	token, err := h.JWT.Sign(u.ID, u.NickName, u.TokenVersion)
	if err != nil {
		fail(c, errInternal("会话签发失败，请重新登录"))
		return
	}
	resp := accountOut(u)
	resp["token"] = token
	c.JSON(http.StatusOK, resp)
}

// PATCH /api/v1/auth/password（JWT，{old, new}）
func (h *AccountHandler) ChangePassword(c *gin.Context) {
	var in struct {
		Old string `json:"old" binding:"required"`
		New string `json:"new" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {old, new}"))
		return
	}
	u, err := h.Account.ChangePassword(c.Request.Context(), uid(c), in.Old, in.New)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	// 改密已 +1 token_version（其他设备失效）→ 给当前设备顺发新 token
	token, err := h.JWT.Sign(u.ID, u.NickName, u.TokenVersion)
	if err != nil {
		fail(c, errInternal("会话签发失败，请重新登录"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "token": token})
}

// PATCH /api/v1/auth/me（JWT，{nick_name?, avatar_emoji?, avatar_bg?} 任意子集）
func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	var in service.UpdateProfileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {nick_name?, avatar_emoji?, avatar_bg?} 的任意子集"))
		return
	}
	u, err := h.Account.UpdateProfile(c.Request.Context(), uid(c), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, accountOut(u))
}

// RunGenVapid 生成 VAPID 密钥对（go run ./cmd/app -gen-vapid）
func RunGenVapid() {
	pub, priv, err := service.GenerateVapid()
	if err != nil {
		os.Stderr.WriteString("生成失败: " + err.Error() + "\n")
		os.Exit(1)
	}
	os.Stdout.WriteString("VAPID_PUBLIC_KEY=" + pub + "\nVAPID_PRIVATE_KEY=" + priv + "\n")
	os.Exit(0)
}
