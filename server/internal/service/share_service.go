package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// ShareService —— 笔记分享：一篇笔记一个活跃分享（部分唯一索引兜底），token 公开只读。
// snapshot=快照冻结（撤销后内容仍在分享行内，不再可读）；live=跟随原笔记（删除即失效）。
type ShareService struct {
	DB    *gorm.DB
	Notes *NoteService
}

var (
	ErrShareNotFound      = errors.New("share not found")
	ErrShareExpired       = errors.New("share expired")
	ErrShareSourceDeleted = errors.New("share source note deleted")
)

const shareExpireDaysMax = 3650 // 10 年上限（防滥用溢出；永久用 0 表达）

// ShareUpsert 创建/更新分享：已有活跃分享则保留 token 更新设置并重冻快照，无则新建。
func (s *ShareService) Upsert(ctx context.Context, userID, repoID int64, rawPath string, mode model.ShareMode, expireDays int, view *PermView) (*model.NoteShare, error) {
	if !mode.Valid() {
		return nil, &UserError{"mode 只能是 snapshot 或 live"}
	}
	if expireDays < 0 || expireDays > shareExpireDaysMax {
		return nil, &UserError{"expire_days 取值 0（永久）~ 3650"}
	}
	full, err := s.Notes.Get(ctx, userID, repoID, rawPath, view)
	if err != nil {
		return nil, err // 笔记不存在（ErrNoteNotFound）/ 目录权限拒绝（ErrNoteDenied）或路径非法
	}
	var expireAt *time.Time
	if expireDays > 0 {
		t := time.Now().AddDate(0, 0, expireDays)
		expireAt = &t
	}

	var share model.NoteShare
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND note_id = ? AND is_revoked = 0", userID, full.ID).First(&share).Error
		if err == gorm.ErrRecordNotFound {
			tok, terr := newShareToken()
			if terr != nil {
				return terr
			}
			share = model.NoteShare{
				UserID: userID, NoteID: full.ID, Token: tok,
				Title: full.Title, Content: full.Content, ContentHash: full.ContentHash,
				Tags: full.Tags, Mode: mode, ExpireTime: expireAt,
			}
			return tx.Create(&share).Error
		}
		if err != nil {
			return err
		}
		// 已有活跃分享：保留 token，重冻快照 + 更新设置（update_time 由 autoUpdateTime 维护）
		return tx.Model(&model.NoteShare{}).Where("id = ?", share.ID).Updates(map[string]any{
			"title": full.Title, "content": full.Content, "content_hash": full.ContentHash,
			"tags": full.Tags, "mode": mode, "expire_time": expireAt,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	// 回读返回完整行（Updates map 不回填 struct；须用零值变量——复用旧 struct 时 NULL 列不会重置已有指针字段）
	var fresh model.NoteShare
	if err := s.DB.WithContext(ctx).First(&fresh, share.ID).Error; err != nil {
		return nil, err
	}
	return &fresh, nil
}

// GetByPath owner 视角：该笔记的活跃分享（无则 nil，不报错）。path 为 repoID 仓库内相对路径。
func (s *ShareService) GetByPath(ctx context.Context, userID, repoID int64, rawPath string) (*model.NoteShare, error) {
	p, err := CleanPath(rawPath)
	if err != nil {
		return nil, err
	}
	var note model.Note
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND repo_id = ? AND path = ? AND is_deleted = 0", userID, repoID, p).First(&note).Error; err != nil {
		return nil, ErrNoteNotFound
	}
	var share model.NoteShare
	err = s.DB.WithContext(ctx).
		Where("user_id = ? AND note_id = ? AND is_revoked = 0", userID, note.ID).First(&share).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &share, nil
}

// Revoke 停止分享（终态；行保留）。
func (s *ShareService) Revoke(ctx context.Context, userID int64, token string) error {
	res := s.DB.WithContext(ctx).Model(&model.NoteShare{}).
		Where("user_id = ? AND token = ? AND is_revoked = 0", userID, token).
		Updates(map[string]any{"is_revoked": 1})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrShareNotFound
	}
	return nil
}

// ShareView 公开读取视图（不含快照冗余列与归属信息）
type ShareView struct {
	Title      string          `json:"title"`
	Content    string          `json:"content"`
	Tags       []string        `json:"tags"`
	Mode       model.ShareMode `json:"mode"`
	CreateTime time.Time       `json:"create_time"`
	UpdateTime time.Time       `json:"update_time"`
	ExpireTime *time.Time      `json:"expire_time,omitempty"`
}

// loadActive 取有效分享行：不存在/已撤销→ErrShareNotFound、已过期→ErrShareExpired。
func (s *ShareService) loadActive(ctx context.Context, token string) (*model.NoteShare, error) {
	var share model.NoteShare
	err := s.DB.WithContext(ctx).
		Where("token = ? AND is_revoked = 0", token).First(&share).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrShareNotFound
	}
	if err != nil {
		return nil, err
	}
	if share.ExpireTime != nil && time.Now().After(*share.ExpireTime) {
		return nil, ErrShareExpired
	}
	return &share, nil
}

// resolveContent 按 mode 解析分享内容：snapshot=行内快照；live=实时读原笔记（已删→ErrShareSourceDeleted）。
func (s *ShareService) resolveContent(ctx context.Context, share *model.NoteShare) (title, content string, tags []string, updated time.Time, err error) {
	if share.Mode != model.ShareModeLive {
		return share.Title, share.Content, share.Tags, share.UpdateTime, nil
	}
	var note model.Note
	if err := s.DB.WithContext(ctx).
		Where("id = ? AND is_deleted = 0", share.NoteID).First(&note).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", nil, time.Time{}, ErrShareSourceDeleted
		}
		return "", "", nil, time.Time{}, err
	}
	var c model.NoteContent
	if err := s.DB.WithContext(ctx).First(&c, "note_id = ?", note.ID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", nil, time.Time{}, ErrShareSourceDeleted
		}
		return "", "", nil, time.Time{}, err
	}
	return note.Title, c.Content, note.Tags, note.UpdateTime, nil
}

// PublicGet 公开读取（游客）：校验有效性 → 按 mode 取内容 → 原子计数。
// 失效语义三分：不存在/已撤销→ErrShareNotFound、已过期→ErrShareExpired、live 源笔记已删→ErrShareSourceDeleted。
func (s *ShareService) PublicGet(ctx context.Context, token string) (*ShareView, error) {
	share, err := s.loadActive(ctx, token)
	if err != nil {
		return nil, err
	}
	title, content, tags, updated, err := s.resolveContent(ctx, share)
	if err != nil {
		return nil, err
	}
	// 浏览计数：读取成功才 +1（原子自增，不阻塞响应语义）
	s.DB.WithContext(ctx).Model(&model.NoteShare{}).Where("id = ?", share.ID).
		UpdateColumn("view_count", gorm.Expr("view_count + 1"))
	return &ShareView{
		Title: title, Content: content, Tags: tags,
		Mode: share.Mode, CreateTime: share.CreateTime, UpdateTime: updated,
		ExpireTime: share.ExpireTime,
	}, nil
}

// CopyToLibrary 把有效分享复制到指定用户的默认仓库根目录（复制=新建，不覆盖已有内容）。
// 路径=标题.md；同名（含软删占位）自动追加 -2/-3…；返回落库后的新笔记。
func (s *ShareService) CopyToLibrary(ctx context.Context, userID, repoID int64, token string, view *PermView) (*model.Note, error) {
	share, err := s.loadActive(ctx, token)
	if err != nil {
		return nil, err
	}
	title, content, tags, _, err := s.resolveContent(ctx, share)
	if err != nil {
		return nil, err
	}
	base := copyPathBase(title)
	if !view.Allowed(repoID, base+".md") { // 复制目标在默认仓库根目录下（读写同权；候选路径全在根，查一次即覆盖）
		return nil, ErrNoteDenied
	}
	for i := 0; i < 20; i++ {
		p := base + ".md"
		if i > 0 {
			p = fmt.Sprintf("%s-%d.md", base, i+1)
		}
		note, err := s.createNoteOnce(ctx, userID, repoID, p, title, content, tags)
		if err == nil {
			return note, nil
		}
		if !isUniquePathViolation(err) {
			return nil, err
		}
	}
	return nil, &UserError{"同名笔记过多，请先整理知识库"}
}

// createNoteOnce 单次尝试建笔记（撞唯一索引由调用方换名重试）。
func (s *ShareService) createNoteOnce(ctx context.Context, userID, repoID int64, path, title, content string, tags []string) (*model.Note, error) {
	if title == "" {
		title = path
	}
	note := model.Note{UserID: userID, RepoID: repoID, Path: path, Title: title, Tags: normalizeTags(tags)}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&note).Error; err != nil {
			return err
		}
		return tx.Create(&model.NoteContent{NoteID: note.ID, Content: content, ContentHash: hashHex(content)}).Error
	})
	if err != nil {
		return nil, err
	}
	return &note, nil
}

// copyPathBase 标题 → 路径基名：去斜杠（防建子目录）、剥尾部 .md（笔记 title 惯例含扩展名，
// 直接拼会出 xxx.md.md）、首尾空白，空标题兜底。
func copyPathBase(title string) string {
	b := strings.TrimSpace(strings.ReplaceAll(title, "/", "-"))
	if len(b) >= 3 && strings.EqualFold(b[len(b)-3:], ".md") {
		b = strings.TrimSpace(b[:len(b)-3])
	}
	if b == "" {
		b = "未命名笔记"
	}
	return b
}

// isUniquePathViolation 是否撞 uk_note_repo_path 唯一索引（含软删占位行）。
func isUniquePathViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "uk_note_repo_path")
}

// newShareToken 32 字符 hex（16 字节 crypto/rand）
func newShareToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
