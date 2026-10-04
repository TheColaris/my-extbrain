// Package geoip —— IP 归属地解析（ip2region v4 离线库，Apache-2.0 OR MIT 双许可，
// 源码与数据来自 github.com/lionsoul2014/ip2region，许可文件见 xdb/LICENSE.md）。
// v4 xdb 全量驻内存（约 11MB），查询微秒级、零网络；IPv6 暂不支持（返回空）。
package geoip

import (
	_ "embed"
	"net"
	"strings"
	"sync"

	"extbrain-server/internal/geoip/xdb"
)

//go:embed ip2region_v4.xdb
var xdbData []byte

var (
	once     sync.Once
	searcher *xdb.Searcher
	initErr  error
)

func get() (*xdb.Searcher, error) {
	once.Do(func() {
		searcher, initErr = xdb.NewWithBuffer(xdb.IPv4, xdbData)
	})
	return searcher, initErr
}

// Search 返回展示用归属地，如 "中国 · 上海 · 电信"；内网/回环 → "内网"；查不到 → ""。
func Search(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return "内网"
	}
	if parsed.To4() == nil {
		return "" // IPv6：v2 再上 v6 库（36MB 暂不嵌入）
	}
	s, err := get()
	if err != nil {
		return ""
	}
	region, err := s.Search(ip)
	if err != nil {
		return ""
	}
	// xdb 格式：国家|区域|省|市|ISP；取 国家/省/ISP（"0"=无数据；ISP 位若为国家码如 CN 则跳过）
	parts := strings.Split(region, "|")
	out := make([]string, 0, 3)
	for _, i := range []int{0, 2, 4} {
		if len(parts) > i && parts[i] != "" && parts[i] != "0" {
			if i == 4 && len(parts[i]) <= 2 { // "CN"/"US" 等国家码不是 ISP
				continue
			}
			out = append(out, parts[i])
		}
	}
	return strings.Join(out, " · ")
}
