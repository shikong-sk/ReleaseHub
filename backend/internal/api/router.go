package api

import (
	"net/http"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/middleware"
	githubsvc "releasehub/backend/internal/services/github"
	"releasehub/backend/internal/services/health"

	"github.com/gin-gonic/gin"
	_ "releasehub/backend/internal/api/docs" // swag init 生成的文档
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/files"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Dependencies struct {
	Config *config.Config
	DB     *gorm.DB
	Logger *zap.Logger
	Scheduler SchedulerUpdater
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	healthService := health.NewService(deps.DB)

	// 公开接口
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, healthService.Check(c.Request.Context()))
	})
	router.GET("/api/metrics", metricsHandler(deps.DB))

	// Prometheus 指标端点
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Swagger UI
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 认证路由
	registerAuthRoutes(router, deps.DB)
	registerConfigRoutes(router, deps.Config, deps.Scheduler)

	if deps.Config.Auth.Enabled {
		router.Use(middleware.APIKeyOrAuth(deps.DB))
		router.Use(middleware.AuthorizeRequest())
	}

	// 核心 API
	githubClient, githubClientErr := githubsvc.NewClient(deps.Config.GitHub.APIBaseURL)
	registerRepositoryRoutes(router, deps.DB, deps.Config.Storage, deps.Config.GitHub.APIBaseURL, githubClient, githubClientErr)
	registerReleaseRoutes(router, deps.DB, deps.Config.Storage)
	registerTaskRoutes(router, deps.DB)
	registerFileRoutes(router, deps.DB)
	registerTokenRoutes(router, deps.DB)
	registerTokenHealthRoutes(router, deps.DB, deps.Config.GitHub.APIBaseURL)
	registerStorageRoutes(router, deps.DB)
	registerProxyRoutes(router, deps.DB)
	registerNotificationRoutes(router, deps.DB)
	registerFilterRoutes(router)
	registerSearchRoutes(router, deps.DB)
	registerStatsRoutes(router, deps.DB)
	registerUploadRoutes(router, deps.DB, deps.Config.Storage)
	registerReconcileRoutes(router, deps.DB, deps.Config.Storage, deps.Logger)
	registerAPIKeyRoutes(router, deps.DB)

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "接口不存在",
			"path":  c.Request.URL.Path,
		})
	})

	// 设置 JWT 密钥
	if deps.Config.App.JWTSecret != "" {
		middleware.SetJWTSecret(deps.Config.App.JWTSecret)
	}

	return router
}
