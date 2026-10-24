package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// safehttp —— 用户可配置外部地址（机器人 Webhook / Web Push endpoint）的出站防护：
//  1. 只允许 https；
//  2. 连接时解析目标 IP，拒绝回环/私网/链路本地/组播/CGNAT 等内网地址（防 SSRF）；
//  3. 不跟随重定向（防 302 跳内网）；
//  4. 统一超时（外部地址不可控，必须封顶）。
//
// 说明：DNS 在校验后直接用解析结果连接，避免「校验一次、连接再解析」的 rebinding 窗口。

var errUnsafeTarget = errors.New("目标地址不允许（内网/回环地址）")

// IsPublicHTTPS 配置期快速校验（不做 DNS 解析；连接期由 NewSafeHTTPClient 兜底）。
func IsPublicHTTPS(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
		return false
	}
	return true
}

// NewSafeHTTPClient 带内网防护与超时的出站客户端。
func NewSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		// 不走环境代理：代理会代替我们解析目标，IP 校验就失效（SSRF 防护必须由本客户端直连）
		Proxy:                 nil,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("域名解析为空: %s", host)
			}
			for _, ip := range ips {
				if isBlockedIP(ip.IP) {
					return nil, errUnsafeTarget
				}
			}
			var lastErr error
			for _, ip := range ips {
				conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if derr == nil {
					return conn, nil
				}
				lastErr = derr
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("不允许重定向")
		},
	}
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 运营商级 NAT（IsPrivate 不覆盖）
		if v4[0] == 100 && v4[1]&0xc0 == 64 {
			return true
		}
		// 0.0.0.0/8
		if v4[0] == 0 {
			return true
		}
	}
	return false
}
