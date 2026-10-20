package handler

import (
	"time"

	"polaris/internal/repository"
)

// userDTO 用户响应用 DTO。
// 安全约定：任何情况下都不得向客户端下发密码哈希、TOTP 密钥、恢复码等敏感字段。
type userDTO struct {
	ID            int64  `json:"id"`
	UID           string `json:"uid"`
	InstitutionID int64  `json:"institution_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	DepartmentID  *int64 `json:"department_id,omitempty"`
	// DepartmentIDs 多值部门归属（Spec Q2 = A；列表与详情均下发，主部门在前）
	DepartmentIDs []int64 `json:"department_ids,omitempty"`
	// JobLevelID 职级（Spec Q4.1 = A：仅属性，不参与授权）
	JobLevelID *int64 `json:"job_level_id,omitempty"`
	// JobLevel 职级名称（服务端补全，供列表/详情直接展示）
	JobLevel string `json:"job_level,omitempty"`
	// DepartmentPath 主部门展示路径（服务端批量补全；部门为空时不下发）
	DepartmentPath string `json:"department_path,omitempty"`
	// DepartmentPaths 全部部门归属的展示路径（Spec Q2 = A：一个人可能在多个部门挂职）
	DepartmentPaths []string   `json:"department_paths,omitempty"`
	DisplayName     string     `json:"display_name,omitempty"`
	FirstName       string     `json:"first_name,omitempty"`
	LastName        string     `json:"last_name,omitempty"`
	Avatar          string     `json:"avatar,omitempty"`
	Email           string     `json:"email,omitempty"`
	EmailVerified   bool       `json:"email_verified"`
	Phone           string     `json:"phone,omitempty"`
	PhoneVerified   bool       `json:"phone_verified"`
	CountryCode     string     `json:"country_code,omitempty"`
	Location        string     `json:"location,omitempty"`
	Address         string     `json:"address,omitempty"`
	Affiliation     string     `json:"affiliation,omitempty"`
	Title           string     `json:"title,omitempty"`
	Homepage        string     `json:"homepage,omitempty"`
	Bio             string     `json:"bio,omitempty"`
	Language        string     `json:"language,omitempty"`
	Gender          string     `json:"gender,omitempty"`
	Birthday        string     `json:"birthday,omitempty"`
	Education       string     `json:"education,omitempty"`
	Score           int        `json:"score"`
	IsAdmin         bool       `json:"is_admin"`
	IsForbidden     bool       `json:"is_forbidden"`
	IsDeleted       bool       `json:"is_deleted"`
	IsOnline        bool       `json:"is_online"`
	MfaEnabled      bool       `json:"mfa_enabled"`
	HasPassword     bool       `json:"has_password"`
	RegisterType    string     `json:"register_type,omitempty"`
	RegisterSource  string     `json:"register_source,omitempty"`
	LastSigninTime  *time.Time `json:"last_signin_time,omitempty"`
	LastSigninIP    string     `json:"last_signin_ip,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// toUserDTO 将用户实体转换为响应 DTO（剥离敏感字段）。
func toUserDTO(u *repository.User) *userDTO {
	return toUserDTOWithDepartments(u, nil)
}

// toUserDTOWithDepartments 转换用户 DTO，并用「部门主键 → 展示路径」映射补全归属。
func toUserDTOWithDepartments(u *repository.User, departments map[int64]string) *userDTO {
	if u == nil {
		return nil
	}
	return toUserDTOFields(u)
}

func toUserDTOFields(u *repository.User) *userDTO {
	return &userDTO{
		ID: u.ID, UID: u.UID, InstitutionID: u.InstitutionID, Name: u.Name, Type: u.Type,
		DepartmentID: u.DepartmentID, JobLevelID: u.JobLevelID,
		DisplayName: u.DisplayName, FirstName: u.FirstName, LastName: u.LastName, Avatar: u.Avatar,
		Email: u.Email, EmailVerified: u.EmailVerified, Phone: u.Phone, PhoneVerified: u.PhoneVerified,
		CountryCode: u.CountryCode,
		Location:    u.Location, Address: u.Address, Affiliation: u.Affiliation, Title: u.Title,
		Homepage: u.Homepage, Bio: u.Bio, Language: u.Language, Gender: u.Gender, Birthday: u.Birthday,
		Education: u.Education, Score: u.Score, IsAdmin: u.IsAdmin, IsForbidden: u.IsForbidden,
		IsDeleted: u.IsDeleted, IsOnline: u.IsOnline, MfaEnabled: u.MfaEnabled,
		HasPassword:  u.PasswordHash != "",
		RegisterType: u.RegisterType, RegisterSource: u.RegisterSource,
		LastSigninTime: u.LastSigninTime, LastSigninIP: u.LastSigninIP,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}

// applyDepartmentPaths 把多值部门主键补成展示路径（主部门在前，顺序与 ids 一致）。
func applyDepartmentPaths(dto *userDTO, ids []int64, departments map[int64]string) {
	if dto == nil || len(ids) == 0 {
		return
	}
	dto.DepartmentIDs = ids
	dto.DepartmentID = &ids[0]
	if label := departments[ids[0]]; label != "" {
		dto.DepartmentPath = label
	}
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		if label := departments[id]; label != "" {
			paths = append(paths, label)
		}
	}
	dto.DepartmentPaths = paths
}

// toUserDTOs 批量转换。
func toUserDTOs(users []*repository.User) []*userDTO {
	return toUserDTOsWithDepartments(users, nil)
}

// toUserDTOsWithDepartments 批量转换用户 DTO，并用「部门主键 → 展示路径」映射补全归属。
func toUserDTOsWithDepartments(users []*repository.User, departments map[int64]string) []*userDTO {
	out := make([]*userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, toUserDTOWithDepartments(u, departments))
	}
	return out
}

// institutionDTO 组织响应用 DTO（剥离主密码哈希）。
type institutionDTO struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	DisplayName          string    `json:"display_name"`
	Domain               string    `json:"domain,omitempty"`
	WebsiteURL           string    `json:"website_url,omitempty"`
	Logo                 string    `json:"logo,omitempty"`
	Favicon              string    `json:"favicon,omitempty"`
	Description          string    `json:"description,omitempty"`
	Captcha              string    `json:"captcha,omitempty"`
	PasswordType         string    `json:"password_type"`
	HasPasswordSalt      bool      `json:"has_password_salt"`
	PasswordOptions      []string  `json:"password_options,omitempty"`
	PasswordExpireDays   int       `json:"password_expire_days"`
	PasswordHistoryCount int       `json:"password_history_count"`
	TokenRetentionDays   int       `json:"token_retention_days"`
	RecordRetentionDays  int       `json:"record_retention_days"`
	DefaultApplication   string    `json:"default_application,omitempty"`
	HasDefaultPassword   bool      `json:"has_default_password"`
	UseEmailAsUsername   bool      `json:"use_email_as_username"`
	DisableSignin        bool      `json:"disable_signin"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// toInstitutionDTO 将组织实体转换为响应 DTO。
func toInstitutionDTO(o *repository.Institution) *institutionDTO {
	if o == nil {
		return nil
	}
	return &institutionDTO{
		ID: o.ID, Name: o.Name, DisplayName: o.DisplayName, Domain: o.Domain, WebsiteURL: o.WebsiteURL,
		Logo: o.Logo, Favicon: o.Favicon, Description: o.Description, Captcha: o.Captcha,
		PasswordType:    o.PasswordType,
		HasPasswordSalt: o.PasswordSalt != "",
		PasswordOptions: o.PasswordOptions, PasswordExpireDays: o.PasswordExpireDays,
		PasswordHistoryCount: o.PasswordHistoryCount, TokenRetentionDays: o.TokenRetentionDays,
		RecordRetentionDays: o.RecordRetentionDays, DefaultApplication: o.DefaultApplication,
		HasDefaultPassword: o.DefaultPasswordEnc != "",
		UseEmailAsUsername: o.UseEmailAsUsername, DisableSignin: o.DisableSignin,
		CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

// toInstitutionDTOs 批量转换。
func toInstitutionDTOs(orgs []*repository.Institution) []*institutionDTO {
	out := make([]*institutionDTO, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toInstitutionDTO(o))
	}
	return out
}

// departmentDTO 部门响应 DTO。
type departmentDTO struct {
	ID            int64  `json:"id"`
	InstitutionID int64  `json:"institution_id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description,omitempty"`
	ParentID      *int64 `json:"parent_id,omitempty"`
	CodePath      string `json:"code_path"`
	NamePath      string `json:"name_path"`
	Level         int    `json:"level"`
	HasChild      bool   `json:"has_child"`
	SortIndex     int    `json:"sort_index"`
	// IsRoot 组织根节点（Spec Q4.5：每个组织恰好一行；界面渲染为"组织"本身，禁止改名/移动/删除）
	IsRoot    bool      `json:"is_root"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// toDepartmentDTO 将部门实体转换为响应 DTO。
func toDepartmentDTO(d *repository.Department) *departmentDTO {
	if d == nil {
		return nil
	}
	return &departmentDTO{
		ID: d.ID, InstitutionID: d.InstitutionID, Name: d.Name, DisplayName: d.DisplayName,
		Description: d.Description, ParentID: d.ParentID, CodePath: d.CodePath, NamePath: d.NamePath,
		Level: d.Level, HasChild: d.HasChild, SortIndex: d.SortIndex, IsRoot: d.IsRoot,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// toDepartmentDTOs 批量转换。
func toDepartmentDTOs(items []*repository.Department) []*departmentDTO {
	out := make([]*departmentDTO, 0, len(items))
	for _, d := range items {
		out = append(out, toDepartmentDTO(d))
	}
	return out
}

// toSyncerDTO 同步器响应 DTO：敏感字段一律以是否已配置的形式表达。
func toSyncerDTO(s *repository.Syncer) map[string]interface{} {
	if s == nil {
		return nil
	}
	// nil 集合归一化为空集合（§3.7）：前端不必为"有没有映射"写判空分支
	accountMapping := s.AccountMapping
	if accountMapping == nil {
		accountMapping = map[string]string{}
	}
	departmentMapping := s.DepartmentMapping
	if departmentMapping == nil {
		departmentMapping = map[string]string{}
	}
	jobTitleMapping := s.JobTitleMapping
	if jobTitleMapping == nil {
		jobTitleMapping = map[string]string{}
	}
	return map[string]interface{}{
		"id": s.ID, "institution_id": s.InstitutionID, "name": s.Name, "display_name": s.DisplayName,
		"type": s.Type, "sub_type": s.SubType, "is_enabled": s.IsEnabled,
		"host": s.Host, "port": s.Port, "user": s.User, "database": s.Database, "table": s.Table,
		"database_type": s.DatabaseType, "ssl_mode": s.SSLMode,
		"app_id": s.AppID, "corp_id": s.CorpID,
		"account_mapping": accountMapping, "department_mapping": departmentMapping,
		"job_title_mapping": jobTitleMapping,
		// 编辑页回填依赖的字段必须全部回显，否则提交时会用空值覆盖（"编辑即重置"）：
		// 同步范围与计划存在 sync_config / target_department_id，绑定关系存在 schedule_job_id。
		"sync_config":          s.SyncConfig,
		"target_department_id": s.TargetDepartmentID,
		"schedule_job_id":      s.ScheduleJobID,
		"sync_users":           s.SyncUsers, "sync_groups": s.SyncGroups, "soft_delete": s.SoftDelete,
		"has_secret": s.Password != "", "has_app_secret": s.AppSecret != "",
		"last_run_time": s.LastRunTime, "last_status": s.LastStatus, "last_message": s.LastMessage,
		"last_counts": s.LastCounts, "created_at": s.CreatedAt, "updated_at": s.UpdatedAt,
	}
}

// connectorDTO 下游连接器响应 DTO（凭证以 has_credentials 表达，不下发明文）。
type connectorDTO struct {
	ID            int64  `json:"id"`
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application,omitempty"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Type          string `json:"type"`
	// Config 类型专属的非敏感配置（键由类型目录声明）
	Config map[string]string `json:"config"`
	// HasSecrets 敏感键是否已配置（密钥/口令一律不下发明文）
	HasSecrets map[string]bool `json:"has_secrets"`
	State      string          `json:"state"`
	// SyncConfig 出方向计划（定时对齐检查）；ScheduleJobID 是本连接器持有的 asyncd 任务主键
	SyncConfig    repository.SyncConfig `json:"sync_config"`
	ScheduleJobID *int64                `json:"schedule_job_id"`
	LastRunTime   *time.Time            `json:"last_run_time,omitempty"`
	LastStatus    string                `json:"last_status"`
	LastMessage   string                `json:"last_message,omitempty"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
}

// toConnectorDTO 将连接器实体转换为响应 DTO（敏感值只表达"已配置"）。
func toConnectorDTO(c *repository.Connector) *connectorDTO {
	if c == nil {
		return nil
	}
	connectorType := c.Type
	if connectorType == "" {
		connectorType = "http" // 历史数据没有 type：按通用 HTTP 契约处理
	}
	config := map[string]string{}
	hasSecrets := map[string]bool{}
	for key, value := range c.Values {
		if c.HasSecrets[key] {
			hasSecrets[key] = true
			continue
		}
		config[key] = value
	}
	return &connectorDTO{
		ID: c.ID, InstitutionID: c.InstitutionID, Application: c.Application, Name: c.Name,
		DisplayName: c.DisplayName, Type: connectorType,
		Config: config, HasSecrets: hasSecrets,
		State:         c.State,
		SyncConfig:    c.SyncConfig,
		ScheduleJobID: c.ScheduleJobID,
		LastRunTime:   c.LastRunTime, LastStatus: c.LastStatus, LastMessage: c.LastMessage,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

// toConnectorDTOs 批量转换。
func toConnectorDTOs(items []*repository.Connector) []*connectorDTO {
	out := make([]*connectorDTO, 0, len(items))
	for _, c := range items {
		out = append(out, toConnectorDTO(c))
	}
	return out
}

// accountStrategyDTO 账号策略响应 DTO。
type accountStrategyDTO struct {
	ID            int64             `json:"id"`
	InstitutionID int64             `json:"institution_id"`
	ApplicationID *int64            `json:"application_id,omitempty"`
	Application   string            `json:"application,omitempty"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name"`
	IsEnabled     bool              `json:"is_enabled"`
	Mapping       map[string]string `json:"mapping"`
	Policy        map[string]string `json:"policy"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// toAccountStrategyDTO 将账号策略实体转换为响应 DTO。
func toAccountStrategyDTO(a *repository.AccountStrategy) *accountStrategyDTO {
	if a == nil {
		return nil
	}
	return &accountStrategyDTO{
		ID: a.ID, InstitutionID: a.InstitutionID, ApplicationID: a.ApplicationID, Application: a.Application, Name: a.Name,
		DisplayName: a.DisplayName, IsEnabled: a.IsEnabled,
		Mapping: orEmptyMapCopy(a.Mapping), Policy: orEmptyMapCopy(a.Policy),
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// toAccountStrategyDTOs 批量转换。
func toAccountStrategyDTOs(items []*repository.AccountStrategy) []*accountStrategyDTO {
	out := make([]*accountStrategyDTO, 0, len(items))
	for _, a := range items {
		out = append(out, toAccountStrategyDTO(a))
	}
	return out
}

// applicationAccountDTO 应用账号响应 DTO（下游口令以 has_password 表达，不下发明文）。
type applicationAccountDTO struct {
	ID              int64     `json:"id"`
	InstitutionID   int64     `json:"institution_id"`
	ApplicationID   *int64    `json:"application_id,omitempty"`
	Application     string    `json:"application"`
	UserID          *int64    `json:"user_id,omitempty"`
	User            string    `json:"user"`
	Username        string    `json:"username"`
	RelatedUsername string    `json:"related_username,omitempty"`
	HasPassword     bool      `json:"has_password"`
	CreateType      string    `json:"create_type"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// toApplicationAccountDTO 将应用账号实体转换为响应 DTO。
func toApplicationAccountDTO(a *repository.ApplicationAccount) *applicationAccountDTO {
	if a == nil {
		return nil
	}
	return &applicationAccountDTO{
		ID: a.ID, InstitutionID: a.InstitutionID, ApplicationID: a.ApplicationID, Application: a.Application,
		UserID: a.UserID, User: a.User,
		Username: a.Username, RelatedUsername: a.RelatedUsername, HasPassword: a.HasPassword,
		CreateType: a.CreateType, Status: a.Status, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

// toApplicationAccountDTOs 批量转换。
func toApplicationAccountDTOs(items []*repository.ApplicationAccount) []*applicationAccountDTO {
	out := make([]*applicationAccountDTO, 0, len(items))
	for _, a := range items {
		out = append(out, toApplicationAccountDTO(a))
	}
	return out
}

// provisionHistoryDTO 供给历史响应 DTO。
type provisionHistoryDTO struct {
	ID            int64     `json:"id"`
	InstitutionID int64     `json:"institution_id"`
	Connector     string    `json:"connector,omitempty"`
	ApplicationID *int64    `json:"application_id,omitempty"`
	Application   string    `json:"application,omitempty"`
	User          string    `json:"user,omitempty"`
	Action        string    `json:"action"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// toProvisionHistoryDTO 将供给历史实体转换为响应 DTO。
func toProvisionHistoryDTO(h *repository.ProvisionHistory) *provisionHistoryDTO {
	if h == nil {
		return nil
	}
	return &provisionHistoryDTO{
		ID: h.ID, InstitutionID: h.InstitutionID, Connector: h.Connector,
		ApplicationID: h.ApplicationID, Application: h.Application,
		User: h.User, Action: h.Action, Status: h.Status, Message: h.Message, CreatedAt: h.CreatedAt,
	}
}

// toProvisionHistoryDTOs 批量转换。
func toProvisionHistoryDTOs(items []*repository.ProvisionHistory) []*provisionHistoryDTO {
	out := make([]*provisionHistoryDTO, 0, len(items))
	for _, h := range items {
		out = append(out, toProvisionHistoryDTO(h))
	}
	return out
}
