package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// ApplicationHandler 应用处理器
type ApplicationHandler struct{ svc *service.ApplicationService }

// NewApplicationHandler 创建应用处理器
func NewApplicationHandler(svc *service.ApplicationService) *ApplicationHandler {
	return &ApplicationHandler{svc: svc}
}

// List 分页列出应用
func (h *ApplicationHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toApplicationDTOs(items), "total": total, "page": page, "size": size})
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
		ClientSecret:   created.ClientSecret,
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
