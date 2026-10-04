package api

import (
	"net/http"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

// NotePermHandler 目录权限（AI Key 可见性白名单）——Web JWT 专属（AI 不可自改权限）。
type NotePermHandler struct{ Perm *service.NotePermService }

// Get GET /api/v1/note-perm —— 全部规则（树锁标记 + 弹窗预填）。
func (h *NotePermHandler) Get(c *gin.Context) {
	rules, err := h.Perm.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

// Put PUT /api/v1/note-perm —— body {folder_path, mode: open|allow, key_ids}。
// mode=open 删除规则（回到开放/继承）；mode=allow 允许 key_ids 空=对 AI 完全封闭。
func (h *NotePermHandler) Put(c *gin.Context) {
	var in service.NotePermInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {repo_id, folder_path（''=仓库级默认规则）, mode: open|allow, key_ids: []}"))
		return
	}
	label := in.FolderPath
	if label == "" {
		label = "(根目录)"
	}
	c.Set(CtxAuditTarget, label)
	if err := h.Perm.Put(c.Request.Context(), uid(c), in); err != nil {
		respondServiceErr(c, err)
		return
	}
	rules, err := h.Perm.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": true, "rules": rules})
}
