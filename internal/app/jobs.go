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
