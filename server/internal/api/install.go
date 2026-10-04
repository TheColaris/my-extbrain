package api

import (
	_ "embed"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed install.sh
var installScript string

//go:embed guide.md
var guideDoc string

//go:embed guide-mcp.md
var guideMcpDoc string

// hostRe HTTP Host 的安全子集：`;`/引号/`$`/反引号等 shell 元字符是合法 Host 字符，
// 但会随回退路径进入脚本文本（curl | sh 执行），回退前必须校验。
var hostRe = regexp.MustCompile(`^[0-9A-Za-z.\-:\[\]]+$`)

// resolveBase 解析对外服务地址：优先配置 PUBLIC_BASE_URL（推荐：反代/CDN 缓存场景下
// 请求 Host 可被伪造注入，配置后内容恒定）；未配置时回退反射请求 Host。
// ok=false = 未配置 PUBLIC_BASE_URL 且 Host 不合法（调用方应 400）。
func resolveBase(publicBase string, c *gin.Context) (string, bool) {
	base := strings.TrimSuffix(publicBase, "/")
	if base != "" {
		return base, true
	}
	host := c.Request.Host
	if !hostRe.MatchString(host) {
		return "", false
	}
	scheme := "http"
	if c.GetHeader("X-Forwarded-Proto") == "https" || c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + host, true
}

// InstallSh GET /install.sh —— 一键安装脚本（__BASE__ 注入当前服务地址）。恒 no-store 防中间层缓存投毒。
func InstallSh(publicBase string) gin.HandlerFunc {
	return func(c *gin.Context) {
		base, ok := resolveBase(publicBase, c)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": "Host 头不合法（未配置 PUBLIC_BASE_URL 时拒绝反射可疑 Host）"}})
			return
		}
		c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusOK, "%s", strings.ReplaceAll(installScript, "__BASE__", base))
	}
}

// GuideMd GET /guide.md —— 操作手册（Skill/CLI 版；{{BASE}} 注入当前服务地址；可整份交给 AI）。
// 占位符用 {{BASE}} 而非 __BASE__：markdown 会把 __x__ 解析成粗体。
func GuideMd(publicBase string) gin.HandlerFunc {
	return func(c *gin.Context) {
		base, ok := resolveBase(publicBase, c)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": "Host 头不合法"}})
			return
		}
		c.Header("Content-Type", "text/markdown; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusOK, "%s", strings.ReplaceAll(guideDoc, "{{BASE}}", base))
	}
}

// GuideMcpMd GET /guide-mcp.md —— MCP 接入手册（与 /guide.md 同口径；二选一）。
func GuideMcpMd(publicBase string) gin.HandlerFunc {
	return func(c *gin.Context) {
		base, ok := resolveBase(publicBase, c)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "invalid_host", "message": "Host 头不合法"}})
			return
		}
		c.Header("Content-Type", "text/markdown; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusOK, "%s", strings.ReplaceAll(guideMcpDoc, "{{BASE}}", base))
	}
}

// Downloads 静态目录（CLI 四平台包 + skill 包）；目录缺失时 404 明确提示
func Downloads(dir string) gin.HandlerFunc {
	fs := http.FileServer(http.Dir(dir))
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": gin.H{"code": "method_not_allowed", "message": "仅支持 GET"}})
			return
		}
		http.StripPrefix("/downloads", fs).ServeHTTP(c.Writer, c.Request)
	}
}
