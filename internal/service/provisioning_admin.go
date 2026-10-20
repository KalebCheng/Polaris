package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/connection"
	"polaris/internal/service/provisioner"
)

type ConnectorInput struct {
	// Connector 承载通用字段（name / display_name / application / type / state）
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
	if _, err := s.orgs.GetByID(ctx, c.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", c.InstitutionID)
	}
	if err := s.bindConnectorApplication(ctx, c, nil); err != nil {
		return nil, err
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
	// 保存即接线：把「定时对齐检查」变成 asyncd 的定时任务，并把任务主键回填到连接器
	if err := s.applyConnectorScheduleJob(ctx, c); err != nil {
		return nil, err
	}
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
	// 通用字段由连接器实体承载（name / display_name / application_id / state），
	// 只有"类型专属"字段才走 config / secrets。
	generic := map[string]bool{
		"name": true, "display_name": true, "application_id": true, "state": true,
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
	if err := s.bindConnectorApplication(ctx, c, existing); err != nil {
		return err
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
	// Update 按 (institution_id, name) 定位，主键与既有绑定从现有记录继承
	c.ID = existing.ID
	c.ScheduleJobID = existing.ScheduleJobID
	if err := s.connectors.Update(ctx, c); err != nil {
		return err
	}
	if err := s.applyConnectorScheduleJob(ctx, c); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, c.InstitutionID, "connector.update", c.Name, nil)
	return nil
}

// bindConnectorApplication resolves the application key and keeps the name as display data only.
func (s *ProvisioningService) bindConnectorApplication(ctx context.Context, c, existing *repository.Connector) error {
	if c.ApplicationID == nil {
		if existing == nil {
			return bizerrors.ErrInvalidParams.WithDetail("application_id", "required")
		}
		c.ApplicationID = existing.ApplicationID
		c.Application = existing.Application
		return nil
	}
	app, err := s.apps.GetByID(ctx, *c.ApplicationID)
	if err != nil || app == nil || app.InstitutionID != c.InstitutionID {
		return bizerrors.ErrInvalidParams.WithDetail("application_id", *c.ApplicationID)
	}
	c.Application = app.Code
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

// bindStrategyApplication 把策略的应用引用收敛为主键。
//
// application_id 是唯一写入入口；application 字段只用于响应展示。
func (s *ProvisioningService) bindStrategyApplication(ctx context.Context, a *repository.AccountStrategy) error {
	if a == nil {
		return nil
	}
	if a.ApplicationID != nil && *a.ApplicationID > 0 {
		app, err := s.apps.GetByID(ctx, *a.ApplicationID)
		if err != nil || app == nil || app.InstitutionID != a.InstitutionID {
			return bizerrors.ErrInvalidParams.WithDetail("application_id", *a.ApplicationID)
		}
		a.Application = app.Code
		return nil
	}
	a.ApplicationID = nil
	a.Application = ""
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
	if err := s.bindStrategyApplication(ctx, a); err != nil {
		return nil, err
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
	// A missing application_id preserves the existing binding; explicit zero clears it.
	if a.ApplicationID == nil {
		a.ApplicationID = existing.ApplicationID
		a.Application = existing.Application
	}
	if err := s.bindStrategyApplication(ctx, a); err != nil {
		return err
	}
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
func (s *ProvisioningService) ListStrategies(ctx context.Context, institutionID int64, keyword string, applicationID int64, offset, limit int) ([]*repository.AccountStrategy, int64, error) {
	q := withApplicationFilter(institutionQuery(institutionID, keyword, offset, limit), applicationID)
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

// ListApplicationAccounts 分页列出应用账号。（口令已不落库，DTO 只以 has_password 表达"设置过初始口令"）
func (s *ProvisioningService) ListApplicationAccounts(ctx context.Context, institutionID int64, keyword string, applicationID int64, offset, limit int) ([]*repository.ApplicationAccount, int64, error) {
	q := withApplicationFilter(institutionQuery(institutionID, keyword, offset, limit), applicationID)
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

// ListApplicationAccountsForUser 列出某用户的应用账号（门户"我的应用账号"只读视图）。
func (s *ProvisioningService) ListApplicationAccountsForUser(ctx context.Context, institutionID int64, user string) ([]*repository.ApplicationAccount, error) {
	items, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID, Field: "user_id", Value: strconv.FormatInt(s.resolveUserID(ctx, institutionID, user), 10), Limit: 200,
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

// ListApplicationsForUser 列出组织内所有启用应用，并按当前用户的访问授权过滤。
//
// 不能从 application_accounts / JIT 策略反推应用：OIDC 应用通常没有本地供给账号，
// 却同样可以通过部门授权或「全员可访问」进入应用中心。访问判定仍只走
// ApplicationAccessService.Allowed，与登录 / 发令牌保持同一套语义。
func (s *ProvisioningService) ListApplicationsForUser(ctx context.Context, institutionID int64, user string) ([]PortalApplication, error) {
	if s.apps == nil {
		return []PortalApplication{}, nil
	}
	apps, err := s.apps.List(ctx, repository.ListQuery{InstitutionID: institutionID})
	if err != nil {
		return nil, err
	}
	var currentUser *repository.User
	if s.access != nil && s.users != nil {
		currentUser, err = s.users.GetByName(ctx, institutionID, user)
		if err != nil {
			// 已认证用户不存在（例如会话与账号状态短暂不一致）：按空列表处理，不泄露内部错误。
			return []PortalApplication{}, nil
		}
	}
	if s.access != nil && currentUser == nil {
		// Allowed(nil 用户) 会按放行处理；门户入口必须显式拒绝这种不完整状态。
		return []PortalApplication{}, nil
	}
	applications := make([]PortalApplication, 0, len(apps))
	for _, app := range apps {
		if app == nil || app.InstitutionID != institutionID || app.State != "active" {
			continue
		}
		if s.access != nil {
			allowed, err := s.access.Allowed(ctx, app, currentUser)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
		}
		applications = append(applications, portalApplication(app))
	}
	return applications, nil
}

func portalApplication(app *repository.Application) PortalApplication {
	displayName := app.Name
	if displayName == "" {
		displayName = app.Code
	}
	// 一键 SSO 入口：已注册回跳地址的应用才可发起 OIDC 授权（取第一个注册地址）。
	redirectURI := ""
	if len(app.RedirectURIs) > 0 {
		redirectURI = app.RedirectURIs[0]
	}
	return PortalApplication{
		Name: app.Code, DisplayName: displayName, Description: app.Description,
		Logo: app.Logo, HomepageURL: app.HomepageURL,
		ClientID: app.ClientID, RedirectURI: redirectURI,
	}
}

// CreateApplicationAccount 手动建号（create_type=manual）。
func (s *ProvisioningService) CreateApplicationAccount(ctx context.Context, actor string, institutionID, applicationID, userID int64) error {
	app, err := s.apps.GetByID(ctx, applicationID)
	if err != nil || app.InstitutionID != institutionID {
		return bizerrors.ErrNotFound.WithDetail("application_id", applicationID)
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil || user.InstitutionID != institutionID {
		return bizerrors.ErrNotFound.WithDetail("user_id", userID)
	}
	if _, err := s.primaryConnectorByAppID(ctx, institutionID, applicationID); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, "application has no enabled connector")
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: app.Code, User: user.Name, Action: ProvisionActionCreate})
	s.audits.Record(ctx, actor, institutionID, "app_account.manual_create", user.Name,
		map[string]interface{}{"application_id": applicationID})
	return nil
}

// DeleteApplicationAccount 删除应用账号：先派发下游删除任务，成功后才删本地台账。
//
// 历史实现直接 `accounts.Delete`，下游账号仍然存在，属于"界面删了、下游还在"的假删除。
// 现在与禁用/离职走同一条 processRecycle(delete) 链路；失败会保留台账并写入供给历史，
// 管理员可以再次点击删除重试。
func (s *ProvisioningService) DeleteApplicationAccount(ctx context.Context, actor string, institutionID, applicationID, userID int64) error {
	acc, err := s.accounts.GetByUserApp(ctx, institutionID, applicationID, userID)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("app_account", userID)
	}
	if _, err := s.primaryConnectorByAppID(ctx, institutionID, applicationID); err != nil {
		return bizerrors.New(bizerrors.CodeInvalidParams, "application has no enabled connector")
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: acc.Application, User: acc.User, Action: ProvisionActionDelete})
	s.audits.Record(ctx, actor, institutionID, "app_account.delete_queued", acc.User,
		map[string]interface{}{"application_id": applicationID})
	return nil
}

// RetryApplicationAccount 手动重试某应用账号的建号。
func (s *ProvisioningService) RetryApplicationAccount(ctx context.Context, actor string, institutionID, applicationID, userID int64) error {
	acc, err := s.accounts.GetByUserApp(ctx, institutionID, applicationID, userID)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("app_account", userID)
	}
	s.enqueue(ctx, provisionTask{InstitutionID: institutionID, Application: acc.Application, User: acc.User, Action: ProvisionActionRetry})
	s.audits.Record(ctx, actor, institutionID, "app_account.retry", acc.User,
		map[string]interface{}{"application_id": applicationID})
	return nil
}

// ListHistoryForUser 分页列出某用户的供给历史（用户详情"关联"标签）。
func (s *ProvisioningService) ListHistoryForUser(ctx context.Context, institutionID int64, user string, offset, limit int) ([]*repository.ProvisionHistory, int64, error) {
	q := repository.ListQuery{
		InstitutionID: institutionID,
		Field:         "user_id",
		Value:         strconv.FormatInt(s.resolveUserID(ctx, institutionID, user), 10),
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
func (s *ProvisioningService) ListHistory(ctx context.Context, institutionID int64, keyword string, applicationID int64, connector string, offset, limit int) ([]*repository.ProvisionHistory, int64, error) {
	q := withConnectorFilter(withApplicationFilter(institutionQuery(institutionID, keyword, offset, limit), applicationID), connector)
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

// withApplicationFilter 按应用主键追加列表过滤（0 = 不过滤）。
func withApplicationFilter(q repository.ListQuery, applicationID int64) repository.ListQuery {
	if applicationID > 0 {
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "application_id", Value: applicationID})
	}
	return q
}

// withConnectorFilter 按连接器名称追加列表过滤（空 = 不过滤）。
func withConnectorFilter(q repository.ListQuery, connector string) repository.ListQuery {
	name := strings.TrimSpace(connector)
	if name != "" {
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "connector", Value: name})
	}
	return q
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
		if _, err := fmt.Sscanf(value, "%f", &parsed); err != nil {
			return 0
		}
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

// nullableID 把"0 表示未解析到"的 ID 转成可空主键（NULL 用于保留历史行、不阻断删除流程）。
func nullableID(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}
