package model

import "testing"

// 枚举线格式基线：API 传输的字符串值一旦发布即是对外契约，禁止悄悄变更。
// web 侧（web/src/lib/enums.ts）与 model 枚举两处对齐。
func TestEnumsWireFormat(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{string(TodoStatusActive), "active"},
		{string(TodoStatusDone), "done"},
		{string(TodoSortCreated), "created"},
		{string(TodoSortDue), "due"},
		{string(TodoSourceWeb), "web"},
		{string(TodoSourceCLI), "cli"},
		{string(KeyScopeAll), "all"},
		{string(KeyScopeTodo), "todo"},
		{string(KeyScopeNotes), "notes"},
		{string(ChannelTypeDingTalk), "dingtalk"},
		{string(ChannelTypeFeishu), "feishu"},
		{string(BindChannelPhone), "phone"},
		{string(BindChannelEmail), "email"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("枚举线格式漂移: got %q want %q（对外契约禁止变更，需走版本化迁移）", c.got, c.want)
		}
	}
}

func TestTodoStatusValid(t *testing.T) {
	for _, ok := range []TodoStatus{TodoStatusActive, TodoStatusDone} {
		if !ok.Valid() {
			t.Errorf("%q 应为合法状态", ok)
		}
	}
	for _, bad := range []TodoStatus{"", "todo", "doing", "finished", "TODO"} {
		if bad.Valid() {
			t.Errorf("%q 应为非法状态", bad)
		}
	}
}

func TestTodoSortValid(t *testing.T) {
	for _, ok := range []TodoSort{TodoSortCreated, TodoSortDue} {
		if !ok.Valid() {
			t.Errorf("%q 应为合法排序", ok)
		}
	}
	for _, bad := range []TodoSort{"", "smart", "completed", "DUE"} {
		if bad.Valid() {
			t.Errorf("%q 应为非法排序", bad)
		}
	}
}

func TestKeyScopeAllows(t *testing.T) {
	if !KeyScopeAll.Allows(KeyScopeNotes) || !KeyScopeAll.Allows(KeyScopeTodo) {
		t.Error("all 应覆盖一切资源域")
	}
	if KeyScopeTodo.Allows(KeyScopeNotes) {
		t.Error("todo scope 不应覆盖 notes")
	}
	if !KeyScopeTodo.Allows(KeyScopeTodo) {
		t.Error("scope 应覆盖自身")
	}
}

// 审计动作线格式基线（对齐 web ACTION_LABEL 的 value）
func TestActionTypeWire(t *testing.T) {
	want := map[ActionType]string{
		ActionTodoCreate: "todo.create", ActionTodoUpdate: "todo.update",
		ActionTodoDelete: "todo.delete", ActionTodoRestore: "todo.restore", ActionTodoRead: "todo.read",
		ActionMemoCreate: "memo.create", ActionMemoUpdate: "memo.update",
		ActionMemoDelete: "memo.delete", ActionMemoRead: "memo.read",
		ActionNoteUpsert: "note.upsert", ActionNoteRead: "note.read",
		ActionNoteDelete: "note.delete", ActionNoteMove: "note.move",
		ActionNoteRestore: "note.restore", ActionSearch: "search.query",
		ActionKeyCreate: "key.create", ActionKeyUpdate: "key.update",
		ActionKeyDelete: "key.delete", ActionKeyRead: "key.read",
		ActionWebLogin: "web.login", ActionRegister: "auth.register",
		ActionAuthFailed: "auth.failed",
	}
	for a, w := range want {
		if string(a) != w {
			t.Errorf("%s != %s", a, w)
		}
	}
	if string(ActionOf("todo", "create")) != "todo.create" {
		t.Error("ActionOf 组装异常")
	}
}
