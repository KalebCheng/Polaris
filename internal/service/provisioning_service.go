package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/asyncd"
	"polaris/internal/service/connection"
	"polaris/internal/service/provisioner"
)

// 供给动作（写入 provision_history.action）。
const (
	ProvisionActionCreate  = "create"
	ProvisionActionUpdate  = "update"
	ProvisionActionDisable = "disable"
	ProvisionActionDelete  = "delete"
	ProvisionActionRetry   = "retry"
)

// 供给状态（写入 application_accounts.status / provision_history.status）。
const (
	ProvisionStatusActive   = "active"
	ProvisionStatusDisabled = "disabled"
	ProvisionStatusPending  = "pending"
	ProvisionStatusFailed   = "failed"
	ProvisionStatusSuccess  = "success"
	// ProvisionStatusSkipped 下游不支持该动作（如钉钉没有"停用态"）：如实记录跳过，不重试
	ProvisionStatusSkipped = "skipped"
)

// 建号触发策略（account_strategies.policy["createWhen"]）。
const (
	CreateWhenManual   = "manual"   // 仅手动建号
	CreateWhenOnCreate = "onCreate" // 用户创建即建号（默认）
	CreateWhenOnGrant  = "onGrant"  // 角色授予即建号
	CreateWhenJIT      = "jit"      // 首次登录即时建号
)

// 回收触发策略（account_strategies.policy["revokeWhen"]）。
const (
	RevokeWhenDisable  = "disable"  // 用户禁用即禁用下游账号（默认）
	RevokeWhenDelete   = "delete"   // 用户删除即删除下游账号
	RevokeWhenOffboard = "offboard" // 离职（软删除）即删除下游账号
)

// provisionTask 一次供给任务。
type provisionTask struct {
	InstitutionID int64
	Application   string
	User          string
	Action        string
}

// taskEnqueuer 抽出 asyncd 的入队能力，便于单测验证"删除/ JIT 是否真的派发了任务"。
type taskEnqueuer interface {
	Enqueue(ctx context.Context, jobName string, payload any, uniqueKey string, runAt time.Time, opts ...asyncd.EnqueueOption) (int64, bool, error)
}

// ProvisioningService 应用账号供给与生命周期服务（P2）。
//
// 职责：
//   - 连接器 / 账号策略 / 应用账号 / 供给历史的配置管理；
//   - 供给任务编排（进程内工作池异步执行 + scheduler 兜底重试）；
//   - 与用户/角色变更事件挂钩（创建 / 禁用 / 离职 / 角色授予 / 登录 JIT）。
//
// 幂等键：(institutionID, application, user)；下游失败以 application_accounts.status='failed' 记录，
// 由 scheduler 的 provision-retry 任务重新入队直至成功。
type ProvisioningService struct {
	connectors repository.ConnectorRepository
	strategies repository.AccountStrategyRepository
	accounts   repository.ApplicationAccountRepository
	history    repository.ProvisionHistoryRepository
	users      repository.UserRepository
	apps       repository.ApplicationRepository
	orgs       repository.InstitutionRepository
	cipher     *auth.SecretCipher
	audits     *AuditRecorder

	// access 应用访问授权（可选；装配阶段注入）：门户应用中心按它过滤（FR-5）。
	access *ApplicationAccessService

	// asyncd 任务入队（异步执行 + 单飞）：由 app 装配注入。
	// 说明：此前用的是进程内 channel，**队列满会直接丢任务**，且多实例各自为战——
	// 换成 asyncd 后，任务落库、可重试、可观测，且同一账号不会被重复排队。
	asyncd taskEnqueuer
	// scheduler 出方向计划需要的 asyncd 能力（与入方向的 asyncdScheduler 对称）。
	// 归属方向是"连接器 → 任务"：asyncd 不存任何业务属性。
	scheduler connectorScheduler
}

// NewProvisioningService 创建供给服务。
func NewProvisioningService(
	connectors repository.ConnectorRepository,
	strategies repository.AccountStrategyRepository,
	accounts repository.ApplicationAccountRepository,
	history repository.ProvisionHistoryRepository,
	users repository.UserRepository,
	apps repository.ApplicationRepository,
	orgs repository.InstitutionRepository,
	cipher *auth.SecretCipher,
	audits *AuditRecorder,
	asyncd *asyncd.Service,
	access *ApplicationAccessService,
) *ProvisioningService {
	return &ProvisioningService{
		connectors: connectors, strategies: strategies, accounts: accounts, history: history,
		users: users, apps: apps, orgs: orgs, cipher: cipher, audits: audits,
		asyncd: asyncd, scheduler: asyncd, access: access,
	}
}

// TaskTypeProvisionAccount 供给任务类型（asyncd 处理器注册名）
const TaskTypeProvisionAccount = "provision.account"

// resolveAppID 把应用标识（code）解析为主键（0 = 未解析到）。
// 任务载荷（provisionTask）里存的是 code / 账号名——它是**瞬时命令**而非存储；
// 落库一律用主键（§3.8），因此在写库前于服务层完成 code → id 解析。
func (s *ProvisioningService) resolveAppID(ctx context.Context, institutionID int64, code string) int64 {
	if code == "" || s.apps == nil {
		return 0
	}
	app, err := s.apps.GetByCode(ctx, institutionID, code)
	if err != nil || app == nil {
		return 0
	}
	return app.ID
}

// resolveUserID 把用户账号名解析为主键（0 = 未解析到）
func (s *ProvisioningService) resolveUserID(ctx context.Context, institutionID int64, name string) int64 {
	if name == "" || s.users == nil {
		return 0
	}
	u, err := s.users.GetByName(ctx, institutionID, name)
	if err != nil || u == nil {
		return 0
	}
	return u.ID
}

// ProvisionTaskFromPayload 解析 asyncd 任务载荷为供给任务（供装配层构造调用）。
func ProvisionTaskFromPayload(payload map[string]any) ProvisionTask {
	return ProvisionTask{
		InstitutionID: int64(payloadNumber(payload, "institution_id")),
		Application:   payloadString(payload, "application"),
		User:          payloadString(payload, "user"),
		Action:        payloadString(payload, "action"),
	}
}

// Field 目录字段定义（连接器配置的 schema 来源）
type Field = connection.Field

// ProvisionTask 一次供给任务（对装配层可见；避免把内部结构体暴露给 app 包）
type ProvisionTask = provisionTask

// enqueue 把一次供给动作交给 asyncd 异步执行。
// 单飞键 = 机构×应用×用户（与 application_accounts 的幂等键一致）：同一账号在途时不会重复排队。
func (s *ProvisioningService) enqueue(ctx context.Context, task provisionTask) {
	if s.asyncd == nil {
		logger.Error("provisioning: asyncd 未装配，任务被丢弃", "application", task.Application, "user", task.User)
		return
	}
	_, _, err := s.asyncd.Enqueue(ctx, TaskTypeProvisionAccount, map[string]interface{}{
		"institution_id": task.InstitutionID, "application": task.Application,
		"user": task.User, "action": task.Action,
	}, ProvisionUniqueKey(task.InstitutionID, task.Application, task.User), time.Time{})
	if err != nil {
		logger.Warn("provisioning enqueue failed", "application", task.Application, "user", task.User, "error", err)
	}
}

// ProvisionUniqueKey 供给任务的单飞键（与 application_accounts 幂等键一致，导出以便测试与排查）
func ProvisionUniqueKey(institutionID int64, application, user string) string {
	return fmt.Sprintf("provision:%d:%s:%s", institutionID, application, user)
}

// ── 触发点（供 UserService / RoleService / AuthService 调用）────

// TriggerUserProvisioning 用户创建：对租户内所有"创建即建号"的账号策略派发建号任务。
func (s *ProvisioningService) TriggerUserProvisioning(ctx context.Context, institutionID int64, user string) {
	for _, app := range s.autoCreateApplications(ctx, institutionID, CreateWhenOnCreate) {
		s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: app, User: user, Action: ProvisionActionCreate})
	}
}

// TriggerRoleGranted 角色授予：对"授权即建号"的账号策略派发建号任务。
func (s *ProvisioningService) TriggerRoleGranted(ctx context.Context, institutionID int64, user string) {
	for _, app := range s.autoCreateApplications(ctx, institutionID, CreateWhenOnGrant) {
		s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: app, User: user, Action: ProvisionActionCreate})
	}
}

// TriggerUserStatusChange 用户状态变更（禁用 / 离职）：按账号策略的回收触发策略派发回收任务。
//   - revokeWhen=delete：任何停用都删除下游账号；
//   - revokeWhen=offboard：仅离职（deleted）删除，普通禁用保留；
//   - revokeWhen=disable（默认）：停用即禁用下游账号。
func (s *ProvisioningService) TriggerUserStatusChange(ctx context.Context, institutionID int64, user string, deleted bool) {
	for _, acc := range s.userAccounts(ctx, institutionID, user) {
		strategy, _ := s.strategies.FindForApplication(ctx, institutionID, s.resolveAppID(ctx, institutionID, acc.Application))
		var action string
		switch policyRevokeWhen(strategy) {
		case RevokeWhenDelete:
			action = ProvisionActionDelete
		case RevokeWhenOffboard:
			if !deleted {
				continue // 普通禁用保留下游账号
			}
			action = ProvisionActionDelete
		default:
			action = ProvisionActionDisable
		}
		s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: acc.Application, User: user, Action: action})
	}
}

// TriggerJIT 登录即时建号：以账号策略 policy.createWhen=jit 为唯一触发源。
//
// 历史上连接器上还有一个 just_in_time 开关（Spec D12），同一件事两个开关导致"配了不生效"。
// 该字段已由迁移 048 删除；这里只读策略，且仍要求应用存在启用的出方向连接器作为执行者。
func (s *ProvisioningService) TriggerJIT(ctx context.Context, institutionID int64, application, user string) {
	appID := s.resolveAppID(ctx, institutionID, application)
	if appID == 0 || s.strategies == nil {
		return
	}
	strategy, err := s.strategies.FindForApplication(ctx, institutionID, appID)
	if err != nil || strategy == nil || !strategy.IsEnabled || policyCreateWhen(strategy) != CreateWhenJIT {
		return
	}
	if s.connectors == nil {
		return
	}
	if _, err := s.primaryConnector(ctx, institutionID, application); err != nil {
		return
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: application, User: user, Action: ProvisionActionCreate})
}

// TriggerRetry 兜底重试：扫描失败的应用账号，重新派发建号任务。
func (s *ProvisioningService) TriggerRetry(ctx context.Context) {
	const maxRetry = 50
	q := repository.ListQuery{Field: "status", Value: ProvisionStatusFailed, Limit: maxRetry}
	items, err := s.accounts.List(ctx, q)
	if err != nil {
		logger.Warn("provision retry scan failed", "error", err)
		return
	}
	for _, acc := range items {
		s.enqueue(ctx, provisionTask{
			InstitutionID: acc.InstitutionID, Application: acc.Application,
			User: acc.User, Action: ProvisionActionCreate,
		})
	}
}

// autoCreateApplications 返回租户内启用且创建触发策略匹配的应用名集合（去重）。
func (s *ProvisioningService) autoCreateApplications(ctx context.Context, institutionID int64, trigger string) []string {
	strategies, err := s.strategies.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 500})
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, st := range strategies {
		if !st.IsEnabled || st.Application == "" {
			continue
		}
		if policyCreateWhen(st) != trigger {
			continue
		}
		if seen[st.Application] {
			continue
		}
		seen[st.Application] = true
		out = append(out, st.Application)
	}
	return out
}

// userAccounts 返回某用户在本租户下已有的应用账号。
func (s *ProvisioningService) userAccounts(ctx context.Context, institutionID int64, user string) []*repository.ApplicationAccount {
	items, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "user_id", Value: strconv.FormatInt(s.resolveUserID(ctx, institutionID, user), 10), Limit: 500,
	})
	if err != nil {
		return nil
	}
	return items
}

// ── 任务执行 ──────────────────────────────────────────────────

// ProcessTask 执行一次供给任务（由 asyncd 的 worker 调用）。
// 幂等：以 (institutionID, application, user) 为键；失败由 asyncd 退避重试，超限进死信。
func (s *ProvisioningService) ProcessTask(ctx context.Context, task ProvisionTask) error {
	user, err := s.users.GetByName(ctx, task.InstitutionID, task.User)
	if err != nil {
		logger.Warn("provision: user not found", "user", task.User, "institutionID", task.InstitutionID)
		return nil
	}
	conn, err := s.primaryConnector(ctx, task.InstitutionID, task.Application)
	if err != nil {
		// 应用未配置连接器：静默跳过（未纳入供给范围）
		return nil
	}
	prov, err := provisioner.Get(connectorProvisionerType(conn))
	if err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return err
	}
	cfg := s.connectorConfig(conn)
	caps := prov.Capabilities(cfg)

	// 下游能力前置校验：不支持的动作**提前拒绝**（而不是发请求后失败，更不是无限重试）。
	// 例：钉钉通讯录没有"停用态"（只能删除），选择"禁用即停用下游"的策略时如实记录 skipped。
	switch task.Action {
	case ProvisionActionDisable:
		if !caps.Disable {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持停用账号，已跳过（账号保留）")
			return nil
		}
	case ProvisionActionDelete:
		if !caps.Delete {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持删除账号，已跳过")
			return nil
		}
	}
	if (task.Action == ProvisionActionUpdate || task.Action == ProvisionActionRetry) && !caps.Update {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持更新账号，已跳过")
		return nil
	}

	switch task.Action {
	case ProvisionActionCreate, ProvisionActionUpdate, ProvisionActionRetry:
		return s.processCreate(ctx, task, prov, cfg, user, conn)
	case ProvisionActionDisable:
		return s.processRecycle(ctx, task, prov, cfg, user, conn, ProvisionActionDisable)
	case ProvisionActionDelete:
		return s.processRecycle(ctx, task, prov, cfg, user, conn, ProvisionActionDelete)
	}
	return nil
}

func (s *ProvisioningService) processCreate(
	ctx context.Context, task provisionTask, prov provisioner.Provisioner,
	cfg provisioner.Config, user *repository.User, conn *repository.Connector,
) error {
	strategy, _ := s.strategies.FindForApplication(ctx, task.InstitutionID, s.resolveAppID(ctx, task.InstitutionID, task.Application))
	existing, _ := s.accounts.GetByUserApp(ctx, task.InstitutionID,
		s.resolveAppID(ctx, task.InstitutionID, task.Application), s.resolveUserID(ctx, task.InstitutionID, task.User))

	account, plainPassword := s.buildAccount(user, strategy, existing, prov.Capabilities(cfg).Password)

	// 已存在：走更新（改密 / 属性同步）
	if existing != nil {
		account.Username = firstNonEmptyStr(existing.RelatedUsername, existing.Username)
		if err := prov.Update(ctx, cfg, account); err != nil {
			existing.Status = ProvisionStatusFailed
			_ = s.accounts.Update(ctx, existing)
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return err
		}
		markPasswordSet(existing, plainPassword)
		existing.Status = ProvisionStatusActive
		if err := s.accounts.Update(ctx, existing); err != nil {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return err
		}
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
		s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.update", task.User,
			map[string]interface{}{"application": task.Application})
		return nil
	}

	// 首次建号：先落 pending 记录（保证幂等键），成功后再置 active
	downstreamName, err := prov.Provision(ctx, cfg, account)
	if err != nil {
		s.createFailedAccount(ctx, task, account, err)
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return err
	}
	if downstreamName == "" {
		downstreamName = account.Username
	}
	record := &repository.ApplicationAccount{
		InstitutionID: task.InstitutionID,
		ApplicationID: nullableID(s.resolveAppID(ctx, task.InstitutionID, task.Application)),
		UserID:        nullableID(s.resolveUserID(ctx, task.InstitutionID, task.User)),
		Username:      user.Name, RelatedUsername: downstreamName, CreateType: "automatic",
		Status: ProvisionStatusActive,
	}
	markPasswordSet(record, plainPassword)
	if _, err := s.accounts.Create(ctx, record); err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return err
	}
	s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
	s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.create", task.User,
		map[string]interface{}{"application": task.Application, "downstream": downstreamName})
	return nil
}

func (s *ProvisioningService) processRecycle(
	ctx context.Context, task provisionTask, prov provisioner.Provisioner,
	cfg provisioner.Config, user *repository.User, conn *repository.Connector, action string,
) error {
	acc, err := s.accounts.GetByUserApp(ctx, task.InstitutionID,
		s.resolveAppID(ctx, task.InstitutionID, task.Application), s.resolveUserID(ctx, task.InstitutionID, task.User))
	if err != nil {
		return nil // 无下游账号，无需回收
	}
	account := &provisioner.Account{Username: firstNonEmptyStr(acc.RelatedUsername, acc.Username)}

	if action == ProvisionActionDisable {
		if err := prov.Disable(ctx, cfg, account); err != nil {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return err
		}
		acc.Status = ProvisionStatusDisabled
		if err := s.accounts.Update(ctx, acc); err != nil {
			return err
		}
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
		s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.disable", task.User,
			map[string]interface{}{"application": task.Application})
		return nil
	}
	// delete：先确认下游是否还存在。failed 账号可能从未建成功；重复删除也应幂等——
	// 确认不存在时只清理本地台账，不再调用下游 delete 把"删除不存在的账号"记成失败。
	if reporter, ok := prov.(provisioner.ExistsReporter); ok {
		result, err := reporter.ExistsUser(ctx, cfg, account.Username)
		if err == nil && result.State == provisioner.ExistsNo {
			if err := s.accounts.Delete(ctx, acc.ID); err != nil {
				return err
			}
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游账号已不存在，已清理本地台账")
			s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.delete", task.User,
				map[string]interface{}{"application": task.Application, "downstream": account.Username})
			return nil
		}
	}
	if err := prov.Delete(ctx, cfg, account); err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return err
	}
	if err := s.accounts.Delete(ctx, acc.ID); err != nil {
		return err
	}
	s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
	s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.delete", task.User,
		map[string]interface{}{"application": task.Application})
	return nil
}

// createFailedAccount 记录一条失败的 pending 账号，供 scheduler 兜底重试。
func (s *ProvisioningService) createFailedAccount(ctx context.Context, task provisionTask, account *provisioner.Account, cause error) {
	appID := s.resolveAppID(ctx, task.InstitutionID, task.Application)
	userID := s.resolveUserID(ctx, task.InstitutionID, task.User)
	if _, err := s.accounts.GetByUserApp(ctx, task.InstitutionID, appID, userID); err == nil {
		return // 已存在记录，无需重建
	}
	rec := &repository.ApplicationAccount{
		InstitutionID: task.InstitutionID, ApplicationID: &appID, UserID: &userID,
		Username: account.Username, RelatedUsername: account.Username,
		CreateType: "automatic", Status: ProvisionStatusFailed,
	}
	if _, err := s.accounts.Create(ctx, rec); err != nil {
		logger.Warn("record failed app account", "error", err, "user", task.User)
	}
}

// buildAccount 依据用户与账号策略构造下游账号载荷；返回账号与本次生成的明文口令。
func (s *ProvisioningService) buildAccount(
	user *repository.User, strategy *repository.AccountStrategy, existing *repository.ApplicationAccount, withPassword bool,
) (*provisioner.Account, string) {
	plainPassword := ""
	if withPassword {
		plainPassword = randomHex(8) // 16 位随机明文口令（下游首次建号所需）
	}
	account := &provisioner.Account{
		Username:    user.Name,
		DisplayName: user.DisplayName,
		Email:       user.Email,
		Phone:       user.Phone,
		Attributes:  map[string]string{},
	}
	// 已有账号：改密沿用原下游账号名，不重置口令
	if existing != nil {
		account.Username = firstNonEmptyStr(existing.RelatedUsername, existing.Username)
		return account, ""
	}
	if plainPassword != "" {
		account.Password = plainPassword
	}

	// 策略 mapping：本地属性 → 下游属性
	for local, downstream := range strategyMapping(strategy) {
		if v, ok := localAttribute(user, local); ok && downstream != "" {
			account.Attributes[downstream] = v
		}
	}
	if len(account.Attributes) == 0 {
		account.Attributes = nil
	}
	return account, plainPassword
}

// markPasswordSet 记录"本次向下游设置过初始口令"这一事实（口令本体不落库）。
func markPasswordSet(acc *repository.ApplicationAccount, plain string) {
	if plain != "" {
		acc.HasPassword = true
	}
}

// connectorConfig 解密连接器凭证并构造运行时配置。
func (s *ProvisioningService) connectorConfig(conn *repository.Connector) provisioner.Config {
	values := map[string]string{}
	for key, value := range conn.Values {
		if conn.HasSecrets[key] {
			// 敏感键：解密后交给连接器（密文不出服务层）
			if s.cipher != nil {
				if plain, err := s.cipher.Decrypt(value); err == nil {
					values[key] = plain
					continue
				}
			}
			continue
		}
		values[key] = value
	}
	return provisioner.Config{Type: conn.Type, Values: values}
}

// primaryConnector 返回应用下首个启用连接器。
func (s *ProvisioningService) primaryConnector(ctx context.Context, institutionID int64, application string) (*repository.Connector, error) {
	return s.primaryConnectorByAppID(ctx, institutionID, s.resolveAppID(ctx, institutionID, application))
}

func (s *ProvisioningService) primaryConnectorByAppID(ctx context.Context, institutionID, applicationID int64) (*repository.Connector, error) {
	conns, err := s.connectors.ListEnabled(ctx, institutionID, applicationID)
	if err != nil {
		return nil, err
	}
	if len(conns) == 0 {
		return nil, bizerrors.ErrNotFound.WithDetail("application_id", applicationID)
	}
	return conns[0], nil
}

func (s *ProvisioningService) recordHistory(ctx context.Context, task provisionTask, connector, status, message string) {
	appID := s.resolveAppID(ctx, task.InstitutionID, task.Application)
	userID := s.resolveUserID(ctx, task.InstitutionID, task.User)
	_, _ = s.history.Create(ctx, &repository.ProvisionHistory{
		InstitutionID: task.InstitutionID, Connector: connector,
		ApplicationID: nullableID(appID), UserID: nullableID(userID),
		Action: task.Action, Status: status, Message: message,
	})
}

// ── 配置管理（连接器 / 账号策略）────────────────────────────────

// ConnectorInput 连接器创建/更新入参。
