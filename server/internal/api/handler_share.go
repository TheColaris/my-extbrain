package api

import (
	"errors"
	"net/http"
	"strings"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type ShareHandler struct {
	Shares *service.ShareService
	Repos  *service.RepoService     // 分享路径的仓库解析（缺省=默认仓库）
	Perm   *service.NotePermService // 分享建/复制写入时的目录权限校验
}

// CreateShare POST /api/v1/shares —— body {path, mode, expire_days}：创建/更新该笔记的活跃分享
// （已有则保留 token 更新设置并重冻快照）。expire_days 0=永久。
func (h *ShareHandler) Create(c *gin.Context) {
	var in struct {
		Path       string `json:"path" binding:"required"`
		Repo       string `json:"repo"`
		Mode       string `json:"mode" binding:"required"`
		ExpireDays *int   `json:"expire_days" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {path, repo?, mode: snapshot|live, expire_days: 0=永久|天数}"))
		return
	}
	c.Set(CtxAuditTarget, in.Path)
	repoID, ok := h.resolveRepo(c, in.Repo)
	if !ok {
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	share, err := h.Shares.Upsert(c.Request.Context(), uid(c), repoID, in.Path, model.ShareMode(in.Mode), *in.ExpireDays, view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrNoteNotFound):
			fail(c, errNotFound("笔记不存在："+in.Path))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"share": share})
}

// GetShare GET /api/v1/shares?path= —— owner 视角查该笔记的活跃分享（无则 share=null）
func (h *ShareHandler) Get(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		fail(c, errBadRequest("invalid_request", "path 不能为空"))
		return
	}
	c.Set(CtxAuditTarget, path)
	repoID, ok := h.resolveRepo(c, c.Query("repo"))
	if !ok {
		return
	}
	share, err := h.Shares.GetByPath(c.Request.Context(), uid(c), repoID, path)
	if err != nil {
		if errors.Is(err, service.ErrNoteNotFound) {
			fail(c, errNotFound("笔记不存在："+path))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"share": share})
}

// RevokeShare DELETE /api/v1/shares/:token —— 停止分享（终态；再分享会生成新 token）
func (h *ShareHandler) Revoke(c *gin.Context) {
	token := c.Param("token")
	if err := h.Shares.Revoke(c.Request.Context(), uid(c), token); err != nil {
		if errors.Is(err, service.ErrShareNotFound) {
			fail(c, errNotFound("分享不存在或已停止"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

// CopyShare POST /api/v1/shares/:token/copy —— 把有效分享复制到当前用户的知识库（复制=新建，不覆盖）
func (h *ShareHandler) Copy(c *gin.Context) {
	token := c.Param("token")
	c.Set(CtxAuditTarget, token)
	repoID, err := h.Repos.DefaultID(c.Request.Context(), uid(c)) // 复制目标=默认仓库根目录
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	view, ok := permView(c, h.Perm)
	if !ok {
		return
	}
	note, err := h.Shares.CopyToLibrary(c.Request.Context(), uid(c), repoID, token, view)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrShareNotFound):
			fail(c, errNotFound("分享不存在或已停止"))
		case errors.Is(err, service.ErrNoteDenied):
			failNoteDenied(c)
		case errors.Is(err, service.ErrShareExpired):
			fail(c, &ApiError{Code: "share_expired", Message: "分享已过期", Status: http.StatusNotFound})
		case errors.Is(err, service.ErrShareSourceDeleted):
			fail(c, &ApiError{Code: "source_deleted", Message: "原笔记已删除，分享随之失效", Status: http.StatusNotFound})
		default:
			respondServiceErr(c, err)
		}
		return
	}
	note.RepoName = "默认仓库" // 复制目标=默认仓库根目录（返回面注记）
	c.JSON(http.StatusOK, gin.H{"note": note})
}

// resolveRepo 分享路径的仓库解析（body/query 显式仓库名；空=默认仓库）。
func (h *ShareHandler) resolveRepo(c *gin.Context, name string) (int64, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		id, err := h.Repos.DefaultID(c.Request.Context(), uid(c))
		if err != nil {
			fail(c, errInternal("默认仓库读取失败，请重试"))
			return 0, false
		}
		return id, true
	}
	r, err := h.Repos.ResolveByName(c.Request.Context(), uid(c), name)
	if err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			fail(c, errNotFound("仓库不存在："+name))
			return 0, false
		}
		fail(c, errInternal("仓库解析失败，请重试"))
		return 0, false
	}
	return r.ID, true
}

// PublicGetShare GET /api/v1/public/share/:token —— 公开只读（游客）：失效三分 {not_found|expired|source_deleted}
func (h *ShareHandler) PublicGet(c *gin.Context) {
	token := c.Param("token")
	view, err := h.Shares.PublicGet(c.Request.Context(), token)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrShareNotFound):
			fail(c, errNotFound("分享不存在或已停止"))
		case errors.Is(err, service.ErrShareExpired):
			fail(c, &ApiError{Code: "share_expired", Message: "分享已过期", Status: http.StatusNotFound})
		case errors.Is(err, service.ErrShareSourceDeleted):
			fail(c, &ApiError{Code: "source_deleted", Message: "原笔记已删除，分享随之失效", Status: http.StatusNotFound})
		default:
			respondServiceErr(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, view)
}
