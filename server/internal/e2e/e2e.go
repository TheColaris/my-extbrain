// Package e2e —— E2E 洁净室运行时：三重门判定 + 外发截获信箱。
//
// 三重门 fail-closed（全部满足才启用；E2E_MODE=true 但任一门不过 → 启动直接报错）：
//  1. 门① 意图门：环境变量 E2E_MODE=true（显式声明洁净启动）；
//  2. 门② 配置门：DATABASE_URL 指向的库名必须是 extbrain_e2e（防配置漂移指到 dev/生产库）；
//  3. 门③ 数据门：洁净库内存在标记行 tp_system_config.e2e_cleanroom_marker（由 e2e.sh reset 写入，
//     防库被换而配置未动）。
//
// 门全开后的行为边界（外部影响零泄漏）：
//   - 钉钉/飞书 webhook（service.postJSON）与浏览器推送（push_service.webPush）不外发，
//     截获落 tl_e2e_mailbox，返回成功——绝不回落真实外发；
//   - 信箱 API（/api/v1/e2e/mailbox）仅门全开时注册路由，门不开=404；
//   - embedding 等其余外部调用不走本包：洁净配方用 mock 端点（script/e2e）。
package e2e

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// CleanDBName 门②认可的唯一洁净库名；MarkerKey 门③认可的标记行键。
const CleanDBName = "extbrain_e2e"
const MarkerKey = "e2e_cleanroom_marker"

// E2EChannel 信箱截获通道（tl_e2e_mailbox.channel）。
const (
	ChannelWebhook = "webhook" // 钉钉/飞书机器人
	ChannelWebPush = "webpush" // 浏览器推送
)

var mu sync.RWMutex
var enabled bool

// Enabled 三重门是否全开（进程内恒定：仅 Evaluate 在启动期写一次）。
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// Evaluate 启动期三重门判定。e2eMode=false（门①不开）→ 静默禁用；
// e2eMode=true 但门②/③不过 → 返回错误（调用方应拒绝启动，对齐「声称 E2E 却没过门=配置漂移」纪律）。
func Evaluate(e2eMode bool, databaseURL string, db *gorm.DB) error {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
	if !e2eMode {
		return nil
	}
	if name := dbName(databaseURL); name != CleanDBName {
		return fmt.Errorf("[E2E][启动断言失败] 门② DATABASE_URL 库名=%q 不是 %s——洁净启动禁止指向其他库", name, CleanDBName)
	}
	var n int64
	if err := db.Model(&model.SystemConfig{}).Where("config_key = ?", MarkerKey).Count(&n).Error; err != nil {
		return fmt.Errorf("[E2E][启动断言失败] 门③ 标记行查询失败: %v", err)
	}
	if n == 0 {
		return fmt.Errorf("[E2E][启动断言失败] 门③ 洁净库缺少标记行 %s（先跑 script/e2e/e2e.sh reset）", MarkerKey)
	}
	enabled = true
	log.Printf("[E2E] 洁净室已启用（三重门全过；外发截获落 %s）", model.E2EMailbox{}.TableName())
	return nil
}

// Capture 截获一条外发内容落信箱。仅 Enabled 时由捕获分支调用（双保险再判一次）；
// 落库失败只打日志、不向上返回——E2E 模式下调用方语义恒为「已捕获」，绝不回落真实外发。
func Capture(db *gorm.DB, channel, target, title, body, payload string) {
	if db == nil || !Enabled() {
		return
	}
	row := model.E2EMailbox{
		Channel: channel,
		Target:  cut(target, 500),
		Title:   cut(title, 255),
		Body:    cut(body, 2000),
		Payload: cut(payload, 60000),
	}
	if err := db.Create(&row).Error; err != nil {
		log.Printf("[E2E] 信箱落库失败（仍按已捕获处理）: %v", err)
	}
}

// dbName 从连接串解析库名（postgres://user:pass@host:port/db?params → db）。
// scheme 非 postgres/postgresql 一律视为解析失败（返回空串 → 门②不过）。
func dbName(databaseURL string) string {
	u, err := url.Parse(strings.TrimSpace(databaseURL))
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path == "" {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

// cut 按 rune 截断（按字节切会碎 UTF-8，PG 拒收整行）。
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
