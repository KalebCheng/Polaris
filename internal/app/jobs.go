package app

import (
	"context"
	"fmt"
	"time"

	"polaris/internal/logger"
	"polaris/internal/service"
)

// 本文件承载可由「任务 + 调度」基础能力驱动的周期性维护逻辑（Spec §5.7）。
// 执行入口统一是任务处理器（见 initServer 的注册），这里只放具体动作。

func (a *Application) cleanupTokens(ctx context.Context) error {
	before := time.Now().AddDate(0, 0, -30)
	tokens, err := a.tokenRepo.ListExpired(ctx, before, 500)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tokens))
	for _, t := range tokens {
		ids = append(ids, t.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return a.tokenRepo.DeleteBatch(ctx, ids)
}

func (a *Application) cleanupRecords(ctx context.Context) error {
	_, err := a.recordRepo.DeleteBefore(ctx, time.Now().AddDate(0, 0, -90))
	return err
}

// cleanupAsyncd 执行实例/执行记录/执行器实例的保留期清理。
//
// 保留期来自任务定义的 payload（运行时读取，改了立即生效）：
//
//	retention_days        执行实例与尝试记录的保留天数（默认 14）
//	worker_retention_days 失联/停止的执行器行保留天数（默认 3）
func (a *Application) cleanupAsyncd(ctx context.Context, payload map[string]any) error {
	if a.asyncdSvc == nil {
		return nil
	}
	runsDays := int(payloadNumber(payload, "retention_days"))
	if runsDays <= 0 {
		runsDays = 14
	}
	workerDays := int(payloadNumber(payload, "worker_retention_days"))
	if workerDays <= 0 {
		workerDays = 3
	}
	now := time.Now()
	// 一次最多删 1000 条实例，剩下的留给下一轮（避免长时间持锁）
	runs, workers, err := a.asyncdSvc.Purge(ctx,
		now.AddDate(0, 0, -runsDays), now.AddDate(0, 0, -workerDays), 1000)
	if err != nil {
		return err
	}
	if runs > 0 || workers > 0 {
		logger.Info("asyncd retention purged", "runs", runs, "workers", workers,
			"retention_days", runsDays, "worker_retention_days", workerDays)
	}
	return nil
}

func (a *Application) runScheduledSyncers(ctx context.Context) error {
	syncers, err := a.syncerRepo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, s := range syncers {
		// 是否到点：优先看「同步配置」里的定时校验（每天多时刻 + 执行窗口 / 间隔），
		// 未配置时退回历史的间隔秒数字段（见 service.DueForSync）。
		if !service.DueForSync(s, now) {
			continue
		}
		// 到点即**入队**（而不是就地执行）：执行交给 asyncd 的 worker，
		// 多实例安全，且同一数据源靠单飞键保证不会并发跑两次。
		if _, existing, err := a.asyncdSvc.Enqueue(ctx, service.TaskTypeSyncerRun, map[string]interface{}{
			"syncer_id": s.ID, "institution_id": s.InstitutionID, "trigger": service.SyncTriggerSchedule,
		}, fmt.Sprintf("syncer:%d", s.ID), time.Time{}); err != nil {
			logger.Warn("enqueue scheduled syncer failed", "syncer", s.Name, "error", err)
		} else if existing {
			logger.Warn("syncer still running, skipped this round", "syncer", s.Name)
		}
	}
	return nil
}

// retryProvisioning 兜底重试：扫描失败的应用账号并重新入队供给任务。
func (a *Application) retryProvisioning(ctx context.Context) error {
	if a.provisionSvc == nil {
		return nil
	}
	a.provisionSvc.TriggerRetry(ctx)
	return nil
}
