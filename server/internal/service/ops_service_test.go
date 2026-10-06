package service

import (
	"context"
	"math"
	"os"
	"testing"
	"time"
)

// 运营看板聚合：窗口长度 / 本地日分桶（含 23:30 边界）/ 口径（活跃去重、错误率、Top 榜）+ 参数回落。
// 断言一律用「基线差值」——聚合是全库口径，测试库可能残留其他用例的行。
func TestOpsSummary(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL 未设置，跳过集成测试")
	}
	db := testDB(t) // 会 TRUNCATE tu_user/tu_api_key/sys_cache/tf_repo（与既有测试同口径）
	ctx := context.Background()
	const uid = int64(88)
	clean := func() {
		db.Exec("DELETE FROM tl_api_log WHERE user_id = ?", uid)
		db.Exec("DELETE FROM tl_push_log WHERE user_id = ?", uid)
		db.Exec("DELETE FROM tf_todo WHERE user_id = ?", uid)
		db.Exec("DELETE FROM tf_memo WHERE user_id = ?", uid)
		db.Exec("DELETE FROM tf_note WHERE user_id = ?", uid)
		db.Exec("DELETE FROM tu_user WHERE id = ?", uid)
	}
	clean()
	t.Cleanup(clean)

	svc := &OpsService{DB: db}
	base, err := svc.Summary(ctx, 14)
	if err != nil {
		t.Fatalf("基线读取失败: %v", err)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	mustExec(t, db, `INSERT INTO tu_user (id, phone, password_hash, nick_name, create_time, update_time)
	         OVERRIDING SYSTEM VALUE VALUES (?, '13900000088', 'x', 'ops测试', ?, ?)`,
		uid, today.Add(30*time.Minute), today.Add(30*time.Minute))
	rid := testRepoID(t, db, uid)
	mustExec(t, db, `INSERT INTO tf_note (user_id, repo_id, path, title, create_time) VALUES (?, ?, 'ops/看板.md', '看板', ?)`,
		uid, rid, today.Add(9*time.Hour))
	mustExec(t, db, `INSERT INTO tf_todo (user_id, title, status, create_time) VALUES (?, '运营看板', 'active', ?)`,
		uid, today.Add(9*time.Hour))
	mustExec(t, db, `INSERT INTO tf_memo (user_id, content, create_time) VALUES (?, '看板口径', ?)`,
		uid, today.Add(9*time.Hour))
	// 今天：AI 2（含 1 条 401）+ Web 1；3 天前本地 23:30 一条（分桶边界=必须落在当天，不得漂到次日）
	mustExec(t, db, `INSERT INTO tl_api_log (user_id, api_key_id, action, target, status_code, client_ip, create_time) VALUES
	         (?, 7, 'note.upsert', 'ops/看板.md', 200, '1.1.1.1', ?),
	         (?, 7, 'search.query', '', 401, '1.1.1.1', ?),
	         (?, 0, 'web.login', '', 200, '1.1.1.1', ?),
	         (?, 7, 'todo.create', '', 200, '1.1.1.1', ?)`,
		uid, today.Add(9*time.Hour), uid, today.Add(9*time.Hour+time.Minute), uid, today.Add(9*time.Hour+2*time.Minute),
		uid, today.AddDate(0, 0, -3).Add(23*time.Hour+30*time.Minute))
	// 榜位保证：今天再补 40 条（Top 用户按窗口调用量倒序）
	for i := 0; i < 40; i++ {
		mustExec(t, db, `INSERT INTO tl_api_log (user_id, api_key_id, action, status_code, client_ip, create_time)
		         VALUES (?, 7, 'memo.create', 200, '1.1.1.1', ?)`, uid, today.Add(10*time.Hour+time.Duration(i)*time.Minute))
	}
	// 面板轮询噪声存量行：放 10 天前（聚合窗内、7 日均窗外），聚合必须排除不进榜
	mustExec(t, db, `INSERT INTO tl_api_log (user_id, api_key_id, action, status_code, client_ip, create_time) VALUES
	         (?, 0, 'dashboard.read', 200, '1.1.1.1', ?),
	         (?, 0, 'push.read', 200, '1.1.1.1', ?)`,
		uid, today.AddDate(0, 0, -10).Add(9*time.Hour), uid, today.AddDate(0, 0, -10).Add(9*time.Hour))
	// 推送：今天成功 1 / 失败 1（近 7 日窗口）
	mustExec(t, db, `INSERT INTO tl_push_log (user_id, event_type, event_key, channel_type, channel_id, title, body, status, create_time)
	         VALUES (?, 'test', 'k1', 'web', 0, 't', 'b', 'ok', ?), (?, 'test', 'k2', 'web', 0, 't', 'b', 'fail', ?)`,
		uid, today.Add(time.Hour), uid, today.Add(time.Hour))

	got, err := svc.Summary(ctx, 14)
	if err != nil {
		t.Fatalf("聚合失败: %v", err)
	}
	ov, bo := got.Overview, base.Overview

	// --- 概览（差值断言）---
	if ov.UsersTotal != bo.UsersTotal+1 || ov.UsersToday != bo.UsersToday+1 {
		t.Errorf("用户数：total=%d(+%d) today=%d(+%d)，期望各 +1", ov.UsersTotal, ov.UsersTotal-bo.UsersTotal, ov.UsersToday, ov.UsersToday-bo.UsersToday)
	}
	if ov.NotesToday != bo.NotesToday+1 || ov.TodosTotal != bo.TodosTotal+1 || ov.MemosToday != bo.MemosToday+1 {
		t.Errorf("内容增量：note+%d todo+%d memo+%d，期望各 +1", ov.NotesToday-bo.NotesToday, ov.TodosTotal-bo.TodosTotal, ov.MemosToday-bo.MemosToday)
	}
	if ov.CallsToday != bo.CallsToday+43 {
		t.Errorf("今日调用 +%d，期望 +43（今天 3 条 + 40 条；第 4 条在 3 天前）", ov.CallsToday-bo.CallsToday)
	}
	if ov.ErrorsToday != bo.ErrorsToday+1 {
		t.Errorf("今日错误 +%d，期望 +1", ov.ErrorsToday-bo.ErrorsToday)
	}
	if ov.ActiveToday != bo.ActiveToday+1 {
		t.Errorf("今日活跃 +%d，期望 +1（去重）", ov.ActiveToday-bo.ActiveToday)
	}
	if ov.Push7dTotal != bo.Push7dTotal+2 || ov.Push7dOk != bo.Push7dOk+1 {
		t.Errorf("推送 7 日：total +%d ok +%d，期望 +2/+1", ov.Push7dTotal-bo.Push7dTotal, ov.Push7dOk-bo.Push7dOk)
	}
	if ov.CallsToday > 0 {
		want := float64(ov.ErrorsToday) / float64(ov.CallsToday)
		if math.Abs(ov.ErrorRateToday-want) > 1e-9 {
			t.Errorf("错误率 %v，期望 %v（错误/调用）", ov.ErrorRateToday, want)
		}
	}

	// --- 每日分桶：窗口长度 + 本地日边界 + 近 7 日均 ---
	if len(got.Daily) != 14 {
		t.Fatalf("14 天窗口桶数=%d，期望 14", len(got.Daily))
	}
	if got.Daily[len(got.Daily)-1].Date != today.Format("01-02") {
		t.Errorf("末桶=%s，期望今天 %s", got.Daily[len(got.Daily)-1].Date, today.Format("01-02"))
	}
	find := func(list []OpsDay, date string) *OpsDay {
		for i := range list {
			if list[i].Date == date {
				return &list[i]
			}
		}
		return nil
	}
	d3 := today.AddDate(0, 0, -3).Format("01-02")
	cur3, base3 := find(got.Daily, d3), find(base.Daily, d3)
	if cur3 == nil || base3 == nil {
		t.Fatalf("3 天前桶缺失：%s", d3)
	}
	if cur3.CallsAI != base3.CallsAI+1 {
		t.Errorf("3 天前 23:30 的行未落在当天：calls_ai %d→%d，期望 +1（时区分桶错位）", base3.CallsAI, cur3.CallsAI)
	}
	tb, bb := got.Daily[len(got.Daily)-1], base.Daily[len(base.Daily)-1]
	if tb.CallsAI != bb.CallsAI+42 || tb.CallsWeb != bb.CallsWeb+1 {
		t.Errorf("今日桶 AI %+d / Web %+d，期望 +42/+1", tb.CallsAI-bb.CallsAI, tb.CallsWeb-bb.CallsWeb)
	}
	if tb.Actives != bb.Actives+1 || tb.Err4xx != bb.Err4xx+1 {
		t.Errorf("今日桶 活跃 %+d / 4xx %+d，期望 +1/+1", tb.Actives-bb.Actives, tb.Err4xx-bb.Err4xx)
	}
	var sum7 int64
	for _, d := range got.Daily[len(got.Daily)-7:] {
		sum7 += d.CallsAI + d.CallsWeb
	}
	if math.Abs(ov.Calls7dAvg-float64(sum7)/7) > 1e-9 {
		t.Errorf("7 日均调用 %v，期望 %v", ov.Calls7dAvg, float64(sum7)/7)
	}

	// --- 榜单 ---
	var topMemo bool
	for _, a := range got.TopActions {
		if a.Action == "memo.create" && a.Count >= 40 {
			topMemo = true
		}
	}
	if !topMemo {
		t.Errorf("动作 Top 缺 memo.create(>=40)：%+v", got.TopActions)
	}
	for _, a := range got.TopActions {
		if a.Action == "dashboard.read" || a.Action == "push.read" {
			t.Errorf("动作 Top 混入面板轮询噪声 %s：%+v", a.Action, got.TopActions)
		}
	}
	var err401 bool
	for _, e := range got.TopErrors {
		if e.Status == 401 && e.Count >= 1 {
			err401 = true
		}
	}
	if !err401 {
		t.Errorf("错误 Top 缺 401：%+v", got.TopErrors)
	}
	var me *OpsTopUser
	for i := range got.TopUsers {
		if got.TopUsers[i].UserID == uid {
			me = &got.TopUsers[i]
		}
	}
	if me == nil {
		t.Fatalf("活跃用户榜缺 uid=%d：%+v", uid, got.TopUsers)
	}
	if me.NickName != "ops测试" || me.Calls != 46 || me.Notes != 1 || me.Todos != 1 || me.Memos != 1 {
		t.Errorf("榜行不符：%+v（期望 昵称=ops测试 调用=46=今日43+3天前1+10天前噪声2；噪声行只挡动作榜不挡用户榜）", *me)
	}
	if me.LastActive.Before(today) {
		t.Errorf("最近活跃 %v 早于今天", me.LastActive)
	}

	// --- 窗口与参数回落 ---
	d30, err := svc.Summary(ctx, 30)
	if err != nil || len(d30.Daily) != 30 {
		t.Errorf("30 天窗口桶数=%d err=%v，期望 30", len(d30.Daily), err)
	}
	dBad, err := svc.Summary(ctx, 99)
	if err != nil || len(dBad.Daily) != 14 {
		t.Errorf("非法 days 应回落 14，实际桶数=%d err=%v", len(dBad.Daily), err)
	}
}

// 本地时区偏移格式（SQL 侧 AT TIME ZONE 入参）
func TestLocalUTCOffset(t *testing.T) {
	off := localUTCOffset()
	if len(off) != 6 || (off[0] != '+' && off[0] != '-') || off[3] != ':' {
		t.Fatalf("偏移格式非法: %q", off)
	}
}
