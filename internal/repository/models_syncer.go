package repository

import "time"

// Syncer 外部用户数据源（对应 Casdoor 的 Syncer）。
// 通过连接器从外部系统（钉钉 / 飞书 / 企业微信 / LDAP / 数据库 / SCIM）拉取用户与组织，写入本地组织。
type Syncer struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Name          string `db:"name" json:"name"`
	DisplayName   string `db:"display_name" json:"display_name"`
	Type          string `db:"type" json:"type"`         // dingtalk | lark | wecom | ldap | database
	SubType       string `db:"sub_type" json:"sub_type"` // 接入变体：ad | ldap | mysql | postgres | feishu | lark
	IsEnabled     bool   `db:"is_enabled" json:"is_enabled"`

	// ── 类型专属配置（JSONB 落库；下列字段是运行期视图，不直接持久化）──
	//
	// Config 保存非敏感连接配置（host/port/user/database/table/database_type/ssl_mode/app_id/corp_id），
	// Secrets 保存凭据密文（password/app_secret，AES-GCM），FieldMapping 分别保存 account / department / job_title 字段映射。
	// 仓储层读取时展开到下面的扁平字段、写入时回收，因此对外 API/DTO 契约保持不变（见 Spec §5.4）。
	Config       map[string]string            `db:"config" json:"-"`
	Secrets      map[string]string            `db:"secrets" json:"-"`
	FieldMapping map[string]map[string]string `db:"field_mapping" json:"-"`

	Host         string `db:"-" json:"host"`
	Port         int    `db:"-" json:"port"`
	User         string `db:"-" json:"user"`
	Password     string `db:"-" json:"-"` // 密文；明文只能经 secret 字段提交（§3.1-7）
	Database     string `db:"-" json:"database"`
	Table        string `db:"-" json:"table"`
	DatabaseType string `db:"-" json:"database_type"`
	SSLMode      string `db:"-" json:"ssl_mode"`

	// 第三方开放平台应用凭证（钉钉/飞书/企业微信）
	AppID     string `db:"-" json:"app_id"`
	AppSecret string `db:"-" json:"-"` // 密文；明文只能经 app_secret_plain 提交
	CorpID    string `db:"-" json:"corp_id"`

	// 字段映射：本地字段 → 源字段（仅支持映射的连接器读取）。
	// account / department / job_title are independent contracts.
	AccountMapping    map[string]string `db:"-" json:"account_mapping"`
	DepartmentMapping map[string]string `db:"-" json:"department_mapping"`
	JobTitleMapping   map[string]string `db:"-" json:"job_title_mapping"`

	// 同步配置（JSONB）：增量、调度（每天时刻 / Cron + 执行窗口）、同步保护阈值
	SyncConfig SyncConfig `db:"sync_config" json:"sync_config"`
	// ScheduleJobID 本连接持有的 asyncd 定时任务主键（nil = 尚未绑定）。
	// 归属方向是"连接 → 任务"：asyncd 保持通用，不知道 syncer 的存在。
	ScheduleJobID *int64 `db:"schedule_job_id" json:"schedule_job_id"`
	// 同步目标：外部组织与账号挂到该部门节点之下（nil = 组织根）
	TargetDepartmentID *int64 `db:"target_department_id" json:"target_department_id"`

	// 同步策略
	SyncUsers  bool `db:"sync_users" json:"sync_users"`
	SyncGroups bool `db:"sync_groups" json:"sync_groups"`
	SoftDelete bool `db:"soft_delete" json:"soft_delete"` // 源端缺失的用户是否在本地停用

	LastRunTime *time.Time `db:"last_run_time" json:"last_run_time"`
	LastStatus  string     `db:"last_status" json:"last_status"` // idle | running | success | failed
	LastMessage string     `db:"last_message" json:"last_message"`
	LastCounts  []byte     `db:"last_counts" json:"last_counts"` // JSONB

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// SyncerRun 一次同步执行记录
type SyncerRun struct {
	ID            int64     `db:"id" json:"id"`
	SyncerID      int64     `db:"syncer_id" json:"syncer_id"`
	InstitutionID int64     `db:"institution_id" json:"institution_id"`
	Status        string    `db:"status" json:"status"`   // success | failed | skipped | aborted
	Trigger       string    `db:"trigger" json:"trigger"` // manual | schedule | retry
	Message       string    `db:"message" json:"message,omitempty"`
	Counts        []byte    `db:"counts" json:"counts,omitempty"` // JSONB：本次统计
	Errors        []byte    `db:"errors" json:"errors,omitempty"` // JSONB：逐条失败原因（最多 200 条）
	DurationMS    int       `db:"duration_ms" json:"duration_ms"` // 本次耗时（毫秒）
	StartedAt     time.Time `db:"started_at" json:"started_at"`
	FinishedAt    time.Time `db:"finished_at" json:"finished_at"`
}

// SyncSchedule 定时校验周期：每天多时刻（上限 24）或 Cron 表达式，二选一
type SyncSchedule struct {
	Mode string `json:"mode"` // daily | cron | ""（不自动）
	// Times 每天时刻（形如 "03:00"）；上限由服务层校验
	Times []string `json:"times,omitempty"`
	// IntervalSeconds 间隔模式下的周期（秒）
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	Cron            string `json:"cron,omitempty"`
	// WindowMinutes 执行窗口：到点后在该窗口内首次触达即执行一次（默认 30）
	WindowMinutes int `json:"window_minutes,omitempty"`
}

// SyncGuard 同步保护：需要删除的条目达到阈值即取消本次任务（0 = 不限制）
type SyncGuard struct {
	MaxUserDeletes int `json:"max_user_deletes"`
	MaxDeptDeletes int `json:"max_dept_deletes"`
}

// SyncConfig 入方向同步配置（syncers.sync_config）
type SyncConfig struct {
	// Incremental 增量同步（仅声明支持的连接器生效）
	Incremental bool         `json:"incremental"`
	Schedule    SyncSchedule `json:"schedule"`
	Guard       SyncGuard    `json:"guard"`
}

// Connector 下游应用连接器（推送型供给，对应 MaxKey 的 mxk_connectors）。
// 与 Syncer（上游目录拉取）相反：Connector 把本地账号推送到下游应用。
// 复用 service/syncer 的 HTTP 客户端基础设施。
type Connector struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID 关联应用主键（Spec Q5：存储只认主键）；nil = 通用连接器
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	// Application 只读 JOIN 产物（应用 code），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	Name        string `db:"name" json:"name"`
	DisplayName string `db:"display_name" json:"display_name"`
	// Type 连接器类型（http | dingtalk | lark | wecom | ldap），决定使用哪个 provisioner 实现
	Type string `db:"type" json:"type"`

	// Config/Secrets：与入方向同构（config = provider_url/principal；secrets = credentials 密文）
	Config  map[string]string `db:"config" json:"-"`
	Secrets map[string]string `db:"secrets" json:"-"`

	// 类型专属配置与凭据的**运行期视图**（存储为 config / secrets 两个 JSONB 列）。
	// 键由类型目录（service/connection）声明，写入时按声明校验，因此新增一种下游连接器
	// 不需要改表、也不需要改实体（避免"宽表 + 请求绑定漏字段导致配置被静默丢弃"）。
	Values map[string]string `db:"-" json:"-"`
	// HasSecrets 各敏感键是否已配置（响应只表达"已配置"，不下发明文）
	HasSecrets map[string]bool `db:"-" json:"-"`

	State string `db:"state" json:"state"` // active | disabled

	// SyncConfig 出方向计划（定时对齐检查）：与入方向同构的调度语义
	SyncConfig SyncConfig `db:"sync_config" json:"sync_config"`
	// ScheduleJobID 本连接器持有的 asyncd 定时任务主键（nil = 尚未绑定）。
	// 与入方向同构：归属方向是"连接器 → 任务"，asyncd 不知道 connector 的存在。
	ScheduleJobID *int64 `db:"schedule_job_id" json:"schedule_job_id"`

	LastRunTime *time.Time `db:"last_run_time" json:"last_run_time"`
	LastStatus  string     `db:"last_status" json:"last_status"` // idle | success | failed
	LastMessage string     `db:"last_message" json:"last_message"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// AccountStrategy 账号策略（对应 MaxKey 的 AccountsStrategy）。
// mapping：本地属性 → 下游属性；policy：何时建号 / 何时回收。
type AccountStrategy struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID 关联应用主键；nil = 租户内默认策略
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	// Application 只读 JOIN 产物（应用 code），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	Name        string `db:"name" json:"name"`
	DisplayName string `db:"display_name" json:"display_name"`
	IsEnabled   bool   `db:"is_enabled" json:"is_enabled"`

	Mapping map[string]string `db:"mapping" json:"mapping"` // 本地属性 → 下游属性
	Policy  map[string]string `db:"policy" json:"policy"`   // 建号/回收触发策略

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ApplicationAccount 应用账号（本地用户 × 应用 → 下游账号，对应 MaxKey 的 mxk_accounts）。
// 幂等键：(institution, application, user)。
type ApplicationAccount struct {
	ID            int64 `db:"id" json:"id"`
	InstitutionID int64 `db:"institution_id" json:"institution_id"`
	// ApplicationID / UserID 主键关联（Spec Q5 第 3 批；幂等键改为 (institution_id, application_id, user_id)）
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	UserID        *int64 `db:"user_id" json:"user_id,omitempty"`
	// Application / User 只读 JOIN 产物（应用 code / 用户账号名），仅为保持对外契约不变
	Application string `db:"-" json:"application,omitempty"`
	User        string `db:"-" json:"user,omitempty"`

	Username string `db:"username" json:"username"` // 本地用户名快照（= users.name）

	RelatedUsername string `db:"related_username" json:"related_username"` // 下游应用中的账号名
	// HasPassword 建号时是否向下游设置过初始口令。
	// 说明：口令本体**不落库**（既不存密文也不进审计）——本地没有任何需要它的场景，
	// 存下来只会让"一次库泄露"等于"所有下游账号被接管"（Spec FR-11 / Q6）。
	HasPassword bool `db:"has_password" json:"has_password"`

	CreateType string `db:"create_type" json:"create_type"` // automatic | manual
	Status     string `db:"status" json:"status"`           // active | disabled | pending | failed

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ProvisionHistory 供给历史（对应 MaxKey 的 mxk_history_provisions）。
type ProvisionHistory struct {
	ID            int64  `db:"id" json:"id"`
	InstitutionID int64  `db:"institution_id" json:"institution_id"`
	Connector     string `db:"connector" json:"connector"`
	// ApplicationID / UserID 主键关联（可空：对象删除后保留历史行）
	ApplicationID *int64 `db:"application_id" json:"application_id,omitempty"`
	UserID        *int64 `db:"user_id" json:"user_id,omitempty"`
	// Application / User 只读 JOIN 产物，仅为保持对外契约不变
	Application string    `db:"-" json:"application,omitempty"`
	User        string    `db:"-" json:"user,omitempty"`
	Action      string    `db:"action" json:"action"` // create | update | disable | delete | retry
	Status      string    `db:"status" json:"status"` // success | failed | pending
	Message     string    `db:"message" json:"message,omitempty"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// ── asyncd：任务定义 / 执行实例 / 执行记录 / 执行器实例 ───────────────────────────
// 四维度模型见 docs/specs/261015-asyncd-task-model.md §4。

// 任务定义的类别（asyncd_jobs.kind）
const (
	// AsyncdJobKindAsync 事件驱动：由业务动作按需入队，没有周期
	AsyncdJobKindAsync = "async"
	// AsyncdJobKindScheduled 定时任务：由调度器按周期/时刻入队
	AsyncdJobKindScheduled = "scheduled"
)

// 调度规则（asyncd_jobs.schedule_kind；仅 kind=scheduled 使用）
const (
	ScheduleKindInterval = "interval" // 每 N 秒
	ScheduleKindDaily    = "daily"    // 每天若干时刻（HH:MM），配合执行窗口
	ScheduleKindCron     = "cron"     // 标准 5 段 Cron 表达式（分 时 日 月 周），见 internal/service/cron
)

// 任务定义的上次调度结果（asyncd_jobs.last_status）
const (
	AsyncdJobStatusIdle    = "idle"    // 还没调度过
	AsyncdJobStatusQueued  = "queued"  // 本次已入队
	AsyncdJobStatusSkipped = "skipped" // 单飞键命中：窗口内已有待执行/执行中的任务，本次跳过
	AsyncdJobStatusFailed  = "failed"  // 入队失败（详见服务端日志）
)

// AsyncdJob 任务定义：一个任务"是什么"（处理器绑定 + 调度规则 + 启停）。
//
// 由代码在启动时声明并幂等 upsert（界面不提供增删改）；运行时**真的读它**：
// 调度器按它判到期、入队按它解析处理器、目录与执行记录按它归属。
type AsyncdJob struct {
	ID       int64  `db:"id" json:"id"`
	Name     string `db:"name" json:"name"`           // 定义名（历史记录按它归属）
	TaskType string `db:"task_type" json:"task_type"` // 处理器注册名（asyncd.Registry）
	Kind     string `db:"kind" json:"kind"`           // async | scheduled
	// ScheduleKind 调度规则：'' | interval | daily | cron
	ScheduleKind    string   `db:"schedule_kind" json:"schedule_kind"`
	IntervalSeconds int      `db:"interval_seconds" json:"interval_seconds"`
	DailyTimes      []string `db:"daily_times" json:"daily_times"`
	// CronExpr schedule_kind=cron 时的 5 段表达式
	CronExpr string `db:"cron_expr" json:"cron_expr"`
	// WindowMinutes 执行窗口：到点后该窗口内入队一次（窗口内已有任务则跳过）
	WindowMinutes int `db:"window_minutes" json:"window_minutes"`
	// Payload 调度入队时的载荷模板（异步任务的载荷由调用方给出）
	Payload []byte `db:"payload" json:"payload"` // JSONB
	// Enabled 仅 kind=scheduled 可停用（异步任务停用会变成静默丢任务）
	Enabled bool `db:"enabled" json:"enabled"`
	// UniqueKey 声明式单飞键（空 = 调度器回落 job:<name>）。
	// 业务侧可自定义（如身份连接写 syncer:<id>，与手工「立即同步」共用同一个键）。
	UniqueKey string `db:"unique_key" json:"unique_key"`
	// RetiredAt 代码声明退役时间（保留历史执行记录；任务目录默认不列）
	RetiredAt *time.Time `db:"retired_at" json:"retired_at,omitempty"`
	NextRunAt time.Time  `db:"next_run_at" json:"next_run_at"`
	LastRunAt *time.Time `db:"last_run_at" json:"last_run_at,omitempty"`
	// LastStatus idle | queued | skipped | failed
	LastStatus string    `db:"last_status" json:"last_status"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time `db:"updated_at" json:"updated_at"`
}

// 执行实例的状态（asyncd_job_runs.status）
const (
	AsyncdJobRunQueued    = "queued"    // 待领取（含延迟执行与退避等待）
	AsyncdJobRunRunning   = "running"   // 已被某个执行器槽位领取（持租约）
	AsyncdJobRunSucceeded = "succeeded" // 成功完成
	AsyncdJobRunDead      = "dead"      // 死信：重试耗尽或不可重试，等待人工处理
)

// AsyncdJobRun 执行实例：一次执行的载体（队列语义 + 状态机 + 租约 + 单飞键）。
//
// 与"执行记录"（AsyncdJobRunAttempt）的分工：本表回答"现在怎么样"（会被领取、回收、重试），
// 尝试表回答"当时发生了什么"（append-only，每次尝试一行）。
type AsyncdJobRun struct {
	ID    int64 `db:"id" json:"id"`
	JobID int64 `db:"job_id" json:"job_id"`
	// Status queued | running | succeeded | dead
	Status         string    `db:"status" json:"status"`
	Payload        []byte    `db:"payload" json:"payload"` // JSONB
	PayloadVersion int       `db:"payload_version" json:"payload_version"`
	Priority       int       `db:"priority" json:"priority"`
	Attempts       int       `db:"attempts" json:"attempts"`
	MaxAttempts    int       `db:"max_attempts" json:"max_attempts"`
	NextAttemptAt  time.Time `db:"next_attempt_at" json:"next_attempt_at"`
	// Deadline 任务级超时（零值 = 不限）
	Deadline *time.Time `db:"deadline" json:"deadline,omitempty"`
	// UniqueKey 单飞键：同一键在 queued/running 下唯一（空 = 不去重）
	UniqueKey string `db:"unique_key" json:"unique_key,omitempty"`
	// WorkerID 领取者（见 AsyncdWorker.ID，形如 host-pid#slot），配合租约实现崩溃恢复
	WorkerID       string     `db:"worker_id" json:"worker_id,omitempty"`
	LeaseExpiresAt *time.Time `db:"lease_expires_at" json:"lease_expires_at,omitempty"`
	LastError      string     `db:"last_error" json:"last_error,omitempty"`
	InstitutionID  *int64     `db:"institution_id" json:"institution_id,omitempty"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at" json:"updated_at"`
	FinishedAt     *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	// TaskType 处理器类型：由 Claim 从任务定义 join 带出（**不是 asyncd_job_runs 的列**），
	// 执行器据此找到处理器，因此不落列、不落缓存。
	TaskType string `db:"-" json:"task_type,omitempty"`
}

// 执行记录（每次尝试）的状态（asyncd_job_run_attempts.status）
const (
	AsyncdJobRunAttemptRunning   = "running"    // 正在跑
	AsyncdJobRunAttemptSucceeded = "succeeded"  // 本次尝试成功
	AsyncdJobRunAttemptFailed    = "failed"     // 本次尝试失败（可能还有下一次）
	AsyncdJobRunAttemptLeaseLost = "lease_lost" // 租约过期被判"执行者失联"，结果不可信
)

// AsyncdJobRunAttempt 执行记录：一次执行里的**单次尝试**（append-only）。
//
// 为什么要独立于执行实例：重试会覆盖实例上的 last_error / finished_at，
// 于是"第 2 次为什么失败、花了多久"这类问题在第一阶段的模型里答不出来。
type AsyncdJobRunAttempt struct {
	ID         int64      `db:"id" json:"id"`
	JobRunID   int64      `db:"job_run_id" json:"job_run_id"`
	AttemptNo  int        `db:"attempt_no" json:"attempt_no"`
	WorkerID   string     `db:"worker_id" json:"worker_id"`
	Status     string     `db:"status" json:"status"`
	StartedAt  time.Time  `db:"started_at" json:"started_at"`
	FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
	// DurationMs 本次尝试的真实耗时（不含退避等待）
	DurationMs int    `db:"duration_ms" json:"duration_ms"`
	Error      string `db:"error" json:"error,omitempty"`
}

// AsyncdWorker 执行器实例：一个可领取任务的槽位（进程 × 槽位序号）。
//
// 在线状态**不存列**，由 last_heartbeat_at / stopped_at 计算得出（见 Service.Workers）；
// 在跑任务数同样按 asyncd_job_runs.worker_id 实时聚合，避免派生标记漂移。
type AsyncdWorker struct {
	ID              string     `db:"id" json:"id"`                   // <host>-<pid>#<slot>
	InstanceID      string     `db:"instance_id" json:"instance_id"` // <host>-<pid>
	Host            string     `db:"host" json:"host"`
	PID             int        `db:"pid" json:"pid"`
	SlotIndex       int        `db:"slot_index" json:"slot_index"`
	StartedAt       time.Time  `db:"started_at" json:"started_at"`
	LastHeartbeatAt time.Time  `db:"last_heartbeat_at" json:"last_heartbeat_at"`
	StoppedAt       *time.Time `db:"stopped_at" json:"stopped_at,omitempty"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
}
