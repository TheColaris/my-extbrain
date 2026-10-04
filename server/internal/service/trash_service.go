package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TrashService —— 回收站：三实体软删的统一找回视图 + 彻底删除 + 保留期清理。
// 注：笔记「恢复冲突」在现 schema 下不存在（uk_note_user_path 含软删行 + Upsert 复活语义，
// 同路径不可能同时有活跃笔记与软删笔记），恢复即成功。
type TrashService struct{ DB *gorm.DB }

const trashRetentionDays = 30

// TrashItem 统一条目（三实体映射到同一形态）
type TrashItem struct {
	Type      string    `json:"type"` // todo/memo/note（model.TrashType）
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle,omitempty"` // 备注 / 原信息（原截止、并入去向等）
	Tags      []string  `json:"tags"`
	RepoName  string    `json:"repo_name,omitempty"` // 笔记所属仓库（跨仓库回收站区分来源）
	DeletedAt time.Time `json:"deleted_at"`
	LeftDays  int       `json:"left_days"` // 距自动清理剩余天数（≥0）
	Source    string    `json:"source"`    // 删除来源：Web / CLI · <Key 名> / 未知
}

// SourceLookup 反查删除来源：按删除类审计日志的 target（todo/memo=id，note=path）匹配最近一条。
// 审计保留 90 天 > 回收站 30 天，时间窗必然覆盖。
func (s *TrashService) SourceLookup(ctx context.Context, userID int64) map[string]string {
	type logRow struct {
		Action   string
		Target   string
		APIKeyID int64 `gorm:"column:api_key_id"`
	}
	var rows []logRow
	s.DB.WithContext(ctx).Model(&model.APILog{}).
		Select("action, target, api_key_id").
		Where("user_id = ? AND action IN ? AND create_time > ?",
			userID, []string{"todo.delete", "memo.delete", "note.delete"},
			time.Now().AddDate(0, 0, -(trashRetentionDays+5))).
		Order("id DESC").Limit(1000).Scan(&rows)
	if len(rows) == 0 {
		return map[string]string{}
	}
	// key 名一次装填（0=Web 会话）
	needKeys := map[int64]bool{}
	first := map[string]int64{}
	for _, r := range rows {
		t := strings.TrimPrefix(r.Target, "/")
		k := r.Action + "|" + t
		if _, ok := first[k]; !ok {
			first[k] = r.APIKeyID
			if r.APIKeyID != 0 {
				needKeys[r.APIKeyID] = true
			}
		}
	}
	keyNames := map[int64]string{}
	if len(needKeys) > 0 {
		ids := make([]int64, 0, len(needKeys))
		for id := range needKeys {
			ids = append(ids, id)
		}
		var keys []model.APIKey
		s.DB.WithContext(ctx).Select("id, key_name").Where("id IN ?", ids).Find(&keys)
		for _, k := range keys {
			keyNames[k.ID] = k.KeyName
		}
	}
	out := map[string]string{}
	for k, keyID := range first {
		if keyID == 0 {
			out[k] = "Web"
			continue
		}
		if name := keyNames[keyID]; name != "" {
			out[k] = "CLI · " + name
		} else {
			out[k] = "CLI"
		}
	}
	return out
}

func leftDays(deletedAt time.Time) int {
	d := trashRetentionDays - int(time.Since(deletedAt).Hours()/24)
	if d < 0 {
		return 0
	}
	return d
}

// List 回收站列表（update_time 倒序 = 最近删除在前）+ 分类型计数。
func (s *TrashService) List(ctx context.Context, userID int64) ([]TrashItem, map[string]int64, error) {
	src := s.SourceLookup(ctx, userID)
	items := make([]TrashItem, 0)
	counts := map[string]int64{"todo": 0, "memo": 0, "note": 0}

	var todos []model.Todo
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 1", userID).
		Order("update_time DESC").Limit(200).Find(&todos).Error; err != nil {
		return nil, nil, err
	}
	for _, t := range todos {
		sub := ""
		if t.DueTime != nil {
			sub = "原截止 " + t.DueTime.Format("01-02 15:04")
		}
		items = append(items, TrashItem{
			Type: string(model.TrashTodo), ID: t.ID, Title: t.Title, Subtitle: sub,
			Tags: t.Tags, DeletedAt: t.UpdateTime, LeftDays: leftDays(t.UpdateTime),
			Source: src["todo.delete|"+itoa(t.ID)],
		})
		counts["todo"]++
	}

	var memos []model.Memo
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 1", userID).
		Order("update_time DESC").Limit(200).Find(&memos).Error; err != nil {
		return nil, nil, err
	}
	for _, m := range memos {
		items = append(items, TrashItem{
			Type: string(model.TrashMemo), ID: m.ID, Title: cut(m.Content, 80),
			Tags: m.Tags, DeletedAt: m.UpdateTime, LeftDays: leftDays(m.UpdateTime),
			Source: src["memo.delete|"+itoa(m.ID)],
		})
		counts["memo"]++
	}

	var notes []model.Note
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 1", userID).
		Order("update_time DESC").Limit(200).Find(&notes).Error; err != nil {
		return nil, nil, err
	}
	repoNames := map[int64]string{}
	if len(notes) > 0 {
		repoIDs := make([]int64, 0, len(notes))
		for _, n := range notes {
			repoIDs = append(repoIDs, n.RepoID)
		}
		var repos []model.Repo
		if err := s.DB.WithContext(ctx).Select("id, name").
			Where("id IN ?", repoIDs).Find(&repos).Error; err != nil {
			return nil, nil, err
		}
		for _, r := range repos {
			repoNames[r.ID] = r.Name
		}
	}
	for _, n := range notes {
		items = append(items, TrashItem{
			Type: string(model.TrashNote), ID: n.ID, Title: n.Path, Subtitle: n.Title,
			Tags: n.Tags, RepoName: repoNames[n.RepoID], DeletedAt: n.UpdateTime, LeftDays: leftDays(n.UpdateTime),
			Source: src["note.delete|"+strings.TrimPrefix(n.Path, "/")],
		})
		counts["note"]++
	}

	// 混排：删除时间倒序
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].DeletedAt.After(items[i].DeletedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	return items, counts, nil
}

var ErrTrashNotFound = errors.New("回收站中不存在该条目")

// Restore 恢复一条（todo/memo 翻 is_deleted；note 同理——无冲突场景，见类型注释）。
func (s *TrashService) Restore(ctx context.Context, userID int64, tt model.TrashType, id int64) error {
	now := time.Now()
	switch tt {
	case model.TrashTodo:
		return s.restoreWhere(ctx, &model.Todo{}, userID, id, now)
	case model.TrashMemo:
		return s.restoreWhere(ctx, &model.Memo{}, userID, id, now)
	case model.TrashNote:
		return s.restoreWhere(ctx, &model.Note{}, userID, id, now)
	}
	return &UserError{"type 必须是 todo / memo / note"}
}

func (s *TrashService) restoreWhere(ctx context.Context, m any, userID, id int64, now time.Time) error {
	res := s.DB.WithContext(ctx).Model(m).
		Where("id = ? AND user_id = ? AND is_deleted = 1", id, userID).
		Updates(map[string]any{"is_deleted": 0, "update_time": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTrashNotFound
	}
	return nil
}

// PurgeItem 彻底删除一条（物理 DELETE；笔记连带正文，事务）。
func (s *TrashService) PurgeItem(ctx context.Context, userID int64, tt model.TrashType, id int64) error {
	switch tt {
	case model.TrashTodo:
		return s.purgeWhere(ctx, &model.Todo{}, userID, id)
	case model.TrashMemo:
		return s.purgeWhere(ctx, &model.Memo{}, userID, id)
	case model.TrashNote:
		return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var n model.Note
			// FOR UPDATE 锁行：并发 Restore 的 UPDATE 等待至本事务提交，之后其 UPDATE 影响 0 行——
			// 消除「先删正文、后被恢复」的窗口（否则主行活着正文已丢）
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND user_id = ? AND is_deleted = 1", id, userID).First(&n).Error; err != nil {
				return ErrTrashNotFound
			}
			// 正文与主行删除都带 is_deleted=1 条件：与并发 Restore 交错时宁可不删（404），不可硬删已恢复的活跃笔记
			if err := tx.Exec(`DELETE FROM tf_note_content WHERE note_id IN
				(SELECT id FROM tf_note WHERE id = ? AND user_id = ? AND is_deleted = 1)`, n.ID, userID).Error; err != nil {
				return err
			}
			res := tx.Exec(`DELETE FROM tf_note WHERE id = ? AND user_id = ? AND is_deleted = 1`, n.ID, userID)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrTrashNotFound
			}
			return nil
		})
	}
	return &UserError{"type 必须是 todo / memo / note"}
}

func (s *TrashService) purgeWhere(ctx context.Context, m any, userID, id int64) error {
	res := s.DB.WithContext(ctx).Where("id = ? AND user_id = ? AND is_deleted = 1", id, userID).Delete(m)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrTrashNotFound
	}
	return nil
}

// PurgeAll 清空回收站（该用户全部软删条目物理删除；笔记连带正文）。
func (s *TrashService) PurgeAll(ctx context.Context, userID int64) (map[string]int64, error) {
	out := map[string]int64{}
	r := s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 1", userID).Delete(&model.Todo{})
	if r.Error != nil {
		return nil, r.Error
	}
	out["todo"] = r.RowsAffected
	r = s.DB.WithContext(ctx).Where("user_id = ? AND is_deleted = 1", userID).Delete(&model.Memo{})
	if r.Error != nil {
		return nil, r.Error
	}
	out["memo"] = r.RowsAffected
	// 笔记：先正文后主行（同一软删集合的子查询；主行删除带 is_deleted=1，
	// 与并发 Restore 交错时该行保留且不误删正文，计数取实际删除行数）
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先 FOR UPDATE 锁全部软删行：并发 Restore 阻塞至本事务提交，窗口归零
		if err := tx.Exec(`SELECT id FROM tf_note WHERE user_id = ? AND is_deleted = 1 FOR UPDATE`, userID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM tf_note_content WHERE note_id IN
			(SELECT id FROM tf_note WHERE user_id = ? AND is_deleted = 1)`, userID).Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM tf_note WHERE user_id = ? AND is_deleted = 1`, userID)
		if res.Error != nil {
			return res.Error
		}
		out["note"] = res.RowsAffected
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// CleanupExpired 保留期清理（硬删 is_deleted=1 且 update_time 超期的行；笔记连带正文）。
func (s *TrashService) CleanupExpired(ctx context.Context, days int) (map[string]int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	out := map[string]int64{}
	r := s.DB.WithContext(ctx).Where("is_deleted = 1 AND update_time < ?", cutoff).Delete(&model.Todo{})
	if r.Error != nil {
		return out, r.Error
	}
	out["todo"] = r.RowsAffected
	r = s.DB.WithContext(ctx).Where("is_deleted = 1 AND update_time < ?", cutoff).Delete(&model.Memo{})
	if r.Error != nil {
		return out, r.Error
	}
	out["memo"] = r.RowsAffected
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先 FOR UPDATE 锁超期软删行：与并发 Restore/软删更新的竞态窗口归零
		if err := tx.Exec(`SELECT id FROM tf_note WHERE is_deleted = 1 AND update_time < ? FOR UPDATE`, cutoff).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM tf_note_content WHERE note_id IN
			(SELECT id FROM tf_note WHERE is_deleted = 1 AND update_time < ?)`, cutoff).Error; err != nil {
			return err
		}
		res := tx.Exec(`DELETE FROM tf_note WHERE is_deleted = 1 AND update_time < ?`, cutoff)
		if res.Error != nil {
			return res.Error
		}
		out["note"] = res.RowsAffected
		return nil
	}); err != nil {
		return out, err
	}
	return out, nil
}
