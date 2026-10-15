package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
)

// fakeScheduler 假的 asyncd 能力：记录"对账做了什么"，供断言
type fakeScheduler struct {
	owned    map[int64]*repository.AsyncdJob // 现有定义（按 owner）
	upserts  []asyncd.OwnedJob
	enableds []struct {
		Owner   int64
		Enabled bool
	}
}

func newFakeScheduler() *fakeScheduler {
	return &fakeScheduler{owned: map[int64]*repository.AsyncdJob{}}
}

func (f *fakeScheduler) Enqueue(context.Context, string, any, string, time.Time, ...asyncd.EnqueueOption) (int64, bool, error) {
	return 1, false, nil
}
func (f *fakeScheduler) UpsertOwnedJob(_ context.Context, owned asyncd.OwnedJob) error {
	f.upserts = append(f.upserts, owned)
	job := &repository.AsyncdJob{
		Name: owned.Name, TaskType: owned.TaskType, Kind: repository.AsyncdJobKindScheduled,
		ScheduleKind: owned.Schedule.Kind, IntervalSeconds: owned.Schedule.IntervalSeconds,
		DailyTimes: owned.Schedule.DailyTimes, CronExpr: owned.Schedule.Cron,
		WindowMinutes: owned.Schedule.WindowMinutes, Enabled: owned.Enabled, UniqueKey: owned.UniqueKey,
	}
	owner := owned.OwnerSyncerID
	job.OwnerSyncerID = &owner
	f.owned[owner] = job
	return nil
}
func (f *fakeScheduler) SetOwnedJobEnabled(_ context.Context, ownerSyncerID int64, enabled bool) error {
	f.enableds = append(f.enableds, struct {
		Owner   int64
		Enabled bool
	}{ownerSyncerID, enabled})
	if job := f.owned[ownerSyncerID]; job != nil {
		job.Enabled = enabled
	}
	return nil
}
func (f *fakeScheduler) OwnedJobs(context.Context) ([]*repository.AsyncdJob, error) {
	out := make([]*repository.AsyncdJob, 0, len(f.owned))
	for _, job := range f.owned {
		out = append(out, job)
	}
	return out, nil
}

// fakeSyncerRepo 假的连接仓库：只实现对账用得到的 List
type fakeSyncerRepo struct{ items []*repository.Syncer }

func (f *fakeSyncerRepo) Create(context.Context, *repository.Syncer) (int64, error) {
	return 0, errors.New("not used")
}
func (f *fakeSyncerRepo) GetByID(context.Context, int64) (*repository.Syncer, error) {
	return nil, errors.New("not used")
}
func (f *fakeSyncerRepo) GetByName(context.Context, int64, string) (*repository.Syncer, error) {
	return nil, errors.New("not used")
}
func (f *fakeSyncerRepo) Update(context.Context, *repository.Syncer) error {
	return errors.New("not used")
}
func (f *fakeSyncerRepo) Delete(context.Context, int64) error { return errors.New("not used") }
func (f *fakeSyncerRepo) List(_ context.Context, _ repository.ListQuery) ([]*repository.Syncer, error) {
	return f.items, nil
}
func (f *fakeSyncerRepo) Count(context.Context, repository.ListQuery) (int64, error) { return 0, nil }
func (f *fakeSyncerRepo) ListEnabled(context.Context) ([]*repository.Syncer, error) {
	return f.items, nil
}
func (f *fakeSyncerRepo) UpdateRunResult(context.Context, int64, string, string, time.Time, repository.SyncCounts) error {
	return nil
}
func (f *fakeSyncerRepo) CreateRun(context.Context, *repository.SyncerRun) (int64, error) {
	return 0, nil
}
func (f *fakeSyncerRepo) ListRuns(context.Context, int64, int, int) ([]*repository.SyncerRun, error) {
	return nil, nil
}

func syncerFixture(id int64, enabled bool, mode string, over func(*repository.SyncSchedule), legacy string) *repository.Syncer {
	schedule := repository.SyncSchedule{Mode: mode}
	if over != nil {
		over(&schedule)
	}
	return &repository.Syncer{
		ID: id, InstitutionID: 7, Name: "corp-ldap", IsEnabled: enabled,
		Schedule: legacy, SyncConfig: repository.SyncConfig{Schedule: schedule},
	}
}

// 回归：连接的「同步计划」必须被翻译成 asyncd 任务定义——
// cron 直接成为任务规则；unique_key 与手工触发共用；payload 带上 trigger=schedule。
func TestSyncerScheduleJobTranslation(t *testing.T) {
	cases := []struct {
		name       string
		sy         *repository.Syncer
		wantKind   string
		wantCron   string
		wantSecs   int
		wantTimes  []string
		wantHasJob bool
	}{
		{
			name: "cron 模式",
			sy: syncerFixture(1, true, "cron", func(s *repository.SyncSchedule) {
				s.Cron = "0 3 * * *"
			}, ""),
			wantKind: repository.ScheduleKindCron, wantCron: "0 3 * * *", wantHasJob: true,
		},
		{
			name: "每天定时",
			sy: syncerFixture(2, true, "daily", func(s *repository.SyncSchedule) {
				s.Times = []string{"03:00", "15:30"}
				s.WindowMinutes = 45
			}, ""),
			wantKind: repository.ScheduleKindDaily, wantTimes: []string{"03:00", "15:30"}, wantHasJob: true,
		},
		{
			name:     "间隔模式",
			sy:       syncerFixture(3, true, "interval", func(s *repository.SyncSchedule) { s.IntervalSeconds = 900 }, ""),
			wantKind: repository.ScheduleKindInterval, wantSecs: 900, wantHasJob: true,
		},
		{
			name:     "历史字段兼容（同步周期下拉）",
			sy:       syncerFixture(4, true, "", nil, "1800"),
			wantKind: repository.ScheduleKindInterval, wantSecs: 1800, wantHasJob: true,
		},
		{name: "不自动（两个字段都空）", sy: syncerFixture(5, true, "", nil, ""), wantHasJob: false},
		{name: "未知模式按无计划处理", sy: syncerFixture(6, true, "weird", nil, ""), wantHasJob: false},
	}

	for _, tc := range cases {
		spec, hasJob := syncerScheduleJob(tc.sy)
		if hasJob != tc.wantHasJob {
			t.Fatalf("%s：hasJob 期望 %v 实际 %v", tc.name, tc.wantHasJob, hasJob)
		}
		if !hasJob {
			continue
		}
		if spec.Name != SyncerScheduleJobName(tc.sy.ID) {
			t.Fatalf("%s：定义名应为 syncer.schedule:<id>，实际 %s", tc.name, spec.Name)
		}
		if spec.UniqueKey != "syncer:"+itoa(tc.sy.ID) {
			t.Fatalf("%s：单飞键应与手工触发共用 syncer:<id>，实际 %s", tc.name, spec.UniqueKey)
		}
		if spec.TaskType != TaskTypeSyncerRun {
			t.Fatalf("%s：task_type 应复用 syncer.run（到点即同步，一跳完成），实际 %s", tc.name, spec.TaskType)
		}
		if spec.Schedule.Kind != tc.wantKind || spec.Schedule.Cron != tc.wantCron ||
			spec.Schedule.IntervalSeconds != tc.wantSecs {
			t.Fatalf("%s：规则翻译不符 %+v", tc.name, spec.Schedule)
		}
		if len(tc.wantTimes) > 0 && (len(spec.Schedule.DailyTimes) != len(tc.wantTimes) ||
			spec.Schedule.DailyTimes[0] != tc.wantTimes[0]) {
			t.Fatalf("%s：每天时刻不符 %+v", tc.name, spec.Schedule.DailyTimes)
		}
		payload, ok := spec.Payload.(map[string]any)
		if !ok || payload["syncer_id"] != tc.sy.ID || payload["trigger"] != SyncTriggerSchedule {
			t.Fatalf("%s：payload 应定位连接并声明 trigger=schedule，实际 %+v", tc.name, spec.Payload)
		}
		if spec.OwnerSyncerID != tc.sy.ID {
			t.Fatalf("%s：定义必须归属该连接（否则对账与级联删除都失效）", tc.name)
		}
	}
}

// 回归：连接停用 → 计划任务也要停用（enabled 是连接状态的投影）
func TestSyncerScheduleJobFollowsConnectionEnabled(t *testing.T) {
	spec, ok := syncerScheduleJob(syncerFixture(9, false, "cron", func(s *repository.SyncSchedule) {
		s.Cron = "*/5 * * * *"
	}, ""))
	if !ok || spec.Enabled {
		t.Fatalf("停用的连接不应启用计划任务: %+v", spec)
	}
}

// 回归：对账（自愈）——缺任务补建、规则漂移覆盖、计划清空/连接停用只停用不删除
func TestReconcileScheduleJobs(t *testing.T) {
	ctx := context.Background()
	plan := func(s *repository.SyncSchedule) { s.Cron = "0 3 * * *" }
	syncers := &fakeSyncerRepo{items: []*repository.Syncer{
		syncerFixture(1, true, "cron", plan, ""),  // 缺任务 → 补建
		syncerFixture(2, true, "cron", plan, ""),  // 规则漂移 → 覆盖
		syncerFixture(3, false, "cron", plan, ""), // 连接停用 → 停用任务
		syncerFixture(4, true, "", nil, ""),       // 计划清空 → 停用任务（不删）
		syncerFixture(5, true, "", nil, ""),       // 无任务也无计划 → 什么都不做
	}}
	scheduler := newFakeScheduler()
	stale := int64(2)
	scheduler.owned[2] = &repository.AsyncdJob{
		ID: 22, Name: SyncerScheduleJobName(2), OwnerSyncerID: &stale, Enabled: true,
		ScheduleKind: repository.ScheduleKindInterval, IntervalSeconds: 60,
	}
	offOwner := int64(3)
	scheduler.owned[3] = &repository.AsyncdJob{
		ID: 33, Name: SyncerScheduleJobName(3), OwnerSyncerID: &offOwner, Enabled: true,
		ScheduleKind: repository.ScheduleKindCron, CronExpr: "0 3 * * *",
	}
	emptyOwner := int64(4)
	scheduler.owned[4] = &repository.AsyncdJob{
		ID: 44, Name: SyncerScheduleJobName(4), OwnerSyncerID: &emptyOwner, Enabled: true,
		ScheduleKind: repository.ScheduleKindInterval, IntervalSeconds: 1800,
	}

	svc := NewSyncerService(syncers, nil, nil, nil, nil, nil)
	svc.SetAsyncd(scheduler)
	if err := svc.ReconcileScheduleJobs(ctx); err != nil {
		t.Fatalf("对账失败: %v", err)
	}

	created := map[string]bool{}
	for _, spec := range scheduler.upserts {
		created[spec.Name] = true
	}
	if !created[SyncerScheduleJobName(1)] {
		t.Fatal("缺任务的连接应被补建")
	}
	if !created[SyncerScheduleJobName(2)] {
		t.Fatal("规则漂移的连接应被覆盖（60s → cron）")
	}
	if created[SyncerScheduleJobName(4)] {
		t.Fatal("计划已清空的连接不应重新创建任务（只停用）")
	}

	enabledChanges := map[int64]bool{}
	for _, item := range scheduler.enableds {
		enabledChanges[item.Owner] = item.Enabled
	}
	if enabledChanges[3] != false {
		t.Fatal("连接停用后应停用其计划任务")
	}
	if enabledChanges[4] != false {
		t.Fatal("计划清空后应停用其计划任务（保留定义与历史）")
	}
	if _, touched := enabledChanges[5]; touched {
		t.Fatal("既无计划也无任务的连接不应被触碰")
	}
	if job := scheduler.owned[2]; job.ScheduleKind != repository.ScheduleKindCron || job.CronExpr != "0 3 * * *" {
		t.Fatalf("漂移的定义应被覆盖为 cron: %+v", job)
	}
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
