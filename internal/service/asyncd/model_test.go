package asyncd

import (
	"context"
	"errors"
	"testing"
	"time"

	"polaris/internal/repository"
)

// ── 仓库桩：只记录任务中心真正用到的输入 ─────────────────────────────────────────

type fakeJobRepo struct {
	definitions    []*repository.AsyncdJob
	upserted       []*repository.AsyncdJob
	owned          []*repository.AsyncdJob
	ownedReset     []bool
	includeRetired bool
	retired        []string
	byName         []struct {
		Name    string
		Enabled bool
	}
	marked []struct {
		ID     int64
		Status string
	}
	enabledCalls []int64
}

func (f *fakeJobRepo) UpsertJob(_ context.Context, job *repository.AsyncdJob, resetNextRunAt bool) (int64, error) {
	f.owned = append(f.owned, job)
	f.ownedReset = append(f.ownedReset, resetNextRunAt)
	return int64(len(f.owned)), nil
}
func (f *fakeJobRepo) SetEnabledByName(_ context.Context, name string, enabled bool) error {
	f.byName = append(f.byName, struct {
		Name    string
		Enabled bool
	}{name, enabled})
	return nil
}
func (f *fakeJobRepo) RetireUndeclared(_ context.Context, activeNames []string, _ time.Time) (int64, error) {
	f.retired = activeNames
	return 0, nil
}

func (f *fakeJobRepo) Upsert(_ context.Context, job *repository.AsyncdJob) error {
	f.upserted = append(f.upserted, job)
	return nil
}
func (f *fakeJobRepo) GetByName(_ context.Context, name string) (*repository.AsyncdJob, error) {
	for _, job := range f.definitions {
		if job.Name == name {
			return job, nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeJobRepo) GetByID(_ context.Context, id int64) (*repository.AsyncdJob, error) {
	for _, job := range f.definitions {
		if job.ID == id {
			return job, nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeJobRepo) ClaimDue(context.Context, time.Time, int) ([]*repository.AsyncdJob, error) {
	return nil, nil
}
func (f *fakeJobRepo) MarkRun(_ context.Context, id int64, status string, _ time.Time, _ time.Time) error {
	f.marked = append(f.marked, struct {
		ID     int64
		Status string
	}{id, status})
	return nil
}
func (f *fakeJobRepo) List(_ context.Context, includeRetired bool) ([]*repository.AsyncdJob, error) {
	f.includeRetired = includeRetired
	return f.definitions, nil
}
func (f *fakeJobRepo) SetEnabled(_ context.Context, id int64, _ bool) error {
	f.enabledCalls = append(f.enabledCalls, id)
	return nil
}

var _ repository.AsyncdJobRepository = (*fakeJobRepo)(nil)

type fakeRunRepo struct {
	stats     []repository.AsyncdJobRunStat
	enqueued  []*repository.AsyncdJobRun
	byID      map[int64]*repository.AsyncdJobRun
	query     repository.AsyncdJobRunQuery
	purgeFrom time.Time
}

func (f *fakeRunRepo) Enqueue(_ context.Context, run *repository.AsyncdJobRun) (int64, bool, error) {
	f.enqueued = append(f.enqueued, run)
	return int64(len(f.enqueued)), false, nil
}
func (f *fakeRunRepo) Claim(context.Context, string, time.Duration, int) ([]*repository.AsyncdJobRun, error) {
	return nil, nil
}
func (f *fakeRunRepo) RenewLease(context.Context, int64, string, time.Duration) (bool, error) {
	return true, nil
}
func (f *fakeRunRepo) Complete(context.Context, int64, string) error { return nil }
func (f *fakeRunRepo) Retry(context.Context, int64, string, string, time.Time) error {
	return nil
}
func (f *fakeRunRepo) Kill(context.Context, int64, string, string) error { return nil }
func (f *fakeRunRepo) ReclaimExpired(context.Context, int) ([]int64, error) {
	return nil, nil
}
func (f *fakeRunRepo) Counts(_ context.Context, query repository.AsyncdJobRunQuery) (map[string]int, error) {
	f.query = query
	return map[string]int{}, nil
}
func (f *fakeRunRepo) List(_ context.Context, query repository.AsyncdJobRunQuery, _, _ int) ([]*repository.AsyncdJobRun, error) {
	f.query = query
	return nil, nil
}
func (f *fakeRunRepo) GetByID(_ context.Context, id int64) (*repository.AsyncdJobRun, error) {
	if run, ok := f.byID[id]; ok {
		return run, nil
	}
	return nil, errors.New("not found")
}
func (f *fakeRunRepo) Stats(context.Context) ([]repository.AsyncdJobRunStat, error) {
	return f.stats, nil
}
func (f *fakeRunRepo) RetryDead(context.Context, int64) error { return nil }
func (f *fakeRunRepo) PurgeFinishedBefore(_ context.Context, before time.Time, _ int) (int64, error) {
	f.purgeFrom = before
	return 3, nil
}

var _ repository.AsyncdJobRunRepository = (*fakeRunRepo)(nil)

type fakeAttemptRepo struct {
	started []*repository.AsyncdJobRunAttempt
}

func (f *fakeAttemptRepo) Start(_ context.Context, attempt *repository.AsyncdJobRunAttempt) (int64, error) {
	f.started = append(f.started, attempt)
	return int64(len(f.started)), nil
}
func (f *fakeAttemptRepo) Finish(context.Context, int64, string, string) error { return nil }
func (f *fakeAttemptRepo) ListByRun(context.Context, int64) ([]*repository.AsyncdJobRunAttempt, error) {
	return nil, nil
}

var _ repository.AsyncdJobRunAttemptRepository = (*fakeAttemptRepo)(nil)

type fakeWorkerRepo struct {
	slots      []repository.AsyncdWorkerStat
	registered []*repository.AsyncdWorker
	stopped    []string
	purgeFrom  time.Time
}

func (f *fakeWorkerRepo) Register(_ context.Context, worker *repository.AsyncdWorker) error {
	f.registered = append(f.registered, worker)
	return nil
}
func (f *fakeWorkerRepo) Heartbeat(context.Context, string) error { return nil }
func (f *fakeWorkerRepo) Stop(_ context.Context, workerID string) error {
	f.stopped = append(f.stopped, workerID)
	return nil
}
func (f *fakeWorkerRepo) List(context.Context) ([]repository.AsyncdWorkerStat, error) {
	return f.slots, nil
}
func (f *fakeWorkerRepo) CountOnline(context.Context, time.Duration) (int, error) { return 0, nil }
func (f *fakeWorkerRepo) PurgeStaleBefore(_ context.Context, before time.Time) (int64, error) {
	f.purgeFrom = before
	return 1, nil
}

var _ repository.AsyncdWorkerRepository = (*fakeWorkerRepo)(nil)

func newTestService(jobs *fakeJobRepo, runs *fakeRunRepo, attempts *fakeAttemptRepo, workers *fakeWorkerRepo) *Service {
	svc := NewService(jobs, runs, attempts, workers)
	svc.instanceID = "host-1"
	return svc
}

// 回归：任务定义必须自洽——定时任务要带调度规则，处理器必须已注册；
// 异步任务没有"停用"语义，恒为启用（停用等于静默丢任务）。
func TestRegisterJobValidatesDefinition(t *testing.T) {
	jobs := &fakeJobRepo{}
	svc := newTestService(jobs, &fakeRunRepo{}, &fakeAttemptRepo{}, &fakeWorkerRepo{})
	svc.registry.Register("syncer.run", func(context.Context, *repository.AsyncdJobRun, map[string]any) error { return nil })

	if err := svc.RegisterJob(context.Background(), Job{
		Name: "syncer.run", TaskType: "syncer.run", Kind: repository.AsyncdJobKindAsync,
	}); err != nil {
		t.Fatalf("异步任务定义应可注册: %v", err)
	}
	if !jobs.upserted[0].Enabled {
		t.Fatal("异步任务定义必须恒为启用（停用它 = 业务动作静默丢任务）")
	}

	if err := svc.RegisterJob(context.Background(), Job{
		Name: "job.interval", TaskType: "syncer.run", Kind: repository.AsyncdJobKindScheduled,
		Schedule: Schedule{Kind: repository.ScheduleKindInterval},
	}); err == nil {
		t.Fatal("间隔为 0 的定时任务必须被拒绝")
	}

	if err := svc.RegisterJob(context.Background(), Job{
		Name: "job.norule", TaskType: "syncer.run", Kind: repository.AsyncdJobKindScheduled,
	}); err == nil {
		t.Fatal("缺少调度规则的定时任务必须被拒绝")
	}

	if err := svc.RegisterJob(context.Background(), Job{
		Name: "job.ghost", TaskType: "ghost.task", Kind: repository.AsyncdJobKindAsync,
	}); err == nil {
		t.Fatal("处理器未注册的任务定义必须被拒绝（避免入了队没人执行）")
	}
}

// 回归：入队按**定义名**解析（定义决定 job_id 与处理器），定义不存在或处理器缺失一律拒绝。
func TestEnqueueResolvesDefinition(t *testing.T) {
	jobs := &fakeJobRepo{definitions: []*repository.AsyncdJob{
		{ID: 11, Name: "syncer.run", TaskType: "syncer.run", Kind: repository.AsyncdJobKindAsync},
		{ID: 12, Name: "job.retired", TaskType: "retired.task", Kind: repository.AsyncdJobKindAsync},
	}}
	runs := &fakeRunRepo{}
	svc := newTestService(jobs, runs, &fakeAttemptRepo{}, &fakeWorkerRepo{})
	svc.registry.Register("syncer.run", func(context.Context, *repository.AsyncdJobRun, map[string]any) error { return nil })

	id, existing, err := svc.Enqueue(context.Background(), "syncer.run",
		map[string]any{"syncer_id": 3}, "syncer:3", time.Time{})
	if err != nil || existing || id == 0 {
		t.Fatalf("入队应成功: id=%d existing=%v err=%v", id, existing, err)
	}
	if runs.enqueued[0].JobID != 11 {
		t.Fatalf("入队必须落在定义上（job_id=11）: %+v", runs.enqueued[0])
	}
	if runs.enqueued[0].UniqueKey != "syncer:3" || len(runs.enqueued[0].Payload) == 0 {
		t.Fatalf("单飞键与载荷应落库: %+v", runs.enqueued[0])
	}

	if _, _, err := svc.Enqueue(context.Background(), "nope", nil, "", time.Time{}); err == nil {
		t.Fatal("未注册的任务定义必须拒绝入队")
	}
	if _, _, err := svc.Enqueue(context.Background(), "job.retired", nil, "", time.Time{}); err == nil {
		t.Fatal("定义指向的处理器缺失时必须拒绝入队")
	}
}

// 回归：任务目录来自**定义表**，因此从未执行过的任务也要在列（统计为零）。
func TestJobsCatalogIncludesNeverRunDefinition(t *testing.T) {
	jobs := &fakeJobRepo{definitions: []*repository.AsyncdJob{
		{ID: 1, Name: "maintenance.token_cleanup", Kind: repository.AsyncdJobKindScheduled},
		{ID: 2, Name: "syncer.run", Kind: repository.AsyncdJobKindAsync},
	}}
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	runs := &fakeRunRepo{stats: []repository.AsyncdJobRunStat{
		{JobID: 1, Status: repository.AsyncdJobRunSucceeded, Count: 9, LastQueuedAt: now.Add(-time.Hour)},
		{JobID: 1, Status: repository.AsyncdJobRunDead, Count: 1, LastQueuedAt: now},
	}}
	svc := newTestService(jobs, runs, &fakeAttemptRepo{}, &fakeWorkerRepo{})

	catalog, err := svc.Jobs(context.Background())
	if err != nil {
		t.Fatalf("组装任务目录失败: %v", err)
	}
	if len(catalog) != 2 {
		t.Fatalf("目录应包含全部定义（含从未执行过的）: %+v", catalog)
	}
	cleanup := catalog[0].Stats
	if cleanup.Total != 10 || cleanup.ByStatus[repository.AsyncdJobRunSucceeded] != 9 ||
		cleanup.ByStatus[repository.AsyncdJobRunDead] != 1 {
		t.Fatalf("统计应按状态合并: %+v", cleanup)
	}
	if cleanup.LastQueuedAt == nil || !cleanup.LastQueuedAt.Equal(now) {
		t.Fatalf("最近入队时间应取最新一次: %+v", cleanup.LastQueuedAt)
	}
	never := catalog[1].Stats
	if never.Total != 0 || never.ByStatus == nil || never.LastQueuedAt != nil {
		t.Fatalf("从未执行过的任务应以零统计出现: %+v", never)
	}
}

// 回归：只有定时任务可以停用（异步任务停用会变成静默丢任务）。
// 说明："业务持有的任务不接受界面启停"属于业务语义，由 handler + owner 解析器负责，asyncd 不判断。
func TestSetJobEnabledOnlyForScheduled(t *testing.T) {
	jobs := &fakeJobRepo{definitions: []*repository.AsyncdJob{
		{ID: 1, Name: "syncer.schedule.reconcile", Kind: repository.AsyncdJobKindScheduled},
		{ID: 2, Name: "syncer.run", Kind: repository.AsyncdJobKindAsync},
	}}
	svc := newTestService(jobs, &fakeRunRepo{}, &fakeAttemptRepo{}, &fakeWorkerRepo{})

	if err := svc.SetJobEnabled(context.Background(), 1, false); err != nil {
		t.Fatalf("定时任务应可停用: %v", err)
	}
	if len(jobs.enabledCalls) != 1 || jobs.enabledCalls[0] != 1 {
		t.Fatalf("启停应落到仓库（按主键）: %+v", jobs.enabledCalls)
	}
	if err := svc.SetJobEnabled(context.Background(), 2, false); err == nil {
		t.Fatal("异步任务不允许停用")
	}
	if err := svc.SetJobEnabled(context.Background(), 99, false); err == nil {
		t.Fatal("不存在的定义必须报错")
	}
}

// 回归：执行器在线状态由心跳算出——停止 > 心跳过期 > 在线；在跑任务数按 worker_id 实时聚合。
func TestWorkersDeriveStateFromHeartbeat(t *testing.T) {
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	stoppedAt := now.Add(-time.Minute)
	workers := &fakeWorkerRepo{slots: []repository.AsyncdWorkerStat{
		{Worker: repository.AsyncdWorker{ID: "h-1#0", LastHeartbeatAt: now.Add(-5 * time.Second)}, Running: 2},
		{Worker: repository.AsyncdWorker{ID: "h-1#1", LastHeartbeatAt: now.Add(-WorkerStaleAfter - time.Second)}},
		{Worker: repository.AsyncdWorker{ID: "h-2#0", LastHeartbeatAt: now.Add(-time.Hour), StoppedAt: &stoppedAt}},
	}}
	svc := newTestService(&fakeJobRepo{}, &fakeRunRepo{}, &fakeAttemptRepo{}, workers)
	svc.clock = func() time.Time { return now }

	list, err := svc.Workers(context.Background())
	if err != nil {
		t.Fatalf("取执行器实例失败: %v", err)
	}
	want := []struct {
		id      string
		state   string
		running int
	}{
		{"h-1#0", WorkerStateOnline, 2},
		{"h-1#1", WorkerStateStale, 0},
		{"h-2#0", WorkerStateStopped, 0},
	}
	if len(list) != len(want) {
		t.Fatalf("执行器数量不符: %+v", list)
	}
	for i, expected := range want {
		if list[i].ID != expected.id || list[i].State != expected.state || list[i].Running != expected.running {
			t.Fatalf("第 %d 个执行器不符: 期望 %+v 实际 %+v", i, expected, list[i])
		}
	}
}

// 回归：只有死信可以人工重试（running/queued 重试会造成重复执行）。
func TestRetryDeadOnlyForDead(t *testing.T) {
	runs := &fakeRunRepo{byID: map[int64]*repository.AsyncdJobRun{
		1: {ID: 1, Status: repository.AsyncdJobRunDead},
		2: {ID: 2, Status: repository.AsyncdJobRunRunning},
	}}
	svc := newTestService(&fakeJobRepo{}, runs, &fakeAttemptRepo{}, &fakeWorkerRepo{})

	if err := svc.RetryDeadJobRun(context.Background(), 1); err != nil {
		t.Fatalf("死信应可人工重试: %v", err)
	}
	if err := svc.RetryDeadJobRun(context.Background(), 2); err == nil {
		t.Fatal("执行中的实例不允许人工重试")
	}
	if err := svc.RetryDeadJobRun(context.Background(), 3); err == nil {
		t.Fatal("不存在的实例必须报错")
	}
}

// 回归：保留期清理按"完成时间/心跳时间"截断，且一次只删一批（由调用方传 limit）。
func TestPurgeUsesRetentionCutoff(t *testing.T) {
	runs := &fakeRunRepo{}
	workers := &fakeWorkerRepo{}
	svc := newTestService(&fakeJobRepo{}, runs, &fakeAttemptRepo{}, workers)
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

	deletedRuns, deletedWorkers, err := svc.Purge(context.Background(),
		now.AddDate(0, 0, -14), now.AddDate(0, 0, -3), 1000)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if !runs.purgeFrom.Equal(now.AddDate(0, 0, -14)) {
		t.Fatalf("执行实例保留期不符: %v", runs.purgeFrom)
	}
	if !workers.purgeFrom.Equal(now.AddDate(0, 0, -3)) {
		t.Fatalf("执行器保留期不符: %v", workers.purgeFrom)
	}
	if deletedRuns != 3 || deletedWorkers != 1 {
		t.Fatalf("应回传删除条数: runs=%d workers=%d", deletedRuns, deletedWorkers)
	}
}

// 回归：业务持有的任务定义（身份连接的计划）在**规则或启用态变化时重置 next_run_at**——
// 否则把"每天 03:00"改成"每 10 分钟"要等到旧的下次时间才生效（又一次"配了不生效"）。
func TestUpsertJobResetsNextRunOnRuleChange(t *testing.T) {
	ctx := context.Background()
	jobs := &fakeJobRepo{definitions: []*repository.AsyncdJob{}}
	svc := newTestService(jobs, &fakeRunRepo{}, &fakeAttemptRepo{}, &fakeWorkerRepo{})
	svc.registry.Register("syncer.run", func(context.Context, *repository.AsyncdJobRun, map[string]any) error { return nil })

	base := Job{
		Name: "syncer.schedule:3", TaskType: "syncer.run", Kind: repository.AsyncdJobKindScheduled,
		Schedule: Schedule{Kind: repository.ScheduleKindCron, Cron: "0 3 * * *"},
		Payload:  map[string]any{"syncer_id": 3}, Enabled: true, UniqueKey: "syncer:3",
	}
	if _, err := svc.UpsertJob(ctx, base); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	if !jobs.ownedReset[0] {
		t.Fatal("首次写入应初始化 next_run_at（reset=true）")
	}

	// 规则没变 → 不动 next_run_at；改 cron → 必须重置
	jobs.definitions = []*repository.AsyncdJob{{
		ID: 9, Name: base.Name, Enabled: true,
		ScheduleKind: repository.ScheduleKindCron, CronExpr: "0 3 * * *",
		WindowMinutes: 30, UniqueKey: "syncer:3", Payload: []byte(`{"syncer_id":3}`),
	}}
	if _, err := svc.UpsertJob(ctx, base); err != nil {
		t.Fatalf("幂等写入失败: %v", err)
	}
	if jobs.ownedReset[1] {
		t.Fatal("规则未变化时不应重置 next_run_at（避免无谓抖动）")
	}

	changed := base
	changed.Schedule = Schedule{Kind: repository.ScheduleKindCron, Cron: "*/10 * * * *"}
	if _, err := svc.UpsertJob(ctx, changed); err != nil {
		t.Fatalf("改规则失败: %v", err)
	}
	if !jobs.ownedReset[2] {
		t.Fatal("规则变化必须重置 next_run_at")
	}

	// 从停用恢复启用也要重置（否则会带着旧时间一次性触发）
	jobs.definitions[0].Enabled = false
	reEnabled := base
	reEnabled.Enabled = true
	if _, err := svc.UpsertJob(ctx, reEnabled); err != nil {
		t.Fatalf("重新启用失败: %v", err)
	}
	if !jobs.ownedReset[3] {
		t.Fatal("从停用恢复启用必须重置 next_run_at")
	}
}

// 回归：业务持有的任务必须返回主键（持有方要把它记在自己的绑定列上），且单飞键落库。
func TestUpsertJobReturnsIDAndKeepsUniqueKey(t *testing.T) {
	jobs := &fakeJobRepo{}
	svc := newTestService(jobs, &fakeRunRepo{}, &fakeAttemptRepo{}, &fakeWorkerRepo{})
	svc.registry.Register("syncer.run", func(context.Context, *repository.AsyncdJobRun, map[string]any) error { return nil })
	id, err := svc.UpsertJob(context.Background(), Job{
		Name: "syncer.schedule:8", TaskType: "syncer.run", Kind: repository.AsyncdJobKindScheduled,
		Schedule: Schedule{Kind: repository.ScheduleKindInterval, IntervalSeconds: 900},
		Enabled:  true, UniqueKey: "syncer:8",
	})
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if id <= 0 {
		t.Fatalf("必须返回任务主键（持有方要回填绑定列），实际 %d", id)
	}
	if jobs.owned[0].UniqueKey != "syncer:8" {
		t.Fatalf("单飞键必须落库（与手工触发共用）: %+v", jobs.owned[0])
	}
	if jobs.owned[0].Name != "syncer.schedule:8" {
		t.Fatalf("任务名必须落库（幂等键）: %+v", jobs.owned[0])
	}
}

// 回归：代码里删掉的任务声明走"退役"而不是 DELETE（删定义会级联删掉历史执行记录）
func TestRetireUndeclaredPassesActiveNames(t *testing.T) {
	jobs := &fakeJobRepo{}
	svc := newTestService(jobs, &fakeRunRepo{}, &fakeAttemptRepo{}, &fakeWorkerRepo{})
	if _, err := svc.RetireUndeclared(context.Background(), []string{"a", "b"}); err != nil {
		t.Fatalf("退役失败: %v", err)
	}
	if len(jobs.retired) != 2 || jobs.retired[0] != "a" {
		t.Fatalf("应把已声明的名单透传给仓库: %+v", jobs.retired)
	}
}
