package api

import (
	"fmt"
	"net/http"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerMetricsRoutes(router *gin.Engine, db *gorm.DB) {
	router.GET("/api/metrics", metricsHandler(db))
}

func metricsHandler(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 简化的 Prometheus 格式指标导出
		var repoCount, enabledRepoCount int64
		var releaseCount, assetCount, verifiedAssetCount, failedAssetCount int64
		var totalSize int64
		var pendingTask, runningTask, failedTask, succeededTask int64

		db.WithContext(c.Request.Context()).Model(&models.Repository{}).Count(&repoCount)
		db.WithContext(c.Request.Context()).Model(&models.Repository{}).Where("enabled = ?", true).Count(&enabledRepoCount)
		db.WithContext(c.Request.Context()).Model(&models.Release{}).Count(&releaseCount)
		db.WithContext(c.Request.Context()).Model(&models.Asset{}).Count(&assetCount)
		db.WithContext(c.Request.Context()).Model(&models.Asset{}).Where("status = ?", "verified").Count(&verifiedAssetCount)
		db.WithContext(c.Request.Context()).Model(&models.Asset{}).Where("status = ?", "failed").Count(&failedAssetCount)
		db.WithContext(c.Request.Context()).Model(&models.Asset{}).Where("status = ?", "verified").Select("COALESCE(SUM(size), 0)").Scan(&totalSize)
		db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "pending").Count(&pendingTask)
		db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "running").Count(&runningTask)
		db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "failed").Count(&failedTask)
		db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "succeeded").Count(&succeededTask)

		metrics := fmt.Sprintf(`# HELP releasehub_repositories Total number of repositories
# TYPE releasehub_repositories gauge
releasehub_repositories{enabled="true"} %d
releasehub_repositories{enabled="false"} %d
releasehub_repositories_total %d

# HELP releasehub_releases Total number of releases
# TYPE releasehub_releases gauge
releasehub_releases_total %d

# HELP releasehub_assets Total number of assets
# TYPE releasehub_assets gauge
releasehub_assets{status="verified"} %d
releasehub_assets{status="failed"} %d
releasehub_assets_total %d

# HELP releasehub_storage_bytes Total storage used in bytes
# TYPE releasehub_storage_bytes gauge
releasehub_storage_bytes %d

# HELP releasehub_tasks Number of tasks by status
# TYPE releasehub_tasks gauge
releasehub_tasks{status="pending"} %d
releasehub_tasks{status="running"} %d
releasehub_tasks{status="failed"} %d
releasehub_tasks{status="succeeded"} %d
`,
			enabledRepoCount, repoCount-enabledRepoCount, repoCount,
			releaseCount,
			verifiedAssetCount, failedAssetCount, assetCount,
			totalSize,
			pendingTask, runningTask, failedTask, succeededTask,
		)

		c.Header("Content-Type", "text/plain; version=0.0.4")
		c.String(http.StatusOK, metrics)
	}
}
