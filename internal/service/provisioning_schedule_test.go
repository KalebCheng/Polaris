package service

import (
	"context"
	"errors"
	"testing"

	"polaris/internal/repository"
)

// fakeConnectorRepo 假的连接器仓库：只实现对账用得到的 List / SetScheduleJobID。
type fakeConnectorRepo struct {
	repository.ConnectorRepository
	items []*repository.Connector
}

func (f *fakeConnectorRepo) List(context.Context, repository.ListQuery) ([]*repository.Connector, error) {
	return f.items, nil
}
func (f *fakeConnectorRepo) SetScheduleJobID(_ context.Context, id int64, jobID int64) error {
	for _, c := range f.items {
		if c.ID == id {
			v := jobID
			c.ScheduleJobID = &v
			return nil
		}
	}
	return errors.New("not found")
}
func (f *fakeConnectorRepo) ListByScheduleJobIDs(context.Context, []int64) ([]*repository.Connector, error) {
	return nil, nil
}
func (f *fakeConnectorRepo) ListScheduleJobNames(context.Context) ([]string, error) { return nil, nil }

func connectorFixture(id int64, state, mode string, over func(*repository.SyncSchedule)) *repository.Connector {
	schedule := repository.SyncSchedule{Mode: mode}
	if over != nil {
		over(&schedule)
	}
	return &repository.Connector{
		ID: id, InstitutionID: 7, Name: "app-conn", State: state,
		SyncConfig: repository.SyncConfig{Schedule: schedule},
	}
}

// 回归：连接器的「定时对齐检查」必须被翻译成 asyncd 任务定义——
// cron/daily/interval 各按其规则；unique_key 与手动对齐检查共用；payload 定位连接器。
func TestConnectorScheduleJobTranslation(t *testing.T) {
	cases := []struct {
		name       string
		conn       *repository.Connector
		wantKind   string
		wantCron   string
		wantSecs   int
		wantTimes  []string
		wantHasJob bool
	}{
		{
			name: "cron 模式",
			conn: connectorFixture(1, "active", "cron", func(s *repository.SyncSchedule) {
				s.Cron = "0 3 * * *"
			}),
			wantKind: repository.ScheduleKindCron, wantCron: "0 3 * * *", wantHasJob: true,
		},
		{
			name: "每天定时",
			conn: connectorFixture(2, "active", "daily", func(s *repository.SyncSchedule) {
				s.Times = []string{"03:00"}
				s.WindowMinutes = 45
			}),
			wantKind: repository.ScheduleKindDaily, wantTimes: []string{"03:00"}, wantHasJob: true,
		},
		{
			name: "间隔模式",
			conn: connectorFixture(3, "active", "interval", func(s *repository.SyncSchedule) {
				s.IntervalSeconds = 3600
			}),
			wantKind: repository.ScheduleKindInterval, wantSecs: 3600, wantHasJob: true,
		},
		{name: "不自动", conn: connectorFixture(4, "active", "", nil), wantHasJob: false},
		{name: "未知模式按无计划处理", conn: connectorFixture(5, "active", "weird", nil), wantHasJob: false},
	}

	for _, tc := range cases {
		spec, hasJob := connectorScheduleJob(tc.conn)
		if hasJob != tc.wantHasJob {
			t.Fatalf("%s：hasJob 期望 %v 实际 %v", tc.name, tc.wantHasJob, hasJob)
		}
		if !hasJob {
			continue
		}
		if spec.Name != ConnectorScheduleJobName(tc.conn.ID) {
			t.Fatalf("%s：定义名应为 connector.schedule:<id>，实际 %s", tc.name, spec.Name)
		}
		if spec.UniqueKey != "connector:"+itoa(tc.conn.ID) {
			t.Fatalf("%s：单飞键应与手动对齐检查共用 connector:<id>，实际 %s", tc.name, spec.UniqueKey)
		}
		if spec.TaskType != TaskTypeConnectorReconcile {
			t.Fatalf("%s：task_type 应为 connector.reconcile，实际 %s", tc.name, spec.TaskType)
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
		if !ok || payload["connector_id"] != tc.conn.ID || payload["trigger"] != SyncTriggerSchedule {
			t.Fatalf("%s：payload 应定位连接器并声明 trigger=schedule，实际 %+v", tc.name, spec.Payload)
		}
	}
}

// 回归：连接器停用 → 计划任务也要停用（enabled 是连接器状态的投影）
func TestConnectorScheduleJobFollowsState(t *testing.T) {
	spec, ok := connectorScheduleJob(connectorFixture(9, "disabled", "cron", func(s *repository.SyncSchedule) {
		s.Cron = "*/5 * * * *"
	}))
	if !ok || spec.Enabled {
		t.Fatalf("停用的连接器不应启用计划任务: %+v", spec)
	}
}

// 回归：出方向对账——缺任务补建并回填绑定、规则漂移覆盖、计划清空/连接器停用只停用不删除。
func TestReconcileConnectorScheduleJobs(t *testing.T) {
	ctx := context.Background()
	plan := func(s *repository.SyncSchedule) { s.Cron = "0 3 * * *" }
	jobID := func(v int64) *int64 { return &v }

	c1 := connectorFixture(1, "active", "cron", plan) // 有计划、未绑定 → 补建并回填
	c2 := connectorFixture(2, "active", "cron", plan) // 规则漂移 → 覆盖
	c2.ScheduleJobID = jobID(22)
	c3 := connectorFixture(3, "disabled", "cron", plan) // 连接器停用 → 停用任务
	c3.ScheduleJobID = jobID(33)
	c4 := connectorFixture(4, "active", "", nil) // 计划清空 → 停用任务（不删）
	c4.ScheduleJobID = jobID(44)
	c5 := connectorFixture(5, "active", "", nil) // 无计划也无绑定 → 什么都不做
	connectors := &fakeConnectorRepo{items: []*repository.Connector{c1, c2, c3, c4, c5}}

	scheduler := newFakeScheduler()
	scheduler.jobs = map[int64]*repository.AsyncdJob{
		22: {ID: 22, Name: ConnectorScheduleJobName(2), Enabled: true,
			ScheduleKind: repository.ScheduleKindInterval, IntervalSeconds: 60},
		33: {ID: 33, Name: ConnectorScheduleJobName(3), Enabled: true,
			ScheduleKind: repository.ScheduleKindCron, CronExpr: "0 3 * * *"},
		44: {ID: 44, Name: ConnectorScheduleJobName(4), Enabled: true,
			ScheduleKind: repository.ScheduleKindInterval, IntervalSeconds: 1800},
	}

	svc := &ProvisioningService{connectors: connectors, scheduler: scheduler}
	if err := svc.ReconcileConnectorScheduleJobs(ctx); err != nil {
		t.Fatalf("对账失败: %v", err)
	}

	created := map[string]bool{}
	for _, spec := range scheduler.upserts {
		created[spec.Name] = true
	}
	if !created[ConnectorScheduleJobName(1)] {
		t.Fatal("缺任务的连接器应被补建")
	}
	if c1.ScheduleJobID == nil {
		t.Fatal("补建后必须把任务主键回填到连接器上（绑定关系在连接器侧）")
	}
	if !created[ConnectorScheduleJobName(2)] {
		t.Fatal("规则漂移的连接器应被覆盖（60s → cron）")
	}
	if created[ConnectorScheduleJobName(4)] {
		t.Fatal("计划已清空的连接器不应重新创建任务（只停用）")
	}

	enabled := map[string]bool{}
	for _, item := range scheduler.enabled {
		enabled[item.Name] = item.Enabled
	}
	if enabled[ConnectorScheduleJobName(3)] != false {
		t.Fatal("连接器停用后应停用其计划任务")
	}
	if enabled[ConnectorScheduleJobName(4)] != false {
		t.Fatal("计划清空后应停用其计划任务（保留定义与历史）")
	}
	if _, touched := enabled[ConnectorScheduleJobName(5)]; touched {
		t.Fatal("既无计划也无绑定的连接器不应被触碰")
	}
	if job := scheduler.jobs[22]; job.ScheduleKind != repository.ScheduleKindCron || job.CronExpr != "0 3 * * *" {
		t.Fatalf("漂移的定义应被覆盖为 cron: %+v", job)
	}
}
