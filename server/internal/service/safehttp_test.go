package service

import (
	"net"
	"testing"
)

func TestIsPublicHTTPS(t *testing.T) {
	ok := []string{
		"https://oapi.dingtalk.com/robot/send?access_token=x",
		"https://open.feishu.cn/open-apis/bot/v2/hook/x",
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://example.com:8443/hook",
	}
	bad := []string{
		"http://example.com/hook",        // 非 https
		"https://localhost/hook",         // localhost
		"https://foo.localhost/hook",     // *.localhost
		"https://router.local/hook",      // *.local
		"https://127.0.0.1/hook",         // 回环
		"https://10.1.2.3/hook",          // 私网
		"https://192.168.1.1/hook",       // 私网
		"https://100.64.0.1/hook",        // CGNAT
		"https://169.254.169.254/latest", // 链路本地（云元数据）
		"https://[::1]/hook",             // IPv6 回环
		"https://[fd00::1]/hook",         // IPv6 ULA
		"https:///hook",                  // 无主机
		"not-a-url",
	}
	for _, u := range ok {
		if !IsPublicHTTPS(u) {
			t.Fatalf("应允许: %s", u)
		}
	}
	for _, u := range bad {
		if IsPublicHTTPS(u) {
			t.Fatalf("应拒绝: %s", u)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1", "169.254.169.254", "100.64.1.2", "0.0.0.0", "::1", "fd00::1", "fe80::1", "224.0.0.1"}
	allowed := []string{"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888"}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Fatalf("应拦截: %s", s)
		}
	}
	for _, s := range allowed {
		if isBlockedIP(net.ParseIP(s)) {
			t.Fatalf("应允许: %s", s)
		}
	}
	if !isBlockedIP(nil) {
		t.Fatal("nil IP 应拦截")
	}
}
