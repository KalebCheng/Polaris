package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// ApplicationHandler 应用处理器。
// access 承载"谁能使用该应用"的读写（访问模式 + 三类授权主体），与 svc 同一资源、不同关注点。
type ApplicationHandler struct {
	svc    *service.ApplicationService
	access *service.ApplicationAccessService
}

// NewApplicationHandler 创建应用处理器
func NewApplicationHandler(svc *service.ApplicationService, access *service.ApplicationAccessService) *ApplicationHandler {
	return &ApplicationHandler{svc: svc, access: access}
}

// List 分页列出应用
func (h *ApplicationHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	dtos := toApplicationDTOs(items)
	h.attachAccessCounts(c, items, dtos)
	bizerrors.RenderSuccess(c, gin.H{"items": dtos, "total": total, "page": page, "size": size})
}

// attachAccessCounts 给受限应用补上"授权主体数"（列表「访问范围」列）。
// 一次批量查询（不做 N+1）；计数失败不影响列表主数据（降级为 0，界面显示"受限"）。
func (h *ApplicationHandler) attachAccessCounts(c *gin.Context, apps []*repository.Application, dtos []*applicationDTO) {
	counts, err := h.access.SubjectCounts(c.Request.Context(), apps)
	if err != nil {
		return
	}
	for i, app := range apps {
		if i < len(dtos) && app != nil {
			dtos[i].AccessSubjectCount = counts[app.ID]
		}
	}
}

// Access 读取应用的访问授权（访问模式 + 三类授权主体）。
func (h *ApplicationHandler) Access(c *gin.Context) {
	app, err := h.svc.Get(c.Request.Context(), instID(c), h.codeByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	access, err := h.access.Access(c.Request.Context(), app)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, access)
}

// SetAccess 整体替换应用的访问授权（模式 + 名单，单事务）。
//
// 空名单是合法输入（= 该应用暂时无人可访问，见 Spec Q4）；被移除主体的成员在该应用下的
// 会话会被立即失效（FR-7）。
func (h *ApplicationHandler) SetAccess(c *gin.Context) {
	var in service.ApplicationAccessInput
	if !bindJSON(c, &in) {
		return
	}
	app, err := h.svc.Get(c.Request.Context(), instID(c), h.codeByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	access, err := h.access.SetAccess(c.Request.Context(), currentUser(c), app, in)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, access)
}

// SubjectOptions 返回可授权主体候选（关键词 + 上限）。
//
// 独立端点的动机：应用管理员往往没有 user:read / group:read / department:read，
// 复用那些列表接口会让选择器空转——"配了但选不出人"是最难排查的死配置。
//
// 作用域是**组织**而不是某个应用：应用向导在"新建"阶段还没有应用主键，
// 候选主体必须先能取到（路由见 server/router.go）。
func (h *ApplicationHandler) SubjectOptions(c *gin.Context) {
	limit, offset, ok := pageQuery(c)
	if !ok {
		return
	}
	var parentID *int64
	if raw := query(c, "parent_id"); raw != "" {
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("parent_id", raw))
			return
		}
		parentID = &id
	}
	page, err := h.access.SubjectOptions(c.Request.Context(), instID(c),
		query(c, "type"), query(c, "keyword"), parentID, offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, page)
}

// pageQuery 解析候选择器的 limit / offset（部门树按层加载与扁平列表分页共用）。
func pageQuery(c *gin.Context) (limit, offset int, ok bool) {
	if raw := query(c, "limit"); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n <= 0 {
			bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("limit", raw))
			return 0, 0, false
		}
		limit = n
	}
	if raw := query(c, "offset"); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n < 0 {
			bizerrors.Fail(c, bizerrors.ErrInvalidParams.WithDetail("offset", raw))
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

// Get 查询应用
func (h *ApplicationHandler) Get(c *gin.Context) {
	app, err := h.svc.Get(c.Request.Context(), instID(c), h.codeByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toApplicationDTO(app))
}

// Create 创建应用
func (h *ApplicationHandler) Create(c *gin.Context) {
	var app repository.Application
	if !bindJSON(c, &app) {
		return
	}
	if app.InstitutionID == 0 {
		app.InstitutionID = instID(c)
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &app)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	// 创建响应是 client_secret 明文唯一出现的时机之一（另一次是重置密钥）：
	// 常规 DTO 恒不含密钥，这里显式附上本次生成的值与端点清单。
	bizerrors.RenderSuccess(c, applicationCreatedDTO{
		applicationDTO: toApplicationDTO(created),
		ClientSecret:   created.ClientSecretPlain,
		Endpoints:      applicationEndpoints(c),
	}, http.StatusCreated)
}

// Update 更新应用
func (h *ApplicationHandler) Update(c *gin.Context) {
	var app repository.Application
	if !bindJSON(c, &app) {
		return
	}
	app.InstitutionID = instID(c)
	// 应用是唯一"标识与名称分离"的资源：路径 id 解析出的是**标识 code**，必须写回 Code。
	// 历史教训：这里曾写成 `app.Name = h.codeByID(c)`（当时 name 还是标识），
	// 导致每次保存都把用户填的「名称」覆盖成标识（如 "Net Bird" → "net-bird"），
	// 且只带 name 的部分更新会因为 Code 为空而 404。
	app.Code = h.codeByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &app); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除应用
func (h *ApplicationHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.codeByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ResetSecret 重置应用密钥
func (h *ApplicationHandler) ResetSecret(c *gin.Context) {
	secret, err := h.svc.ResetSecret(c.Request.Context(), currentUser(c), instID(c), h.codeByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"client_secret": secret, "has_client_secret": true})
}

// applicationCreatedDTO 创建响应：在常规 DTO 之上追加一次性密钥与端点清单。
// 内嵌 *applicationDTO 使 JSON 保持扁平（code/name/client_id/client_secret/endpoints 同级）。
type applicationCreatedDTO struct {
	*applicationDTO
	ClientSecret string `json:"client_secret"`
	Endpoints    gin.H  `json:"endpoints"`
}

// applicationEndpoints 返回该应用的 OIDC 端点清单（基于当前请求 Host 拼绝对地址），
// 供创建完成页一次性展示，避免使用者手工拼接。
func applicationEndpoints(c *gin.Context) gin.H {
	base := strings.TrimRight(requestBaseURL(c), "/")
	return gin.H{
		"issuer":    base,
		"authorize": base + "/oauth/authorize",
		"token":     base + "/oauth/token",
		"userinfo":  base + "/oauth/userinfo",
		"jwks":      base + "/.well-known/jwks",
	}
}

// requestBaseURL 依据请求推导平台对外地址（优先 X-Forwarded-*，回退 Host）。
func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = strings.Split(proto, ",")[0]
	} else if c.Request.TLS != nil {
		scheme = "https"
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	if idx := strings.Index(host, ","); idx >= 0 {
		host = strings.TrimSpace(host[:idx])
	}
	return scheme + "://" + host
}

// codeByID 解析路径中的 id 为应用标识 code（对外路径统一用 id）。
func (h *ApplicationHandler) codeByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	code, err := h.svc.CodeByID(c.Request.Context(), instID(c), id)
	if err != nil {
		return ""
	}
	return code
}
