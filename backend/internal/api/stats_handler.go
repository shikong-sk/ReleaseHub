package api

import (
	"net/http"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type statsHandler struct {
	db *gorm.DB
}

type dashboardStats struct {
	TotalRepositories  int64 `json:"totalRepositories"`
	EnabledRepositories int64 `json:"enabledRepositories"`
	HealthyRepositories int64 `json:"healthyRepositories"`
	FailedRepositories  int64 `json:"failedRepositories"`
	TotalReleases      int64 `json:"totalReleases"`
	TotalAssets        int64 `json:"totalAssets"`
	DownloadedAssets   int64 `json:"downloadedAssets"`
	FailedAssets       int64 `json:"failedAssets"`
	TotalStorageBytes  int64 `json:"totalStorageBytes"`
	PendingTasks       int64 `json:"pendingTasks"`
	RunningTasks       int64 `json:"runningTasks"`
	FailedTasks        int64 `json:"failedTasks"`
}

type trendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type trendStats struct {
	Releases []trendPoint `json:"releases"`
	Assets   []trendPoint `json:"assets"`
}

func registerStatsRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &statsHandler{db: db}
	router.GET("/api/stats/dashboard", handler.dashboard)
	router.GET("/api/stats/trend", handler.trend)
}

func (h *statsHandler) dashboard(c *gin.Context) {
	var stats dashboardStats

	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).Count(&stats.TotalRepositories)
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).Where("enabled = ?", true).Count(&stats.EnabledRepositories)
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).Where("last_status = ?", "healthy").Count(&stats.HealthyRepositories)
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).Where("last_status = ?", "failed").Count(&stats.FailedRepositories)
	h.db.WithContext(c.Request.Context()).Model(&models.Release{}).Count(&stats.TotalReleases)
	h.db.WithContext(c.Request.Context()).Model(&models.Asset{}).Count(&stats.TotalAssets)
	h.db.WithContext(c.Request.Context()).Model(&models.Asset{}).Where("status = ?", "verified").Count(&stats.DownloadedAssets)
	h.db.WithContext(c.Request.Context()).Model(&models.Asset{}).Where("status = ?", "failed").Count(&stats.FailedAssets)
	h.db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "pending").Count(&stats.PendingTasks)
	h.db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "running").Count(&stats.RunningTasks)
	h.db.WithContext(c.Request.Context()).Model(&models.Task{}).Where("status = ?", "failed").Count(&stats.FailedTasks)

	// 存储用量
	var totalSize int64
	h.db.WithContext(c.Request.Context()).Model(&models.Asset{}).
		Where("status = ?", "verified").
		Select("COALESCE(SUM(size), 0)").
		Scan(&totalSize)
	stats.TotalStorageBytes = totalSize

	c.JSON(http.StatusOK, stats)
}

func (h *statsHandler) trend(c *gin.Context) {
	days := 30
	if d, err := time.ParseDuration(c.Query("days") + "h"); err == nil && d.Hours() > 0 && d.Hours() <= 365 {
		days = int(d.Hours() / 24)
	}

	releaseTrend := make([]trendPoint, 0, days)
	assetTrend := make([]trendPoint, 0, days)

	for i := days - 1; i >= 0; i-- {
		date := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		dayStart, _ := time.Parse("2006-01-02", date)
		dayEnd := dayStart.Add(24 * time.Hour)

		var releaseCount int64
		h.db.WithContext(c.Request.Context()).Model(&models.Release{}).
			Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
			Count(&releaseCount)
		releaseTrend = append(releaseTrend, trendPoint{Date: date, Count: releaseCount})

		var assetCount int64
		h.db.WithContext(c.Request.Context()).Model(&models.Asset{}).
			Where("downloaded_at >= ? AND downloaded_at < ?", dayStart, dayEnd).
			Count(&assetCount)
		assetTrend = append(assetTrend, trendPoint{Date: date, Count: assetCount})
	}

	c.JSON(http.StatusOK, trendStats{
		Releases: releaseTrend,
		Assets:   assetTrend,
	})
}
