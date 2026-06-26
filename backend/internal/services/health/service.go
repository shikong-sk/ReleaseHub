package health

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Status struct {
	Status    string            `json:"status"`
	Service   string            `json:"service"`
	Checks    map[string]string `json:"checks"`
	CheckedAt time.Time         `json:"checkedAt"`
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Check(ctx context.Context) Status {
	checks := map[string]string{
		"database": "ok",
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		checks["database"] = "unavailable"
		return newStatus("degraded", checks)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		checks["database"] = "unavailable"
		return newStatus("degraded", checks)
	}

	return newStatus("ok", checks)
}

func newStatus(status string, checks map[string]string) Status {
	return Status{
		Status:    status,
		Service:   "releasehub-api",
		Checks:    checks,
		CheckedAt: time.Now().UTC(),
	}
}
