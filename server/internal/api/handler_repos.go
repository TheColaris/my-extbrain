package api

import (
	"errors"
	"net/http"
	"strconv"

	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

// RepoHandler 知识库仓库管理（迁移 0012）——Web JWT 专属（仓库管理=owner 面板行为，AI 不可建/删仓库）。
type RepoHandler struct{ Repos *service.RepoService }

// List GET /api/v1/repos —— 活跃仓库列表（默认在前）+ 各仓活跃笔记数。
func (h *RepoHandler) List(c *gin.Context) {
	repos, err := h.Repos.List(c.Request.Context(), uid(c))
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"repos": repos})
}

// Create POST /api/v1/repos —— body {name, description?}；受数量配额（tp_system_config repo.quota）约束。
func (h *RepoHandler) Create(c *gin.Context) {
	var in struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {name, description?}"))
		return
	}
	c.Set(CtxAuditTarget, in.Name)
	repo, err := h.Repos.Create(c.Request.Context(), uid(c), in.Name, in.Description)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"repo": repo})
}

// Update PATCH /api/v1/repos/:id —— body {name?, description?}（默认仓库可改名）。
func (h *RepoHandler) Update(c *gin.Context) {
	var in struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {name?, description?}"))
		return
	}
	id, perr := strconv.ParseInt(c.Param("id"), 10, 64)
	if perr != nil || id <= 0 {
		fail(c, errBadRequest("invalid_request", "仓库 id 不合法"))
		return
	}
	if in.Name != nil {
		c.Set(CtxAuditTarget, *in.Name)
	}
	repo, err := h.Repos.Update(c.Request.Context(), uid(c), id, in.Name, in.Description)
	if err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			fail(c, errNotFound("仓库不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"repo": repo})
}

// Delete DELETE /api/v1/repos/:id —— 默认仓库不可删；非空仓库禁删（先移走或清空笔记）。
func (h *RepoHandler) Delete(c *gin.Context) {
	id, perr := strconv.ParseInt(c.Param("id"), 10, 64)
	if perr != nil || id <= 0 {
		fail(c, errBadRequest("invalid_request", "仓库 id 不合法"))
		return
	}
	if err := h.Repos.Delete(c.Request.Context(), uid(c), id); err != nil {
		if errors.Is(err, service.ErrRepoNotFound) {
			fail(c, errNotFound("仓库不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
