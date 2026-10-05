package api

import (
	"net/http"

	"extbrain-server/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// E2EHandler 洁净室信箱（仅三重门全开时注册路由；门不开=404）。
// 无鉴权：洁净环境本身与外网隔离，旅程脚本从外部读信箱断言外发截获。
type E2EHandler struct{ DB *gorm.DB }

// Mailbox GET /api/v1/e2e/mailbox?channel=&target= —— 倒序列出信箱（上限 200）。
func (h *E2EHandler) Mailbox(c *gin.Context) {
	q := h.DB.Model(&model.E2EMailbox{})
	if v := c.Query("channel"); v != "" {
		q = q.Where("channel = ?", v)
	}
	if v := c.Query("target"); v != "" {
		q = q.Where("target = ?", v)
	}
	rows := make([]model.E2EMailbox, 0)
	if err := q.Order("id DESC").Limit(200).Find(&rows).Error; err != nil {
		fail(c, errInternal("信箱查询失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": rows, "count": len(rows)})
}

// MailboxClear DELETE /api/v1/e2e/mailbox?channel=&target= —— 清箱（带参清一箱，全省清全部）。
func (h *E2EHandler) MailboxClear(c *gin.Context) {
	q := h.DB.Where("1=1")
	if v := c.Query("channel"); v != "" {
		q = q.Where("channel = ?", v)
	}
	if v := c.Query("target"); v != "" {
		q = q.Where("target = ?", v)
	}
	res := q.Delete(&model.E2EMailbox{})
	if res.Error != nil {
		fail(c, errInternal("清箱失败: "+res.Error.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": res.RowsAffected})
}
