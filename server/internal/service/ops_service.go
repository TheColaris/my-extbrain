package service

import (
	"context"
	"fmt"
	"time"

	"extbrain-server/internal/model"

	"gorm.io/gorm"
)

// OpsService —— 超管运营看板聚合（只读；窗口内按天分桶 + 固定条数聚合查询，不落库不建表）。
//
// 口径：
//   - 行为类指标来自 tl_api_log：活跃=当日有审计事件的用户；2026-10-04 起 Web 面板读类成功不记审计，
//     故活跃口径=「当日有 AI 调用或 Web 写操作/登录」的用户（纯 Web 浏览不计，偏保守）。
//   - 按天分桶在 SQL 侧用显式 UTC 偏移完成（不受 DB 会话时区影响），偏移取自服务器本地时区，
//     与个人仪表盘「本地时区分桶」同口径。
//   - 窗口上限受 LOG_RETENTION_DAYS（默认 90 天）约束；存量类指标来自业务表，不受限。
type OpsService struct{ DB *gorm.DB }

const (
	opsTopActions = 8
	opsTopErrors  = 5
	opsTopUsers   = 10
)

type OpsSummary struct {
	Overview   OpsOverview       `json:"overview"`
	Daily      []OpsDay          `json:"daily"`
	TopActions []OpsActionResult `json:"top_actions"`
	TopErrors  []OpsErrorRow     `json:"top_errors"`
	TopUsers   []OpsTopUser      `json:"top_users"`
}

type OpsOverview struct {
	UsersTotal     int64   `json:"users_total"`
	UsersToday     int64   `json:"users_today"`
	ActiveToday    int64   `json:"active_today"`
	Active7d       int64   `json:"active_7d"`
	NotesTotal     int64   `json:"notes_total"`
	NotesToday     int64   `json:"notes_today"`
	TodosTotal     int64   `json:"todos_total"` // 未完成待办
	MemosTotal     int64   `json:"memos_total"`
	MemosToday     int64   `json:"memos_today"`
	CallsToday     int64   `json:"calls_today"`
	Calls7dAvg     float64 `json:"calls_7d_avg"`
	ErrorsToday    int64   `json:"errors_today"`
	ErrorRateToday float64 `json:"error_rate_today"`
	Push7dTotal    int64   `json:"push_7d_total"`
	Push7dOk       int64   `json:"push_7d_ok"`
}

// OpsDay 每日分桶（date=MM-DD；窗口内无数据的天为 0，不缺口）
type OpsDay struct {
	Date      string `json:"date"`
	Registers int64  `json:"registers"`
	Actives   int64  `json:"actives"`
	CallsAI   int64  `json:"calls_ai"`
	CallsWeb  int64  `json:"calls_web"`
	Notes     int64  `json:"notes"`
	Todos     int64  `json:"todos"`
	Memos     int64  `json:"memos"`
	Err4xx    int64  `json:"err4xx"`
	Err5xx    int64  `json:"err5xx"`
}

type OpsActionResult struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

type OpsErrorRow struct {
	Status int16 `json:"status"`
	Count  int64 `json:"count"`
}

type OpsTopUser struct {
	UserID     int64     `json:"user_id"`
	NickName   string    `json:"nick_name"`
	Calls      int64     `json:"calls"`
	Notes      int64     `json:"notes"`
	Todos      int64     `json:"todos"`
	Memos      int64     `json:"memos"`
	LastActive time.Time `json:"last_active"`
}

// Summary 聚合一次（days ∈ {14,30}，其他值回落 14）；查询条数固定，与窗口无关。
func (s *OpsService) Summary(ctx context.Context, days int) (*OpsSummary, error) {
	if days != 14 && days != 30 {
		days = 14
	}
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	start := todayStart.AddDate(0, 0, -(days - 1))
	weekStart := todayStart.AddDate(0, 0, -6)
	offset := localUTCOffset() // 例 "+08:00"：SQL 侧按本地日分桶
	// 本地日 → 输出下标的映射（桶按本地日，缺数据的天补 0）
	idx := make(map[string]*OpsDay, days)
	out := &OpsSummary{Daily: make([]OpsDay, 0, days)}
	for i := 0; i < days; i++ {
		d := start.AddDate(0, 0, i)
		out.Daily = append(out.Daily, OpsDay{Date: d.Format("01-02")})
		idx[d.Format("2006-01-02")] = &out.Daily[len(out.Daily)-1]
	}

	// 1) 用户总量 / 今日新增
	var urow struct{ Total, Today int64 }
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT count(*) FILTER (WHERE is_deleted = 0) AS total,
		       count(*) FILTER (WHERE is_deleted = 0 AND create_time >= ?) AS today
		FROM tu_user`, todayStart).Scan(&urow).Error; err != nil {
		return nil, err
	}
	out.Overview.UsersTotal, out.Overview.UsersToday = urow.Total, urow.Today

	// 2) 内容存量 / 今日新增（待办口径=未完成）
	var crow struct{ NotesTotal, NotesToday, TodosTotal, MemosTotal, MemosToday int64 }
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT (SELECT count(*) FROM tf_note WHERE is_deleted = 0) AS notes_total,
		       (SELECT count(*) FROM tf_note WHERE is_deleted = 0 AND create_time >= ?) AS notes_today,
		       (SELECT count(*) FROM tf_todo WHERE is_deleted = 0 AND status <> ?) AS todos_total,
		       (SELECT count(*) FROM tf_memo WHERE is_deleted = 0) AS memos_total,
		       (SELECT count(*) FROM tf_memo WHERE is_deleted = 0 AND create_time >= ?) AS memos_today`,
		todayStart, model.TodoStatusDone, todayStart).Scan(&crow).Error; err != nil {
		return nil, err
	}
	out.Overview.NotesTotal, out.Overview.NotesToday = crow.NotesTotal, crow.NotesToday
	out.Overview.TodosTotal, out.Overview.MemosTotal, out.Overview.MemosToday = crow.TodosTotal, crow.MemosTotal, crow.MemosToday

	// 3) 每日注册（tu_user）
	if err := s.scanDaily(ctx, idx, `
		SELECT date_trunc('day', create_time AT TIME ZONE ?::interval)::date AS day, count(*) AS count
		FROM tu_user WHERE is_deleted = 0 AND create_time >= ? GROUP BY 1`,
		[]any{offset, start}, func(d *OpsDay, n int64) { d.Registers = n }); err != nil {
		return nil, err
	}
	// 4) 每日内容产出（三表各一条）
	for _, q := range []struct {
		sql string
		set func(d *OpsDay, n int64)
	}{
		{`SELECT date_trunc('day', create_time AT TIME ZONE ?::interval)::date AS day, count(*) AS count
		  FROM tf_note WHERE is_deleted = 0 AND create_time >= ? GROUP BY 1`, func(d *OpsDay, n int64) { d.Notes = n }},
		{`SELECT date_trunc('day', create_time AT TIME ZONE ?::interval)::date AS day, count(*) AS count
		  FROM tf_todo WHERE is_deleted = 0 AND create_time >= ? GROUP BY 1`, func(d *OpsDay, n int64) { d.Todos = n }},
		{`SELECT date_trunc('day', create_time AT TIME ZONE ?::interval)::date AS day, count(*) AS count
		  FROM tf_memo WHERE is_deleted = 0 AND create_time >= ? GROUP BY 1`, func(d *OpsDay, n int64) { d.Memos = n }},
	} {
		if err := s.scanDaily(ctx, idx, q.sql, []any{offset, start}, q.set); err != nil {
			return nil, err
		}
	}
	// 5) 每日调用/活跃/错误（tl_api_log，一条查询多聚合）
	var lrows []struct {
		Day      time.Time
		CallsAI  int64
		CallsWeb int64
		Actives  int64
		Err4xx   int64
		Err5xx   int64
	}
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT date_trunc('day', create_time AT TIME ZONE ?::interval)::date AS day,
		       count(*) FILTER (WHERE api_key_id > 0) AS calls_ai,
		       count(*) FILTER (WHERE api_key_id = 0) AS calls_web,
		       count(DISTINCT user_id) FILTER (WHERE user_id > 0) AS actives,
		       count(*) FILTER (WHERE status_code >= 400 AND status_code < 500) AS err4xx,
		       count(*) FILTER (WHERE status_code >= 500) AS err5xx
		FROM tl_api_log WHERE create_time >= ? GROUP BY 1`, offset, start).Scan(&lrows).Error; err != nil {
		return nil, err
	}
	for _, r := range lrows {
		if d, ok := idx[r.Day.Format("2006-01-02")]; ok {
			d.CallsAI, d.CallsWeb, d.Actives = r.CallsAI, r.CallsWeb, r.Actives
			d.Err4xx, d.Err5xx = r.Err4xx, r.Err5xx
		}
	}

	// 6) 今日概览（取自今日桶）+ 近 7 日均调用
	if t, ok := idx[todayStart.Format("2006-01-02")]; ok {
		out.Overview.ActiveToday = t.Actives
		out.Overview.CallsToday = t.CallsAI + t.CallsWeb
		out.Overview.ErrorsToday = t.Err4xx + t.Err5xx
		if out.Overview.CallsToday > 0 {
			out.Overview.ErrorRateToday = float64(out.Overview.ErrorsToday) / float64(out.Overview.CallsToday)
		}
	}
	var calls7 int64
	for _, d := range out.Daily[len(out.Daily)-7:] {
		calls7 += d.CallsAI + d.CallsWeb
	}
	out.Overview.Calls7dAvg = float64(calls7) / 7

	// 7) 近 7 日活跃（跨天去重，不能由每日桶求和）
	if err := s.DB.WithContext(ctx).Raw(
		`SELECT count(DISTINCT user_id) AS count FROM tl_api_log WHERE user_id > 0 AND create_time >= ?`,
		weekStart).Scan(&out.Overview.Active7d).Error; err != nil {
		return nil, err
	}

	// 8) 推送近 7 日成败
	var prow struct{ Total, Ok int64 }
	if err := s.DB.WithContext(ctx).Raw(
		`SELECT count(*) AS total, count(*) FILTER (WHERE status = ?) AS ok FROM tl_push_log WHERE create_time >= ?`,
		string(model.PushStatusOK), weekStart).Scan(&prow).Error; err != nil {
		return nil, err
	}
	out.Overview.Push7dTotal, out.Overview.Push7dOk = prow.Total, prow.Ok

	// 9) 动作 Top / 错误 Top（dashboard/push 读类=面板轮询噪声，skipWebRead 起已停记，存量行不进榜）
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT action, count(*) AS count FROM tl_api_log
		WHERE create_time >= ? AND action NOT IN ('dashboard.read', 'push.read')
		GROUP BY action ORDER BY count DESC LIMIT ?`, start, opsTopActions).Scan(&out.TopActions).Error; err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT status_code AS status, count(*) AS count FROM tl_api_log
		WHERE create_time >= ? AND status_code >= 400
		GROUP BY status_code ORDER BY count DESC LIMIT ?`, start, opsTopErrors).Scan(&out.TopErrors).Error; err != nil {
		return nil, err
	}

	// 10) 活跃用户 Top（调用量倒序 + 昵称 + 内容存量 + 最近活跃）
	var trows []struct {
		UserID     int64
		Calls      int64
		LastActive time.Time
	}
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT user_id, count(*) AS calls, max(create_time) AS last_active
		FROM tl_api_log WHERE create_time >= ? AND user_id > 0
		GROUP BY user_id ORDER BY calls DESC LIMIT ?`, start, opsTopUsers).Scan(&trows).Error; err != nil {
		return nil, err
	}
	if len(trows) > 0 {
		ids := make([]int64, 0, len(trows))
		for _, r := range trows {
			ids = append(ids, r.UserID)
		}
		var names []struct {
			ID       int64
			NickName string
		}
		if err := s.DB.WithContext(ctx).Raw(`SELECT id, nick_name FROM tu_user WHERE id IN ?`, ids).Scan(&names).Error; err != nil {
			return nil, err
		}
		nameOf := make(map[int64]string, len(names))
		for _, n := range names {
			nameOf[n.ID] = n.NickName
		}
		type contentRow struct {
			UserID int64
			Notes  int64
			Todos  int64
			Memos  int64
		}
		var contents []contentRow
		if err := s.DB.WithContext(ctx).Raw(`
			SELECT user_id, sum(notes) AS notes, sum(todos) AS todos, sum(memos) AS memos FROM (
			  SELECT user_id, count(*) AS notes, 0 AS todos, 0 AS memos FROM tf_note WHERE is_deleted = 0 AND user_id IN ? GROUP BY user_id
			  UNION ALL
			  SELECT user_id, 0, count(*), 0 FROM tf_todo WHERE is_deleted = 0 AND user_id IN ? GROUP BY user_id
			  UNION ALL
			  SELECT user_id, 0, 0, count(*) FROM tf_memo WHERE is_deleted = 0 AND user_id IN ? GROUP BY user_id
			) t GROUP BY user_id`, ids, ids, ids).Scan(&contents).Error; err != nil {
			return nil, err
		}
		contentOf := make(map[int64]contentRow, len(contents))
		for _, c := range contents {
			contentOf[c.UserID] = c
		}
		out.TopUsers = make([]OpsTopUser, 0, len(trows))
		for _, r := range trows {
			nick := nameOf[r.UserID]
			if nick == "" {
				nick = fmt.Sprintf("用户#%d", r.UserID)
			}
			c := contentOf[r.UserID]
			out.TopUsers = append(out.TopUsers, OpsTopUser{
				UserID: r.UserID, NickName: nick, Calls: r.Calls,
				Notes: c.Notes, Todos: c.Todos, Memos: c.Memos, LastActive: r.LastActive,
			})
		}
	}
	return out, nil
}

// scanDaily 跑一条「按天分组计数」查询并写入对应桶（窗口外的行忽略）。
func (s *OpsService) scanDaily(ctx context.Context, idx map[string]*OpsDay, sql string, args []any, set func(d *OpsDay, n int64)) error {
	var rows []struct {
		Day   time.Time
		Count int64
	}
	if err := s.DB.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if d, ok := idx[r.Day.Format("2006-01-02")]; ok {
			set(d, r.Count)
		}
	}
	return nil
}

// localUTCOffset 服务器本地时区相对 UTC 的固定偏移（"+08:00"），供 SQL 侧 AT TIME ZONE 分桶。
// 用固定偏移而非时区名：与 Go 侧 time.Local 判定一致，且不受 DB 会话时区影响（生产 TZ=Asia/Shanghai）。
func localUTCOffset() string {
	_, sec := time.Now().Zone()
	sign := "+"
	if sec < 0 {
		sign, sec = "-", -sec
	}
	return fmt.Sprintf("%s%02d:%02d", sign, sec/3600, (sec%3600)/60)
}
