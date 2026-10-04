package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// ExportService —— 导出全部个人数据为 zip：notes 目录树（原始 .md）+ todos/memos JSON + README。
// 只含内容数据；不含密钥/分享/日志（敏感或可重建）。
type ExportService struct{ DB *gorm.DB }

type exportNote struct {
	RepoID     int64
	Path       string
	Title      string
	Content    string
	Tags       pq.StringArray
	UpdateTime time.Time
}

type exportTodo struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Remark      string     `json:"remark"`
	Status      string     `json:"status"`
	DueTime     *time.Time `json:"due_time,omitempty"`
	Tags        []string   `json:"tags"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreateTime  time.Time  `json:"create_time"`
	UpdateTime  time.Time  `json:"update_time"`
}

type exportMemo struct {
	ID         int64     `json:"id"`
	Content    string    `json:"content"`
	Tags       []string  `json:"tags"`
	IsPinned   bool      `json:"is_pinned"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// Export 打包到 w（zip）；root=压缩包内顶层目录名（如 extbrain-export-20261003-2230）。
func (s *ExportService) Export(ctx context.Context, userID int64, w io.Writer, root string) error {
	zw := zip.NewWriter(w)
	now := time.Now().Format("2006-01-02 15:04:05")

	// 1) 知识库：原始 MD（zip 内 <仓库名>/notes/<仓库内相对路径>，保留目录结构）
	type exportRepo struct {
		ID   int64
		Name string
	}
	var repos []exportRepo
	if err := s.DB.WithContext(ctx).Table("tf_repo").
		Select("id, name").
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("is_default DESC, sort ASC, id ASC").Scan(&repos).Error; err != nil {
		return err
	}
	repoNames := map[int64]string{}
	for _, r := range repos {
		repoNames[r.ID] = r.Name
	}
	notes := make([]exportNote, 0)
	if err := s.DB.WithContext(ctx).Table("tf_note n").
		Select("n.repo_id, n.path, n.title, c.content, n.tags, n.update_time").
		Joins("JOIN tf_note_content c ON c.note_id = n.id").
		Where("n.user_id = ? AND n.is_deleted = 0", userID).
		Order("n.repo_id ASC, n.path ASC").Scan(&notes).Error; err != nil {
		return err
	}
	for _, n := range notes {
		f, err := zw.Create(root + "/" + repoNames[n.RepoID] + "/notes/" + n.Path)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(n.Content)); err != nil {
			return err
		}
	}

	// 2) 待办 / 便签：JSON（全字段，可再导入）
	todos := make([]exportTodo, 0)
	if err := s.DB.WithContext(ctx).Table("tf_todo").
		Select("id, title, remark, status, due_time, tags, completed_at, create_time, update_time").
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("id ASC").Scan(&todos).Error; err != nil {
		return err
	}
	memos := make([]exportMemo, 0)
	if err := s.DB.WithContext(ctx).Table("tf_memo").
		Select("id, content, tags, is_pinned, create_time, update_time").
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("id ASC").Scan(&memos).Error; err != nil {
		return err
	}
	if err := writeJSON(zw, root+"/todos.json", todos); err != nil {
		return err
	}
	if err := writeJSON(zw, root+"/memos.json", memos); err != nil {
		return err
	}

	// 3) README
	f, err := zw.Create(root + "/README.txt")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(f, `my-extbrain 数据导出
=====================
导出时间: %s
内容说明:
	  <仓库名>/notes/  知识库原始 Markdown（按仓库分目录，目录结构 = 仓库内笔记路径）
  todos.json  待办（全字段）
  memos.json  便签（全字段）
说明: 本导出只含内容数据；API 密钥、分享链接、操作日志不在其中。
`, now); err != nil {
		return err
	}
	return zw.Close()
}

func writeJSON(zw *zip.Writer, name string, v any) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	return err
}
