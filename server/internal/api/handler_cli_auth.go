package api

import (
	"errors"
	"net/http"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type CLIAuthHandler struct {
	Auth *service.CLIAuthService
	Base func(c *gin.Context) string // 当前服务基址（供 auth_url）
}

// POST /api/v1/cli-auth/start（公开）
func (h *CLIAuthHandler) Start(c *gin.Context) {
	code, err := h.Auth.Start(c.Request.Context())
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"code":       code,
		"expires_in": 300,
		"auth_url":   h.Base(c) + "/cli-auth?code=" + code,
	})
}

// POST /api/v1/cli-auth/approve（JWT）——用户在浏览器确认
func (h *CLIAuthHandler) Approve(c *gin.Context) {
	var in struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {code}"))
		return
	}
	if err := h.Auth.Approve(c.Request.Context(), uid(c), in.Code); err != nil {
		if errors.Is(err, service.ErrCLIAuthExpired) || errors.Is(err, service.ErrCLIAuthConsumed) {
			fail(c, errConflict("cli_auth_invalid", err.Error()))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"approved": true})
}

// GET /api/v1/cli-auth/poll?code=（公开，一次性取 Key）
func (h *CLIAuthHandler) Poll(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		fail(c, errBadRequest("invalid_code", "缺少 code"))
		return
	}
	status, key, err := h.Auth.Poll(c.Request.Context(), code)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	resp := gin.H{"status": status}
	if status == "approved" {
		resp["key"] = key
	}
	c.JSON(http.StatusOK, resp)
}
