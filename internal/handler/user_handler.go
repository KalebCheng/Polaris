package handler

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// UserHandler 用户处理器。
//
// 用户是 IAM 平台的核心对象，因此这里聚集了"一个人"的全部运维面：
// 资料与归属（概览）、凭证与安全（安全与认证）、授权（授权）、外部关系（关联）、审计（活动）。
type UserHandler struct {
	svc        *service.UserService
	lockouts   *service.LoginLockoutService
	sessions   *service.SessionService
	provision  *service.ProvisioningService
	records    *service.RecordService
	tokenSvc   *service.TokenService
	storageSvc *service.StorageService
}

// NewUserHandler 创建用户处理器
func NewUserHandler(
	svc *service.UserService,
	lockouts *service.LoginLockoutService,
	sessions *service.SessionService,
	provision *service.ProvisioningService,
	records *service.RecordService,
	tokenSvc *service.TokenService,
	storageSvc *service.StorageService,
) *UserHandler {
	return &UserHandler{
		svc: svc, lockouts: lockouts, sessions: sessions,
		provision: provision, records: records, tokenSvc: tokenSvc, storageSvc: storageSvc,
	}
}

// UploadAvatar 上传当前用户头像（multipart/form-data，字段名 file）
func (h *UserHandler) UploadAvatar(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	defer file.Close()

	url, err := h.storageSvc.UploadAvatar(
		c.Request.Context(), authCtx.InstitutionID, authCtx.Name,
		fileHeader.Filename, fileHeader.Header.Get("Content-Type"), file, fileHeader.Size,
	)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"avatar": url})
}

// ServeLocalStorage 提供本地存储文件的只读访问（对象存储部署下由对象存储域名直接提供）
func (h *UserHandler) ServeLocalStorage(c *gin.Context) {
	key := strings.TrimPrefix(pathParam(c, "path"), "/")
	full, err := h.storageSvc.ResolveLocalFile(c.Request.Context(), key)
	if err != nil {
		bizerrors.AbortError(c, bizerrors.ErrNotFound)
		return
	}
	if _, err := os.Stat(full); err != nil {
		bizerrors.AbortError(c, bizerrors.ErrNotFound)
		return
	}
	c.File(full)
}

// userListQuery 解析用户列表的目录筛选（关键词 + 部门 + 状态 + MFA）。
func userListQuery(c *gin.Context, offset, limit int) (repository.ListQuery, error) {
	q := repository.ListQuery{
		InstitutionID: instID(c),
		Keyword:       query(c, "keyword"),
		Offset:        offset,
		Limit:         limit,
	}
	if raw := query(c, "department_id"); raw != "" {
		departmentID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || departmentID <= 0 {
			return q, bizerrors.ErrInvalidParams.WithDetail("department_id", raw)
		}
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "department_id", Value: departmentID})
	}
	// 账号状态：active（未禁用未删除）| disabled（已禁用）| deleted（已删除）
	switch status := query(c, "status"); status {
	case "":
	case "active":
		q.Filters = append(q.Filters,
			repository.ColumnFilter{Field: "is_forbidden", Value: false},
			repository.ColumnFilter{Field: "is_deleted", Value: false},
		)
	case "disabled":
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "is_forbidden", Value: true})
	case "deleted":
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "is_deleted", Value: true})
	default:
		return q, bizerrors.ErrInvalidParams.WithDetail("status", status)
	}
	if raw := query(c, "mfa"); raw != "" {
		bound, err := strconv.ParseBool(raw)
		if err != nil {
			return q, bizerrors.ErrInvalidParams.WithDetail("mfa", raw)
		}
		q.Filters = append(q.Filters, repository.ColumnFilter{Field: "mfa_enabled", Value: bound})
	}
	return q, nil
}

// List 分页列出用户
func (h *UserHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	q, err := userListQuery(c, offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items, total, err := h.svc.ListQuery(c.Request.Context(), q)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	departments, _ := h.svc.DepartmentLabels(c.Request.Context(), instID(c))
	bizerrors.RenderSuccess(c, gin.H{
		"items": toUserDTOsWithDepartments(items, departments),
		"total": total, "page": page, "size": size,
	})
}

// Get 查询用户（详情概览）
func (h *UserHandler) Get(c *gin.Context) {
	u, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	departments, _ := h.svc.DepartmentLabels(c.Request.Context(), instID(c))
	bizerrors.RenderSuccess(c, toUserDTOWithDepartments(u, departments))
}

// createUserRequest 创建用户请求体。
//
// 两个约定：
//  1. 刻意使用显式 DTO 而非直接绑定实体：实体里带着 password_hash / mfa_secret 等列，
//     允许客户端直接提交等于把"自选哈希"和"自选 MFA 密钥"写进库（§3.1-7）；
//  2. **资料字段集合必须与 service.UpdateUserProfileInput 完全一致**（由
//     `user_payload_test.go` 反射锁定）——否则会出现"新建表单能填、后端静默丢弃"，
//     与第四阶段"新增连接器配置被丢弃"是同一类事故。
type createUserRequest struct {
	// 标识与归属（编辑时不可改）
	InstitutionID int64  `json:"institution_id"`
	Name          string `json:"name" binding:"required"`
	DepartmentID  *int64 `json:"department_id"`
	// 管理员标记：创建时可授予（服务层会做防呆与 type 派生）
	IsAdmin bool `json:"is_admin"`
	// 初始密码：留空则用组织默认密码（若已配置）
	Password string `json:"password"`

	// ── 以下资料字段与 UpdateUserProfileInput 一一对应 ──
	DisplayName   string `json:"display_name"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Avatar        string `json:"avatar"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Phone         string `json:"phone"`
	PhoneVerified bool   `json:"phone_verified"`
	CountryCode   string `json:"country_code"`
	Location      string `json:"location"`
	Address       string `json:"address"`
	Affiliation   string `json:"affiliation"`
	Title         string `json:"title"`
	Homepage      string `json:"homepage"`
	Bio           string `json:"bio"`
	Language      string `json:"language"`
	Gender        string `json:"gender"`
	Birthday      string `json:"birthday"`
	Education     string `json:"education"`
}

// Create 创建用户
func (h *UserHandler) Create(c *gin.Context) {
	var req createUserRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.InstitutionID == 0 {
		req.InstitutionID = instID(c)
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), service.CreateUserInput{
		User: &repository.User{
			InstitutionID: req.InstitutionID,
			Name:          req.Name,
			DisplayName:   req.DisplayName,
			FirstName:     req.FirstName,
			LastName:      req.LastName,
			Avatar:        req.Avatar,
			Email:         req.Email,
			EmailVerified: req.EmailVerified,
			Phone:         req.Phone,
			PhoneVerified: req.PhoneVerified,
			CountryCode:   req.CountryCode,
			Location:      req.Location,
			Address:       req.Address,
			Affiliation:   req.Affiliation,
			Title:         req.Title,
			Homepage:      req.Homepage,
			Bio:           req.Bio,
			Language:      req.Language,
			Gender:        req.Gender,
			Birthday:      req.Birthday,
			Education:     req.Education,
			DepartmentID:  req.DepartmentID,
			IsAdmin:       req.IsAdmin,
		},
		Password: req.Password,
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	departments, _ := h.svc.DepartmentLabels(c.Request.Context(), instID(c))
	bizerrors.RenderSuccess(c, toUserDTOWithDepartments(created, departments), http.StatusCreated)
}

// UpdateProfile 更新用户资料（部分更新：只改本次提交的字段）。
func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req service.UpdateUserProfileInput
	if !bindJSON(c, &req) {
		return
	}
	updated, err := h.svc.UpdateProfile(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	departments, _ := h.svc.DepartmentLabels(c.Request.Context(), instID(c))
	bizerrors.RenderSuccess(c, toUserDTOWithDepartments(updated, departments))
}

// SetDepartment 变更用户部门归属
func (h *UserHandler) SetDepartment(c *gin.Context) {
	var req struct {
		DepartmentID *int64 `json:"department_id"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetDepartment(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.DepartmentID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Disable 禁用账号
func (h *UserHandler) Disable(c *gin.Context) { h.setDisabled(c, true) }

// Enable 启用账号
func (h *UserHandler) Enable(c *gin.Context) { h.setDisabled(c, false) }

func (h *UserHandler) setDisabled(c *gin.Context, disabled bool) {
	if err := h.svc.SetDisabled(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), disabled); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetAdmin 设置 / 取消管理员
func (h *UserHandler) SetAdmin(c *gin.Context) {
	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetAdmin(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.IsAdmin); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除用户（软删除）
func (h *UserHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ResetPassword 管理员重置密码
func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req auth.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.NewPassword); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ResetMFA 管理员重置 MFA（清空绑定；需 user:reset_mfa 权限）
func (h *UserHandler) ResetMFA(c *gin.Context) {
	if err := h.svc.AdminResetMFA(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Unlock 解除登录失败锁定（需 user:unlock 权限）
func (h *UserHandler) Unlock(c *gin.Context) {
	cleared, err := h.lockouts.Unlock(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok", "cleared": cleared})
}

// Security 安全与认证概览：密码状态 + MFA 状态 + 当前登录失败锁定条目。
// 活动会话复用平台级 /sessions?user=，不在此重复下发。
func (h *UserHandler) Security(c *gin.Context) {
	ctx := c.Request.Context()
	name := h.nameByID(c)
	status, err := h.svc.SecurityStatus(ctx, instID(c), name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	lockouts, err := h.lockouts.Entries(ctx, instID(c), name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"password": status, "lockouts": lockouts})
}

// Authorizations 授权全景（直接/继承角色、用户组、直接权限、有效权限含来源）
func (h *UserHandler) Authorizations(c *gin.Context) {
	result, err := h.svc.Authorizations(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, result)
}

// SetRoles 整体替换用户的直接角色
func (h *UserHandler) SetRoles(c *gin.Context) {
	var req struct {
		Roles []string `json:"roles"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetDirectRoles(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Roles); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// SetGroups 整体替换用户的用户组归属
func (h *UserHandler) SetGroups(c *gin.Context) {
	var req struct {
		Groups []string `json:"groups"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.SetUserGroups(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Groups); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// AppAccounts 该用户的下游应用账号（只读；创建/重试仍在「应用账号」页）
func (h *UserHandler) AppAccounts(c *gin.Context) {
	items, err := h.provision.ListAppAccountsForUser(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toAppAccountDTOs(items)})
}

// ProvisionHistory 该用户的供给历史（分页）
func (h *UserHandler) ProvisionHistory(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.provision.ListHistoryForUser(c.Request.Context(), instID(c), h.nameByID(c), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Records 该用户的审计记录（分页；管理面，需 record:read）
func (h *UserHandler) Records(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.records.ListForUser(c.Request.Context(), instID(c), h.nameByID(c), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Identities 列出用户绑定的外部身份
func (h *UserHandler) Identities(c *gin.Context) {
	items, err := h.svc.Identities(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if items == nil {
		items = []*repository.UserIdentity{}
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// MyIdentities 列出当前用户自己的外部身份。
func (h *UserHandler) MyIdentities(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	items, err := h.svc.Identities(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if items == nil {
		items = []*repository.UserIdentity{}
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// UnbindMyIdentity 解绑当前用户自己的外部身份。
func (h *UserHandler) UnbindMyIdentity(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	var id int64
	if _, err := fmtSscan(c.Param("identityID"), &id); err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	if err := h.svc.UnbindOwnIdentity(c.Request.Context(), authCtx.InstitutionID, authCtx.Name, id); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// UnbindIdentity 解绑外部身份
func (h *UserHandler) UnbindIdentity(c *gin.Context) {
	var id int64
	if _, err := fmtSscan(c.Param("identityID"), &id); err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	if err := h.svc.UnbindIdentity(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), id); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// IssueAPIKey 为用户签发 API 令牌
func (h *UserHandler) IssueAPIKey(c *gin.Context) {
	var req struct {
		Name          string `json:"name"`
		ExpireInHours int    `json:"expire_in_hours"`
	}
	if !bindJSON(c, &req) {
		return
	}
	hours := req.ExpireInHours
	if hours <= 0 {
		hours = 24 * 30
	}
	token, err := h.tokenSvc.CreateAPIKey(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c), req.Name, timeHour(hours))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"name": token.Name, "token": token.AccessToken, "expires_at": token.ExpireTime}, http.StatusCreated)
}

// Me 返回当前登录用户
func (h *UserHandler) Me(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	u, err := h.svc.Get(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	departments, _ := h.svc.DepartmentLabels(c.Request.Context(), authCtx.InstitutionID)
	bizerrors.RenderSuccess(c, gin.H{
		"user":        toUserDTOWithDepartments(u, departments),
		"roles":       authCtx.Roles,
		"permissions": authCtx.Permissions,
	})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *UserHandler) nameByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	name, err := h.svc.NameByID(c.Request.Context(), instID(c), id)
	if err != nil {
		return ""
	}
	return name
}
