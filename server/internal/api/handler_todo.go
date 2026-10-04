package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
)

type TodoHandler struct{ Todos *service.TodoService }

// POST /api/v1/todos
func (h *TodoHandler) Create(c *gin.Context) {
	var in service.TodoCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {title, remark?, due_time?, tags?}"))
		return
	}
	in.Source = authSource(c)
	t, err := h.Todos.Create(c.Request.Context(), uid(c), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, t)
}

// GET /api/v1/todos?status=&limit=&before_id=&sort=
// 响应含 counts（各状态全量计数，与筛选无关）与 sort（实际生效的排序）；
// sort 省略时取用户偏好（tu_user.todo_sort，未设置=created）。
func (h *TodoHandler) List(c *gin.Context) {
	f := service.TodoFilter{Status: model.TodoStatus(c.Query("status")), Sort: c.Query("sort")}
	if f.Status != "" && !f.Status.Valid() {
		fail(c, errBadRequest("invalid_status", "status 必须是 active（在途）/ done（已完成）"))
		return
	}
	if f.Sort != "" && !model.TodoSort(f.Sort).Valid() {
		fail(c, errBadRequest("invalid_sort", "sort 必须是 created（创建时间）/ due（截止时间）"))
		return
	}
	if f.Sort == "" {
		if pref, err := h.Todos.SortPref(c.Request.Context(), uid(c)); err == nil && model.TodoSort(pref).Valid() {
			f.Sort = pref
		}
	}
	if f.Sort == "" {
		f.Sort = string(model.TodoSortCreated) // 兜底（含历史空值行）
	}
	f.Limit, _ = strconv.Atoi(c.Query("limit"))
	f.BeforeID, _ = strconv.ParseInt(c.Query("before_id"), 10, 64)
	todos, counts, err := h.Todos.List(c.Request.Context(), uid(c), f)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"todos": todos, "counts": counts, "sort": f.Sort})
}

// PUT /api/v1/todo-sort（Web JWT）：持久化当前用户的待办排序偏好
func (h *TodoHandler) SetSortPref(c *gin.Context) {
	var in struct {
		Sort string `json:"sort"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {sort}，sort=created|due"))
		return
	}
	if err := h.Todos.SetSortPref(c.Request.Context(), uid(c), in.Sort); err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"todo_sort": in.Sort})
}

// PATCH /api/v1/todos/:id（状态流转即改 status）
func (h *TodoHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	var in service.TodoUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {title?, remark?, status?, due_time?, clear_due?, tags?}，省略=不修改"))
		return
	}
	t, err := h.Todos.Update(c.Request.Context(), uid(c), id, in)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			fail(c, errNotFound("待办不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// POST /api/v1/todos/:id/restore（撤销删除）
func (h *TodoHandler) Restore(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	t, err := h.Todos.Restore(c.Request.Context(), uid(c), id)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			fail(c, errNotFound("待办不存在或未被删除"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

// DELETE /api/v1/todos/:id
func (h *TodoHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	if err := h.Todos.Delete(c.Request.Context(), uid(c), id); err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			fail(c, errNotFound("待办不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type MemoHandler struct{ Memos *service.MemoService }

func (h *MemoHandler) Create(c *gin.Context) {
	var in service.MemoCreate
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {content, tags?}"))
		return
	}
	m, err := h.Memos.Create(c.Request.Context(), uid(c), in)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, m)
}

func (h *MemoHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.Query("limit"), 10, 64)
	beforeID, _ := strconv.ParseInt(c.Query("before_id"), 10, 64)
	memos, err := h.Memos.List(c.Request.Context(), uid(c), limit, beforeID)
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"memos": memos})
}

func (h *MemoHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	var in service.MemoUpdate
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {content?, tags?, is_pinned?}"))
		return
	}
	m, err := h.Memos.Update(c.Request.Context(), uid(c), id, in)
	if err != nil {
		if errors.Is(err, service.ErrMemoNotFound) {
			fail(c, errNotFound("便签不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, m)
}

func (h *MemoHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(c, errBadRequest("invalid_id", "id 必须是正整数"))
		return
	}
	if err := h.Memos.Delete(c.Request.Context(), uid(c), id); err != nil {
		if errors.Is(err, service.ErrMemoNotFound) {
			fail(c, errNotFound("便签不存在"))
			return
		}
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type DashboardHandler struct{ Dash *service.DashboardService }

// GET /api/v1/dashboard
func (h *DashboardHandler) Summary(c *gin.Context) {
	d, err := h.Dash.Summary(c.Request.Context(), uid(c), "")
	if err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func authSource(c *gin.Context) model.TodoSource {
	if v, ok := c.Get(CtxAuthCh); ok {
		if s, _ := v.(string); s == "cli" {
			return model.TodoSourceCLI
		}
	}
	return model.TodoSourceWeb
}

var _ = time.Now
