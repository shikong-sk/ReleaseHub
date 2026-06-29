package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// SyncTotal 同步执行总数
	SyncTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasehub_sync_total",
		Help: "Total number of sync executions",
	}, []string{"repository", "status"})

	// DownloadBytes 下载总字节数
	DownloadBytes = promauto.NewCounter(prometheus.CounterOpts{
		Name: "releasehub_download_bytes_total",
		Help: "Total bytes downloaded",
	})

	// DownloadTotal 下载执行总数
	DownloadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasehub_download_total",
		Help: "Total number of download executions",
	}, []string{"status"})

	// APIRequests API 请求总数
	APIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "releasehub_api_requests_total",
		Help: "Total number of API requests",
	}, []string{"method", "path", "status"})

	// ActiveTasks 当前活跃任务数
	ActiveTasks = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "releasehub_active_tasks",
		Help: "Current number of active tasks",
	})

	// RepositoriesCurrent 当前仓库数
	RepositoriesCurrent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "releasehub_repositories_current",
		Help: "Current number of repositories",
	})

	// AssetsCurrent 当前资产数
	AssetsCurrent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "releasehub_assets_current",
		Help: "Current number of assets",
	})
)
