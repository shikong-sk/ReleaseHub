package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"releasehub/backend/internal/api"
	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"
	githubsvc "releasehub/backend/internal/services/github"
	releasesvc "releasehub/backend/internal/services/release"
	retentionsvc "releasehub/backend/internal/services/retention"
	schedulersvc "releasehub/backend/internal/services/scheduler"
	providersvc "releasehub/backend/internal/services/provider"
	syncersvc "releasehub/backend/internal/services/syncer"
	tasklogsvc "releasehub/backend/internal/services/tasklog"
	auditlogsvc "releasehub/backend/internal/services/auditlog"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger, err := zap.NewProduction()
	if cfg.App.Env == "development" {
		logger, err = zap.NewDevelopment()
	}
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	db, err := database.Open(cfg.Database)
	if err != nil {
		logger.Fatal("数据库初始化失败", zap.Error(err))
	}

	if err := database.Migrate(db); err != nil {
		logger.Fatal("数据库迁移失败", zap.Error(err))
	}

	// 确保至少存在一个管理员账户（默认 admin/admin，可通过环境变量 RELEASEHUB_AUTH_DEFAULT_ADMIN/RELEASEHUB_AUTH_DEFAULT_PASSWORD 配置）
	if err := database.SeedDefaultAdmin(db, cfg.Auth.DefaultAdmin, cfg.Auth.DefaultPassword); err != nil {
		logger.Warn("创建默认管理员失败", zap.Error(err))
	}

	if err := database.SeedDefaultStorage(db, cfg.Storage.DataDir); err != nil {
		logger.Warn("创建默认本地存储失败", zap.Error(err))
	}

	// 回填已有资产的 StorageID（存量数据迁移，幂等操作）
	if err := database.BackfillAssetStorageID(db); err != nil {
		logger.Warn("回填资产 StorageID 失败", zap.Error(err))
	}

	// 清理软删除数据并迁移到硬删除（一次性迁移）
	if err := database.MigrateDropDeletedAt(db); err != nil {
		logger.Warn("清理软删除数据失败", zap.Error(err))
	}

	// 迁移 Asset 唯一索引（支持多存储）
	if err := database.MigrateAssetUniqueIndex(db); err != nil {
		logger.Warn("迁移 Asset 唯一索引失败", zap.Error(err))
	}

	// 回填存量资产的 download_bytes（已下载但 download_bytes=0 的用 size 补上，一次性）
	if err := database.MigrateBackfillDownloadBytes(db); err != nil {
		logger.Warn("回填 download_bytes 失败", zap.Error(err))
	}

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()

	// 启动时加载持久化配置（auth.enabled / syncer 并发数）到 cfg，
	// 必须在 syncService 初始化之前完成，确保持久化值能注入运行时
	api.LoadPersistedSettings(db, cfg)

	var scheduler *schedulersvc.Service
	var syncService *syncersvc.Service
	if cfg.Scheduler.Enabled {
		githubClient, err := githubsvc.NewClient(cfg.GitHub.APIBaseURL)
		if err != nil {
			logger.Fatal("GitHub Client 初始化失败", zap.Error(err))
		}
		checkService := releasesvc.NewCheckService(db, githubClient).
			WithGitHubFactory(githubsvc.NewClientFactory(cfg.GitHub.APIBaseURL, db)).
			WithProviderRegistry(providersvc.NewRegistry(cfg.GitHub.APIBaseURL))
		retentionService := retentionsvc.NewServiceWithFactory(db, cfg.Storage)
		checkService.WithRetention(retentionService)

		syncService, err = syncersvc.NewService(db, checkService, cfg.Storage)
		if err != nil {
			logger.Fatal("同步服务初始化失败", zap.Error(err))
		}

		scheduler = schedulersvc.NewServiceWithConcurrency(
			db,
			checkService,
			logger,
			time.Duration(cfg.Scheduler.TickSeconds)*time.Second,
			cfg.Scheduler.MaxConcurrent,
		)
		// 定时任务执行同步（检查+下载），而非仅检查
		scheduler.WithSyncer(syncService)
		scheduler.Start(appCtx)
	}

	// 应用 syncer 并发配置到运行时（此时 cfg.Syncer 已含持久化值或环境变量默认值）
	if syncService != nil {
		syncService.UpdateMaxConcurrentTasks(cfg.Syncer.MaxConcurrentTasks)
		syncService.UpdateMaxConcurrentDownloads(cfg.Syncer.MaxConcurrentDownloads)
	}

	// 启动任务日志保留清理 goroutine：每小时清理一次超过保留天数的日志，
	// RetentionDays 为 0 时跳过清理。cfg 为指针，运行时通过配置 API 修改后立即生效。
	taskLogSvc := tasklogsvc.NewService(db)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-appCtx.Done():
				return
			case <-ticker.C:
				days := cfg.TaskLog.RetentionDays
				if days <= 0 {
					continue
				}
				if deleted, err := taskLogSvc.Cleanup(appCtx, days); err != nil {
					logger.Warn("清理过期任务日志失败", zap.Error(err))
				} else if deleted > 0 {
					logger.Info("清理过期任务日志", zap.Int64("deleted", deleted), zap.Int("retentionDays", days))
				}
			}
		}
	}()

	// 启动操作日志保留清理 goroutine：每小时清理一次超过保留天数的操作日志，
	// RetentionDays 为 0 时跳过清理。cfg 为指针，运行时通过配置 API 修改后立即生效。
	opLogSvc := auditlogsvc.NewService(db)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-appCtx.Done():
				return
			case <-ticker.C:
				days := cfg.OpLog.RetentionDays
				if days <= 0 {
					continue
				}
				if deleted, err := opLogSvc.Cleanup(appCtx, days); err != nil {
					logger.Warn("清理过期操作日志失败", zap.Error(err))
				} else if deleted > 0 {
					logger.Info("清理过期操作日志", zap.Int64("deleted", deleted), zap.Int("retentionDays", days))
				}
			}
		}
	}()

	router := api.NewRouter(api.Dependencies{
		Config:        cfg,
		DB:            db,
		Logger:        logger,
		Scheduler:     scheduler,
		Syncer:        syncService,
		SyncerService: syncService,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("ReleaseHub API 服务启动", zap.String("addr", cfg.HTTP.Addr()))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("HTTP 服务异常退出", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	stopApp()

	// 停止 syncer worker pool，等待在途任务完成，避免 goroutine 泄漏与半截任务状态
	if syncService != nil {
		syncService.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("HTTP 服务关闭失败", zap.Error(err))
	}

	logger.Info("ReleaseHub API 服务已关闭")
}
