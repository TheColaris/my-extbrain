package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"extbrain-server/internal/auth"
	"extbrain-server/internal/model"

	"github.com/lib/pq"
)

// 账户资料扩展：部分更新（昵称/头像）、非法值拒绝、改密/换绑 token_version +1
func TestAccountProfileFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_todo, tf_memo")
	cs := &CacheService{DB: db}
	authSvc := &AuthService{DB: db, Cache: cs, JWT: auth.NewManager("test-secret")}
	acc := &AccountService{DB: db, Auth: authSvc}

	// 邮箱注册（验证码直放缓存模拟发码；手机号注册已下掉）
	_ = cs.Set(ctx, emailCodeKey("profile-flow@test.dev"), "123456", emailCodeTTL)
	u, err := authSvc.Register(ctx, RegisterInput{Account: "profile-flow@test.dev", Password: "password123", EmailCode: "123456"})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if u.TokenVersion != 0 {
		t.Fatalf("初始 token_version 应为 0: %d", u.TokenVersion)
	}

	// 部分更新：只改头像
	emoji, bg := "🧠", "yellow"
	got, err := acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{AvatarEmoji: &emoji, AvatarBg: &bg})
	if err != nil || got.AvatarEmoji != "🧠" || got.AvatarBg != "yellow" || got.NickName != u.NickName {
		t.Fatalf("头像更新异常: %v %+v", err, got)
	}
	// 只改昵称（头像不动）
	nick := "新昵称"
	got, err = acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{NickName: &nick})
	if err != nil || got.NickName != "新昵称" || got.AvatarEmoji != "🧠" {
		t.Fatalf("昵称更新异常: %v %+v", err, got)
	}
	// 非法值
	badBg := "rainbow"
	if _, err := acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{AvatarBg: &badBg}); err == nil {
		t.Fatal("非法底色应报错")
	}
	empty := ""
	if _, err := acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{NickName: &empty}); err == nil {
		t.Fatal("空昵称应报错")
	}
	if _, err := acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{}); err == nil {
		t.Fatal("空更新应报错")
	}
	// 恢复默认头像
	none := ""
	got, err = acc.UpdateProfile(ctx, u.ID, UpdateProfileInput{AvatarEmoji: &none, AvatarBg: &none})
	if err != nil || got.AvatarEmoji != "" || got.AvatarBg != "" {
		t.Fatalf("恢复默认异常: %v %+v", err, got)
	}

	// 改密：token_version +1，返回新版本；旧密码失效
	u2, err := acc.ChangePassword(ctx, u.ID, "password123", "newpassword456")
	if err != nil || u2.TokenVersion != 1 {
		t.Fatalf("改密应 +1 版本: %v %+v", err, u2)
	}
	if _, err := acc.ChangePassword(ctx, u.ID, "password123", "anotherpass789"); err == nil {
		t.Fatal("旧密码应已失效")
	}
	// 换绑：token_version 再 +1
	u3, err := acc.Bind(ctx, u.ID, model.BindChannelEmail, "profile@test.dev", "newpassword456")
	if err != nil || u3.TokenVersion != 2 || u3.Email == nil || *u3.Email != "profile@test.dev" {
		t.Fatalf("换绑应 +1 版本: %v %+v", err, u3)
	}
	// Me 返回带头像与时间
	me, err := acc.Me(ctx, u.ID)
	if err != nil || me.CreateTime.IsZero() {
		t.Fatalf("Me 异常: %v %+v", err, me)
	}
}

// 导出：zip 内含 notes 目录树 + todos/memos JSON + README
func TestExportFlow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	db.Exec("TRUNCATE tf_note, tf_note_content, tf_todo, tf_memo")
	notes := &NoteService{DB: db}
	uid := int64(77)
	rid := testRepoID(t, db, uid)

	if _, _, err := notes.Upsert(ctx, uid, rid, "导出/笔记一.md", NoteUpsert{Content: "# 笔记一\n内容 A", Title: "笔记一", Tags: []string{"t"}}, nil); err != nil {
		t.Fatalf("建笔记失败: %v", err)
	}
	if _, _, err := notes.Upsert(ctx, uid, rid, "根级笔记.md", NoteUpsert{Content: "根级内容"}, nil); err != nil {
		t.Fatalf("建笔记失败: %v", err)
	}
	if err := db.Create(&model.Todo{UserID: uid, Title: "导出待办", Status: model.TodoStatusActive, Source: model.TodoSourceWeb, Tags: pq.StringArray{}}).Error; err != nil {
		t.Fatalf("建待办失败: %v", err)
	}
	if err := db.Create(&model.Memo{UserID: uid, Content: "导出便签", Tags: pq.StringArray{}}).Error; err != nil {
		t.Fatalf("建便签失败: %v", err)
	}
	// 其他用户数据不应出现
	db.Create(&model.Memo{UserID: uid + 1, Content: "别人的便签", Tags: pq.StringArray{}})

	var buf bytes.Buffer
	if err := (&ExportService{DB: db}).Export(ctx, uid, &buf, "extbrain-export-test"); err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip 解析失败: %v", err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	if c, ok := files["extbrain-export-test/默认仓库/notes/导出/笔记一.md"]; !ok || c != "# 笔记一\n内容 A" {
		t.Fatalf("notes 目录树异常: %v", keys(files))
	}
	if _, ok := files["extbrain-export-test/默认仓库/notes/根级笔记.md"]; !ok {
		t.Fatalf("根级笔记缺失: %v", keys(files))
	}
	var todos []map[string]any
	if err := json.Unmarshal([]byte(files["extbrain-export-test/todos.json"]), &todos); err != nil || len(todos) != 1 || todos[0]["title"] != "导出待办" {
		t.Fatalf("todos.json 异常: %v %v", err, files["extbrain-export-test/todos.json"])
	}
	var memos []map[string]any
	if err := json.Unmarshal([]byte(files["extbrain-export-test/memos.json"]), &memos); err != nil || len(memos) != 1 || memos[0]["content"] != "导出便签" {
		t.Fatalf("memos.json 异常（跨用户隔离）: %v %v", err, files["extbrain-export-test/memos.json"])
	}
	if !strings.Contains(files["extbrain-export-test/README.txt"], "my-extbrain 数据导出") {
		t.Fatal("README 缺失")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
