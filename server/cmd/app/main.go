// extbrain-server 入口：配置 → 迁移 → HTTP 服务。
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"extbrain-server/internal/api"
	authpkg "extbrain-server/internal/auth"
	"extbrain-server/internal/config"
	"extbrain-server/internal/e2e"
	"extbrain-server/internal/migrate"
	"extbrain-server/internal/model"
	"extbrain-server/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed all:dist
var distFS embed.FS

var version = "0.3.4-dev"

// e2ePubLimit/e2eV1Limit 洁净室放宽 per-IP 限流配额（普通模式用生产默认值）。
func e2ePubLimit() int {
	if e2e.Enabled() {
		return 200
	}
	return 10
}

func e2eV1Limit() int {
	if e2e.Enabled() {
		return 1200
	}
	return 240
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-gen-vapid" {
		api.RunGenVapid()
	}
	cfg := config.Load()

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// E2E 洁净室三重门（fail-closed）：E2E_MODE=true 但库名/标记行不过 → 拒绝启动
	if err := e2e.Evaluate(cfg.E2EMode, cfg.DatabaseURL, db); err != nil {
		log.Fatalf("%v", err)
	}

	if cfg.MigrateAuto {
		if err := migrate.Up(cfg.DatabaseURL); err != nil {
			log.Fatalf("迁移失败: %v", err)
		}
	}

	// 服务依赖装配
	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		// 空=本地开发兜底：随机生成（重启全会话失效，代价自担）；显式配置的弱值=要上生产，必须拦
		jwtSecret = authpkg.RandomSecret()
		log.Print("⚠ JWT_SECRET 未配置，已生成随机密钥（重启后 Web 会话全部失效）")
	} else if jwtSecret == "change-me-in-prod" || len(jwtSecret) < 32 {
		log.Fatalf("JWT_SECRET 为已知占位值或长度不足 32——仓库默认值是公开的，公网部署可被离线伪造任意账户 JWT，拒绝启动。生成: openssl rand -hex 32")
	}
	jwt := authpkg.NewManager(jwtSecret)
	pushLogSvc := &service.PushLogService{DB: db}
	// 平台配置 + 向量检索（迁移 0009；provider 未配置/调用失败时整链降级纯关键词）
	sysCfg := service.NewSysConfig(db)
	if err := sysCfg.Load(context.Background()); err != nil {
		log.Printf("⚠ 平台配置加载失败（按空配置启动，语义检索关闭）: %v", err)
	}
	indexSvc := service.NewIndexService(db, sysCfg)
	indexSvc.Start(context.Background())
	emailSvc := service.NewEmailService(db, sysCfg)
	svcs := &api.Deps{
		DB:      db,
		Cfg:     cfg,
		JWT:     jwt,
		Auth:    &service.AuthService{DB: db, Cache: &service.CacheService{DB: db}, JWT: jwt, Email: emailSvc},
		Keys:    &service.APIKeyService{DB: db},
		Logs:    &service.LogQueryService{DB: db},
		Todos:   &service.TodoService{DB: db},
		Memos:   &service.MemoService{DB: db},
		Dash:    &service.DashboardService{DB: db},
		Notes:   &service.NoteService{DB: db, Index: indexSvc, Vector: service.NewVectorSearcher(db, sysCfg)},
		Sys:     sysCfg,
		Email:   emailSvc,
		Index:   indexSvc,
		Ops:     &service.OpsService{DB: db},
		Notify:  service.NewNotifyService(db, pushLogSvc),
		CLIAuth: &service.CLIAuthService{Cache: &service.CacheService{DB: db}, Keys: &service.APIKeyService{DB: db}},
		Push: &service.PushService{
			DB: db, Cache: &service.CacheService{DB: db},
			Notify:      service.NewNotifyService(db, pushLogSvc),
			PushLog:     pushLogSvc,
			VapidPublic: cfg.VapidPublicKey, VapidPriv: cfg.VapidPrivateKey,
		},
		Account: &service.AccountService{DB: db},
		Audit:   service.NewAudit(db),
		Limiter: service.NewMemRateLimiter(60, time.Minute),
		// 公开端点 per-IP 限流（防无认证写放大灌库）：发码/注册/登录从严，poll 容纳 CLI 2s 轮询
		// 洁净室放宽：旅程多轮快跑会打满 10/min——限流逻辑自身由单测保证，洁净模式只放宽配额
		PublicLimiter: service.NewMemRateLimiter(e2ePubLimit(), time.Minute),
		PollLimiter:   service.NewMemRateLimiter(60, time.Minute),
		// v1 组级宽松限流：匿名 401 洪泛（伪造 token 打业务端点刷审计行）的最后防线
		V1Limiter: service.NewMemRateLimiter(e2eV1Limit(), time.Minute),
		Trash:     &service.TrashService{DB: db},
		PushLog:   pushLogSvc,
		Shares:    &service.ShareService{DB: db, Notes: &service.NoteService{DB: db}},
		Export:    &service.ExportService{DB: db},
		Perm:      &service.NotePermService{DB: db},
		Repos:     &service.RepoService{DB: db, Sys: sysCfg},

		Version: version, // MCP /mcp 的 serverInfo
	}

	r := api.Router(*svcs)

	// 前端静态托管（embed dist；release 构建时由 web 仓产物填充）
	dist, err := fs.Sub(distFS, "dist")
	if err == nil {
		r.NoRoute(func(c *gin.Context) {
			if c.Request.Method != http.MethodGet {
				c.JSON(404, gin.H{"error": gin.H{"code": "not_found", "message": "路由不存在"}})
				return
			}
			p := c.Request.URL.Path
			if p != "/" {
				if _, err := fs.Stat(dist, p[1:]); err == nil {
					c.FileFromFS(p, http.FS(dist))
					return
				}
			}
			// SPA 回退：直接回 index.html 字节（http.ServeFile 对显式 index.html 会 301 "./" 死循环）
			b, err := fs.ReadFile(dist, "index.html")
			if err != nil {
				c.JSON(404, gin.H{"error": gin.H{"code": "not_found", "message": "未找到资源"}})
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", b)
		})
	}

	// 到期扫描调度：internal=进程内每小时；pg_cron=外部调 /internal/push/due
	if cfg.CronMode == "internal" {
		go func() {
			for {
				time.Sleep(time.Hour)
				if n, err := svcs.Push.DueScan(context.Background()); err != nil {
					log.Printf("[cron] 到期扫描失败: %v", err)
				} else if n > 0 {
					log.Printf("[cron] 到期扫描完成，推送 %d 位用户", n)
				}
			}
		}()
	}

	// 保留期清理：每日一次——回收站硬删（TRASH_RETENTION_DAYS，默认 30）+
	// 审计日志/推送记录（LOG_RETENTION_DAYS，默认 90；此前只配未实现，此处一并落地）
	go func() {
		sweep := func() {
			ctx := context.Background()
			trashSvc := &service.TrashService{DB: db}
			if out, err := trashSvc.CleanupExpired(ctx, cfg.TrashRetentionDays); err != nil {
				log.Printf("[cron] 回收站清理失败: %v", err)
			} else {
				log.Printf("[cron] 回收站清理: todo=%d memo=%d note=%d", out["todo"], out["memo"], out["note"])
			}
			if n, err := pushLogSvc.CleanupRetention(ctx, cfg.LogRetentionDays); err != nil {
				log.Printf("[cron] 推送记录清理失败: %v", err)
			} else if n > 0 {
				log.Printf("[cron] 推送记录清理 %d 行", n)
			}
			// 无主切片清理（笔记被彻底删除后残留；搜索侧本就按 join 过滤，这里只回收空间）
			if n, err := indexSvc.CleanupOrphans(ctx); err != nil {
				log.Printf("[cron] 切片孤儿清理失败: %v", err)
			} else if n > 0 {
				log.Printf("[cron] 切片孤儿清理 %d 行", n)
			}
			r := db.WithContext(ctx).
				Where("create_time < ?", time.Now().AddDate(0, 0, -cfg.LogRetentionDays)).
				Delete(&model.APILog{})
			// sys_cache 过期行物理清理：过期删除是惰性的（读到才删），cli-auth 自造码/pwd_fail
			// 计数器若无后续读会永久驻留，必须主动 sweep
			if rc := db.WithContext(ctx).Where("expire_time < ?", time.Now()).Delete(&model.CacheEntry{}); rc.Error != nil {
				log.Printf("[cron] sys_cache 清理失败: %v", rc.Error)
			} else if rc.RowsAffected > 0 {
				log.Printf("[cron] sys_cache 清理 %d 行", rc.RowsAffected)
			}
			if r.Error != nil {
				log.Printf("[cron] 审计日志清理失败: %v", r.Error)
			} else if r.RowsAffected > 0 {
				log.Printf("[cron] 审计日志清理 %d 行", r.RowsAffected)
			}
		}
		sweep()
		for {
			time.Sleep(24 * time.Hour)
			sweep()
		}
	}()

	log.Printf("extbrain-server %s 监听 %s", version, cfg.ListenAddr)
	// ReadHeaderTimeout 防 slowloris；WriteTimeout 置 0——/mcp 的 GET 是长连接流，限时会掐断
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
