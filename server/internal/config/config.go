// Package config —— 环境变量配置（自托管友好：全部有合理默认值）。
package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL        string // Postgres 连接串（唯一 DB 入口：Supabase 或自托管）
	ListenAddr         string // HTTP 监听地址
	JWTSecret          string // Web 会话签名（必填，≥32 位随机串）
	RegisterEnabled    bool   // 注册开关（默认开）
	TrustedProxies     []string
	LogRetentionDays   int    // 审计日志/推送记录保留天数
	TrashRetentionDays int    // 回收站保留天数（超期硬删）
	MigrateAuto        bool   // 启动时自动执行迁移（默认开）
	EmbedFrontend      bool   // 服务 embed 的前端（release 构建为 true）
	DownloadsDir       string // CLI/Skill 分发包目录（/downloads 静态路由与 /install.sh）
	VapidPublicKey     string
	VapidPrivateKey    string
	InternalToken      string // pg_cron 调内部端点的共享密钥
	CronMode           string // internal=进程内定时（默认，self-host）；pg_cron=外部 pg_cron+pg_net 调内部端点
	PublicBaseURL      string // 对外服务基址（如 https://my.example.com）；配置后 /install.sh 不再反射请求 Host（防反代缓存投毒）
	MCPEnabled         bool   // MCP 接入开关（/mcp Streamable HTTP；默认开）
	E2EMode            bool   // E2E 洁净室意图门（internal/e2e 三重门之一；true 但门②③不过=拒绝启动）
}

func Load() *Config {
	return &Config{
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://extbrain:extbrain@127.0.0.1:5432/extbrain?sslmode=disable"),
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		JWTSecret:          getEnv("JWT_SECRET", ""),
		RegisterEnabled:    getEnv("REGISTER_ENABLED", "true") == "true",
		TrustedProxies:     splitCSV(getEnv("TRUSTED_PROXIES", "127.0.0.1")),
		LogRetentionDays:   getEnvInt("LOG_RETENTION_DAYS", 90),
		TrashRetentionDays: getEnvInt("TRASH_RETENTION_DAYS", 30),
		MigrateAuto:        getEnv("MIGRATE_AUTO", "true") == "true",
		EmbedFrontend:      getEnv("EMBED_FRONTEND", "true") == "true",
		DownloadsDir:       getEnv("DOWNLOADS_DIR", "./downloads"),
		VapidPublicKey:     getEnv("VAPID_PUBLIC_KEY", ""),
		VapidPrivateKey:    getEnv("VAPID_PRIVATE_KEY", ""),
		InternalToken:      getEnv("INTERNAL_TOKEN", ""),
		CronMode:           getEnv("CRON_MODE", "internal"),
		PublicBaseURL:      strings.TrimSuffix(getEnv("PUBLIC_BASE_URL", ""), "/"),
		MCPEnabled:         getEnv("MCP_ENABLED", "true") == "true",
		E2EMode:            getEnv("E2E_MODE", "") == "true",
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
