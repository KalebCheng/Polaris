package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"polaris/internal/service/cron"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/syncer"
)

// SyncerService 外部用户数据源服务：配置管理 + 同步执行。
type SyncerService struct {
	syncers     repository.SyncerRepository
	users       repository.UserRepository
	departments repository.DepartmentRepository
	// links / userDepts：外部身份锚点与多部门归属（Spec FR-1 / Q2 = A）
	links     repository.SyncerLinkRepository
	userDepts repository.UserDepartmentRepository
	jobTitles repository.JobTitleRepository
	orgs      repository.InstitutionRepository
	// providers：绑定到本连接的外部身份提供商（同源身份连接的反向认领用）。
	providers repository.ProviderRepository
	cipher    *auth.SecretCipher
	audits    *AuditRecorder
	// asyncd 任务底盘（入队 + 连接持有的计划任务）；由 app 装配，避免同步执行阻塞 HTTP 请求
	asyncd asyncdScheduler
}

// TaskTypeSyncerRun 同步器运行任务类型（asyncd 处理器注册名）
const TaskTypeSyncerRun = "syncer.run"

// NewSyncerService 创建同步器服务
func NewSyncerService(
	syncers repository.SyncerRepository,
	users repository.UserRepository,
	departments repository.DepartmentRepository,
	links repository.SyncerLinkRepository,
	userDepts repository.UserDepartmentRepository,
	jobTitles repository.JobTitleRepository,
	orgs repository.InstitutionRepository,
	providers repository.ProviderRepository,
	cipher *auth.SecretCipher,
	audits *AuditRecorder,
	asyncd asyncdScheduler,
) *SyncerService {
	return &SyncerService{
		syncers: syncers, users: users, departments: departments,
		links: links, userDepts: userDepts, jobTitles: jobTitles, orgs: orgs,
		providers: providers,
		cipher:    cipher, audits: audits, asyncd: asyncd,
	}
}

// SyncerInput 创建/更新入参
type SyncerInput struct {
	Syncer    *repository.Syncer
	Secret    string // 明文口令（数据库/LDAP）
	AppSecret string // 明文应用密钥（钉钉/飞书/企业微信）
}

// Create 创建同步器（敏感字段加密落库）
func (s *SyncerService) Create(ctx context.Context, actor string, in SyncerInput) (*repository.Syncer, error) {
	sy := in.Syncer
	normalizeSyncerMappings(sy)
	if sy.InstitutionID == 0 || sy.Name == "" || sy.Type == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institutionID, name and type are required")
	}
	if _, err := syncer.Get(sy.Type); err != nil {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	if err := validateSyncSchedule(sy.SyncConfig.Schedule); err != nil {
		return nil, err
	}
	if _, err := s.orgs.GetByID(ctx, sy.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", sy.InstitutionID)
	}
	if _, err := s.syncers.GetByName(ctx, sy.InstitutionID, sy.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "syncer already exists")
	}
	if err := s.encryptSecrets(sy, in); err != nil {
		return nil, err
	}
	id, err := s.syncers.Create(ctx, sy)
	if err != nil {
		return nil, err
	}
	sy.ID = id
	// 保存即接线：把「同步计划」变成 asyncd 的定时任务（失败会返回错误并由对账任务自愈）
	if err := s.applyScheduleJob(ctx, sy); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, actor, sy.InstitutionID, "syncer.create", sy.Name, nil)
	return sy, nil
}

// Update 更新同步器
func (s *SyncerService) Update(ctx context.Context, actor string, in SyncerInput) error {
	sy := in.Syncer
	normalizeSyncerMappings(sy)
	if err := validateSyncSchedule(sy.SyncConfig.Schedule); err != nil {
		return err
	}
	existing, err := s.syncers.GetByName(ctx, sy.InstitutionID, sy.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("syncer", sy.Name)
	}
	sy.ID = existing.ID
	// 未提供新密钥时保留原值
	if in.Secret == "" {
		sy.Password = existing.Password
	}
	if in.AppSecret == "" {
		sy.AppSecret = existing.AppSecret
	}
	if err := s.encryptSecrets(sy, in); err != nil {
		return err
	}
	if err := s.syncers.Update(ctx, sy); err != nil {
		return err
	}
	if err := s.applyScheduleJob(ctx, sy); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, sy.InstitutionID, "syncer.update", sy.Name, nil)
	return nil
}

// encryptSecrets 加密敏感字段（明文非空时覆盖）
func (s *SyncerService) encryptSecrets(sy *repository.Syncer, in SyncerInput) error {
	if in.Secret != "" {
		enc, err := s.cipher.Encrypt(in.Secret)
		if err != nil {
			return bizerrors.Wrap(bizerrors.CodeInternalError, "加密口令失败", err)
		}
		sy.Password = enc
	}
	if in.AppSecret != "" {
		enc, err := s.cipher.Encrypt(in.AppSecret)
		if err != nil {
			return bizerrors.Wrap(bizerrors.CodeInternalError, "加密应用密钥失败", err)
		}
		sy.AppSecret = enc
	}
	return nil
}

// Get 查询同步器（返回时抹除敏感字段明文）
func (s *SyncerService) Get(ctx context.Context, institutionID int64, name string) (*repository.Syncer, error) {
	sy, err := s.syncers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("syncer", name)
	}
	normalizeSyncerMappings(sy)
	return sy, nil
}

// Delete 删除同步器
func (s *SyncerService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	sy, err := s.syncers.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("syncer", name)
	}
	if err := s.syncers.Delete(ctx, sy.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "syncer.delete", name, nil)
	return nil
}

// List 分页列出同步器
func (s *SyncerService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Syncer, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.syncers.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		normalizeSyncerMappings(item)
	}
	total, err := s.syncers.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Runs 列出同步明细
func (s *SyncerService) Runs(ctx context.Context, institutionID int64, name string, offset, limit int) ([]*repository.SyncerRun, error) {
	sy, err := s.syncers.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("syncer", name)
	}
	return s.syncers.ListRuns(ctx, sy.ID, offset, limit)
}

// EnqueueRun 把一次同步入队到 asyncd（异步执行 + 单飞）。
func (s *SyncerService) EnqueueRun(ctx context.Context, sy *repository.Syncer, trigger string) (int64, bool, error) {
	if s.asyncd == nil {
		return 0, false, bizerrors.New(bizerrors.CodeInternalError, "asyncd service is not wired")
	}
	return s.asyncd.Enqueue(ctx, TaskTypeSyncerRun, map[string]interface{}{
		"syncer_id": sy.ID, "institution_id": sy.InstitutionID, "trigger": trigger,
	}, fmt.Sprintf("syncer:%d", sy.ID), time.Time{})
}

// ByID 按主键加载同步器（含租户归属校验），供 asyncd 任务处理器使用。
func (s *SyncerService) ByID(ctx context.Context, institutionID, id int64) (*repository.Syncer, error) {
	sy, err := s.syncers.GetByID(ctx, id)
	if err != nil || sy == nil || (institutionID != 0 && sy.InstitutionID != institutionID) {
		return nil, bizerrors.ErrNotFound.WithDetail("id", id)
	}
	normalizeSyncerMappings(sy)
	return sy, nil
}

// RunWithTrigger 按指定触发来源执行一次同步：拉取 → 差异 → 同步保护 → 落地 → 运行记录。
//
// 说明：本方法由 asyncd 的处理器调用（手动点击与定时扫描都是"入队 → 执行"这一条路径），
// 不再对外暴露同步执行入口，避免"HTTP 里跑全量同步"把请求拖死。
// 差异计算与试运行（DryRun）**共用同一份逻辑**，避免两套实现漂移。
func (s *SyncerService) RunWithTrigger(
	ctx context.Context, actor, trigger string, sy *repository.Syncer,
) (*repository.SyncCounts, error) {
	connector, err := syncer.Get(sy.Type)
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported syncer type")
	}
	started := time.Now()
	work := s.decrypted(sy)

	plan, err := s.buildPlan(ctx, sy, connector, work)
	if err != nil {
		// 拉取失败：外部数据不可得，绝不"静默成功"
		s.recordRunDetails(ctx, sy, trigger, started, SyncStatusFailed, "fetch external data failed",
			repository.SyncCounts{}, []string{err.Error()})
		return nil, bizerrors.Wrap(bizerrors.CodeServiceUnavailable, "fetch external data failed", err)
	}
	if tripped, message := guardTripped(plan, sy.SyncConfig.Guard); tripped {
		// 同步保护：取消本次任务，不写任何数据
		s.recordRunDetails(ctx, sy, trigger, started, SyncStatusAborted, message,
			repository.SyncCounts{}, nil)
		s.audits.Record(ctx, actor, sy.InstitutionID, "syncer.run_aborted", sy.Name,
			map[string]interface{}{"reason": message})
		return nil, bizerrors.New(bizerrors.CodeSyncGuardTripped, "sync cancelled by guard").
			WithDetail("reason", message)
	}

	counts, errs := s.applyPlan(ctx, sy, plan, work)
	status := SyncStatusSuccess
	if len(errs) > 0 {
		status = SyncStatusFailed
	}
	s.recordRunDetails(ctx, sy, trigger, started, status, summarizeWarnings(plan.warnings), counts, errs)
	s.audits.Record(ctx, actor, sy.InstitutionID, "syncer.run", sy.Name, map[string]interface{}{
		"trigger": trigger, "created": counts.Created, "updated": counts.Updated,
		"skipped": counts.Skipped, "failed": counts.Failed, "disabled": counts.Deleted,
	})
	return &counts, nil
}

func (s *SyncerService) decrypted(sy *repository.Syncer) *repository.Syncer {
	clone := *sy
	normalizeSyncerMappings(&clone)
	if clone.Password != "" {
		if v, err := s.cipher.Decrypt(clone.Password); err == nil {
			clone.Password = v
		}
	}
	if clone.AppSecret != "" {
		if v, err := s.cipher.Decrypt(clone.AppSecret); err == nil {
			clone.AppSecret = v
		}
	}
	return &clone
}

// normalizeSyncerMappings repairs mappings that would route department values
// into job titles. Keeping this in the service boundary makes legacy rows safe
// to read, display, run, and save even before an explicit data migration runs.
func normalizeSyncerMappings(sy *repository.Syncer) {
	if sy == nil {
		return
	}
	sy.AccountMapping = syncer.NormalizeAccountMapping(sy.Type, sy.AccountMapping)
	sy.DepartmentMapping = syncer.NormalizeDepartmentMapping(sy.Type, sy.DepartmentMapping)
	sy.JobTitleMapping = syncer.NormalizeJobTitleMapping(sy.Type, sy.JobTitleMapping)
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Describe 返回同步器类型清单（前端下拉用）。
func SyncerTypes() []string {
	types := syncer.Types()
	out := make([]string, 0, len(types))
	out = append(out, types...)
	return out
}

// applyScheduleJob 把连接的「同步计划」同步进 asyncd（保存路径；对账任务 ReconcileScheduleJobs 是它的自愈兜底）。
//
// 计划为空（「不自动」）→ 停用而不是删除：删定义会级联删掉它的历史执行记录。
func (s *SyncerService) applyScheduleJob(ctx context.Context, sy *repository.Syncer) error {
	if s.asyncd == nil {
		return nil // 未装配（测试/裁剪装配）：计划不生效但不阻断保存
	}
	spec, hasPlan := syncerScheduleJob(sy)
	if !hasPlan {
		// 「不自动」：停用已绑定的任务（保留定义与历史），连接侧仍保留 job_id 以便回显与后续启用
		if sy.ScheduleJobID != nil {
			if err := s.asyncd.SetEnabledByName(ctx, spec.Name, false); err != nil {
				logger.Error("同步计划任务停用失败（等待对账自愈）", "syncer", sy.Name, "error", err)
				return bizerrors.New(bizerrors.CodeInternalError, "schedule job not applied; it will be retried automatically")
			}
		}
		return nil
	}
	// 有计划的路径：upsert 任务（返回主键）→ 把主键回填到连接（连接是归属的唯一记录者）
	jobID, err := s.asyncd.UpsertJob(ctx, spec)
	if err != nil {
		logger.Error("同步计划任务写入失败（等待对账自愈）", "syncer", sy.Name, "error", err)
		return bizerrors.New(bizerrors.CodeInternalError, "schedule job not applied; it will be retried automatically")
	}
	if err := s.syncers.SetScheduleJobID(ctx, sy.ID, jobID); err != nil {
		logger.Error("同步计划任务绑定失败（等待对账自愈）", "syncer", sy.Name, "job_id", jobID, "error", err)
		return bizerrors.New(bizerrors.CodeInternalError, "schedule job not bound; it will be retried automatically")
	}
	sy.ScheduleJobID = &jobID
	return nil
}

// SetEnabled 启停连接：只改启用态，并让它的计划任务跟随（停用 → 任务停用，保留定义与历史）。
//
// 单独一个入口而不是让前端走 PUT：PUT 是全量更新，列表页手里的记录未必带全 sync_config，
// 一旦回写就会把计划配置清掉（"编辑即重置"的老毛病）。
func (s *SyncerService) SetEnabled(ctx context.Context, actor string, institutionID, id int64, enabled bool) error {
	sy, err := s.syncers.GetByID(ctx, id)
	if err != nil || sy == nil || (institutionID != 0 && sy.InstitutionID != institutionID) {
		return bizerrors.ErrNotFound.WithDetail("id", id)
	}
	if err := s.syncers.SetEnabled(ctx, id, enabled); err != nil {
		return err
	}
	sy.IsEnabled = enabled
	if err := s.applyScheduleJob(ctx, sy); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, sy.InstitutionID, "syncer.enable", sy.Name,
		map[string]interface{}{"enabled": enabled})
	return nil
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *SyncerService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.syncers.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}

// validateSyncSchedule 校验「同步配置 → 定时校验」的自洽性：
// interval 模式必须有正的间隔；daily 模式必须有 1–24 个 HH:MM 时刻。
// 放在服务端是为了与界面同源——后端也必须拒绝非法配置，不能只靠前端拦。
func validateSyncSchedule(schedule repository.SyncSchedule) error {
	switch schedule.Mode {
	case "":
		return nil
	case "interval":
		if schedule.IntervalSeconds <= 0 {
			return bizerrors.New(bizerrors.CodeInvalidParams, "interval schedule requires interval_seconds > 0")
		}
	case "cron":
		if _, err := cron.Parse(schedule.Cron); err != nil {
			return bizerrors.New(bizerrors.CodeInvalidParams, "invalid cron expression").WithDetail("error", err.Error())
		}
	case "daily":
		if len(schedule.Times) == 0 {
			return bizerrors.New(bizerrors.CodeInvalidParams, "daily schedule requires at least one time")
		}
		if len(schedule.Times) > 24 {
			return bizerrors.New(bizerrors.CodeInvalidParams, "daily schedule supports at most 24 times")
		}
		for _, item := range schedule.Times {
			if _, err := time.Parse("15:04", item); err != nil {
				return bizerrors.New(bizerrors.CodeInvalidParams, "invalid time format (expect HH:MM)").WithDetail("time", item)
			}
		}
	default:
		return bizerrors.New(bizerrors.CodeInvalidParams, "unsupported schedule mode").WithDetail("mode", schedule.Mode)
	}
	return nil
}
