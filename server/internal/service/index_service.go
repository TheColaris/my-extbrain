package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"extbrain-server/internal/embed"
	"extbrain-server/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Embedder 向量化接口（生产 = embed.Client；测试注入 fake）。
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// EmbedderFactory 按当前平台配置构造 Embedder（保存后热更新=每次现取）。
type EmbedderFactory func(cfg EmbeddingConfig) Embedder

// DefaultEmbedderFactory 生产默认：OpenAI 兼容客户端。
func DefaultEmbedderFactory(cfg EmbeddingConfig) Embedder {
	return embed.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
}

// FormatVector []float32 → pgvector 文本字面量 [v1,v2,...]（float32 最短往返表示）。
func FormatVector(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

const (
	indexQueueSize  = 512
	indexScanBatch  = 200 // 单次懒补偿最多补算笔记数
	indexScanEvery  = 15 * time.Minute
	indexFailedKeep = 5 // 状态里回显的失败明细条数
)

// ReindexProgress 全量重建进度
type ReindexProgress struct {
	Running bool  `json:"running"`
	Done    int64 `json:"done"`
	Total   int64 `json:"total"`
	Failed  int64 `json:"failed"`
}

// FailedNote 失败明细（状态页回显）
type FailedNote struct {
	NoteID int64  `json:"note_id"`
	Path   string `json:"path"`
	Error  string `json:"error"`
}

// IndexStatus 索引状态（平台管理页数字带数据源）
type IndexStatus struct {
	Enabled     bool            `json:"enabled"`
	Notes       int64           `json:"notes"`   // 已索引笔记数（distinct，仅未删）
	Chunks      int64           `json:"chunks"`  // 切片总数
	Pending     int64           `json:"pending"` // 待索引（hash 不一致或无切片）
	Failed      int64           `json:"failed"`
	FailedItems []FailedNote    `json:"failed_items"`
	LastIndexAt *time.Time      `json:"last_index_at"`
	IndexDim    int             `json:"index_dim"`  // 库内列维度
	ConfigDim   int             `json:"config_dim"` // 平台配置维度
	Progress    ReindexProgress `json:"progress"`
}

// IndexService —— 向量索引：写入侧异步重建（channel + worker，复用 audit 模式）+
// 启动/周期懒补偿 + 全量重建（平台管理页触发）。失败只记日志，绝不阻塞保存。
type IndexService struct {
	DB        *gorm.DB
	Cfg       *SysConfig
	NewClient EmbedderFactory

	queue   chan int64
	mu      sync.Mutex
	pending map[int64]bool
	failed  map[int64]string
	prog    ReindexProgress
	started bool
}

func NewIndexService(db *gorm.DB, cfg *SysConfig) *IndexService {
	return &IndexService{
		DB: db, Cfg: cfg, NewClient: DefaultEmbedderFactory,
		queue:   make(chan int64, indexQueueSize),
		pending: map[int64]bool{},
		failed:  map[int64]string{},
	}
}

// Enabled provider 已启用且配置齐备（缺=索引与向量检索整体降级）。
func (s *IndexService) Enabled() bool {
	return s != nil && s.Cfg != nil && s.Cfg.Embedding().Ready()
}

// Start 启动 worker：队列消费 + 周期懒补偿（启动即扫一次）。
func (s *IndexService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go func() {
		t := time.NewTicker(indexScanEvery)
		defer t.Stop()
		s.lazyScan(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-s.queue:
				s.mu.Lock()
				delete(s.pending, id)
				s.mu.Unlock()
				s.process(ctx, id, false)
			case <-t.C:
				s.lazyScan(ctx)
			}
		}
	}()
}

// Enqueue 投递重建（非阻塞；队列满则丢弃，懒补偿兜底）。
func (s *IndexService) Enqueue(noteID int64) {
	if !s.Enabled() || noteID <= 0 {
		return
	}
	s.mu.Lock()
	if s.pending[noteID] {
		s.mu.Unlock()
		return
	}
	s.pending[noteID] = true
	s.mu.Unlock()
	select {
	case s.queue <- noteID:
	default:
		s.mu.Lock()
		delete(s.pending, noteID)
		s.mu.Unlock()
	}
}

func (s *IndexService) process(ctx context.Context, noteID int64, force bool) {
	if err := s.indexNote(ctx, noteID, force); err != nil {
		s.mu.Lock()
		s.failed[noteID] = err.Error()
		s.mu.Unlock()
		gin.DefaultErrorWriter.Write([]byte("[index] 笔记 " + itoa(noteID) + " 索引失败: " + err.Error() + "\n"))
		return
	}
	s.mu.Lock()
	delete(s.failed, noteID)
	s.mu.Unlock()
}

// indexNote 重建单篇：hash 一致（非 force）跳过；事务内 delete+insert 全量替换。
func (s *IndexService) indexNote(ctx context.Context, noteID int64, force bool) error {
	var n model.Note
	if err := s.DB.WithContext(ctx).First(&n, "id = ?", noteID).Error; err != nil {
		return err
	}
	if n.IsDeleted == 1 { // 软删：清切片（恢复时懒补偿重建）
		return s.dropChunks(ctx, noteID)
	}
	var c model.NoteContent
	if err := s.DB.WithContext(ctx).First(&c, "note_id = ?", noteID).Error; err != nil {
		return err
	}
	if !force {
		var cnt int64
		if err := s.DB.WithContext(ctx).Model(&model.NoteChunk{}).
			Where("note_id = ? AND content_hash = ?", noteID, c.ContentHash).
			Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return nil // 内容未变，跳过
		}
	}
	chunks := ChunkMarkdown(c.Content)
	if len(chunks) == 0 {
		return s.dropChunks(ctx, noteID)
	}
	cfg := s.Cfg.Embedding()
	if !cfg.Ready() {
		return errors.New("语义检索未启用或配置不完整")
	}
	cl := s.NewClient(cfg)
	if cl == nil {
		return errors.New("embedding 客户端未装配")
	}
	inputs := make([]string, len(chunks))
	for i, ch := range chunks {
		inputs[i] = n.Title + "\n" + ch
	}
	vecs, err := cl.Embed(ctx, inputs)
	if err != nil {
		return err
	}
	if len(vecs) != len(chunks) {
		return fmt.Errorf("向量数量（%d）与切片数（%d）不一致", len(vecs), len(chunks))
	}
	if got := len(vecs[0]); got != cfg.Dim {
		return fmt.Errorf("模型输出 %d 维，与配置维度 %d 不一致", got, cfg.Dim)
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("note_id = ?", noteID).Delete(&model.NoteChunk{}).Error; err != nil {
			return err
		}
		for i, ch := range chunks {
			if err := tx.Exec(
				`INSERT INTO tf_note_chunk (note_id, user_id, chunk_seq, chunk_text, content_hash, embedding)
				 VALUES (?, ?, ?, ?, ?, ?::vector)`,
				noteID, n.UserID, i, ch, c.ContentHash, FormatVector(vecs[i]),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *IndexService) dropChunks(ctx context.Context, noteID int64) error {
	return s.DB.WithContext(ctx).Where("note_id = ?", noteID).Delete(&model.NoteChunk{}).Error
}

// Kick 立即跑一次懒补偿（平台配置保存后调用；异步，不阻塞请求）。
func (s *IndexService) Kick(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	go s.lazyScan(ctx)
}

// lazyScan 懒补偿：把「hash 不一致 / 无切片」的笔记投递重建（有界批次）。
func (s *IndexService) lazyScan(ctx context.Context) {
	if !s.Enabled() {
		return
	}
	var ids []int64
	err := s.DB.WithContext(ctx).Raw(`
		SELECT n.id FROM tf_note n
		JOIN tf_note_content c ON c.note_id = n.id
		WHERE n.is_deleted = 0
		  AND NOT EXISTS (SELECT 1 FROM tf_note_chunk k WHERE k.note_id = n.id AND k.content_hash = c.content_hash)
		ORDER BY n.update_time DESC
		LIMIT ?`, indexScanBatch).Scan(&ids).Error
	if err != nil {
		gin.DefaultErrorWriter.Write([]byte("[index] 懒补偿扫描失败: " + err.Error() + "\n"))
		return
	}
	for _, id := range ids {
		s.Enqueue(id)
	}
	if len(ids) > 0 {
		log.Printf("[index] 懒补偿投递 %d 篇", len(ids))
	}
}

// ReindexAll 全量重建（异步）：维度对齐（必要时 TRUNCATE + ALTER）+ 忽略 hash 强制重算。
func (s *IndexService) ReindexAll(ctx context.Context) error {
	if !s.Enabled() {
		return &UserError{"语义检索未启用，请先在平台管理页完成配置"}
	}
	cfg := s.Cfg.Embedding()
	if cfg.Dim < 64 || cfg.Dim > 8192 {
		return &UserError{fmt.Sprintf("向量维度 %d 超出支持范围（64~8192）", cfg.Dim)}
	}
	s.mu.Lock()
	if s.prog.Running {
		s.mu.Unlock()
		return &UserError{"重建已在进行中"}
	}
	s.prog = ReindexProgress{Running: true}
	s.mu.Unlock()

	go func() {
		ctx := context.Background()
		defer func() {
			s.mu.Lock()
			s.prog.Running = false
			s.mu.Unlock()
		}()
		if err := s.ensureDim(ctx, cfg.Dim); err != nil {
			gin.DefaultErrorWriter.Write([]byte("[index] 维度对齐失败: " + err.Error() + "\n"))
			s.mu.Lock()
			s.prog.Failed++
			s.mu.Unlock()
			return
		}
		var ids []int64
		if err := s.DB.WithContext(ctx).Model(&model.Note{}).Where("is_deleted = 0").Order("id ASC").Pluck("id", &ids).Error; err != nil {
			gin.DefaultErrorWriter.Write([]byte("[index] 重建取列表失败: " + err.Error() + "\n"))
			return
		}
		s.mu.Lock()
		s.prog.Total = int64(len(ids))
		s.mu.Unlock()
		for _, id := range ids {
			if err := s.indexNote(ctx, id, true); err != nil {
				s.mu.Lock()
				s.failed[id] = err.Error()
				s.prog.Failed++
				s.mu.Unlock()
			} else {
				s.mu.Lock()
				delete(s.failed, id)
				s.mu.Unlock()
			}
			s.mu.Lock()
			s.prog.Done++
			s.mu.Unlock()
		}
		log.Printf("[index] 全量重建完成：%d 篇（失败 %d）", len(ids), s.prog.Failed)
	}()
	return nil
}

// ensureDim 列维度与配置不一致时对齐：清空 + ALTER COLUMN TYPE（空表瞬时，HNSW 索引随之重建）。
func (s *IndexService) ensureDim(ctx context.Context, dim int) error {
	cur, err := s.indexDim(ctx)
	if err != nil {
		return err
	}
	if cur == dim || cur == 0 {
		return nil
	}
	if err := s.DB.WithContext(ctx).Exec("TRUNCATE tf_note_chunk").Error; err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Exec(
		fmt.Sprintf("ALTER TABLE tf_note_chunk ALTER COLUMN embedding TYPE vector(%d)", dim)).Error
}

// indexDim 读列维度（pg_attribute.atttypmod；vector(N) 的 typmod=N；0=列不存在）。
func (s *IndexService) indexDim(ctx context.Context) (int, error) {
	var mods []int
	if err := s.DB.WithContext(ctx).Raw(
		`SELECT atttypmod FROM pg_attribute WHERE attrelid = 'tf_note_chunk'::regclass AND attname = 'embedding'`).
		Scan(&mods).Error; err != nil {
		return 0, err
	}
	if len(mods) == 0 || mods[0] < 0 {
		return 0, nil
	}
	return mods[0], nil
}

// Status 索引状态（平台管理页）。
func (s *IndexService) Status(ctx context.Context) (*IndexStatus, error) {
	st := &IndexStatus{Enabled: s.Enabled(), ConfigDim: s.Cfg.Embedding().Dim}
	if dim, err := s.indexDim(ctx); err == nil {
		st.IndexDim = dim
	}
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT count(DISTINCT k.note_id) FROM tf_note_chunk k
		JOIN tf_note n ON n.id = k.note_id AND n.is_deleted = 0`).Scan(&st.Notes).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&model.NoteChunk{}).Count(&st.Chunks).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT count(*) FROM tf_note n
		JOIN tf_note_content c ON c.note_id = n.id
		WHERE n.is_deleted = 0
		  AND NOT EXISTS (SELECT 1 FROM tf_note_chunk k WHERE k.note_id = n.id AND k.content_hash = c.content_hash)`).
		Scan(&st.Pending).Error; err != nil {
		return nil, err
	}
	var last []time.Time
	if err := s.DB.WithContext(ctx).Model(&model.NoteChunk{}).Select("max(update_time)").Scan(&last).Error; err == nil && len(last) > 0 && !last[0].IsZero() {
		st.LastIndexAt = &last[0]
	}
	s.mu.Lock()
	st.Progress = s.prog
	st.Failed = int64(len(s.failed))
	if len(s.failed) > 0 {
		items := make([]FailedNote, 0, indexFailedKeep)
		for id, msg := range s.failed {
			if len(items) >= indexFailedKeep {
				break
			}
			items = append(items, FailedNote{NoteID: id, Error: msg})
		}
		// 补路径（失败明细可读）
		for i := range items {
			var p []string
			if err := s.DB.WithContext(ctx).Model(&model.Note{}).Where("id = ?", items[i].NoteID).Pluck("path", &p).Error; err == nil && len(p) > 0 {
				items[i].Path = p[0]
			}
		}
		st.FailedItems = items
	}
	s.mu.Unlock()
	return st, nil
}

// CleanupOrphans 清理无主切片（笔记被彻底删除后残留；每日 sweep 调用）。
func (s *IndexService) CleanupOrphans(ctx context.Context) (int64, error) {
	res := s.DB.WithContext(ctx).Exec(
		`DELETE FROM tf_note_chunk k WHERE NOT EXISTS (SELECT 1 FROM tf_note n WHERE n.id = k.note_id)`)
	return res.RowsAffected, res.Error
}
