package service

import (
	"context"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// RecordService 审计记录服务
type RecordService struct {
	records repository.RecordRepository
}

// NewRecordService 创建审计记录服务
func NewRecordService(records repository.RecordRepository) *RecordService {
	return &RecordService{records: records}
}

// List 分页列出审计记录
func (s *RecordService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Record, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.records.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.records.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListForUser 分页列出当前用户自己的审计记录。
func (s *RecordService) ListForUser(ctx context.Context, institutionID int64, user string, offset, limit int) ([]*repository.Record, int64, error) {
	q := repository.ListQuery{
		InstitutionID: institutionID,
		Field:         `"user"`,
		Value:         user,
		Offset:        offset,
		Limit:         limit,
	}
	items, err := s.records.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.records.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get 查询审计记录
func (s *RecordService) Get(ctx context.Context, id int64) (*repository.Record, error) {
	r, err := s.records.GetByID(ctx, id)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("record", id)
	}
	return r, nil
}
