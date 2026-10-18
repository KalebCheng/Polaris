package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// ProvisioningHandler 应用账号供给处理器（连接器 / 账号策略 / 应用账号 / 供给历史）。
type ProvisioningHandler struct{ svc *service.ProvisioningService }

// NewProvisioningHandler 创建供给处理器
func NewProvisioningHandler(svc *service.ProvisioningService) *ProvisioningHandler {
	return &ProvisioningHandler{svc: svc}
}

// ── 连接器 ────────────────────────────────────────────────────

// connectorRequest 连接器请求体：
// 通用字段（name/display_name/application/type/state）走扁平键，
// 类型专属配置走 config（非敏感）与 secrets（明文敏感值），键由类型目录声明并校验。
type connectorRequest struct {
	InstitutionID int64             `json:"institution_id"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name"`
	Application   string            `json:"application"`
	Type          string            `json:"type"`
	State         string            `json:"state"`
	Config        map[string]string `json:"config"`
	Secrets       map[string]string `json:"secrets"`
}

func (r connectorRequest) connector() *repository.Connector {
	return &repository.Connector{
		InstitutionID: r.InstitutionID, Name: r.Name, DisplayName: r.DisplayName,
		Application: r.Application, Type: r.Type, State: r.State,
	}
}

// ConnectorTypes 返回支持的下游连接器类型
func (h *ProvisioningHandler) ConnectorTypes(c *gin.Context) {
	bizerrors.RenderSuccess(c, gin.H{"types": service.ConnectorTypes()})
}

// ListConnectors 分页列出连接器
func (h *ProvisioningHandler) ListConnectors(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.ListConnectors(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toConnectorDTOs(items), "total": total, "page": page, "size": size})
}

// GetConnector 查询连接器
func (h *ProvisioningHandler) GetConnector(c *gin.Context) {
	conn, err := h.svc.GetConnector(c.Request.Context(), instID(c), h.connectorNameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toConnectorDTO(conn))
}

// CreateConnector 创建连接器
func (h *ProvisioningHandler) CreateConnector(c *gin.Context) {
	var req connectorRequest
	if !bindJSON(c, &req) {
		return
	}
	connector := req.connector()
	if connector.InstitutionID == 0 {
		connector.InstitutionID = instID(c)
	}
	created, err := h.svc.CreateConnector(c.Request.Context(), currentUser(c), service.ConnectorInput{
		Connector: connector, Config: req.Config, Secrets: req.Secrets,
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toConnectorDTO(created), http.StatusCreated)
}

// UpdateConnector 更新连接器
func (h *ProvisioningHandler) UpdateConnector(c *gin.Context) {
	var req connectorRequest
	if !bindJSON(c, &req) {
		return
	}
	connector := req.connector()
	connector.InstitutionID = instID(c)
	connector.Name = h.connectorNameByID(c)
	if err := h.svc.UpdateConnector(c.Request.Context(), currentUser(c), service.ConnectorInput{
		Connector: connector, Config: req.Config, Secrets: req.Secrets,
	}); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ReconcileConnector 出方向对齐检查：核对本地应用账号在下游是否真实存在。
// 默认 dry-run（只报告）；显式传 dry_run=false 才会为"确认缺失"的账号补建号。
func (h *ProvisioningHandler) ReconcileConnector(c *gin.Context) {
	var req struct {
		DryRun *bool `json:"dry_run"`
	}
	if !bindJSON(c, &req) {
		return
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	report, err := h.svc.Reconcile(c.Request.Context(), currentUser(c), instID(c), h.connectorNameByID(c), dryRun)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, report)
}

// DeleteConnector 删除连接器
func (h *ProvisioningHandler) DeleteConnector(c *gin.Context) {
	if err := h.svc.DeleteConnector(c.Request.Context(), currentUser(c), instID(c), h.connectorNameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// TestConnector 连通性自检
func (h *ProvisioningHandler) TestConnector(c *gin.Context) {
	if err := h.svc.PingConnector(c.Request.Context(), instID(c), h.connectorNameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ── 账号策略 ──────────────────────────────────────────────────

// ListStrategies 分页列出账号策略
func (h *ProvisioningHandler) ListStrategies(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.ListStrategies(c.Request.Context(), instID(c), query(c, "keyword"), queryIDValue(c, "application_id"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toAccountStrategyDTOs(items), "total": total, "page": page, "size": size})
}

// GetStrategy 查询账号策略
func (h *ProvisioningHandler) GetStrategy(c *gin.Context) {
	a, err := h.svc.GetStrategy(c.Request.Context(), instID(c), h.strategyNameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toAccountStrategyDTO(a))
}

// CreateStrategy 创建账号策略
func (h *ProvisioningHandler) CreateStrategy(c *gin.Context) {
	var a repository.AccountStrategy
	if !bindJSON(c, &a) {
		return
	}
	if a.InstitutionID == 0 {
		a.InstitutionID = instID(c)
	}
	created, err := h.svc.CreateStrategy(c.Request.Context(), currentUser(c), &a)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toAccountStrategyDTO(created), http.StatusCreated)
}

// UpdateStrategy 更新账号策略
func (h *ProvisioningHandler) UpdateStrategy(c *gin.Context) {
	var a repository.AccountStrategy
	if !bindJSON(c, &a) {
		return
	}
	a.InstitutionID = instID(c)
	a.Name = h.strategyNameByID(c)
	if err := h.svc.UpdateStrategy(c.Request.Context(), currentUser(c), &a); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// DeleteStrategy 删除账号策略
func (h *ProvisioningHandler) DeleteStrategy(c *gin.Context) {
	if err := h.svc.DeleteStrategy(c.Request.Context(), currentUser(c), instID(c), h.strategyNameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// ── 应用账号 ──────────────────────────────────────────────────

// ListApplicationAccounts 分页列出应用账号
func (h *ProvisioningHandler) ListApplicationAccounts(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.ListApplicationAccounts(c.Request.Context(), instID(c), query(c, "keyword"), queryIDValue(c, "application_id"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toApplicationAccountDTOs(items), "total": total, "page": page, "size": size})
}

// CreateApplicationAccount 手动建号
func (h *ProvisioningHandler) CreateApplicationAccount(c *gin.Context) {
	var req struct {
		Application string `json:"application"`
		User        string `json:"user"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.CreateApplicationAccount(c.Request.Context(), currentUser(c), instID(c), req.Application, req.User); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"}, http.StatusCreated)
}

// RetryApplicationAccount 手动重试建号
func (h *ProvisioningHandler) RetryApplicationAccount(c *gin.Context) {
	var req struct {
		Application string `json:"application"`
		User        string `json:"user"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.RetryApplicationAccount(c.Request.Context(), currentUser(c), instID(c), req.User, req.Application); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// DeleteApplicationAccount 删除应用账号
func (h *ProvisioningHandler) DeleteApplicationAccount(c *gin.Context) {
	institution := instID(c)
	user := query(c, "user")
	application := query(c, "application")
	if err := h.svc.DeleteApplicationAccount(c.Request.Context(), currentUser(c), institution, user, application); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// MyApplicationAccounts 当前用户自己的应用账号（门户只读视图）
func (h *ProvisioningHandler) MyApplicationAccounts(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	items, err := h.svc.ListApplicationAccountsForUser(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toApplicationAccountDTOs(items)})
}

// MyApplications 返回当前用户可访问应用中心的应用及最小展示信息。
func (h *ProvisioningHandler) MyApplications(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if !authCtx.IsAuthenticated() {
		bizerrors.Fail(c, bizerrors.ErrUnauthorized)
		return
	}
	items, err := h.svc.ListApplicationsForUser(c.Request.Context(), authCtx.InstitutionID, authCtx.Name)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items})
}

// ── 供给历史 ──────────────────────────────────────────────────

// ListHistory 分页列出供给历史
func (h *ProvisioningHandler) ListHistory(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.ListHistory(c.Request.Context(), instID(c), query(c, "keyword"), queryIDValue(c, "application_id"), query(c, "connector"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toProvisionHistoryDTOs(items), "total": total, "page": page, "size": size})
}

// connectorNameByID / strategyNameByID 解析路径中的 id 为资源名（对外路径统一用 id）。
func (h *ProvisioningHandler) connectorNameByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	name, err := h.svc.ConnectorNameByID(c.Request.Context(), instID(c), id)
	if err != nil {
		return ""
	}
	return name
}

func (h *ProvisioningHandler) strategyNameByID(c *gin.Context) string {
	id, ok := pathID(c, "id")
	if !ok {
		return ""
	}
	name, err := h.svc.StrategyNameByID(c.Request.Context(), instID(c), id)
	if err != nil {
		return ""
	}
	return name
}
