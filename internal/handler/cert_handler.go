package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// CertHandler 证书处理器
type CertHandler struct{ svc *service.CertService }

// NewCertHandler 创建证书处理器
func NewCertHandler(svc *service.CertService) *CertHandler { return &CertHandler{svc: svc} }

// List 分页列出证书
func (h *CertHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	items, total, err := h.svc.List(c.Request.Context(), instID(c), query(c, "keyword"), offset, limit)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": toCertDTOs(items), "total": total, "page": page, "size": size})
}

// Get 查询证书
func (h *CertHandler) Get(c *gin.Context) {
	cert, err := h.svc.Get(c.Request.Context(), instID(c), h.nameByID(c))
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toCertDTO(cert))
}

// Create 创建证书
func (h *CertHandler) Create(c *gin.Context) {
	var cert repository.Cert
	if !bindJSON(c, &cert) {
		return
	}
	if cert.InstitutionID == 0 {
		cert.InstitutionID = instID(c)
	}
	created, err := h.svc.Create(c.Request.Context(), currentUser(c), &cert)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toCertDTO(created), http.StatusCreated)
}

// Update 更新证书
func (h *CertHandler) Update(c *gin.Context) {
	var cert repository.Cert
	if !bindJSON(c, &cert) {
		return
	}
	cert.InstitutionID = instID(c)
	cert.Name = h.nameByID(c)
	if err := h.svc.Update(c.Request.Context(), currentUser(c), &cert); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// Delete 删除证书
func (h *CertHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), currentUser(c), instID(c), h.nameByID(c)); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// nameByID 解析路径中的资源 id 为该资源的 name（对外路径统一用 id，服务层仍以 name 语义访问存储）。
// 解析失败返回空串：后续服务调用会以"未找到"收尾（404），避免每个调用点重复写响应。
func (h *CertHandler) nameByID(c *gin.Context) string {
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
