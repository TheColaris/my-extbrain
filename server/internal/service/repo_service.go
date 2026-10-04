package service

import (
	"context"
	"errors"
	"strings"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// RepoService —— 知识库仓库（一等实体，迁移 0012）：仓库 > 文件夹 > 笔记。
// 语义：每用户一个默认仓库（不可删除、不可取消默认，可改名）；
// 删除只删仓库行（软删），非空仓库禁删；裸路径寻址=默认仓库。
// 数量上限：每用户可创建仓库数（不含默认仓库）读平台配置 repo.quota，缺省 3，<=0=不限制。
type RepoService struct {
	DB  *gorm.DB
	Sys *SysConfig // 仓库数量配额（nil=用默认上限）
}

var ErrRepoNotFound = errors.New("repo not found")

const repoNameMax = 255

// RepoView 仓库列表视图（带笔记数；列表页/切换器用）。
type RepoView struct {
	model.Repo
	NoteCount int64 `json:"note_count"`
}

// List 活跃仓库（默认在前，其余按 sort/name）+ 各仓活跃笔记数。
func (s *RepoService) List(ctx context.Context, userID int64) ([]RepoView, error) {
	repos := make([]model.Repo, 0)
	if err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_deleted = 0", userID).
		Order("is_default DESC, sort ASC, id ASC").Find(&repos).Error; err != nil {
		return nil, err
	}
	out := make([]RepoView, 0, len(repos))
	if len(repos) == 0 {
		return out, nil
	}
	var counts []struct {
		RepoID int64
		Cnt    int64
	}
	if err := s.DB.WithContext(ctx).Model(&model.Note{}).
		Select("repo_id, COUNT(*) AS cnt").
		Where("user_id = ? AND is_deleted = 0", userID).
		Group("repo_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	byRepo := make(map[int64]int64, len(counts))
	for _, c := range counts {
		byRepo[c.RepoID] = c.Cnt
	}
	for _, r := range repos {
		out = append(out, RepoView{Repo: r, NoteCount: byRepo[r.ID]})
	}
	return out, nil
}

// DefaultID 默认仓库 id（缺则懒建——覆盖迁移后新注册用户与异常缺省两个口子）。
func (s *RepoService) DefaultID(ctx context.Context, userID int64) (int64, error) {
	var r model.Repo
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND is_default = 1 AND is_deleted = 0", userID).First(&r).Error
	if err == nil {
		return r.ID, nil
	}
	if err != gorm.ErrRecordNotFound {
		return 0, err
	}
	fresh := model.Repo{UserID: userID, Name: "默认仓库", Description: "开启仓库功能时自动创建；存量笔记都在这里", IsDefault: 1}
	if err := s.DB.WithContext(ctx).Create(&fresh).Error; err != nil {
		return 0, err
	}
	return fresh.ID, nil
}

// resolve 校验归属并取活跃仓库。
func (s *RepoService) resolve(ctx context.Context, userID, id int64) (*model.Repo, error) {
	var r model.Repo
	err := s.DB.WithContext(ctx).
		Where("id = ? AND user_id = ? AND is_deleted = 0", id, userID).First(&r).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrRepoNotFound
	}
	return &r, err
}

// ResolveByName 按名取活跃仓库（notes ?repo= / MCP repo 字段 / 跨仓库寻址共用）。
func (s *RepoService) ResolveByName(ctx context.Context, userID int64, name string) (*model.Repo, error) {
	var r model.Repo
	err := s.DB.WithContext(ctx).
		Where("user_id = ? AND name = ? AND is_deleted = 0", userID, name).First(&r).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrRepoNotFound
	}
	return &r, err
}

// nameTaken 活跃仓库名是否已占用（软删行走部分唯一索引天然释放名字）。
func (s *RepoService) nameTaken(ctx context.Context, userID int64, name string, excludeID int64) (bool, error) {
	q := s.DB.WithContext(ctx).Model(&model.Repo{}).
		Where("user_id = ? AND name = ? AND is_deleted = 0", userID, name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := q.Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func validateRepoName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &UserError{"仓库名不能为空"}
	}
	if len([]rune(name)) > repoNameMax {
		return &UserError{"仓库名过长（≤255 字符）"}
	}
	if strings.Contains(name, ":") {
		return &UserError{"仓库名不能包含冒号（: 是跨仓库寻址的分隔符）"}
	}
	// 禁路径分隔符与 ..：仓库名会拼进导出 zip 条目名，防导出侧路径穿越
	if strings.ContainsAny(name, "/\\") || name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
		return &UserError{"仓库名不能包含 / 或 \\"}
	}
	return nil
}

// Create 新建仓库（校验名称唯一 + 数量配额）。
func (s *RepoService) Create(ctx context.Context, userID int64, name, description string) (*model.Repo, error) {
	name = strings.TrimSpace(name)
	if err := validateRepoName(name); err != nil {
		return nil, err
	}
	if len(description) > 500 {
		return nil, &UserError{"描述过长（≤500 字符）"}
	}
	if taken, err := s.nameTaken(ctx, userID, name, 0); err != nil {
		return nil, err
	} else if taken {
		return nil, &UserError{"仓库名「" + name + "」已存在，换一个名字"}
	}
	quota := DefaultRepoQuota
	if s.Sys != nil {
		quota = s.Sys.RepoQuota()
	}
	if quota > 0 {
		var cnt int64
		if err := s.DB.WithContext(ctx).Model(&model.Repo{}).
			Where("user_id = ? AND is_default = 0 AND is_deleted = 0", userID).Count(&cnt).Error; err != nil {
			return nil, err
		}
		if cnt >= int64(quota) {
			return nil, &UserError{"仓库数量已达上限（" + itoa(int64(quota)) + " 个）。可删除不用的仓库后重试"}
		}
	}
	r := model.Repo{UserID: userID, Name: name, Description: strings.TrimSpace(description)}
	if err := s.DB.WithContext(ctx).Create(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// Update 改名 / 改描述（默认仓库可改名；只改仓库名，仓库内笔记不受影响）。
func (s *RepoService) Update(ctx context.Context, userID, id int64, name, description *string) (*model.Repo, error) {
	r, err := s.resolve(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if name != nil {
		newName := strings.TrimSpace(*name)
		if err := validateRepoName(newName); err != nil {
			return nil, err
		}
		if taken, err := s.nameTaken(ctx, userID, newName, id); err != nil {
			return nil, err
		} else if taken {
			return nil, &UserError{"仓库名「" + newName + "」已存在，换一个名字"}
		}
		updates["name"] = newName
	}
	if description != nil {
		if len(*description) > 500 {
			return nil, &UserError{"描述过长（≤500 字符）"}
		}
		updates["description"] = strings.TrimSpace(*description)
	}
	if len(updates) == 0 {
		return r, nil
	}
	if err := s.DB.WithContext(ctx).Model(&model.Repo{}).Where("id = ?", r.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.resolve(ctx, userID, id)
}

// Delete 删除仓库（软删；默认仓库不可删；非空仓库禁删——先移走或清空笔记）。
func (s *RepoService) Delete(ctx context.Context, userID, id int64) error {
	r, err := s.resolve(ctx, userID, id)
	if err != nil {
		return err
	}
	if r.IsDefault == 1 {
		return &UserError{"「" + r.Name + "」是默认仓库，不可删除"}
	}
	var cnt int64
	if err := s.DB.WithContext(ctx).Model(&model.Note{}).
		Where("user_id = ? AND repo_id = ? AND is_deleted = 0", userID, r.ID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return &UserError{"「" + r.Name + "」还有 " + itoa(cnt) + " 篇笔记：先移走或清空，再删除仓库"}
	}
	return s.DB.WithContext(ctx).Model(&model.Repo{}).
		Where("id = ? AND is_deleted = 0", r.ID).Update("is_deleted", 1).Error
}

// SplitCrossRepo 识别「仓库名:路径」形态（跨仓库寻址）：前缀命中该用户活跃仓库名即跨仓库。
// 返回 ok=false 表示不是跨仓库形态（整串按当前仓库内路径处理）。
func (s *RepoService) SplitCrossRepo(ctx context.Context, userID int64, raw string) (repoID int64, rest string, ok bool, err error) {
	i := strings.IndexByte(raw, ':')
	if i <= 0 {
		return 0, raw, false, nil
	}
	r, e := s.ResolveByName(ctx, userID, raw[:i])
	if e != nil {
		if errors.Is(e, ErrRepoNotFound) {
			return 0, raw, false, nil // 前缀不是仓库名 → 非跨仓库形态（路径本身含冒号）
		}
		return 0, "", false, e
	}
	return r.ID, raw[i+1:], true, nil
}
