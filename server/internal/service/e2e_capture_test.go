package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"extbrain-server/internal/e2e"
	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// 洁净室捕获链：三重门强制启用后，webhook/浏览器推送不外发、落信箱、按成功返回。
func forceEnable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&model.E2EMailbox{}); err != nil { // 幂等确保信箱表在（其他测试可能 drop 过）
		t.Fatalf("建信箱表失败: %v", err)
	}
	if err := db.Create(&model.SystemConfig{ConfigKey: e2e.MarkerKey, ConfigValue: "1"}).Error; err != nil {
		t.Fatalf("写标记行失败: %v", err)
	}
	if err := e2e.Evaluate(true, "postgres://u:p@127.0.0.1:5432/"+e2e.CleanDBName, db); err != nil {
		t.Fatalf("强制启用失败: %v", err)
	}
	t.Cleanup(func() {
		db.Where("config_key = ?", e2e.MarkerKey).Delete(&model.SystemConfig{})
		_ = e2e.Evaluate(false, "", nil)
	})
}

func mailboxRows(t *testing.T, db *gorm.DB) []model.E2EMailbox {
	t.Helper()
	rows := make([]model.E2EMailbox, 0)
	if err := db.Order("id DESC").Find(&rows).Error; err != nil {
		t.Fatalf("查信箱失败: %v", err)
	}
	return rows
}

func TestNotifyCaptureInMailbox(t *testing.T) {
	db := testDB(t)
	forceEnable(t, db)
	t.Cleanup(func() { db.Exec("TRUNCATE tl_e2e_mailbox") })

	n := NewNotifyService(db, nil)

	// 钉钉（无签名）：捕获后按成功返回
	c := &model.NotifyChannel{ChannelType: model.ChannelTypeDingTalk, WebhookURL: "https://ding.test/hook", Secret: ""}
	if err := n.Send(c, "到期提醒", "有 2 项逾期"); err != nil {
		t.Fatalf("E2E 模式 Send 应回成功: %v", err)
	}
	rows := mailboxRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("信箱应 1 行, got %d", len(rows))
	}
	// 钉钉 markdown text 自带 "### 标题" 前缀（服务端真实行为），断言按包含语义
	if rows[0].Channel != e2e.ChannelWebhook || rows[0].Target != "https://ding.test/hook" ||
		rows[0].Title != "到期提醒" || !strings.Contains(rows[0].Body, "2 项逾期") {
		t.Fatalf("钉钉捕获字段不符: %+v", rows[0])
	}

	// 钉钉（加签）：payload 含 timestamp/sign，URL 含签名参数
	c2 := &model.NotifyChannel{ChannelType: model.ChannelTypeDingTalk, WebhookURL: "https://ding.test/signed", Secret: "s3cret"}
	if err := n.Send(c2, "加签", "x"); err != nil {
		t.Fatalf("加签 Send 应回成功: %v", err)
	}
	rows = mailboxRows(t, db)
	if rows[0].Target == "https://ding.test/signed" || !strings.Contains(rows[0].Target, "timestamp=") || !strings.Contains(rows[0].Target, "sign=") {
		t.Fatalf("加签目标应带签名参数: %s", rows[0].Target)
	}

	// 飞书（加签）：payload 含 timestamp/sign；title 从 card.header 提取
	c3 := &model.NotifyChannel{ChannelType: model.ChannelTypeFeishu, WebhookURL: "https://feishu.test/hook", Secret: "fs"}
	if err := n.Send(c3, "飞书标题", "飞书正文"); err != nil {
		t.Fatalf("飞书 Send 应回成功: %v", err)
	}
	rows = mailboxRows(t, db)
	if rows[0].Title != "飞书标题" || !strings.Contains(rows[0].Body, "飞书正文") {
		t.Fatalf("飞书 title/body 提取不符: %+v", rows[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].Payload), &payload); err != nil {
		t.Fatalf("payload 应为 JSON: %v", err)
	}
	if payload["timestamp"] == nil || payload["sign"] == nil {
		t.Fatalf("飞书加签 payload 应含 timestamp/sign: %v", payload)
	}

	// 未知渠道类型：不走 postJSON（Send 返回错误且不落信箱）
	c4 := &model.NotifyChannel{ChannelType: "weird", WebhookURL: "https://x.test/hook"}
	if err := n.Send(c4, "t", "b"); err == nil {
		t.Fatal("未知渠道应报错")
	}
	rows = mailboxRows(t, db)
	if len(rows) != 3 {
		t.Fatalf("未知渠道不应落信箱（应仍 3 行）, got %d", len(rows))
	}
}

func TestPushCaptureInMailbox(t *testing.T) {
	db := testDB(t)
	forceEnable(t, db)
	t.Cleanup(func() { db.Exec("TRUNCATE tl_e2e_mailbox, tf_push_subscription") })

	p := &PushService{
		DB: db, Cache: &CacheService{DB: db},
		VapidPublic: "test-public", VapidPriv: "test-priv",
	}
	sub := model.PushSubscription{UserID: 1, Endpoint: "https://fcm.test/sub-e2e", P256DH: "k1", Auth: "k2"}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("种订阅失败: %v", err)
	}

	// 测试推送：webPush 走捕获分支，按成功返回，订阅不被清理
	p.SendToUserForTest(t.Context(), 1)
	rows := mailboxRows(t, db)
	if len(rows) != 1 {
		t.Fatalf("信箱应 1 行, got %d", len(rows))
	}
	if rows[0].Channel != e2e.ChannelWebPush || rows[0].Target != "https://fcm.test/sub-e2e" {
		t.Fatalf("webpush 捕获字段不符: %+v", rows[0])
	}
	if rows[0].Title == "" || rows[0].Body == "" {
		t.Fatalf("推送 title/body 不应为空: %+v", rows[0])
	}
	var alive model.PushSubscription
	if err := db.First(&alive, sub.ID).Error; err != nil || alive.IsDeleted != 0 {
		t.Fatalf("捕获分支不得清理订阅: err=%v deleted=%d", err, alive.IsDeleted)
	}

	// 重试/失败语义：真实 webPush 分支（未启用洁净时）由既有集成与 E2E 旅程覆盖，此处不外发验证
}

// 非洁净模式：postJSON/webPush 走真实外发分支（httptest 本地回收，不外网）。
func TestNotifyRealSendBranch(t *testing.T) {
	db := testDB(t)
	_ = e2e.Evaluate(false, "", db) // 门①关：真实分支
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	n := NewNotifyService(db, nil)
	c := &model.NotifyChannel{ChannelType: model.ChannelTypeDingTalk, WebhookURL: srv.URL}
	if err := n.Send(c, "真实", "外发"); err != nil {
		t.Fatalf("真实分支 Send 应回成功: %v", err)
	}
	if !strings.Contains(string(gotBody), "markdown") {
		t.Fatalf("真实分支应发出钉钉消息体: %s", gotBody)
	}
	// 信箱必须为空（未启用不截获）
	if rows := mailboxRows(t, db); len(rows) != 0 {
		t.Fatalf("非洁净模式不得落信箱: %d", len(rows))
	}
	// 机器人返回错误（errcode!=0）→ Send 报错
	srvErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"errcode":310000,"errmsg":"sign not match"}`))
	}))
	defer srvErr.Close()
	c2 := &model.NotifyChannel{ChannelType: model.ChannelTypeDingTalk, WebhookURL: srvErr.URL}
	if err := n.Send(c2, "t", "b"); err == nil || !strings.Contains(err.Error(), "sign not match") {
		t.Fatalf("机器人错误应上抛: %v", err)
	}
	// 500 无信封
	srv5 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv5.Close()
	c3 := &model.NotifyChannel{ChannelType: model.ChannelTypeDingTalk, WebhookURL: srv5.URL}
	if err := n.Send(c3, "t", "b"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("500 应回 HTTP 码错误: %v", err)
	}
	// webhookTitle/webhookText 对非预期结构不 panic、返回空
	if got := webhookTitle("not-a-map"); got != "" {
		t.Fatalf("非 map title 应回空: %q", got)
	}
	if got := webhookText(nil); got != "" {
		t.Fatalf("nil text 应回空: %q", got)
	}
	if got := webhookTitle(map[string]any{"markdown": map[string]string{"title": "t"}}); got != "t" {
		t.Fatalf("钉钉 title 提取: %q", got)
	}
	if got := webhookText(map[string]any{"card": map[string]any{"elements": []any{map[string]any{"tag": "markdown", "content": "c"}}}}); got != "c" {
		t.Fatalf("飞书 text 提取: %q", got)
	}
}

// fakeP256DH 生成合法 P-256 非压缩公钥（65 字节，0x04||X||Y）的 base64url——webpush 加密阶段会做 ECDH。
func fakeP256DH() string {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	point := append([]byte{4}, key.PublicKey.X.FillBytes(make([]byte, 32))...)
	point = append(point, key.PublicKey.Y.FillBytes(make([]byte, 32))...)
	return base64.RawURLEncoding.EncodeToString(point)
}

func fakeAuth() string {
	return base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))
}

func TestWebPushRealBranch(t *testing.T) {
	db := testDB(t)
	_ = e2e.Evaluate(false, "", db) // 门①关：真实分支
	var status int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	defer srv.Close()

	pub, priv, err := GenerateVapid()
	if err != nil {
		t.Fatalf("生成 VAPID 失败: %v", err)
	}
	p := &PushService{DB: db, Cache: &CacheService{DB: db}, VapidPublic: pub, VapidPriv: priv}
	// 200：发送成功（订阅 keys 需为合法 P-256 公钥 + 16 字节 auth，否则加密阶段即失败）
	status = http.StatusOK
	sub := model.PushSubscription{UserID: 1, Endpoint: srv.URL, P256DH: fakeP256DH(), Auth: fakeAuth()}
	if err := p.webPush(&sub, "t", "b"); err != nil {
		t.Fatalf("200 应成功: %v", err)
	}
	// 410：订阅失效 → 清理并报错
	status = http.StatusGone
	db.Create(&sub)
	if err := p.webPush(&sub, "t", "b"); err == nil {
		t.Fatal("410 应报订阅失效")
	}
	var alive model.PushSubscription
	db.First(&alive, sub.ID)
	if alive.IsDeleted != 1 {
		t.Fatal("410 应回收订阅（is_deleted=1）")
	}
}

// GenerateVapid 返回顺序锁死：pub 必须是 65 字节非压缩公钥（0x04 开头）、priv 必须是 32 字节 scalar。
// 此前直接透传 webpush 库的 (private, public) 顺序导致公私钥颠倒、真实推送必失败——单测锁死防回归。
func TestGenerateVapidKeyOrder(t *testing.T) {
	pub, priv, err := GenerateVapid()
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	pubBytes, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil || len(pubBytes) != 65 || pubBytes[0] != 4 {
		t.Fatalf("pub 应为 65 字节非压缩公钥（0x04 开头）: len=%d head=%v err=%v", len(pubBytes), pubBytes[:1], err)
	}
	privBytes, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil || len(privBytes) != 32 {
		t.Fatalf("priv 应为 32 字节 scalar: len=%d err=%v", len(privBytes), err)
	}
}
