package geoip

import "testing"

func TestSearch(t *testing.T) {
	cases := []struct{ ip, want string }{
		{"127.0.0.1", "内网"},
		{"192.168.1.10", "内网"},
		{"10.0.8.24", "内网"},
		{"114.114.114.114", "中国 · 江苏 · 南京信风"}, // 公共 DNS，验证真实查询链路
		{"8.8.8.8", "美国 · 加利福尼亚"},
	}
	for _, c := range cases {
		got := Search(c.ip)
		t.Logf("%s => %q", c.ip, got)
		if c.ip == "127.0.0.1" && got != "内网" {
			t.Errorf("回环应=内网 got %q", got)
		}
		if c.ip != "127.0.0.1" && c.ip != "192.168.1.10" && c.ip != "10.0.8.24" && got == "" {
			t.Errorf("公网 IP %s 查询不应为空", c.ip)
		}
	}
}
