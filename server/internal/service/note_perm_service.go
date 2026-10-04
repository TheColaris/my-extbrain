package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"extbrain-server/internal/model"

	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NotePermService —— 知识库目录权限（AI Key 可见性白名单；迁移 0012 起挂仓库）。
// 语义：未配置=开放；白名单制；读写同权（看不见也写不进）；
// 最深前缀规则优先（仓库规则 folder_path=” = 本仓库默认，子目录可再收窄/继承）；
// 只约束 AI（Key），Web JWT 不受限。规则跨仓库隔离：A 仓库的规则不影响 B 仓库。
type NotePermService struct{ DB *gorm.DB }

const (
	PermModeOpen  = "open"  // 不限（不落行；PUT open=删除规则）
	PermModeAllow = "allow" // 白名单：仅 key_ids 内的 Key 可见（空列表=对 AI 完全封闭）
)

// PermView 一次请求的可见性判定视图（nil=不受限）。加载该用户全部规则后内存匹配。
type PermView struct {
	keyID int64
	rules []model.NotePerm
}

// View 解析请求视图：API Key → 加载该用户全部规则；Web JWT（keyID=0）→ nil 不受限。
func (s *NotePermService) View(ctx context.Context, userID, keyID int64) (*PermView, error) {
	if keyID == 0 {
		return nil, nil
	}
	var rules []model.NotePerm
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("folder_path ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return &PermView{keyID: keyID, rules: rules}, nil
}

// Allowed 路径（仓库内相对路径）对该 Key 是否可见：取祖先链上最深已配置规则（含
// 仓库级规则 folder_path=”）；无=开放。规则按仓库隔离（repoID 精确匹配）。
// 非法路径一律拒绝（不可见）。
func (v *PermView) Allowed(repoID int64, rawPath string) bool {
	if v == nil {
		return true
	}
	p, err := CleanPath(rawPath)
	if err != nil {
		return false
	}
	segs := strings.Split(p, "/")
	allowed, depth := true, -1
	for i := 0; i <= len(segs); i++ {
		prefix := strings.Join(segs[:i], "/") // i=0 → ''（仓库级）
		for _, r := range v.rules {
			if r.RepoID == repoID && r.Mode == PermModeAllow && r.FolderPath == prefix && i > depth {
				depth = i
				allowed = permAllowsID(r.KeyIDs, v.keyID)
			}
		}
	}
	return allowed
}

func permAllowsID(ids pq.StringArray, id int64) bool {
	s := strconv.FormatInt(id, 10)
	for _, x := range ids {
		if x == s {
			return true
		}
	}
	return false
}

// List 该用户全部规则（树锁标记/弹窗预填用；含 repo_id，前端按当前仓库过滤）。
func (s *NotePermService) List(ctx context.Context, userID int64) ([]model.NotePerm, error) {
	rules := make([]model.NotePerm, 0)
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("folder_path ASC").
		Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

// NotePermInput PUT /note-perm 入参。mode=open 删除规则（回到开放/继承）；
// mode=allow 允许 key_ids 为空=对 AI 完全封闭。RepoID 必填（仓库级规则 folder_path=”）。
type NotePermInput struct {
	RepoID     int64   `json:"repo_id"`
	FolderPath string  `json:"folder_path"` // '' = 仓库级默认规则
	Mode       string  `json:"mode"`
	KeyIDs     []int64 `json:"key_ids"`
}

// Put 保存目录/仓库规则（upsert；Web JWT 专属——AI 不可自改权限）。
func (s *NotePermService) Put(ctx context.Context, userID int64, in NotePermInput) error {
	if in.RepoID <= 0 {
		return &UserError{"repo_id 必填（规则归属仓库）"}
	}
	var cnt int64
	if err := s.DB.WithContext(ctx).Model(&model.Repo{}).
		Where("id = ? AND user_id = ? AND is_deleted = 0", in.RepoID, userID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt == 0 {
		return &UserError{"仓库不存在（规则归属仓库须为本人的活跃仓库）"}
	}
	folder := ""
	if strings.TrimSpace(in.FolderPath) != "" {
		p, err := CleanPath(in.FolderPath)
		if err != nil {
			return err
		}
		folder = p
	}
	if in.Mode != PermModeOpen && in.Mode != PermModeAllow {
		return &UserError{"mode 必须是 open（不限）/ allow（白名单）"}
	}

	if in.Mode == PermModeOpen {
		// 删除规则=回到开放（或继承最近已配置祖先）；硬删——软删行会占用 uk
		return s.DB.WithContext(ctx).
			Where("user_id = ? AND repo_id = ? AND folder_path = ?", userID, in.RepoID, folder).
			Delete(&model.NotePerm{}).Error
	}

	// 去重 + 校验 Key 归属（本人的、未吊销的）
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(in.KeyIDs))
	for _, id := range in.KeyIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		var cnt int64
		if err := s.DB.WithContext(ctx).Model(&model.APIKey{}).
			Where("user_id = ? AND is_revoked = 0 AND is_deleted = 0 AND id IN ?", userID, ids).
			Count(&cnt).Error; err != nil {
			return err
		}
		if int(cnt) != len(ids) {
			return &UserError{"勾选的 Key 不存在或已吊销，请刷新后重试"}
		}
	}
	idStrs := make(pq.StringArray, 0, len(ids))
	for _, id := range ids {
		idStrs = append(idStrs, strconv.FormatInt(id, 10))
	}

	row := model.NotePerm{UserID: userID, RepoID: in.RepoID, FolderPath: folder, Mode: PermModeAllow, KeyIDs: idStrs}
	return s.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "repo_id"}, {Name: "folder_path"}},
		DoUpdates: clause.Assignments(map[string]any{
			"mode": PermModeAllow, "key_ids": idStrs, "update_time": time.Now(),
		}),
	}).Create(&row).Error
}
