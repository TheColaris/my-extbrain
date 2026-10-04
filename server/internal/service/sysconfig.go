package service

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 平台参数配置键（tp_system_config.config_key；枚举真源=本处常量，禁散落字符串）
const (
	CfgEmbeddingEnabled  = "embedding.enabled"   // "true"/"false"
	CfgEmbeddingBaseURL  = "embedding.base_url"  // OpenAI 兼容端点，如 https://api.siliconflow.cn/v1
	CfgEmbeddingAPIKey   = "embedding.api_key"   // 敏感：接口出参只回打码
	CfgEmbeddingModel    = "embedding.model"     // 如 BAAI/bge-m3
	CfgEmbeddingDim      = "embedding.dim"       // 向量维度（须与 tf_note_chunk.embedding 列一致）
	CfgEmbeddingLastTest = "embedding.last_test" // JSON：最近一次测试连接结果
	CfgRepoQuota         = "repo.quota"          // 每用户可创建的仓库数上限（不含默认仓库；<=0=不限制）
)

// 每用户仓库数默认上限（普通用户 3 个；可在 tp_system_config 配 repo.quota 调整；
// 值 <=0 或缺省解析失败 = 不限制。付费扩容为后续规划，代码与界面不展示）
const DefaultRepoQuota = 3

// 默认值（首次进入平台管理页时的表单预填；硅基流动 + bge-m3 为默认推荐）
const (
	DefaultEmbeddingBaseURL = "https://api.siliconflow.cn/v1"
	DefaultEmbeddingModel   = "BAAI/bge-m3"
	DefaultEmbeddingDim     = 1024
)

// SysConfig 平台参数配置（KV）：启动加载进内存，保存后热更新；读路径零 DB。
type SysConfig struct {
	db   *gorm.DB
	mu   sync.RWMutex
	vals map[string]string
}

func NewSysConfig(db *gorm.DB) *SysConfig {
	return &SysConfig{db: db, vals: map[string]string{}}
}

// Load 全量加载（启动时调用；表不存在等错误由调用方决定是否致命）。
func (s *SysConfig) Load(ctx context.Context) error {
	var rows []model.SystemConfig
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.ConfigKey] = r.ConfigValue
	}
	s.mu.Lock()
	s.vals = m
	s.mu.Unlock()
	return nil
}

func (s *SysConfig) Get(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vals[key]
}

// Set 批量 upsert 并热更新内存（secretKeys 中的键标记 is_secret=1）。
func (s *SysConfig) Set(ctx context.Context, kv map[string]string, secretKeys ...string) error {
	if len(kv) == 0 {
		return nil
	}
	secret := make(map[string]bool, len(secretKeys))
	for _, k := range secretKeys {
		secret[k] = true
	}
	for k, v := range kv {
		var isSecret int16
		if secret[k] {
			isSecret = 1
		}
		row := model.SystemConfig{ConfigKey: k, ConfigValue: v, IsSecret: isSecret}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "config_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"config_value", "is_secret", "update_time"}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	s.mu.Lock()
	for k, v := range kv {
		s.vals[k] = v
	}
	s.mu.Unlock()
	return nil
}

// EmbeddingConfig 当前生效的 embedding 配置（读内存）。
type EmbeddingConfig struct {
	Enabled bool
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
}

// Embedding 读取当前配置；未配置项回落到默认（Enabled 只认显式 true）。
func (s *SysConfig) Embedding() EmbeddingConfig {
	c := EmbeddingConfig{
		Enabled: s.Get(CfgEmbeddingEnabled) == "true",
		BaseURL: s.Get(CfgEmbeddingBaseURL),
		APIKey:  s.Get(CfgEmbeddingAPIKey),
		Model:   s.Get(CfgEmbeddingModel),
	}
	if c.BaseURL == "" {
		c.BaseURL = DefaultEmbeddingBaseURL
	}
	if c.Model == "" {
		c.Model = DefaultEmbeddingModel
	}
	c.Dim = DefaultEmbeddingDim
	if n, err := strconv.Atoi(strings.TrimSpace(s.Get(CfgEmbeddingDim))); err == nil && n > 0 {
		c.Dim = n
	}
	return c
}

// Ready provider 已启用且三要素齐备（缺失=搜索链降级纯关键词）。
func (c EmbeddingConfig) Ready() bool {
	return c.Enabled && c.BaseURL != "" && c.APIKey != "" && c.Model != ""
}

// RepoQuota 每用户仓库数上限（不含默认仓库）。
// 未配置/解析失败=默认上限；显式配置 <=0 = 不限制（返回 0 语义化表示无限）。
func (s *SysConfig) RepoQuota() int {
	raw := strings.TrimSpace(s.Get(CfgRepoQuota))
	if raw == "" {
		return DefaultRepoQuota
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return DefaultRepoQuota
	}
	if n <= 0 {
		return 0
	}
	return n
}

// MaskKey 密钥打码（保留前缀与尾 4 位）：sk-abcdef…3f2a → sk-ab••••••3f2a
func MaskKey(k string) string {
	if k == "" {
		return ""
	}
	r := []rune(k)
	if len(r) <= 8 {
		return strings.Repeat("•", len(r))
	}
	return string(r[:4]) + strings.Repeat("•", 6) + string(r[len(r)-4:])
}
