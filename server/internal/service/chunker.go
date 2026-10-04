package service

import "strings"

// 切片参数（runes）：目标长度 / 单切片硬上限（列宽 varchar(2000) 留余量）/ 超长段滑窗重叠
const (
	chunkTargetRunes  = 700
	chunkMaxRunes     = 1800
	chunkOverlapRunes = 100
)

// ChunkMarkdown 把笔记正文切成检索切片。
//
// 策略：按 Markdown 标题分节 → 节内段落累积到 ~700 rune 即切 → 超长单行按 rune 滑窗（重叠 100）。
// 返回值均 ≤ chunkMaxRunes；空内容返回 nil（调用方据此清空该笔记切片）。
func ChunkMarkdown(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	chunks := make([]string, 0, 4)
	var buf []string
	bufLen := 0
	flush := func() {
		if bufLen == 0 {
			return
		}
		if text := strings.TrimSpace(strings.Join(buf, "\n")); text != "" {
			chunks = append(chunks, text)
		}
		buf = buf[:0]
		bufLen = 0
	}
	for _, ln := range lines {
		rl := len([]rune(ln))
		isHeading := strings.HasPrefix(strings.TrimSpace(ln), "#")
		if isHeading && bufLen > 0 {
			flush()
		}
		if rl > chunkMaxRunes {
			flush()
			chunks = append(chunks, splitLongLine(strings.TrimSpace(ln))...)
			continue
		}
		if bufLen > 0 && bufLen+rl > chunkTargetRunes {
			flush()
		}
		buf = append(buf, ln)
		bufLen += rl + 1
	}
	flush()
	return chunks
}

// splitLongLine 超长单行按 rune 滑窗切分（重叠 chunkOverlapRunes，最后一片 ≤ 上限）。
func splitLongLine(s string) []string {
	r := []rune(s)
	if len(r) <= chunkMaxRunes {
		return []string{s}
	}
	out := make([]string, 0, len(r)/chunkTargetRunes+1)
	step := chunkMaxRunes - chunkOverlapRunes
	for start := 0; start < len(r); start += step {
		end := start + chunkMaxRunes
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[start:end]))
		if end == len(r) {
			break
		}
	}
	return out
}
