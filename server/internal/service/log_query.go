package service

import (
	"context"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// LogQueryService —— 审计日志查询（面板用；游标分页 before_id）。
type LogQueryService struct{ DB *gorm.DB }

type LogFilter struct {
	KeyID    int64
	Action   string
	Start    *time.Time
	End      *time.Time
	BeforeID int64
	Limit    int
}

type LogPage struct {
	Logs         []LogItem `json:"logs"`
	NextBeforeID int64     `json:"next_before_id,omitempty"` // 0=没有更多
}

// LogItem 审计行 + 返回面注记（笔记类 action 反解所属仓库名，供面板跳转按仓库寻址）。
type LogItem struct {
	model.APILog
	RepoName string `json:"repo_name,omitempty"`
}

func (s *LogQueryService) List(ctx context.Context, userID int64, f LogFilter) (*LogPage, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	q := s.DB.WithContext(ctx).Model(&model.APILog{}).Where("user_id = ?", userID)
	if f.KeyID > 0 {
		q = q.Where("api_key_id = ?", f.KeyID)
	} else if f.KeyID < 0 { // -1 = 仅 Web 会话（api_key_id=0）
		q = q.Where("api_key_id = 0")
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.Start != nil {
		q = q.Where("create_time >= ?", *f.Start)
	}
	if f.End != nil {
		q = q.Where("create_time < ?", *f.End)
	}
	if f.BeforeID > 0 {
		q = q.Where("id < ?", f.BeforeID)
	}
	var logs []model.APILog
	if err := q.Order("id DESC").Limit(f.Limit + 1).Find(&logs).Error; err != nil {
		return nil, err
	}
	page := &LogPage{}
	if len(logs) > f.Limit {
		logs = logs[:f.Limit]
		page.NextBeforeID = logs[len(logs)-1].ID
	}
	page.Logs = s.attachRepoNames(ctx, userID, logs)
	return page, nil
}

// attachRepoNames 笔记类 action（target=仓库内相对路径）批量反解所属仓名。
// 同路径存在于多个仓库时取默认仓库优先（审计行本身不含仓库维度，取最可能目标）；
// move/perm 行 target 非「单一路径」形态（含 → 或目录），跳过。
func (s *LogQueryService) attachRepoNames(ctx context.Context, userID int64, logs []model.APILog) []LogItem {
	items := make([]LogItem, len(logs))
	for i, l := range logs {
		items[i] = LogItem{APILog: l}
	}
	paths := map[string]bool{}
	for _, it := range items {
		if !strings.HasPrefix(it.Action, "note.") || it.Action == "note.perm" || it.Action == "note.move" {
			continue
		}
		p := strings.Trim(it.Target, "/")
		if p != "" {
			paths[p] = true
		}
	}
	if len(paths) == 0 {
		return items
	}
	list := make([]string, 0, len(paths))
	for p := range paths {
		list = append(list, p)
	}
	var rows []struct {
		Path string
		Name string
	}
	// DISTINCT ON：同路径多仓时默认仓库优先
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (n.path) n.path, r.name AS name
		FROM tf_note n JOIN tf_repo r ON r.id = n.repo_id
		WHERE n.user_id = ? AND n.path IN ?
		ORDER BY n.path, r.is_default DESC, r.id ASC`, userID, list).Scan(&rows).Error; err != nil {
		return items // 反解失败不阻塞日志查询（注记字段缺失仅影响跳转体验）
	}
	byPath := make(map[string]string, len(rows))
	for _, r := range rows {
		byPath[r.Path] = r.Name
	}
	for i := range items {
		if !strings.HasPrefix(items[i].Action, "note.") || items[i].Action == "note.perm" || items[i].Action == "note.move" {
			continue
		}
		if name := byPath[strings.Trim(items[i].Target, "/")]; name != "" {
			items[i].RepoName = name
		}
	}
	return items
}
