package api

import (
	"bytes"
	"net/http"
	"time"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type ExportHandler struct {
	Export *service.ExportService
}

// All GET /api/v1/export —— 导出全部个人数据（zip：notes 目录树 + todos/memos JSON + README）
// 先缓冲再写出：出错可正常回 500（流式直写时 header 已发就改不了状态码）。
func (h *ExportHandler) All(c *gin.Context) {
	root := "extbrain-export-" + time.Now().Format("20060102-1504")
	var buf bytes.Buffer
	if err := h.Export.Export(c.Request.Context(), uid(c), &buf, root); err != nil {
		respondServiceErr(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+root+`.zip"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}
