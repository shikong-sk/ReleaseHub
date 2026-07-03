package auditlog

import (
	"context"
	"time"

	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

// Service 系统操作审计日志服务
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Record 记录一条操作日志。actor 为用户名或 "system"；action 为操作类型；
// resource 为受影响资源标识（如 "repository:42"）；status 为 "success" 或 "failed"。
func (s *Service) Record(ctx context.Context, actor, action, resource, detail, status, clientIP string) error {
	entry := models.OperationLog{
		Actor:     actor,
		Action:    action,
		Resource:  resource,
		Detail:    detail,
		Status:    status,
		ClientIP:  clientIP,
		CreatedAt: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Create(&entry).Error
}

// List 查询操作日志，支持按 actor/action/status 筛选，按时间倒序分页
func (s *Service) List(ctx context.Context, params ListParams) ([]models.OperationLog, int64, error) {
	query := s.db.WithContext(ctx).Model(&models.OperationLog{})
	if params.Actor != "" {
		query = query.Where("actor = ?", params.Actor)
	}
	if params.Action != "" {
		query = query.Where("action = ?", params.Action)
	}
	if params.Status != "" {
		query = query.Where("status = ?", params.Status)
	}
	if params.Keyword != "" {
		like := "%" + params.Keyword + "%"
		query = query.Where("detail LIKE ? OR resource LIKE ?", like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pageSize := params.PageSize
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 50
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}

	var logs []models.OperationLog
	err := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&logs).Error
	return logs, total, err
}

// ListParams 操作日志查询参数
type ListParams struct {
	Actor    string
	Action   string
	Status   string
	Keyword  string
	Page     int
	PageSize int
}

// Cleanup 删除早于指定保留天数的操作日志，返回删除行数
// retentionDays <= 0 时跳过清理
func (s *Service) Cleanup(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	result := s.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&models.OperationLog{})
	return result.RowsAffected, result.Error
}
