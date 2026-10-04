package service

import (
	"testing"

	"extbrain-server/internal/model"
)

// 审计噪音治理：只砍 Web 来源的读类成功；AI/Key 来源全量照记；
// export.read / share.read 白名单；失败（>=400）永不跳过；匿名请求精确归类（poll/vapid 不记）。
func TestAuditSkipRules(t *testing.T) {
	cases := []struct {
		name   string
		action string
		status int
		keyID  int64
		want   bool
	}{
		{"Web 读成功=跳过", "todo.read", 200, 0, true},
		{"Web dashboard.read=跳过", "dashboard.read", 200, 0, true},
		{"Web search.read=跳过", "search.read", 200, 0, true},
		{"Web 读失败(探测)=保留", "note.read", 403, 0, false},
		{"AI 读成功=保留", "todo.read", 200, 9, false},
		{"AI 读失败=保留", "note.read", 403, 9, false},
		{"Web 写=保留", "note.upsert", 200, 0, false},
		{"Web 删=保留", "note.delete", 200, 0, false},
		{"导出读=白名单保留", "export.read", 200, 0, false},
		{"分享读=白名单保留", "share.read", 200, 0, false},
		{"排序偏好 upsert=保留", "todo-sort.upsert", 200, 0, false},
		{"仓库读=Web 跳过", "repo.read", 200, 0, true},
	}
	for _, c := range cases {
		if got := skipWebRead(c.action, int64(c.status), c.keyID); got != c.want {
			t.Errorf("%s: skipWebRead(%s,%d,%d)=%v want %v", c.name, c.action, c.status, c.keyID, got, c.want)
		}
	}

	anon := []struct {
		name     string
		fp       string
		status   int
		wantAct  string
		wantSkip bool
	}{
		{"cli-auth poll=不记", "/api/v1/cli-auth/poll", 200, "", true},
		{"cli-auth start=不记", "/api/v1/cli-auth/start", 200, "", true},
		{"vapid=不记", "/api/v1/push/vapid", 200, "", true},
		{"登录成功=web.login", "/api/v1/auth/login", 200, ActionWebLogin, false},
		{"登录失败=auth.failed", "/api/v1/auth/login", 401, ActionAuthFailed, false},
		{"注册成功=register", "/api/v1/auth/register", 200, string(model.ActionRegister), false},
		{"公开分享读取=share.read", "/api/v1/public/share/:token", 404, string(model.ActionShareRead), false},
		{"未认证探测受保护接口=auth.failed", "/api/v1/notes/ai/x.md", 401, ActionAuthFailed, false},
		{"其余匿名成功=不记", "/api/v1/whatever", 200, "", true},
	}
	for _, c := range anon {
		act, skip := anonAction(c.fp, c.status)
		if skip != c.wantSkip || act != c.wantAct {
			t.Errorf("%s: anonAction(%s,%d)=(%q,%v) want (%q,%v)", c.name, c.fp, c.status, act, skip, c.wantAct, c.wantSkip)
		}
	}
}
