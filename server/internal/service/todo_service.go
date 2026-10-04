package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// TodoService —— 待办 CRUD 与状态流转。
type TodoService struct{ DB *gorm.DB }

type TodoCreate struct {
	Title   string           `json:"title" binding:"required"`
	Remark  string           `json:"remark"`
	DueTime *time.Time       `json:"due_time"`
	Tags    []string         `json:"tags"`
	Source  model.TodoSource // 由 handler 按认证通道填（web/cli），不信任客户端
}

// TodoUpdate 部分更新；省略字段=不修改。
// 清除截止时间用 ClearDue 显式语义（due_time 传 null 与省略在 *time.Time 上无法区分）。
type TodoUpdate struct {
	Title    *string           `json:"title"`
	Remark   *string           `json:"remark"`
	Status   *model.TodoStatus `json:"status"`
	DueTime  *time.Time        `json:"due_time"`
	ClearDue bool              `json:"clear_due"`
	Tags     []string          `json:"tags"`
}

var (
	ErrTodoNotFound = errors.New("todo not found")
	titleMax        = 255
)

func (s *TodoService) Create(ctx context.Context, userID int64, in TodoCreate) (*model.Todo, error) {
	t := strings.TrimSpace(in.Title)
	if t == "" {
		return nil, &UserError{"标题不能为空"}
	}
	if len([]rune(t)) > titleMax {
		return nil, &UserError{"标题过长（≤255 字）"}
	}
	if in.Source == "" {
		in.Source = model.TodoSourceWeb
	}
	todo := &model.Todo{
		UserID: userID, Title: t, Remark: in.Remark, Status: model.TodoStatusActive,
		DueTime: in.DueTime, Tags: normalizeTags(in.Tags), Source: in.Source,
	}
	if err := s.DB.WithContext(ctx).Create(todo).Error; err != nil {
		return nil, err
	}
	return todo, nil
}

type TodoFilter struct {
	Status   model.TodoStatus // ""=全部 | active=在途 | done=已完成
	BeforeID int64
	Limit    int
	Sort     string // ""（取用户偏好，未设置=created）| created（创建倒序）| due（截止升序、无截止在后）
}

// TodoCounts 各状态全量计数（与筛选无关，供 Tab 计数展示）。
type TodoCounts struct {
	Active int64 `json:"active"`
	Done   int64 `json:"done"`
}

func (s *TodoService) List(ctx context.Context, userID int64, f TodoFilter) ([]model.Todo, TodoCounts, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	q := s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 0", userID)
	switch f.Status {
	case "":
		// 全部
	case model.TodoStatusDone:
		q = q.Where("status = ?", model.TodoStatusDone)
	default: // active（在途）
		q = q.Where("status = ?", model.TodoStatusActive)
	}
	if f.BeforeID > 0 {
		q = q.Where("id < ?", f.BeforeID)
	}
	switch f.Sort {
	case "due": // 截止升序、无截止在后；同截止新在前
		q = q.Order("(due_time IS NULL) ASC, due_time ASC, id DESC")
	default: // created：创建时间倒序（默认，未设置偏好时同此）
		q = q.Order("create_time DESC, id DESC")
	}
	todos := make([]model.Todo, 0)
	if err := q.Limit(f.Limit).Find(&todos).Error; err != nil {
		return nil, TodoCounts{}, err
	}
	counts, err := s.Counts(ctx, userID)
	if err != nil {
		return nil, TodoCounts{}, err
	}
	return todos, counts, nil
}

// Counts 全量状态计数（一次 GROUP BY；空结果返回全 0）。
func (s *TodoService) Counts(ctx context.Context, userID int64) (TodoCounts, error) {
	var rows []struct {
		Status string
		N      int64
	}
	err := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Select("status, COUNT(*) AS n").
		Where("user_id = ? AND is_deleted = 0", userID).
		Group("status").Scan(&rows).Error
	if err != nil {
		return TodoCounts{}, err
	}
	var c TodoCounts
	for _, r := range rows {
		switch model.TodoStatus(r.Status) {
		case model.TodoStatusActive:
			c.Active = r.N
		case model.TodoStatusDone:
			c.Done = r.N
		}
	}
	return c, nil
}

// SortPref 读取用户排序偏好（tu_user.todo_sort；空=未设置，调用方回退 created）。
func (s *TodoService) SortPref(ctx context.Context, userID int64) (string, error) {
	var u model.User
	if err := s.DB.WithContext(ctx).Select("id, todo_sort").First(&u, userID).Error; err != nil {
		return "", err
	}
	return u.TodoSort, nil
}

// SetSortPref 校验并持久化用户排序偏好。
func (s *TodoService) SetSortPref(ctx context.Context, userID int64, sort string) error {
	if !model.TodoSort(sort).Valid() {
		return &UserError{"sort 必须是 created（创建时间）/ due（截止时间）"}
	}
	res := s.DB.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND is_deleted = 0", userID).
		Updates(map[string]any{"todo_sort": sort, "update_time": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("user not found") // JWT 已验签但用户行缺失，属异常态，走 500
	}
	return nil
}

func (s *TodoService) Update(ctx context.Context, userID, id int64, in TodoUpdate) (*model.Todo, error) {
	var todo model.Todo
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		First(&todo).Error; err != nil {
		return nil, ErrTodoNotFound
	}
	updates := map[string]any{"update_time": time.Now()}
	if in.Title != nil {
		t := strings.TrimSpace(*in.Title)
		if t == "" {
			return nil, &UserError{"标题不能为空"}
		}
		updates["title"] = t
	}
	if in.Remark != nil {
		updates["remark"] = *in.Remark
	}
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, &UserError{"status 必须是 active / done"}
		}
		updates["status"] = *in.Status
		// completed_at 跟随 done 流转：进入 done 写入，离开 done 清空
		switch {
		case *in.Status == model.TodoStatusDone && todo.Status != model.TodoStatusDone:
			updates["completed_at"] = time.Now()
		case *in.Status != model.TodoStatusDone && todo.Status == model.TodoStatusDone:
			updates["completed_at"] = nil
		}
	}
	if in.ClearDue {
		updates["due_time"] = nil
	} else if in.DueTime != nil {
		updates["due_time"] = *in.DueTime
	}
	if in.Tags != nil {
		// 必须 pq.StringArray：map update 里的裸 []string 写 text[] 会失败（曾出现 500）
		updates["tags"] = pq.StringArray(normalizeTags(in.Tags))
	}
	if err := s.DB.WithContext(ctx).Model(&todo).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.DB.WithContext(ctx).First(&todo, id).Error
	return &todo, nil
}

// TodoSearchHit 全局搜索命中的待办行
type TodoSearchHit struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Snippet    string           `json:"snippet"` // 标题命中=备注摘录；备注/标签命中=备注摘录
	Status     model.TodoStatus `json:"status"`
	DueTime    *time.Time       `json:"due_time,omitempty"`
	Tags       pq.StringArray   `gorm:"column:tags;type:text[]" json:"tags"`
	Source     model.TodoSource `json:"source"`
	APIKeyID   *int64           `json:"api_key_id,omitempty"`
	CreateTime time.Time        `json:"create_time"`
}

// Search 全局搜索的待办域（title/remark/tags ILIKE；状态不限——已完成也搜得到）
func (s *TodoService) Search(ctx context.Context, userID int64, q string, limit int) ([]TodoSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, &UserError{"q 不能为空"}
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	like := "%" + q + "%"
	var rows []TodoSearchHit
	err := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Select("id, title, remark AS snippet, status, due_time, tags, source, api_key_id, create_time").
		Where("user_id = ? AND is_deleted = 0 AND (title ILIKE ? OR remark ILIKE ? OR array_to_string(tags, ' ') ILIKE ?)",
			userID, like, like, like).
		Order("create_time DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []TodoSearchHit{} // 契约：JSON 恒数组（nil→null 会打崩前端 .length）
	}
	lq := strings.ToLower(q)
	for i := range rows {
		if strings.Contains(strings.ToLower(rows[i].Title), lq) {
			continue // 标题命中：高亮主体即标题，摘录给备注原文
		}
		rows[i].Snippet = excerpt(rows[i].Snippet, q, 32)
	}
	return rows, nil
}

func (s *TodoService) Delete(ctx context.Context, userID, id int64) error {
	res := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		Updates(map[string]any{"is_deleted": 1, "update_time": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTodoNotFound
	}
	return nil
}

// Restore 撤销删除（is_deleted 1→0），返回恢复后的记录。
func (s *TodoService) Restore(ctx context.Context, userID, id int64) (*model.Todo, error) {
	res := s.DB.WithContext(ctx).Model(&model.Todo{}).
		Where("id = ? AND user_id = ? AND is_deleted = 1", id, userID).
		Updates(map[string]any{"is_deleted": 0, "update_time": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrTodoNotFound
	}
	var todo model.Todo
	if err := s.DB.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&todo).Error; err != nil {
		return nil, ErrTodoNotFound
	}
	return &todo, nil
}

func normalizeTags(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
