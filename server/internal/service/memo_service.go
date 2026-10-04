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

// MemoService —— 便签时间流。
type MemoService struct{ DB *gorm.DB }

var (
	ErrMemoNotFound = errors.New("memo not found")
	memoMax         = 2000
)

type MemoCreate struct {
	Content string   `json:"content" binding:"required"`
	Tags    []string `json:"tags"`
}

type MemoUpdate struct {
	Content  *string  `json:"content"`
	Tags     []string `json:"tags"`
	IsPinned *int16   `json:"is_pinned"`
}

func (s *MemoService) Create(ctx context.Context, userID int64, in MemoCreate) (*model.Memo, error) {
	c := strings.TrimSpace(in.Content)
	if c == "" {
		return nil, &UserError{"内容不能为空"}
	}
	if len([]rune(c)) > memoMax {
		return nil, &UserError{"便签超 2000 字——长内容请存知识库（note push）"}
	}
	m := &model.Memo{UserID: userID, Content: c, Tags: normalizeTags(in.Tags)}
	if err := s.DB.WithContext(ctx).Create(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (s *MemoService) List(ctx context.Context, userID int64, limit, beforeID int64) ([]model.Memo, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	q := s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 0", userID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	memos := make([]model.Memo, 0)
	err := q.Order("is_pinned DESC, id DESC").Limit(int(limit)).Find(&memos).Error
	return memos, err
}

func (s *MemoService) Update(ctx context.Context, userID, id int64, in MemoUpdate) (*model.Memo, error) {
	var m model.Memo
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		First(&m).Error; err != nil {
		return nil, ErrMemoNotFound
	}
	updates := map[string]any{"update_time": time.Now()}
	if in.Content != nil {
		c := strings.TrimSpace(*in.Content)
		if c == "" {
			return nil, &UserError{"内容不能为空"}
		}
		if len([]rune(c)) > memoMax {
			return nil, &UserError{"便签超 2000 字——长内容请存知识库"}
		}
		updates["content"] = c
	}
	if in.Tags != nil {
		// map 更新写 text[] 必须 pq.StringArray（裸 []string 报 malformed array literal，todo 路径同坑）
		updates["tags"] = pq.StringArray(normalizeTags(in.Tags))
	}
	if in.IsPinned != nil {
		updates["is_pinned"] = *in.IsPinned
	}
	if err := s.DB.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.DB.WithContext(ctx).First(&m, id).Error
	return &m, nil
}

// MemoSearchHit 全局搜索命中的便签行
type MemoSearchHit struct {
	ID         int64          `json:"id"`
	Snippet    string         `json:"snippet"`
	Tags       pq.StringArray `gorm:"column:tags;type:text[]" json:"tags"`
	CreateTime time.Time      `json:"create_time"`
}

// Search 全局搜索的便签域（content/tags ILIKE）
func (s *MemoService) Search(ctx context.Context, userID int64, q string, limit int) ([]MemoSearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, &UserError{"q 不能为空"}
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	like := "%" + q + "%"
	var rows []MemoSearchHit
	err := s.DB.WithContext(ctx).Model(&model.Memo{}).
		Select("id, content AS snippet, tags, create_time").
		Where("user_id = ? AND is_deleted = 0 AND (content ILIKE ? OR array_to_string(tags, ' ') ILIKE ?)", userID, like, like).
		Order("create_time DESC").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []MemoSearchHit{} // 契约：JSON 恒数组（nil→null 会打崩前端 .length）
	}
	for i := range rows {
		rows[i].Snippet = excerpt(rows[i].Snippet, q, 40)
	}
	return rows, nil
}

func (s *MemoService) Delete(ctx context.Context, userID, id int64) error {
	res := s.DB.WithContext(ctx).Model(&model.Memo{}).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).
		Updates(map[string]any{"is_deleted": 1, "update_time": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMemoNotFound
	}
	return nil
}
