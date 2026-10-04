package api

import (
	"errors"
	"net/http"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type TrashHandler struct{ Trash *service.TrashService }

// ListTrash GET /api/v1/trash（Web JWT）
func (h *TrashHandler) List(c *gin.Context) {
	items, counts, err := h.Trash.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "counts": counts})
}

// RestoreTrash POST /api/v1/trash/restore —— body {type, id}
func (h *TrashHandler) Restore(c *gin.Context) {
	var in struct {
		Type string `json:"type" binding:"required"`
		ID   int64  `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {type: todo|memo|note, id}"))
		return
	}
	c.Set(CtxAuditTarget, in.Type+"#"+itoaInt64(in.ID))
	if err := h.Trash.Restore(c.Request.Context(), uid(c), model.TrashType(in.Type), in.ID); err != nil {
		if errors.Is(err, service.ErrTrashNotFound) {
			fail(c, errNotFound("回收站中不存在该条目（可能已被清理或恢复）"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PurgeItem DELETE /api/v1/trash/:type/:id —— 彻底删除一条（物理删除，不可恢复）
func (h *TrashHandler) PurgeItem(c *gin.Context) {
	id, err := paramID(c)
	if err != nil {
		return
	}
	tt := model.TrashType(c.Param("type"))
	if !tt.Valid() {
		fail(c, errBadRequest("invalid_type", "type 必须是 todo / memo / note"))
		return
	}
	c.Set(CtxAuditTarget, string(tt)+"#"+itoaInt64(id))
	if err := h.Trash.PurgeItem(c.Request.Context(), uid(c), tt, id); err != nil {
		if errors.Is(err, service.ErrTrashNotFound) {
			fail(c, errNotFound("回收站中不存在该条目（可能已被清理或恢复）"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"purged": true})
}

// PurgeAll DELETE /api/v1/trash/purge —— 清空回收站（物理删除全部软删条目）
func (h *TrashHandler) PurgeAll(c *gin.Context) {
	out, err := h.Trash.PurgeAll(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.Set(CtxAuditTarget, itoaInt64(out["todo"]+out["memo"]+out["note"])+" 条")
	c.JSON(http.StatusOK, gin.H{"purged": out})
}

func itoaInt64(n int64) string {
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
