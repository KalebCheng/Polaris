package service

import (
	"context"
	"fmt"
	"time"

	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// 身份连接的「同步计划」在 2026-10-15 的第七阶段收敛进 asyncd（Spec：261015-syncer-schedule-into-asyncd）：
//
//	一个连接 → 一条 asyncd 定时任务定义（owner_syncer_id 指向该连接）
//	  task_type  = syncer.run（复用既有处理器 → "到点即同步"，一跳完成）
//	  unique_key = syncer:<id>（与手工「立即同步」共用一个单飞键 → 两者不会并发跑同一连接）
//	  payload    = {syncer_id, institution_id, trigger: "schedule"}
//
// 于是仓库里只剩**一套**定时语义（asyncd 的 interval/daily/cron + 执行窗口 + 多实例抢占），
// 老实现（syncer.schedule 每分钟扫描 + Go 侧 DueForSync 判定）整体退役。
//
// 本文件负责"连接的计划 → 任务定义"的翻译，以及对账（自愈）所需的一致性判断。

// asyncdScheduler 连接需要的 asyncd 能力（消费方定义的小接口：连接服务只用到这四件事）。
// 实现是 *asyncd.Service；抽出来是为了能在单测里注入假实现（对账逻辑必须可测）。
type asyncdScheduler interface {
	Enqueue(ctx context.Context, jobName string, payload any, uniqueKey string, runAt time.Time, opts ...asyncd.EnqueueOption) (int64, bool, error)
	UpsertOwnedJob(ctx context.Context, owned asyncd.OwnedJob) error
	SetOwnedJobEnabled(ctx context.Context, ownerSyncerID int64, enabled bool) error
	OwnedJobs(ctx context.Context) ([]*repository.AsyncdJob, error)
}

// SyncerScheduleJobName 连接的计划任务定义名（稳定可重算，不随连接改名变化）
func SyncerScheduleJobName(syncerID int64) string {
	return fmt.Sprintf("syncer.schedule:%d", syncerID)
}

// syncerScheduleJob 把连接的「定时校验」翻译成 asyncd 的任务定义。
// ok=false 表示这个连接没有计划（「不自动」）→ 应该停用它的计划任务。
//
// 规则映射（与 validateSyncSchedule 同源）：
//
//	mode=cron     → cron（5 段表达式）
//	mode=daily    → daily（每天若干 HH:MM + 执行窗口）
//	mode=interval → interval（interval_seconds）
//	mode=''       → 历史字段 syncers.schedule 的间隔秒数（界面上的「同步周期」仍写这个字段）
//	             秒数也为空 → 没有计划
func syncerScheduleJob(sy *repository.Syncer) (asyncd.OwnedJob, bool) {
	spec := asyncd.OwnedJob{
		Name: SyncerScheduleJobName(sy.ID), TaskType: TaskTypeSyncerRun,
		OwnerSyncerID: sy.ID, UniqueKey: fmt.Sprintf("syncer:%d", sy.ID),
		Payload: map[string]any{
			"syncer_id": sy.ID, "institution_id": sy.InstitutionID, "trigger": SyncTriggerSchedule,
		},
		Enabled: sy.IsEnabled,
	}
	schedule := sy.SyncConfig.Schedule
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
	case "":
		seconds := legacyScheduleSeconds(sy.Schedule)
		if seconds <= 0 {
			return spec, false // 「不自动」：没有计划
		}
		spec.Schedule = asyncd.Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: seconds}
	default:
		// 保存时已被 validateSyncSchedule 拦截；这里按"没有计划"处理，避免脏数据被当成可调度
		return spec, false
	}
	return spec, true
}

// syncerScheduleMatches 判断已存在的任务定义是否与连接的计划一致（对账用；一致就不写库，避免无谓的 updated_at 抖动）
func syncerScheduleMatches(existing *repository.AsyncdJob, spec asyncd.OwnedJob) bool {
	if existing.ScheduleKind != spec.Schedule.Kind ||
		existing.IntervalSeconds != spec.Schedule.IntervalSeconds ||
		existing.CronExpr != spec.Schedule.Cron ||
		existing.WindowMinutes != syncerWindowOrDefault(spec.Schedule.WindowMinutes) ||
		existing.UniqueKey != spec.UniqueKey {
		return false
	}
	if len(existing.DailyTimes) != len(spec.Schedule.DailyTimes) {
		return false
	}
	for i, at := range existing.DailyTimes {
		if at != spec.Schedule.DailyTimes[i] {
			return false
		}
	}
	return true
}

// ReconcileScheduleJobs 对账：让"连接持有的计划任务"与连接的真实状态一致（自愈）。
//
// 为什么需要它：连接的保存与任务定义的写入跨了两张表，而本仓库的约定是
// "一个仓库方法 = 一个事务"（AGENTS §3.1-6）——不做跨域事务，就靠这条对账兜底：
// 保存时即时写入（用户立刻看到生效），任何一次写入失败/手工改库/迁移遗漏都会在下一轮被纠正。
//
// 语义：
//   - 有计划的连接缺任务 → 补建；
//   - 规则漂移 → 覆盖（并重置 next_run_at）；
//   - 计划改回「不自动」或连接停用 → 任务 enabled=false（**不删**：保留历史）；
//   - 连接已删除 → 任务由外键 CASCADE 一并删除（这里无需处理）。
func (s *SyncerService) ReconcileScheduleJobs(ctx context.Context) error {
	if s.asyncd == nil {
		return nil
	}
	syncers, err := s.syncers.List(ctx, repository.ListQuery{Offset: 0, Limit: 500})
	if err != nil {
		return err
	}
	jobs, err := s.asyncd.OwnedJobs(ctx)
	if err != nil {
		return err
	}
	byOwner := make(map[int64]*repository.AsyncdJob, len(jobs))
	for _, job := range jobs {
		if job.OwnerSyncerID != nil {
			byOwner[*job.OwnerSyncerID] = job
		}
	}

	var created, updated, toggled int
	for _, sy := range syncers {
		spec, hasPlan := syncerScheduleJob(sy)
		existing := byOwner[sy.ID]
		switch {
		case existing == nil && hasPlan:
			if err := s.asyncd.UpsertOwnedJob(ctx, spec); err != nil {
				return err
			}
			created++
		case existing == nil:
			// 没有计划也没有任务：无需处理
		case hasPlan && !syncerScheduleMatches(existing, spec):
			if err := s.asyncd.UpsertOwnedJob(ctx, spec); err != nil {
				return err
			}
			updated++
		}
		// 启停始终跟随连接：计划为空或连接停用 ⇒ 停用任务（保留定义与历史）
		desired := hasPlan && sy.IsEnabled
		if existing != nil && existing.Enabled != desired {
			if err := s.asyncd.SetOwnedJobEnabled(ctx, sy.ID, desired); err != nil {
				return err
			}
			toggled++
		}
	}
	if created > 0 || updated > 0 || toggled > 0 {
		logger.Info("syncer schedule reconciled", "created", created, "updated", updated, "toggled", toggled)
	}
	return nil
}

// syncerWindowOrDefault 执行窗口默认值（与 asyncd 侧同口径：30 分钟）
func syncerWindowOrDefault(window int) int {
	if window <= 0 {
		return 30
	}
	return window
}

// legacyScheduleSeconds 解析历史字段 syncers.schedule（纯数字的间隔秒数；非法返回 0）。
//
// 该字段仍然是界面上「同步周期」下拉的落库位置，因此在 mode 为空时它是**真实计划**而非历史残留。
func legacyScheduleSeconds(schedule string) int {
	if schedule == "" {
		return 0
	}
	value := 0
	for _, r := range schedule {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
		if value > 1<<30 {
			return 0
		}
	}
	if value <= 0 {
		return 0
	}
	return value
}
