package handler

import (
	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
)

// StorageHandler 系统级存储与通用上传处理器。
type StorageHandler struct{ svc *service.StorageService }

// NewStorageHandler 创建存储处理器
func NewStorageHandler(svc *service.StorageService) *StorageHandler {
	return &StorageHandler{svc: svc}
}

// GetSettings 读取系统级默认存储配置（密钥脱敏，只回 has_secret）。
func (h *StorageHandler) GetSettings(c *gin.Context) {
	s, err := h.svc.GetSettings(c.Request.Context())
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, toStorageSettingsDTO(s))
}

// storageSettingsRequest 系统存储配置写入载荷（secret_key 为 write-only）。
type storageSettingsRequest struct {
	Type       string `json:"type"`
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	Bucket     string `json:"bucket"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	PathPrefix string `json:"path_prefix"`
	Domain     string `json:"domain"`
	RootPath   string `json:"root_path"`
	PublicBase string `json:"public_base"`
	PathStyle  bool   `json:"path_style"`
}

// UpdateSettings 更新系统级默认存储配置。
func (h *StorageHandler) UpdateSettings(c *gin.Context) {
	var req storageSettingsRequest
	if !bindJSON(c, &req) {
		return
	}
	// SecretKeyEnc 承载"待加密的明文"：非空时服务层用 SecretCipher 加密后落库，空则保留既有值。
	in := &repository.StorageSettings{
		Type: req.Type, Endpoint: req.Endpoint, Region: req.Region, Bucket: req.Bucket,
		AccessKey: req.AccessKey, SecretKeyEnc: req.SecretKey, PathPrefix: req.PathPrefix,
		Domain: req.Domain, RootPath: req.RootPath, PublicBase: req.PublicBase, PathStyle: req.PathStyle,
	}
	if err := h.svc.UpdateSettings(c.Request.Context(), currentUser(c), in); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// TestSettings 系统级存储连通性自检。
func (h *StorageHandler) TestSettings(c *gin.Context) {
	if err := h.svc.TestSettings(c.Request.Context()); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"status": "ok"})
}

// uploadInstitution 解析并校验上传目标组织：
// 组织内资源一律以"请求所属组织"为准（instID 优先取 ?institution=，回落会话组织）——
// 管理员在组织 B 的工作区里上传 Logo 时，必须落到组织 B 的存储提供商；
// 同时守住租户隔离：非平台管理员不得把文件写进别的组织。
func uploadInstitution(c *gin.Context) (int64, error) {
	authCtx := auth.GetAuthContext(c)
	institutionID := instID(c)
	if institutionID == 0 {
		return 0, bizerrors.ErrInvalidParams.WithDetail("institution", "required")
	}
	if authCtx != nil && institutionID != authCtx.InstitutionID && !authCtx.IsAdmin {
		return 0, bizerrors.ErrForbidden.WithDetail("institution_id", institutionID)
	}
	return institutionID, nil
}

// Upload 通用上传（multipart/form-data，字段名 file，可选 category）：写入组织存储并返回 URL。
func (h *StorageHandler) Upload(c *gin.Context) {
	authCtx := auth.GetAuthContext(c)
	if authCtx == nil || !authCtx.IsAuthenticated() {
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
	defer func() { _ = file.Close() }()

	institutionID, err := uploadInstitution(c)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	url, err := h.svc.Upload(
		c.Request.Context(), institutionID, c.PostForm("category"),
		fileHeader.Filename, fileHeader.Header.Get("Content-Type"), file, fileHeader.Size,
	)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"url": url})
}
