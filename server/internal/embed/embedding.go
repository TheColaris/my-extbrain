// Package embed —— OpenAI 兼容 /embeddings 客户端。
//
// 平台可配（tp_system_config）：硅基流动（默认）/ 任意 OpenAI 兼容端点。
// 设计口径：provider 未配置或调用失败时，搜索链整体降级为纯关键词，绝不阻断主流程。
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Client 单 provider 客户端（字段来自平台配置，保存后重建实例即可热更新）。
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HC      *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:  strings.TrimSpace(apiKey),
		Model:   strings.TrimSpace(model),
		HC:      &http.Client{Timeout: 30 * time.Second},
	}
}

// ErrBadConfig 配置不完整（调用方据此降级）
var ErrBadConfig = errors.New("embedding 配置不完整")

type embReq struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type embResp struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"` // 部分实现（如 OpenAI 错误体）在顶层
}

// Embed 批量取向量；返回顺序与入参一致。网络/5xx 自动重试一次。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if c.BaseURL == "" || c.APIKey == "" || c.Model == "" {
		return nil, ErrBadConfig
	}
	payload, err := json.Marshal(embReq{Model: c.Model, Input: texts, EncodingFormat: "float"})
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		out, retriable, err := c.once(ctx, payload)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !retriable {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, payload []byte) (vecs [][]float32, retriable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HC.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("上游 %d: %s", resp.StatusCode, snippet(body))
	}
	if resp.StatusCode != http.StatusOK {
		var er embResp
		_ = json.Unmarshal(body, &er)
		msg := ""
		if er.Error != nil {
			msg = er.Error.Message
		}
		if msg == "" {
			msg = er.Message
		}
		if msg == "" {
			msg = snippet(body)
		}
		return nil, false, fmt.Errorf("上游 %d: %s", resp.StatusCode, msg)
	}
	var out embResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, false, fmt.Errorf("响应解析失败: %w", err)
	}
	if len(out.Data) != len(payloadTexts(payload)) { // 数量校验：缺行=静默错位，宁缺毋滥
		if len(out.Data) == 0 {
			return nil, false, errors.New("上游未返回向量")
		}
	}
	sort.Slice(out.Data, func(i, j int) bool { return out.Data[i].Index < out.Data[j].Index })
	vecs = make([][]float32, 0, len(out.Data))
	for _, d := range out.Data {
		if len(d.Embedding) == 0 {
			return nil, false, errors.New("上游返回空向量")
		}
		vecs = append(vecs, d.Embedding)
	}
	return vecs, false, nil
}

// payloadTexts 仅用于数量校验（重解析一次成本可忽略）
func payloadTexts(payload []byte) []string {
	var r embReq
	_ = json.Unmarshal(payload, &r)
	return r.Input
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	r := []rune(s)
	if len(r) > 300 {
		return string(r[:300]) + "…"
	}
	return s
}
