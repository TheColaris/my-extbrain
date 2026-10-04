package api

import (
	"net/http"
	"strconv"
	"time"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type LogsHandler struct{ Logs *service.LogQueryService }

// GET /api/v1/logs?key_id=&action=&start=&end=&limit=&before_id=（JWT）
func (h *LogsHandler) List(c *gin.Context) {
	f := service.LogFilter{Limit: 20}
	if v, err := strconv.ParseInt(c.Query("key_id"), 10, 64); err == nil {
		f.KeyID = v // >0=指定 Key；-1=仅 Web 会话；0=全部
	}
	f.Action = c.Query("action")
	if s := c.Query("start"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Start = &t
		} else {
			fail(c, errBadRequest("invalid_start", "start 须为 RFC3339，如 2026-10-02T00:00:00Z"))
			return
		}
	}
	if s := c.Query("end"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.End = &t
		} else {
			fail(c, errBadRequest("invalid_end", "end 须为 RFC3339"))
			return
		}
	}
	if v, err := strconv.Atoi(c.Query("limit")); err == nil {
		f.Limit = v
	}
	if v, err := strconv.ParseInt(c.Query("before_id"), 10, 64); err == nil {
		f.BeforeID = v
	}
	page, err := h.Logs.List(c.Request.Context(), uid(c), f)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}
