package api

import (
	"net/http"
	"strconv"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type KeysHandler struct{ Keys *service.APIKeyService }

// GET /api/v1/keys（JWT）
func (h *KeysHandler) List(c *gin.Context) {
	ks, err := h.Keys.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"keys": ks})
}

// POST /api/v1/keys（JWT）——返回一次性完整 Key
func (h *KeysHandler) Issue(c *gin.Context) {
	var in service.IssueInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 必须是 {name} 的 JSON"))
		return
	}
	k, full, err := h.Keys.Issue(c.Request.Context(), uid(c), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"key":         k,
		"full_key":    full,
		"cli_command": "extbrain auth login --server <你的实例地址> --key " + full,
	})
}

// PATCH /api/v1/keys/:id（JWT）——改名称/权限
func (h *KeysHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	var in service.UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {name?, scope?}，字段可省略=不修改"))
		return
	}
	k, err := h.Keys.Update(c.Request.Context(), uid(c), id, in)
	if err != nil {
		if err == service.ErrKeyNotFound {
			fail(c, errNotFound("密钥不存在或已吊销"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": k})
}

// DELETE /api/v1/keys/:id（JWT）——吊销
func (h *KeysHandler) Revoke(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	if err := h.Keys.Revoke(c.Request.Context(), uid(c), id); err != nil {
		if err == service.ErrKeyNotFound {
			fail(c, errNotFound("密钥不存在或已吊销"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

func uid(c *gin.Context) int64 {
	v, _ := c.Get(CtxUserID)
	n, _ := v.(int64)
	return n
}
