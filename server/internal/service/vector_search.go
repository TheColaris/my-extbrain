package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// VectorSearcher 查询侧：query 向量化 + pgvector ANN 检索（切片级命中，调用方映射回笔记）。
type VectorSearcher struct {
	DB        *gorm.DB
	Cfg       *SysConfig
	NewClient EmbedderFactory

	cache     vecCache
	probeOnce sync.Once
	gucOK     atomic.Bool // hnsw.iterative_scan GUC 是否存在（pgvector>=0.8）
}

func NewVectorSearcher(db *gorm.DB, cfg *SysConfig) *VectorSearcher {
	return &VectorSearcher{DB: db, Cfg: cfg, NewClient: DefaultEmbedderFactory}
}

// Enabled provider 已启用且配置齐备。
func (v *VectorSearcher) Enabled() bool {
	return v != nil && v.Cfg != nil && v.Cfg.Embedding().Ready()
}

// VectorHit 切片级命中（含笔记元信息，便于直接映射）
type VectorHit struct {
	NoteID    int64     `json:"note_id"`
	RepoID    int64     `json:"repo_id"`
	RepoName  string    `json:"repo_name"`
	Path      string    `json:"path"`
	Title     string    `json:"title"`
	ChunkText string    `json:"chunk_text"`
	SizeBytes int       `json:"size_bytes"`
	UpdatedAt time.Time `json:"updated_at"`
	// Similarity 余弦相似度（1-距离）；低于 semanticMinSimilarity 的命中在查询内丢弃
	Similarity float64 `json:"similarity"`
}

// semanticMinSimilarity 语义命中相似度地板：低于此值的切片视为不相关直接丢弃。
// 没有地板时 ANN 永远返回 top-k（哪怕是完全无关的笔记），会污染混合检索结果。
// 0.35 为 bge-m3 类中文模型的经验下限（相关≈0.45+ / 无关≈0.2~0.35），可按实测调整。
const semanticMinSimilarity = 0.35

// 查询向量缓存参数（Web 搜索 300ms 防抖=逐字发查询，重复词直接复用，省一跳 embedding API）
const (
	vecCacheTTL = 10 * time.Minute
	vecCacheMax = 512
)

type vecCacheEntry struct {
	vec []float32
	at  time.Time
}

// vecCache 查询向量缓存。key=查询词（向量只由文本决定，与用户无关）；
// 满则先清过期、仍满整体重置（只求有界，不追求 LRU 精度）。
type vecCache struct {
	mu sync.RWMutex
	m  map[string]vecCacheEntry
}

func (c *vecCache) get(k string) ([]float32, bool) {
	c.mu.RLock()
	e, ok := c.m[k]
	c.mu.RUnlock()
	if !ok || time.Since(e.at) > vecCacheTTL {
		return nil, false
	}
	return e.vec, true
}

func (c *vecCache) put(k string, v []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]vecCacheEntry{}
	}
	if len(c.m) >= vecCacheMax {
		for k2, e := range c.m {
			if time.Since(e.at) > vecCacheTTL {
				delete(c.m, k2)
			}
		}
		if len(c.m) >= vecCacheMax {
			c.m = map[string]vecCacheEntry{}
		}
	}
	c.m[k] = vecCacheEntry{vec: v, at: time.Now()}
}

// probeIterScan 惰性探测 hnsw.iterative_scan 是否可用（pgvector>=0.8 引入）。
// **按扩展版本判断，不查 pg_settings**——pgvector 的自定义 GUC 要模块在本连接加载后才注册，
// 连接池下探测查询可能落在没用过 vector 的连接上，误判后会被 sync.Once 永久缓存。
// 另外绝不能 try-and-ignore 直接 SET：GUC 不存在时 SET 报错会把事务打废（后续查询全失败）。
func (v *VectorSearcher) probeIterScan(ctx context.Context) bool {
	v.probeOnce.Do(func() {
		var ver string
		err := v.DB.WithContext(ctx).Raw(`SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&ver).Error
		v.gucOK.Store(err == nil && iterScanSupported(ver))
	})
	return v.gucOK.Load()
}

// iterScanSupported "0.8.2"→true / "0.7.4"→false（iterative_scan 引入于 0.8.0）
func iterScanSupported(ver string) bool {
	parts := strings.SplitN(strings.TrimSpace(ver), ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 0 || minor >= 8
}

// Search 向量检索 top-k（按 cosine 距离升序；同笔记可能多切片命中，由调用方去重）。
// 取 3×limit 候选后按相似度地板过滤（地板不改变排序，只裁剪尾部噪声）。
// 迭代扫描（iterative_scan）让带 user_id 过滤的 ANN 在大库下不丢召回——
// 过滤条件放在 HNSW 扫描之后时，普通模式会在候选不足时提前收手。
func (v *VectorSearcher) Search(ctx context.Context, userID int64, query string, limit int) ([]VectorHit, error) {
	if !v.Enabled() {
		return nil, nil
	}
	cfg := v.Cfg.Embedding()
	vec, ok := v.cache.get(query)
	if !ok {
		cl := v.NewClient(cfg)
		if cl == nil {
			return nil, errors.New("embedding 客户端未装配")
		}
		vecs, err := cl.Embed(ctx, []string{query})
		if err != nil {
			return nil, err
		}
		if len(vecs) != 1 || len(vecs[0]) == 0 {
			return nil, errors.New("查询向量化失败")
		}
		vec = vecs[0]
		v.cache.put(query, vec)
	}
	if got := len(vec); got != cfg.Dim {
		return nil, fmt.Errorf("模型输出 %d 维，与配置维度 %d 不一致", got, cfg.Dim)
	}
	candidates := limit * 3
	if candidates < 10 {
		candidates = 10
	}
	rows := make([]VectorHit, 0, candidates)
	// SET LOCAL 须与查询同连接同事务才生效（连接池下裸 SET 会落在别的连接上）
	err := v.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if v.probeIterScan(ctx) {
			// strict_order=保持距离严格序（RRF 排名与“最佳切片取首个”依赖顺序）
			_ = tx.Exec(`SET LOCAL hnsw.iterative_scan = 'strict_order'`).Error
		}
		return tx.Raw(`
		SELECT k.note_id, n.repo_id, r.name AS repo_name, n.path, n.title, k.chunk_text,
		       octet_length(c.content) AS size_bytes, n.update_time AS updated_at,
		       1 - (k.embedding <=> ?::vector) AS similarity
		FROM tf_note_chunk k
		JOIN tf_note n ON n.id = k.note_id AND n.is_deleted = 0
		JOIN tf_repo r ON r.id = n.repo_id
		JOIN tf_note_content c ON c.note_id = k.note_id
		WHERE k.user_id = ? AND k.embedding IS NOT NULL
		ORDER BY k.embedding <=> ?::vector
		LIMIT ?`, FormatVector(vec), userID, FormatVector(vec), candidates).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}
	out := make([]VectorHit, 0, len(rows))
	for _, r := range rows {
		if r.Similarity < semanticMinSimilarity {
			continue
		}
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// SemanticSnippet 语义命中摘录（去 Markdown 记号 + 单行化 + 截断）。
func SemanticSnippet(chunk string) string {
	return "…" + cut(stripMarkdown(strings.Join(strings.Fields(chunk), " ")), 140) + "…"
}

// stripMarkdown 轻度去记号（标题 # / 列表 - / 引用 > / 强调 * _ / 行内 code 反引号）
func stripMarkdown(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '#', '>', '`', '*', '_':
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
