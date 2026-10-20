package service

import (
	"context"
	"fmt"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// TaskTypeConnectorReconcile 出方向「定时对齐检查」任务类型（asyncd 处理器注册名）。
//
// 到点执行 ProvisioningService.Reconcile 的保守方向：只对"下游确认缺失"的账号补建，
// ReconcileUnknown 绝不当作缺失（不删除下游账号）。
const TaskTypeConnectorReconcile = "connector.reconcile"

// connectorScheduler 出方向计划需要的 asyncd 能力（消费方定义的小接口，便于单测注入假实现）。
type connectorScheduler interface {
	UpsertJob(ctx context.Context, job asyncd.Job) (int64, error)
	SetEnabledByName(ctx context.Context, name string, enabled bool) error
	GetByID(ctx context.Context, id int64) (*repository.AsyncdJob, error)
}

// ConnectorScheduleJobName 连接器的计划任务定义名（稳定可重算，不随改名变化）。
func ConnectorScheduleJobName(connectorID int64) string {
	return fmt.Sprintf("connector.schedule:%d", connectorID)
}

// connectorScheduleJob 把连接器的「定时对齐检查」翻译成 asyncd 的任务定义。
// ok=false 表示这个连接器没有计划（「不自动」）→ 应该停用它的计划任务。
func connectorScheduleJob(c *repository.Connector) (asyncd.Job, bool) {
	spec := asyncd.Job{
		Name: ConnectorScheduleJobName(c.ID), TaskType: TaskTypeConnectorReconcile,
		Kind:      repository.AsyncdJobKindScheduled,
		UniqueKey: fmt.Sprintf("connector:%d", c.ID),
		Payload: map[string]any{
			"connector_id": c.ID, "institution_id": c.InstitutionID, "trigger": SyncTriggerSchedule,
		},
		Enabled: c.State == "active",
	}
	schedule := c.SyncConfig.Schedule
	switch schedule.Mode {
	case "cron":
		spec.Schedule = asyncd.Schedule{Kind: repository.ScheduleKindCron, Cron: schedule.Cron}
	case "daily":
		spec.Schedule = asyncd.Schedule{
			Kind: repository.ScheduleKindDaily, DailyTimes: schedule.Times,
			WindowMinutes: schedule.WindowMinutes,
		}
	case "interval":
		spec.Schedule = asyncd.Schedule{
			Kind: repository.ScheduleKindInterval, IntervalSeconds: schedule.IntervalSeconds,
		}
	default:
		// 保存时已被 validateSyncSchedule 拦截；这里按"没有计划"处理，避免脏数据被当成可调度
		return spec, false
	}
	return spec, true
}

// applyConnectorScheduleJob 保存连接器时把计划写入 asyncd，并把任务主键回填到连接器。
//
// 与入方向同构：先写连接器行 → upsert 任务 → 回填 schedule_job_id；
// 任一步失败都如实报错（不静默成功），由每分钟对账自愈。
func (s *ProvisioningService) applyConnectorScheduleJob(ctx context.Context, c *repository.Connector) error {
	if s.scheduler == nil {
		return nil // 未装配（测试/裁剪装配）：计划不生效但不阻断保存
	}
	spec, hasPlan := connectorScheduleJob(c)
	if !hasPlan {
		if c.ScheduleJobID != nil {
			if err := s.scheduler.SetEnabledByName(ctx, spec.Name, false); err != nil {
				logger.Error("连接器计划任务停用失败（等待对账自愈）", "connector", c.Name, "error", err)
				return bizerrors.New(bizerrors.CodeInternalError, "schedule job not applied; it will be retried automatically")
			}
		}
		return nil
	}
	jobID, err := s.scheduler.UpsertJob(ctx, spec)
	if err != nil {
		logger.Error("连接器计划任务写入失败（等待对账自愈）", "connector", c.Name, "error", err)
		return bizerrors.New(bizerrors.CodeInternalError, "schedule job not applied; it will be retried automatically")
	}
	if err := s.connectors.SetScheduleJobID(ctx, c.ID, jobID); err != nil {
		logger.Error("连接器计划任务绑定失败（等待对账自愈）", "connector", c.Name, "job_id", jobID, "error", err)
		return bizerrors.New(bizerrors.CodeInternalError, "schedule job not bound; it will be retried automatically")
	}
	c.ScheduleJobID = &jobID
	return nil
}

// ReconcileConnectorScheduleJobs 对账：让"连接器持有的计划任务"与连接器的真实状态一致（自愈）。
//
// 语义与入方向一致：缺任务补建并回填绑定、规则漂移覆盖、计划清空/连接器停用只停用不删除。
func (s *ProvisioningService) ReconcileConnectorScheduleJobs(ctx context.Context) error {
	if s.scheduler == nil {
		return nil
	}
	connectors, err := s.connectors.List(ctx, repository.ListQuery{Offset: 0, Limit: 500})
	if err != nil {
		return err
	}
	var created, updated, toggled, rebound int
	for _, c := range connectors {
		spec, hasPlan := connectorScheduleJob(c)
		var existing *repository.AsyncdJob
		if c.ScheduleJobID != nil {
			if job, err := s.scheduler.GetByID(ctx, *c.ScheduleJobID); err == nil {
				existing = job
			}
		}
		switch {
		case existing == nil && hasPlan:
			id, err := s.scheduler.UpsertJob(ctx, spec)
			if err != nil {
				return err
			}
			if err := s.connectors.SetScheduleJobID(ctx, c.ID, id); err != nil {
				return err
			}
			created++
		case existing == nil:
			// 没有计划也没有任务：无需处理
		case hasPlan && !scheduleJobMatches(existing, spec):
			id, err := s.scheduler.UpsertJob(ctx, spec)
			if err != nil {
				return err
			}
			if id != existing.ID {
				if err := s.connectors.SetScheduleJobID(ctx, c.ID, id); err != nil {
					return err
				}
				rebound++
			}
			updated++
		}
		// 启停始终跟随连接器：计划为空或连接器停用 ⇒ 停用任务（保留定义与历史）
		desired := hasPlan && c.State == "active"
		if existing != nil && existing.Enabled != desired {
			if err := s.scheduler.SetEnabledByName(ctx, spec.Name, desired); err != nil {
				return err
			}
			toggled++
		}
	}
	if created > 0 || updated > 0 || toggled > 0 || rebound > 0 {
		logger.Info("connector schedule reconciled", "created", created, "updated", updated,
			"toggled", toggled, "rebound", rebound)
	}
	return nil
}

// RunScheduledReconcile 出方向计划任务的处理器入口：payload 带 connector_id / institution_id，
// 到点执行保守方向的对齐检查（只补建"下游确认缺失"的账号）。
func (s *ProvisioningService) RunScheduledReconcile(ctx context.Context, payload map[string]any) error {
	connectorID := payloadInt64(payload, "connector_id")
	institutionID := payloadInt64(payload, "institution_id")
	if connectorID == 0 || institutionID == 0 {
		return nil
	}
	conn, err := s.connectors.GetByID(ctx, connectorID)
	if err != nil {
		return err
	}
	// 防跨租户：payload 里的机构必须与连接器实际归属一致
	if conn.InstitutionID != institutionID {
		return nil
	}
	_, err = s.Reconcile(ctx, "asyncd", conn.InstitutionID, conn.Name, false)
	return err
}

// payloadInt64 读取 asyncd payload 里的整数（JSON 反序列化后是 float64）。
func payloadInt64(payload map[string]any, key string) int64 {
	switch value := payload[key].(type) {
	case float64:
		return int64(value)
	case int:
		return int64(value)
	case int64:
		return value
	}
	return 0
}
