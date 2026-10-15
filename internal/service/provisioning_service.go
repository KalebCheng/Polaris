package service

import (
	"context"
	"fmt"
	"strings"
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

// 供给状态（写入 app_accounts.status / provision_history.status）。
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

// ProvisioningService 应用账号供给与生命周期服务（P2）。
//
// 职责：
//   - 连接器 / 账号策略 / 应用账号 / 供给历史的配置管理；
//   - 供给任务编排（进程内工作池异步执行 + scheduler 兜底重试）；
//   - 与用户/角色变更事件挂钩（创建 / 禁用 / 离职 / 角色授予 / 登录 JIT）。
//
// 幂等键：(institutionID, application, user)；下游失败以 app_accounts.status='failed' 记录，
// 由 scheduler 的 provision-retry 任务重新入队直至成功。
type ProvisioningService struct {
	connectors repository.ConnectorRepository
	strategies repository.AccountStrategyRepository
	accounts   repository.AppAccountRepository
	history    repository.ProvisionHistoryRepository
	users      repository.UserRepository
	apps       repository.ApplicationRepository
	orgs       repository.InstitutionRepository
	cipher     *auth.SecretCipher
	audits     *AuditRecorder

	// asyncd 任务入队（异步执行 + 单飞）：由 app 装配注入。
	// 说明：此前用的是进程内 channel，**队列满会直接丢任务**，且多实例各自为战——
	// 换成 asyncd 后，任务落库、可重试、可观测，且同一账号不会被重复排队。
	asyncd *asyncd.Service
}

// NewProvisioningService 创建供给服务。
func NewProvisioningService(
	connectors repository.ConnectorRepository,
	strategies repository.AccountStrategyRepository,
	accounts repository.AppAccountRepository,
	history repository.ProvisionHistoryRepository,
	users repository.UserRepository,
	apps repository.ApplicationRepository,
	orgs repository.InstitutionRepository,
	cipher *auth.SecretCipher,
	audits *AuditRecorder,
) *ProvisioningService {
	return &ProvisioningService{
		connectors: connectors, strategies: strategies, accounts: accounts, history: history,
		users: users, apps: apps, orgs: orgs, cipher: cipher, audits: audits,
	}
}

// TaskTypeProvisionAccount 供给任务类型（asyncd 处理器注册名）
const TaskTypeProvisionAccount = "provision.account"

// SetAsyncd 注入 asyncd（装配期调用）
func (s *ProvisioningService) SetAsyncd(service *asyncd.Service) { s.asyncd = service }

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
// 单飞键 = 机构×应用×用户（与 app_accounts 的幂等键一致）：同一账号在途时不会重复排队。
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

// ProvisionUniqueKey 供给任务的单飞键（与 app_accounts 幂等键一致，导出以便测试与排查）
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
		strategy, _ := s.strategies.FindForApplication(ctx, institutionID, acc.Application)
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

// TriggerJIT 登录即时建号：仅当该应用存在 just_in_time 的连接器时派发。
func (s *ProvisioningService) TriggerJIT(ctx context.Context, institutionID int64, application, user string) {
	conns, err := s.connectors.ListEnabled(ctx, institutionID, application)
	if err != nil {
		return
	}
	for _, c := range conns {
		if c.JustInTime {
			s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: application, User: user, Action: ProvisionActionCreate})
			return
		}
	}
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
func (s *ProvisioningService) userAccounts(ctx context.Context, institutionID int64, user string) []*repository.AppAccount {
	items, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "user", Value: user, Limit: 500,
	})
	if err != nil {
		return nil
	}
	return items
}

// ── 任务执行 ──────────────────────────────────────────────────

// ProcessTask 执行一次供给任务（由 asyncd 的 worker 调用）。
// 幂等：以 (institutionID, application, user) 为键；失败由 asyncd 退避重试，超限进死信。
func (s *ProvisioningService) ProcessTask(ctx context.Context, task ProvisionTask) {
	user, err := s.users.GetByName(ctx, task.InstitutionID, task.User)
	if err != nil {
		logger.Warn("provision: user not found", "user", task.User, "institutionID", task.InstitutionID)
		return
	}
	conn, err := s.primaryConnector(ctx, task.InstitutionID, task.Application)
	if err != nil {
		// 应用未配置连接器：静默跳过（未纳入供给范围）
		return
	}
	prov, err := provisioner.Get(connectorProvisionerType(conn))
	if err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return
	}
	cfg := s.connectorConfig(conn)
	caps := prov.Capabilities(cfg)

	// 下游能力前置校验：不支持的动作**提前拒绝**（而不是发请求后失败，更不是无限重试）。
	// 例：钉钉通讯录没有"停用态"（只能删除），选择"禁用即停用下游"的策略时如实记录 skipped。
	switch task.Action {
	case ProvisionActionDisable:
		if !caps.Disable {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持停用账号，已跳过（账号保留）")
			return
		}
	case ProvisionActionDelete:
		if !caps.Delete {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持删除账号，已跳过")
			return
		}
	}
	if (task.Action == ProvisionActionUpdate || task.Action == ProvisionActionRetry) && !caps.Update {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSkipped, "下游连接器不支持更新账号，已跳过")
		return
	}

	switch task.Action {
	case ProvisionActionCreate, ProvisionActionUpdate, ProvisionActionRetry:
		s.processCreate(ctx, task, prov, cfg, user, conn)
	case ProvisionActionDisable:
		s.processRecycle(ctx, task, prov, cfg, user, conn, ProvisionActionDisable)
	case ProvisionActionDelete:
		s.processRecycle(ctx, task, prov, cfg, user, conn, ProvisionActionDelete)
	}
}

func (s *ProvisioningService) processCreate(
	ctx context.Context, task provisionTask, prov provisioner.Provisioner,
	cfg provisioner.Config, user *repository.User, conn *repository.Connector,
) {
	strategy, _ := s.strategies.FindForApplication(ctx, task.InstitutionID, task.Application)
	existing, _ := s.accounts.GetByUserApp(ctx, task.InstitutionID, task.Application, task.User)

	account, plainPassword := s.buildAccount(user, strategy, existing, prov.Capabilities(cfg).Password)

	// 已存在：走更新（改密 / 属性同步）
	if existing != nil {
		account.Username = firstNonEmptyStr(existing.RelatedUsername, existing.Username)
		if err := prov.Update(ctx, cfg, account); err != nil {
			existing.Status = ProvisionStatusFailed
			_ = s.accounts.Update(ctx, existing)
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return
		}
		markPasswordSet(existing, plainPassword)
		existing.Status = ProvisionStatusActive
		if err := s.accounts.Update(ctx, existing); err != nil {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return
		}
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
		s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.update", task.User,
			map[string]interface{}{"application": task.Application})
		return
	}

	// 首次建号：先落 pending 记录（保证幂等键），成功后再置 active
	downstreamName, err := prov.Provision(ctx, cfg, account)
	if err != nil {
		s.createFailedAccount(ctx, task, account, err)
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return
	}
	if downstreamName == "" {
		downstreamName = account.Username
	}
	record := &repository.AppAccount{
		InstitutionID: task.InstitutionID, Application: task.Application, User: task.User,
		Username: user.Name, RelatedUsername: downstreamName, CreateType: "automatic",
		Status: ProvisionStatusActive,
	}
	markPasswordSet(record, plainPassword)
	if _, err := s.accounts.Create(ctx, record); err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return
	}
	s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
	s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.create", task.User,
		map[string]interface{}{"application": task.Application, "downstream": downstreamName})
}

func (s *ProvisioningService) processRecycle(
	ctx context.Context, task provisionTask, prov provisioner.Provisioner,
	cfg provisioner.Config, user *repository.User, conn *repository.Connector, action string,
) {
	acc, err := s.accounts.GetByUserApp(ctx, task.InstitutionID, task.Application, task.User)
	if err != nil {
		return // 无下游账号，无需回收
	}
	account := &provisioner.Account{Username: firstNonEmptyStr(acc.RelatedUsername, acc.Username)}

	if action == ProvisionActionDisable {
		if err := prov.Disable(ctx, cfg, account); err != nil {
			s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
			return
		}
		acc.Status = ProvisionStatusDisabled
		_ = s.accounts.Update(ctx, acc)
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
		s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.disable", task.User,
			map[string]interface{}{"application": task.Application})
		return
	}
	// delete
	if err := prov.Delete(ctx, cfg, account); err != nil {
		s.recordHistory(ctx, task, conn.Name, ProvisionStatusFailed, err.Error())
		return
	}
	_ = s.accounts.Delete(ctx, acc.ID)
	s.recordHistory(ctx, task, conn.Name, ProvisionStatusSuccess, "")
	s.audits.Record(ctx, "provisioning", task.InstitutionID, "app_account.delete", task.User,
		map[string]interface{}{"application": task.Application})
}

// createFailedAccount 记录一条失败的 pending 账号，供 scheduler 兜底重试。
func (s *ProvisioningService) createFailedAccount(ctx context.Context, task provisionTask, account *provisioner.Account, cause error) {
	if _, err := s.accounts.GetByUserApp(ctx, task.InstitutionID, task.Application, task.User); err == nil {
		return // 已存在记录，无需重建
	}
	rec := &repository.AppAccount{
		InstitutionID: task.InstitutionID, Application: task.Application, User: task.User,
		Username: account.Username, RelatedUsername: account.Username,
		CreateType: "automatic", Status: ProvisionStatusFailed,
	}
	if _, err := s.accounts.Create(ctx, rec); err != nil {
		logger.Warn("record failed app account", "error", err, "user", task.User)
	}
}

// buildAccount 依据用户与账号策略构造下游账号载荷；返回账号与本次生成的明文口令。
func (s *ProvisioningService) buildAccount(
	user *repository.User, strategy *repository.AccountStrategy, existing *repository.AppAccount, withPassword bool,
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
func markPasswordSet(acc *repository.AppAccount, plain string) {
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
	conns, err := s.connectors.ListEnabled(ctx, institutionID, application)
	if err != nil {
		return nil, err
	}
	if len(conns) == 0 {
		return nil, bizerrors.ErrNotFound.WithDetail("connector", application)
	}
	return conns[0], nil
}

func (s *ProvisioningService) recordHistory(ctx context.Context, task provisionTask, connector, status, message string) {
	_, _ = s.history.Create(ctx, &repository.ProvisionHistory{
		InstitutionID: task.InstitutionID, Connector: connector, Application: task.Application,
		User: task.User, Action: task.Action, Status: status, Message: message,
	})
}

// ── 配置管理（连接器 / 账号策略）────────────────────────────────

// ConnectorInput 连接器创建/更新入参。
type ConnectorInput struct {
	// Connector 承载通用字段（name / display_name / application / type / state / just_in_time）
	Connector *repository.Connector
	// Config 类型专属的非敏感配置（键必须由目录声明，未知键一律拒绝）
	Config map[string]string
	// Secrets 类型专属的明文敏感值（键为目录中声明为 secret 的字段；缺省或留空表示不修改）
	Secrets map[string]string
}

// ConnectorTypes 返回支持的下游连接器类型（前端下拉用）。
func ConnectorTypes() []string { return provisioner.Types() }

// CreateConnector 创建连接器。
//
// 配置与凭据按**类型目录声明的字段**校验后落库（未声明的键直接拒绝），
// 因此新增一种下游连接器只需在目录里登记字段，无需改表、改实体或改 handler。
func (s *ProvisioningService) CreateConnector(ctx context.Context, actor string, in ConnectorInput) (*repository.Connector, error) {
	c := in.Connector
	if c.InstitutionID == 0 || c.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	connectorType := connectorProvisionerType(c)
	if _, err := provisioner.Get(connectorType); err != nil {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	if c.Application == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "application is required")
	}
	if _, err := s.orgs.GetByID(ctx, c.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", c.InstitutionID)
	}
	if _, err := s.connectors.GetByName(ctx, c.InstitutionID, c.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "connector already exists")
	}
	values, hasSecrets, err := s.buildConnectorValues(connectorType, in.Config, in.Secrets, nil)
	if err != nil {
		return nil, err
	}
	c.Values = values
	c.HasSecrets = hasSecrets
	id, err := s.connectors.Create(ctx, c)
	if err != nil {
		return nil, err
	}
	c.ID = id
	s.audits.Record(ctx, actor, c.InstitutionID, "connector.create", c.Name, nil)
	return c, nil
}

// buildConnectorValues 按目录 schema 组装配置：未知键拒绝、必填校验、敏感值加密、
// existing 非空时"留空即保留原值"。
func (s *ProvisioningService) buildConnectorValues(
	connectorType string, config, secrets map[string]string, existing *repository.Connector,
) (map[string]string, map[string]bool, error) {
	info, ok := connection.Find(connection.DirectionOut, connectorType)
	if !ok {
		return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "unknown connector type").
			WithDetail("type", connectorType)
	}
	// 通用字段由连接器实体承载（name / display_name / application / state），
	// 只有"类型专属"字段才走 config / secrets。
	generic := map[string]bool{
		"name": true, "display_name": true, "application": true, "state": true, "just_in_time": true,
	}
	declared := map[string]Field{}
	for _, field := range info.Fields {
		if generic[field.Key] {
			continue
		}
		declared[field.Key] = field
	}
	// 未知键一律拒绝：避免"前端多传了一个键，后端悄悄存下但没人读"
	for key := range config {
		field, ok := declared[key]
		if !ok {
			return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "unknown config key").WithDetail("key", key)
		}
		if field.Secret {
			// 敏感键必须经 secrets 提交（明文旁路直接拒绝，而不是静默忽略）
			return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams,
				"secret fields must be submitted via secrets").WithDetail("key", key)
		}
	}
	for key := range secrets {
		field, ok := declared[key]
		if !ok || !field.Secret {
			return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "unknown secret key").WithDetail("key", key)
		}
	}

	values := map[string]string{}
	hasSecrets := map[string]bool{}
	for key, field := range declared {
		if field.Secret {
			plain := strings.TrimSpace(secrets[key])
			if plain == "" {
				// 更新场景：留空表示不修改，保留既有密文
				if existing != nil && existing.HasSecrets[key] {
					values[key] = existing.Values[key]
					hasSecrets[key] = true
					continue
				}
				if field.Required {
					return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "secret is required").
						WithDetail("key", key)
				}
				continue
			}
			encrypted, err := s.cipher.Encrypt(plain)
			if err != nil {
				return nil, nil, bizerrors.Wrap(bizerrors.CodeInternalError, "加密连接器凭据失败", err)
			}
			values[key] = encrypted
			hasSecrets[key] = true
			continue
		}
		value := strings.TrimSpace(config[key])
		if value == "" {
			value = field.Default
		}
		if value == "" && field.Required {
			return nil, nil, bizerrors.New(bizerrors.CodeInvalidParams, "config is required").
				WithDetail("key", key)
		}
		if value != "" {
			values[key] = value
		}
	}
	return values, hasSecrets, nil
}

// UpdateConnector 更新连接器（配置与凭据同样按目录 schema 校验；留空即保留原值）
func (s *ProvisioningService) UpdateConnector(ctx context.Context, actor string, in ConnectorInput) error {
	c := in.Connector
	existing, err := s.connectors.GetByName(ctx, c.InstitutionID, c.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("connector", c.Name)
	}
	connectorType := connectorProvisionerType(&repository.Connector{Type: c.Type})
	if c.Type == "" {
		connectorType = connectorProvisionerType(existing)
	}
	if _, err := provisioner.Get(connectorType); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	values, hasSecrets, err := s.buildConnectorValues(connectorType, in.Config, in.Secrets, existing)
	if err != nil {
		return err
	}
	c.Values = values
	c.HasSecrets = hasSecrets
	c.Type = connectorType
	if err := s.connectors.Update(ctx, c); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, c.InstitutionID, "connector.update", c.Name, nil)
	return nil
}

// GetConnector 查询连接器（抹除敏感字段明文）。
func (s *ProvisioningService) GetConnector(ctx context.Context, institutionID int64, name string) (*repository.Connector, error) {
	c, err := s.connectors.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("connector", name)
	}
	return c, nil
}

// DeleteConnector 删除连接器。
func (s *ProvisioningService) DeleteConnector(ctx context.Context, actor string, institutionID int64, name string) error {
	c, err := s.connectors.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("connector", name)
	}
	if err := s.connectors.Delete(ctx, c.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "connector.delete", name, nil)
	return nil
}

// ListConnectors 分页列出连接器。
func (s *ProvisioningService) ListConnectors(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Connector, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.connectors.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.connectors.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// PingConnector 连通性自检。
func (s *ProvisioningService) PingConnector(ctx context.Context, institutionID int64, name string) error {
	c, err := s.connectors.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("connector", name)
	}
	prov, err := provisioner.Get(connectorProvisionerType(c))
	if err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	if err := prov.Ping(ctx, s.connectorConfig(c)); err != nil {
		return bizerrors.Wrap(bizerrors.CodeServiceUnavailable, "connector unreachable", err)
	}
	return nil
}

// CreateStrategy 创建账号策略。
func (s *ProvisioningService) CreateStrategy(ctx context.Context, actor string, a *repository.AccountStrategy) (*repository.AccountStrategy, error) {
	if a.InstitutionID == 0 || a.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, a.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", a.InstitutionID)
	}
	if _, err := s.strategies.GetByName(ctx, a.InstitutionID, a.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "account strategy already exists")
	}
	id, err := s.strategies.Create(ctx, a)
	if err != nil {
		return nil, err
	}
	a.ID = id
	s.audits.Record(ctx, actor, a.InstitutionID, "account_strategy.create", a.Name, nil)
	return a, nil
}

// UpdateStrategy 更新账号策略。
func (s *ProvisioningService) UpdateStrategy(ctx context.Context, actor string, a *repository.AccountStrategy) error {
	existing, err := s.strategies.GetByName(ctx, a.InstitutionID, a.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("account_strategy", a.Name)
	}
	a.ID = existing.ID
	if err := s.strategies.Update(ctx, a); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, a.InstitutionID, "account_strategy.update", a.Name, nil)
	return nil
}

// GetStrategy 查询账号策略。
func (s *ProvisioningService) GetStrategy(ctx context.Context, institutionID int64, name string) (*repository.AccountStrategy, error) {
	a, err := s.strategies.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("account_strategy", name)
	}
	return a, nil
}

// DeleteStrategy 删除账号策略。
func (s *ProvisioningService) DeleteStrategy(ctx context.Context, actor string, institutionID int64, name string) error {
	a, err := s.strategies.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("account_strategy", name)
	}
	if err := s.strategies.Delete(ctx, a.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "account_strategy.delete", name, nil)
	return nil
}

// ListStrategies 分页列出账号策略。
func (s *ProvisioningService) ListStrategies(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.AccountStrategy, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.strategies.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.strategies.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ── 应用账号与供给历史 ──────────────────────────────────────────

// ListAppAccounts 分页列出应用账号。（口令已不落库，DTO 只以 has_password 表达"设置过初始口令"）
func (s *ProvisioningService) ListAppAccounts(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.AppAccount, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.accounts.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.accounts.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListAppAccountsForUser 列出某用户的应用账号（门户"我的应用账号"只读视图）。
func (s *ProvisioningService) ListAppAccountsForUser(ctx context.Context, institutionID int64, user string) ([]*repository.AppAccount, error) {
	items, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "user", Value: user, Limit: 200,
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// PortalApplication 是门户应用中心允许下发的最小应用信息。
type PortalApplication struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Logo        string `json:"logo,omitempty"`
	HomepageURL string `json:"homepage_url,omitempty"`
	// ClientID 与 RedirectURI 用于门户应用中心一键发起 OIDC SSO（非机密，公开展示）；
	// RedirectURI 为该应用已注册的第一个回跳地址，未注册时为空（前端不展示 SSO 入口）。
	ClientID    string `json:"client_id,omitempty"`
	RedirectURI string `json:"redirect_uri,omitempty"`
}

// ListApplicationsForUser 仅列出当前用户已有有效下游账号且仍启用的应用。
func (s *ProvisioningService) ListApplicationsForUser(ctx context.Context, institutionID int64, user string) ([]PortalApplication, error) {
	accounts, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "user", Value: user, Limit: 200,
	})
	if err != nil {
		return nil, err
	}
	applications := make([]PortalApplication, 0, len(accounts))
	seen := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		if account.Status != "active" {
			continue
		}
		s.appendPortalApplication(ctx, institutionID, account.Application, seen, &applications)
	}
	// JIT 应用在首次登录前还没有 app_accounts 记录；有启用的 JIT 连接器时仍应出现在工作台。
	connectors, err := s.connectors.List(ctx, repository.ListQuery{InstitutionID: institutionID, Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, connector := range connectors {
		if connector.State == "active" && connector.JustInTime && connector.Application != "" {
			s.appendPortalApplication(ctx, institutionID, connector.Application, seen, &applications)
		}
	}
	return applications, nil
}

func (s *ProvisioningService) appendPortalApplication(
	ctx context.Context,
	institutionID int64, name string,
	seen map[string]struct{},
	applications *[]PortalApplication,
) {
	if _, ok := seen[name]; ok {
		return
	}
	app, err := s.apps.GetByCode(ctx, institutionID, name)
	if err != nil || app == nil || app.State != "active" {
		return
	}
	seen[name] = struct{}{}
	displayName := app.Name
	if displayName == "" {
		displayName = app.Code
	}
	// 一键 SSO 入口：已注册回跳地址的应用才可发起 OIDC 授权（取第一个注册地址）。
	redirectURI := ""
	if len(app.RedirectURIs) > 0 {
		redirectURI = app.RedirectURIs[0]
	}
	*applications = append(*applications, PortalApplication{
		Name: app.Code, DisplayName: displayName, Description: app.Description,
		Logo: app.Logo, HomepageURL: app.HomepageURL,
		ClientID: app.ClientID, RedirectURI: redirectURI,
	})
}

// CreateAppAccount 手动建号（create_type=manual）。
func (s *ProvisioningService) CreateAppAccount(ctx context.Context, actor string, institutionID int64, application, user string) error {
	if institutionID == 0 || application == "" || user == "" {
		return bizerrors.New(bizerrors.CodeInvalidParams, "institution_id, application and user are required")
	}
	if _, err := s.users.GetByName(ctx, institutionID, user); err != nil {
		return bizerrors.ErrNotFound.WithDetail("user", user)
	}
	if _, err := s.primaryConnector(ctx, institutionID, application); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, "application has no enabled connector")
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: application, User: user, Action: ProvisionActionCreate})
	s.audits.Record(ctx, actor, institutionID, "app_account.manual_create", user,
		map[string]interface{}{"application": application})
	return nil
}

// DeleteAppAccount 删除应用账号（并回收下游账号）。
func (s *ProvisioningService) DeleteAppAccount(ctx context.Context, actor string, institutionID int64, user, application string) error {
	acc, err := s.accounts.GetByUserApp(ctx, institutionID, application, user)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("app_account", user)
	}
	if err := s.accounts.Delete(ctx, acc.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "app_account.delete", user,
		map[string]interface{}{"application": application})
	return nil
}

// RetryAppAccount 手动重试某应用账号的建号。
func (s *ProvisioningService) RetryAppAccount(ctx context.Context, actor string, institutionID int64, user, application string) error {
	acc, err := s.accounts.GetByUserApp(ctx, institutionID, application, user)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("app_account", user)
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: acc.Application, User: acc.User, Action: ProvisionActionRetry})
	s.audits.Record(ctx, actor, institutionID, "app_account.retry", user,
		map[string]interface{}{"application": application})
	return nil
}

// ListHistoryForUser 分页列出某用户的供给历史（用户详情"关联"标签）。
func (s *ProvisioningService) ListHistoryForUser(ctx context.Context, institutionID int64, user string, offset, limit int) ([]*repository.ProvisionHistory, int64, error) {
	q := repository.ListQuery{
		InstitutionID: institutionID,
		Field:         "user",
		Value:         user,
		Offset:        offset,
		Limit:         limit,
	}
	items, err := s.history.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.history.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListHistory 分页列出供给历史。
func (s *ProvisioningService) ListHistory(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.ProvisionHistory, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.history.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.history.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ── 辅助 ──────────────────────────────────────────────────────

// connectorProvisionerType 解析连接器使用的 provisioner 实现（按 type 列；历史数据缺省为通用 HTTP）。
func connectorProvisionerType(c *repository.Connector) string {
	if c != nil && c.Type != "" {
		return c.Type
	}
	return "http"
}

// policyCreateWhen 读取建号触发策略（缺省 onCreate）。
func policyCreateWhen(s *repository.AccountStrategy) string {
	if s == nil || s.Policy == nil {
		return CreateWhenOnCreate
	}
	if v := s.Policy["createWhen"]; v != "" {
		return v
	}
	return CreateWhenOnCreate
}

// policyRevokeWhen 读取回收触发策略（缺省 disable）。
func policyRevokeWhen(s *repository.AccountStrategy) string {
	if s == nil || s.Policy == nil {
		return RevokeWhenDisable
	}
	if v := s.Policy["revokeWhen"]; v != "" {
		return v
	}
	return RevokeWhenDisable
}

func strategyMapping(s *repository.AccountStrategy) map[string]string {
	if s == nil {
		return nil
	}
	return s.Mapping
}

// localAttribute 取用户的本地属性（供映射到下游）。
func localAttribute(u *repository.User, field string) (string, bool) {
	switch field {
	case "name", "username":
		return u.Name, true
	case "displayName", "display_name":
		return u.DisplayName, true
	case "email":
		return u.Email, true
	case "phone":
		return u.Phone, true
	case "uid":
		return u.UID, true
	case "title":
		return u.Title, true
	case "location":
		return u.Location, true
	default:
		return "", false
	}
}

// ConnectorNameByID 按主键解析连接器名（对外 HTTP 路径统一用 id）。
func (s *ProvisioningService) ConnectorNameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.connectors.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}

// StrategyNameByID 按主键解析账号策略名（对外 HTTP 路径统一用 id）。
func (s *ProvisioningService) StrategyNameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.strategies.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}

// payloadNumber / payloadString 读取任务载荷字段（JSON 数字统一为 float64）
func payloadNumber(payload map[string]any, key string) float64 {
	switch value := payload[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case string:
		var parsed float64
		fmt.Sscanf(value, "%f", &parsed)
		return parsed
	}
	return 0
}

func payloadString(payload map[string]any, key string) string {
	if value, ok := payload[key].(string); ok {
		return value
	}
	return ""
}
