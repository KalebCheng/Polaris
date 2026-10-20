package service

import (
	"context"

	"polaris/internal/repository"
)

// InstitutionStats 组织维度的资源聚合（用于组织概览与总览页）
type InstitutionStats struct {
	Institution  string           `json:"institutionID"`
	DisplayName  string           `json:"display_name,omitempty"`
	Counts       map[string]int64 `json:"counts"`
	Total        int64            `json:"total"`
	LastActivity string           `json:"last_activity,omitempty"`
}

// Stats 统计单个组织下的各类资源数量。
func (s *InstitutionService) Stats(ctx context.Context, id int64) (*InstitutionStats, error) {
	org, err := s.orgs.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	stats := &InstitutionStats{
		Institution: org.Name,
		DisplayName: org.DisplayName,
		Counts:      map[string]int64{},
	}
	q := repository.ListQuery{InstitutionID: id}
	collectors := map[string]func(context.Context, repository.ListQuery) (int64, error){
		"users":        s.users.Count,
		"job_titles":   s.jobTitles.Count,
		"job_levels":   s.jobLevels.Count,
		"roles":        s.roles.Count,
		"permissions":  s.permissions.Count,
		"applications": s.apps.Count,
		"providers":    s.providers.Count,
		"syncers":      s.syncers.Count,
		"tokens":       s.tokens.Count,
	}
	for key, count := range collectors {
		n, err := count(ctx, q)
		if err != nil {
			continue // 单项统计失败不影响整体概览
		}
		stats.Counts[key] = n
		stats.Total += n
	}
	if s.records != nil {
		if recent, err := s.records.List(ctx, repository.ListQuery{InstitutionID: id, Limit: 1}); err == nil && len(recent) > 0 {
			stats.LastActivity = recent[0].CreatedAt.Format("2006-01-02 15:04:05")
		}
	}
	return stats, nil
}

// StatsAll 统计全部组织的资源数量（总览页按组织聚合）。
func (s *InstitutionService) StatsAll(ctx context.Context) ([]*InstitutionStats, error) {
	orgs, err := s.orgs.List(ctx, repository.ListQuery{Limit: 200})
	if err != nil {
		return nil, err
	}
	stats := make([]*InstitutionStats, 0, len(orgs))
	for _, org := range orgs {
		item, err := s.Stats(ctx, org.ID)
		if err != nil {
			continue
		}
		stats = append(stats, item)
	}
	return stats, nil
}
