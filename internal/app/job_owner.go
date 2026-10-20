package app

import (
	"context"

	"polaris/internal/handler"
	"polaris/internal/repository"
)

// jobOwnerResolver 任务来源解析：按任务主键批量反查业务侧的绑定列。
//
// asyncd 是通用调度模块（不存任何业务归属），因此"这条任务来自哪个身份连接 / 应用供给"
// 只能在展示层聚合。当前只有入方向连接的「同步计划」持有任务；出方向接入后在这里追加反查。
type jobOwnerResolver struct {
	syncers    repository.SyncerRepository
	connectors repository.ConnectorRepository
}

// ResolveJobOwners 反查持有这些任务的业务资源（未命中 = 代码声明的平台任务）。
func (r *jobOwnerResolver) ResolveJobOwners(ctx context.Context, jobIDs []int64) (map[int64]handler.JobOwner, error) {
	out := make(map[int64]handler.JobOwner, len(jobIDs))
	if len(jobIDs) == 0 || (r.syncers == nil && r.connectors == nil) {
		return out, nil
	}
	if r.syncers == nil {
		return out, nil
	}
	owners, err := r.syncers.ListByScheduleJobIDs(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	for _, sy := range owners {
		if sy.ScheduleJobID == nil {
			continue
		}
		out[*sy.ScheduleJobID] = handler.JobOwner{Kind: "syncer", ID: sy.ID, Name: sy.Name}
	}
	if r.connectors != nil {
		connectors, err := r.connectors.ListByScheduleJobIDs(ctx, jobIDs)
		if err != nil {
			return nil, err
		}
		for _, c := range connectors {
			if c.ScheduleJobID == nil {
				continue
			}
			out[*c.ScheduleJobID] = handler.JobOwner{Kind: "connector", ID: c.ID, Name: c.Name}
		}
	}
	return out, nil
}
