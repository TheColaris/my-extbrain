package api

import (
	"errors"
	"net/http"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	Auth            *service.AuthService
	RegisterEnabled bool
}

// POST /api/v1/auth/register
func (h *AuthHandler) Register(c *gin.Context) {
	if !h.RegisterEnabled {
		fail(c, errForbidden("注册已关闭（REGISTER_ENABLED=false）"))
		return
	}
	var in service.RegisterInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 必须是 {account, password} 的 JSON"))
		return
	}
	u, err := h.Auth.Register(c.Request.Context(), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	token, err := h.Auth.JWT.Sign(u.ID, u.NickName, u.TokenVersion)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"token": token, "user": userOut(u)})
}

// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var in service.LoginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 必须是 {account, password} 的 JSON"))
		return
	}
	u, token, err := h.Auth.Login(c.Request.Context(), in, c.ClientIP())
	if err != nil {
		respondServiceErr(c, err, http.StatusUnauthorized)
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": userOut(u)})
}

func userOut(u *model.User) gin.H {
	return gin.H{
		"id": u.ID, "phone": u.Phone, "email": u.Email, "nick_name": u.NickName,
		"avatar_emoji": u.AvatarEmoji, "avatar_bg": u.AvatarBg,
	}
}

// respondServiceErr 业务错误统一出口：UserError→400（可用 status 覆盖，code 随 status 变化）；其余→500。
func respondServiceErr(c *gin.Context, err error, status ...int) {
	var ue *service.UserError
	if errors.As(err, &ue) {
		st := http.StatusBadRequest
		code := "invalid_request"
		if len(status) > 0 {
			st = status[0]
			if st == http.StatusUnauthorized {
				code = "unauthorized"
			}
		}
		fail(c, &ApiError{Code: code, Message: ue.Msg, Status: st})
		return
	}
	fail(c, errInternal("服务内部错误，请稍后重试"))
}
