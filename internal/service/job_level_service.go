package service

import (
	"context"
	"sort"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// JobLevelService 职级服务（Spec 第六阶段 Q4 / Q4.1 = A）。
//
// 职级**不参与授权**：它只是用户属性，用途固定为三处——用户表单/详情下拉、用户列表筛选与排序、
// 组织概览分布统计。维护方式与职务同构：本地 CRUD + 身份源同步（只拉不推）。
type JobLevelService struct {
	levels repository.JobLevelRepository
	orgs   repository.InstitutionRepository
	audits *AuditRecorder
}

// NewJobLevelService 创建职级服务
func NewJobLevelService(levels repository.JobLevelRepository, orgs repository.InstitutionRepository, audits *AuditRecorder) *JobLevelService {
	return &JobLevelService{levels: levels, orgs: orgs, audits: audits}
}

// Create 创建职级
func (s *JobLevelService) Create(ctx context.Context, actor string, l *repository.JobLevel) (*repository.JobLevel, error) {
	if l.InstitutionID == 0 || l.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, l.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", l.InstitutionID)
	}
	if _, err := s.levels.GetByName(ctx, l.InstitutionID, l.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "job level already exists")
	}
	if l.SortOrder <= 0 {
		l.SortOrder = 100
	}
	l.IsEnabled = true
	id, err := s.levels.Create(ctx, l)
	if err != nil {
		return nil, err
	}
	l.ID = id
	s.audits.Record(ctx, actor, l.InstitutionID, "job_level.create", l.Name, nil)
	return l, nil
}

// Update 更新职级（标识不可改；空值保持原值）
func (s *JobLevelService) Update(ctx context.Context, actor string, l *repository.JobLevel) error {
	existing, err := s.levels.GetByName(ctx, l.InstitutionID, l.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("job_level", l.Name)
	}
	l.ID = existing.ID
	if l.Description == "" {
		l.Description = existing.Description
	}
	if l.SortOrder <= 0 {
		l.SortOrder = existing.SortOrder
	}
	if err := s.levels.Update(ctx, l); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, l.InstitutionID, "job_level.update", l.Name, nil)
	return nil
}

// Delete 删除职级
func (s *JobLevelService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	l, err := s.levels.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("job_level", name)
	}
	if err := s.levels.Delete(ctx, l.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "job_level.delete", name, nil)
	return nil
}

// Get 查询职级
func (s *JobLevelService) Get(ctx context.Context, institutionID int64, name string) (*repository.JobLevel, error) {
	l, err := s.levels.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("job_level", name)
	}
	return l, nil
}

// NameByID 按主键解析职级名（对外路径用 id，服务层以 name 语义访问存储）
func (s *JobLevelService) NameByID(ctx context.Context, id int64) (string, error) {
	l, err := s.levels.GetByID(ctx, id)
	if err != nil {
		return "", bizerrors.ErrNotFound.WithDetail("job_level_id", id)
	}
	return l.Name, nil
}

// List 分页列出职级（按 sort_order 升序）
func (s *JobLevelService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.JobLevel, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.levels.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.levels.Count(ctx, q)
	return items, total, err
}

// Distribution 组织内职级分布（各级别人数；零人职级也返回）
type LevelDistribution struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Distribution 返回职级分布（组织概览用）
func (s *JobLevelService) Distribution(ctx context.Context, institutionID int64) ([]LevelDistribution, error) {
	levels, err := s.levels.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 200})
	if err != nil {
		return nil, err
	}
	counts, err := s.levels.CountByLevel(ctx, institutionID)
	if err != nil {
		return nil, err
	}
	out := make([]LevelDistribution, 0, len(levels))
	for _, l := range levels {
		out = append(out, LevelDistribution{Name: l.Name, Count: counts[l.ID]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
