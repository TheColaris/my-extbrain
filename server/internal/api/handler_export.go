package api

import (
	"bytes"
	"net/http"
	"os"
	"time"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type ExportHandler struct {
	Export *service.ExportService
}

// All GET /api/v1/export —— 导出全部个人数据（zip：notes 目录树 + todos/memos JSON + README）
// 先写临时文件再流式回传：既不把整个 zip 缓存在内存，也能在导出失败时正常回 500。
func (h *ExportHandler) All(c *gin.Context) {
	root := "extbrain-export-" + time.Now().Format("20060102-1504")
	f, err := os.CreateTemp("", "extbrain-export-*.zip")
	if err != nil {
		// 极简环境无 /tmp 时回退内存缓冲（导出仍可用；大库才可能有内存压力）
		var buf bytes.Buffer
		if err := h.Export.Export(c.Request.Context(), uid(c), &buf, root); err != nil {
			respondServiceErr(c, err)
			return
		}
		c.Header("Content-Disposition", `attachment; filename="`+root+`.zip"`)
		c.Data(http.StatusOK, "application/zip", buf.Bytes())
		return
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()

	if err := h.Export.Export(c.Request.Context(), uid(c), f, root); err != nil {
		respondServiceErr(c, err)
		return
	}
	st, err := f.Stat()
	if err != nil {
		fail(c, errInternal("导出失败，请稍后重试"))
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		fail(c, errInternal("导出失败，请稍后重试"))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+root+`.zip"`)
	c.DataFromReader(http.StatusOK, st.Size(), "application/zip", f, nil)
}
