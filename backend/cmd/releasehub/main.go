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

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()

	var scheduler *schedulersvc.Service
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

		syncService, err := syncersvc.NewService(db, checkService, cfg.Storage)
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

	router := api.NewRouter(api.Dependencies{
		Config:    cfg,
		DB:        db,
		Logger:    logger,
		Scheduler: scheduler,
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("HTTP 服务关闭失败", zap.Error(err))
	}

	logger.Info("ReleaseHub API 服务已关闭")
}
