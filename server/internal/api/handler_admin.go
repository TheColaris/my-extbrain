package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"extbrain-server/internal/embed"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdminHandler 平台管理（仅管理员）：embedding 配置 + 邮件服务配置 + 索引状态/重建 + 运营看板。
// 密钥口径：只回打码（MaskKey），保存时「空=保持不变」。
type AdminHandler struct {
	Sys   *service.SysConfig
	Index *service.IndexService
	Ops   *service.OpsService
	DB    *gorm.DB
	Email *service.EmailService
}

var adminEmailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// GET /api/v1/admin/embedding
func (h *AdminHandler) GetEmbedding(c *gin.Context) {
	cfg := h.Sys.Embedding()
	st, err := h.Index.Status(c.Request.Context())
	if err != nil {
		fail(c, errInternal("读取索引状态失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"enabled":      cfg.Enabled,
		"base_url":     cfg.BaseURL,
		"model":        cfg.Model,
		"dim":          cfg.Dim,
		"api_key_hint": service.MaskKey(cfg.APIKey),
		"api_key_set":  cfg.APIKey != "",
		"defaults": gin.H{
			"base_url": service.DefaultEmbeddingBaseURL,
			"model":    service.DefaultEmbeddingModel,
			"dim":      service.DefaultEmbeddingDim,
		},
		"last_test": h.lastTest(),
		"index":     st,
	})
}

func (h *AdminHandler) lastTest() json.RawMessage {
	raw := h.Sys.Get(service.CfgEmbeddingLastTest)
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil
	}
	return json.RawMessage(raw)
}

// PUT /api/v1/admin/embedding（{enabled, base_url, model, dim, api_key?}；api_key 空=不变）
func (h *AdminHandler) SaveEmbedding(c *gin.Context) {
	var in struct {
		Enabled *bool  `json:"enabled"`
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		Dim     int    `json:"dim"`
		APIKey  string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {enabled, base_url, model, dim, api_key?}"))
		return
	}
	cur := h.Sys.Embedding()
	enabled := cur.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	baseURL := strings.TrimSpace(in.BaseURL)
	model := strings.TrimSpace(in.Model)
	dim := in.Dim
	key := strings.TrimSpace(in.APIKey)
	if baseURL == "" {
		baseURL = service.DefaultEmbeddingBaseURL
	}
	if model == "" {
		model = service.DefaultEmbeddingModel
	}
	if dim == 0 {
		dim = service.DefaultEmbeddingDim
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		fail(c, errBadRequest("invalid_base_url", "接口地址须以 http:// 或 https:// 开头"))
		return
	}
	if dim < 64 || dim > 8192 {
		fail(c, errBadRequest("invalid_dim", "向量维度须在 64~8192 之间"))
		return
	}
	if key == "" {
		key = cur.APIKey
	}
	if enabled && key == "" {
		fail(c, errBadRequest("missing_key", "启用语义检索前需填写 API Key"))
		return
	}
	kv := map[string]string{
		service.CfgEmbeddingEnabled: strconv.FormatBool(enabled),
		service.CfgEmbeddingBaseURL: baseURL,
		service.CfgEmbeddingModel:   model,
		service.CfgEmbeddingDim:     strconv.Itoa(dim),
	}
	if key != cur.APIKey {
		kv[service.CfgEmbeddingAPIKey] = key
	}
	if err := h.Sys.Set(c.Request.Context(), kv, service.CfgEmbeddingAPIKey); err != nil {
		fail(c, errInternal("保存失败: "+err.Error()))
		return
	}
	// 首次配置/配置变更后立即补算（维度与索引列不一致时不动，等用户点「重建索引」）
	if enabled {
		if st, err := h.Index.Status(c.Request.Context()); err == nil && (st.IndexDim == 0 || st.IndexDim == dim) {
			h.Index.Kick(c.Request.Context())
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/admin/embedding/test（{base_url?, api_key?, model?, dim?}；缺省用已存配置）
func (h *AdminHandler) TestEmbedding(c *gin.Context) {
	var in struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
		Model   string `json:"model"`
		Dim     int    `json:"dim"`
	}
	_ = c.ShouldBindJSON(&in)
	cur := h.Sys.Embedding()
	baseURL := strings.TrimSpace(in.BaseURL)
	if baseURL == "" {
		baseURL = cur.BaseURL
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = cur.Model
	}
	key := strings.TrimSpace(in.APIKey)
	if key == "" {
		key = cur.APIKey
	}
	dim := in.Dim
	if dim == 0 {
		dim = cur.Dim
	}
	if key == "" {
		c.JSON(http.StatusOK, gin.H{"ok": false, "ms": 0, "dim": 0, "error": "未填写 API Key"})
		return
	}
	start := time.Now()
	cl := embed.NewClient(baseURL, key, model)
	vecs, err := cl.Embed(c.Request.Context(), []string{"连通性测试 ping"})
	ms := time.Since(start).Milliseconds()
	res := gin.H{"ok": true, "ms": ms, "dim": 0, "error": ""}
	switch {
	case err != nil:
		res["ok"] = false
		res["error"] = err.Error()
	case len(vecs) == 1:
		res["dim"] = len(vecs[0])
		if dim > 0 && len(vecs[0]) != dim {
			res["ok"] = false
			res["error"] = fmt.Sprintf("模型输出 %d 维，与配置维度 %d 不一致", len(vecs[0]), dim)
		}
	default:
		res["ok"] = false
		res["error"] = "上游未返回向量"
	}
	// 记录最近测试结果（刷新后回显；非敏感）
	rec, _ := json.Marshal(gin.H{
		"ok": res["ok"], "ms": ms, "dim": res["dim"],
		"error": res["error"], "at": time.Now().Format(time.RFC3339),
	})
	_ = h.Sys.Set(c.Request.Context(), map[string]string{service.CfgEmbeddingLastTest: string(rec)})
	c.JSON(http.StatusOK, res)
}

// GET /api/v1/admin/email
func (h *AdminHandler) GetEmail(c *gin.Context) {
	cfg := h.Sys.Email()
	c.JSON(http.StatusOK, gin.H{
		"enabled":      cfg.Enabled,
		"from_address": cfg.FromAddr,
		"from_name":    cfg.FromName,
		"api_key_hint": service.MaskKey(cfg.APIKey),
		"api_key_set":  cfg.APIKey != "",
		"last_test":    h.emailLastTest(),
	})
}

func (h *AdminHandler) emailLastTest() json.RawMessage {
	raw := h.Sys.Get(service.CfgEmailLastTest)
	if raw == "" || !json.Valid([]byte(raw)) {
		return nil
	}
	return json.RawMessage(raw)
}

// PUT /api/v1/admin/email（{enabled, from_address, from_name, api_key?}；api_key 空=不变）
func (h *AdminHandler) SaveEmail(c *gin.Context) {
	var in struct {
		Enabled     *bool  `json:"enabled"`
		FromAddress string `json:"from_address"`
		FromName    string `json:"from_name"`
		APIKey      string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, errBadRequest("invalid_body", "body 是 {enabled, from_address, from_name, api_key?}"))
		return
	}
	cur := h.Sys.Email()
	enabled := cur.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	fromAddr := strings.TrimSpace(in.FromAddress)
	fromName := strings.TrimSpace(in.FromName)
	key := strings.TrimSpace(in.APIKey)
	if fromAddr != "" && !adminEmailRe.MatchString(fromAddr) {
		fail(c, errBadRequest("invalid_from_address", "发件地址格式不正确"))
		return
	}
	if key == "" {
		key = cur.APIKey
	}
	if enabled && fromAddr == "" {
		fail(c, errBadRequest("missing_from_address", "启用邮件服务前需填写发件地址"))
		return
	}
	if enabled && key == "" {
		fail(c, errBadRequest("missing_key", "启用邮件服务前需填写 API Key"))
		return
	}
	kv := map[string]string{
		service.CfgEmailEnabled:  strconv.FormatBool(enabled),
		service.CfgEmailFromAddr: fromAddr,
		service.CfgEmailFromName: fromName,
	}
	if key != cur.APIKey {
		kv[service.CfgEmailAPIKey] = key
	}
	if err := h.Sys.Set(c.Request.Context(), kv, service.CfgEmailAPIKey); err != nil {
		fail(c, errInternal("保存失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/v1/admin/email/test（发往当前管理员绑定邮箱；结果记 last_test 供刷新回显）
func (h *AdminHandler) TestEmail(c *gin.Context) {
	var u model.User
	if err := h.DB.WithContext(c.Request.Context()).First(&u, uid(c)).Error; err != nil {
		fail(c, errInternal("读取当前用户失败"))
		return
	}
	if u.Email == nil || strings.TrimSpace(*u.Email) == "" {
		fail(c, errBadRequest("no_email", "当前账号未绑定邮箱，无法接收测试邮件"))
		return
	}
	to := strings.TrimSpace(*u.Email)
	start := time.Now()
	err := h.Email.Test(c.Request.Context(), to)
	ms := time.Since(start).Milliseconds()
	res := gin.H{"ok": err == nil, "ms": ms, "to": to, "error": ""}
	if err != nil {
		res["ok"] = false
		res["error"] = err.Error()
	}
	rec, _ := json.Marshal(gin.H{
		"ok": res["ok"], "ms": ms, "to": to,
		"error": res["error"], "at": time.Now().Format(time.RFC3339),
	})
	_ = h.Sys.Set(c.Request.Context(), map[string]string{service.CfgEmailLastTest: string(rec)})
	c.JSON(http.StatusOK, res)
}

// GET /api/v1/admin/index/status
func (h *AdminHandler) IndexStatus(c *gin.Context) {
	st, err := h.Index.Status(c.Request.Context())
	if err != nil {
		fail(c, errInternal("读取索引状态失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, st)
}

// POST /api/v1/admin/index/rebuild（异步；进度走 index/status）
func (h *AdminHandler) Rebuild(c *gin.Context) {
	if err := h.Index.ReindexAll(c.Request.Context()); err != nil {
		respondServiceErr(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}

// GET /api/v1/admin/ops?days=14（days ∈ {14,30}；一次返回全部分区，查询条数固定）
func (h *AdminHandler) OpsSummary(c *gin.Context) {
	days := 14
	if v := c.Query("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 14 && n != 30) {
			fail(c, errBadRequest("invalid_days", "days 只支持 14 或 30"))
			return
		}
		days = n
	}
	out, err := h.Ops.Summary(c.Request.Context(), days)
	if err != nil {
		fail(c, errInternal("读取运营数据失败: "+err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}
