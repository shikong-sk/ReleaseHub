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

	router := api.NewRouter(api.Dependencies{
		Config: cfg,
		DB:     db,
		Logger: logger,
	})

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()

	if cfg.Scheduler.Enabled {
		githubClient, err := githubsvc.NewClient(cfg.GitHub.APIBaseURL)
		if err != nil {
			logger.Fatal("GitHub Client 初始化失败", zap.Error(err))
		}
		checkService := releasesvc.NewCheckService(db, githubClient)
		retentionService, err := retentionsvc.NewService(db, cfg.Storage)
		if err != nil {
			logger.Fatal("保留策略服务初始化失败", zap.Error(err))
		}
		checkService.WithRetention(retentionService)

		syncService, err := syncersvc.NewService(db, checkService, cfg.Storage)
		if err != nil {
			logger.Fatal("同步服务初始化失败", zap.Error(err))
		}

		scheduler := schedulersvc.NewService(
			db,
			checkService,
			logger,
			time.Duration(cfg.Scheduler.TickSeconds)*time.Second,
		)
		// 定时任务执行同步（检查+下载），而非仅检查
		scheduler.WithSyncer(syncService)
		scheduler.Start(appCtx)
	}

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
