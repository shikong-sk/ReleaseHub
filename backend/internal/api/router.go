package api

import (
	"net/http"

	"releasehub/backend/internal/config"
	githubsvc "releasehub/backend/internal/services/github"
	"releasehub/backend/internal/services/health"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Dependencies struct {
	Config *config.Config
	DB     *gorm.DB
	Logger *zap.Logger
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	healthService := health.NewService(deps.DB)

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, healthService.Check(c.Request.Context()))
	})
	githubClient, githubClientErr := githubsvc.NewClient(deps.Config.GitHub.APIBaseURL)
	registerRepositoryRoutes(router, deps.DB, deps.Config.Storage, githubClient, githubClientErr)
	registerReleaseRoutes(router, deps.DB, deps.Config.Storage)
	registerTaskRoutes(router, deps.DB)
	registerFileRoutes(router, deps.DB)
	registerTokenRoutes(router, deps.DB)
	registerConfigRoutes(router, deps.Config)

	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "接口不存在",
			"path":  c.Request.URL.Path,
		})
	})

	return router
}
