package service

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
	"polaris/internal/service/storage"
)

// StorageService 存储服务：把平台托管的文件（用户头像、组织/应用 Logo 等）写入存储后端并返回可访问地址。
//
// 存储后端的选择顺序（storeForInstitution）：
//  1. 组织内的 Storage 提供商（providers 表 category=storage，state=active）；
//  2. 系统级默认存储（storage_settings 单行配置）；
//  3. 都没有 → 返回 PROVIDER_UNAVAILABLE。
//
// 配置映射约定（providers 表字段复用）：
//   - type：local | s3 | minio（声明见 internal/service/provider/catalog.go）
//   - local：endpoint = 根目录绝对路径，domain = 公开访问基址（如 https://iam.example.com）
//   - s3：endpoint = S3 端点（空为 AWS），bucket、region_id、client_id/client_secret = AccessKey、
//     path_prefix = 对象前缀，domain = 自定义 CDN 域名
//
// 系统级默认存储（storage_settings）字段与之同构，secret_key 以密文落库。
type StorageService struct {
	runtime  *providerpkg.Runtime
	users    repository.UserRepository
	settings repository.StorageSettingsRepository
	cipher   *auth.SecretCipher
	audits   *AuditRecorder
}

// NewStorageService 创建存储服务
func NewStorageService(
	runtime *providerpkg.Runtime,
	users repository.UserRepository,
	settings repository.StorageSettingsRepository,
	cipher *auth.SecretCipher,
	audits *AuditRecorder,
) *StorageService {
	return &StorageService{runtime: runtime, users: users, settings: settings, cipher: cipher, audits: audits}
}

// AvatarMaxBytes 头像大小上限（2MB）
const AvatarMaxBytes int64 = 2 << 20

// UploadMaxBytes 通用上传（Logo/图片/文件）大小上限（5MB）
const UploadMaxBytes int64 = 5 << 20

// 存储子类型
const (
	storageTypeLocal = "local"
	storageTypeS3    = "s3"
	storageTypeMinIO = "minio"
)

// activeStorageProvider 返回组织内启用的存储提供商（密钥已解密）
func (s *StorageService) activeStorageProvider(ctx context.Context, institutionID int64) (*repository.Provider, error) {
	if s.runtime == nil {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryStorage))
	}
	return s.runtime.Active(ctx, institutionID, providerpkg.CategoryStorage)
}

// storeForInstitution 解析组织的存储后端：优先组织级提供商，否则回落系统默认存储。
func (s *StorageService) storeForInstitution(ctx context.Context, institutionID int64) (storage.Store, error) {
	if p, err := s.activeStorageProvider(ctx, institutionID); err == nil {
		return s.storeFor(p, "")
	}
	// 组织未配置聚合存储 → 使用系统默认
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.Type) == "" {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("hint", "no organization storage provider nor system default configured")
	}
	return s.platformStore(settings)
}

// storeFor 依据组织级提供商配置构造存储后端
func (s *StorageService) storeFor(provider *repository.Provider, publicBase string) (storage.Store, error) {
	kind := normalizeStorageType(providerpkg.Kind(provider))
	cfg := storage.Config{
		Type:       kind,
		Endpoint:   provider.Endpoint,
		Region:     provider.RegionID,
		Bucket:     provider.Bucket,
		AccessKey:  provider.ClientID,
		SecretKey:  provider.ClientSecret,
		PathPrefix: provider.PathPrefix,
		Domain:     provider.Domain,
		PublicBase: publicBase,
	}
	if kind == storageTypeLocal {
		cfg.RootPath = provider.Endpoint
	}
	return storage.New(cfg)
}

// platformStore 依据系统级默认配置构造存储后端（解密 Secret Key）。
func (s *StorageService) platformStore(settings *repository.StorageSettings) (storage.Store, error) {
	secret := ""
	if settings.SecretKeyEnc != "" && s.cipher != nil {
		if plain, err := s.cipher.Decrypt(settings.SecretKeyEnc); err == nil {
			secret = plain
		}
	}
	cfg := storage.Config{
		Type:       normalizeStorageType(settings.Type),
		Endpoint:   settings.Endpoint,
		Region:     settings.Region,
		Bucket:     settings.Bucket,
		AccessKey:  settings.AccessKey,
		SecretKey:  secret,
		PathPrefix: settings.PathPrefix,
		Domain:     settings.Domain,
		RootPath:   settings.RootPath,
		PublicBase: settings.PublicBase,
		PathStyle:  settings.PathStyle,
	}
	return storage.New(cfg)
}

// UploadAvatar 上传用户头像并写回用户资料，返回可访问 URL。
func (s *StorageService) UploadAvatar(ctx context.Context, institutionID int64, userName, fileName, contentType string, body io.Reader, size int64) (string, error) {
	if size <= 0 || size > AvatarMaxBytes {
		return "", bizerrors.New(bizerrors.CodeFileSizeExceeded, "avatar exceeds size limit").
			WithDetail("max_bytes", AvatarMaxBytes)
	}
	contentType = normalizeImageContentType(contentType, fileName)
	if contentType == "" {
		return "", bizerrors.New(bizerrors.CodeInvalidParams, "unsupported image type").
			WithDetail("allowed", "png, jpeg, gif, webp")
	}

	store, err := s.storeForInstitution(ctx, institutionID)
	if err != nil {
		return "", err
	}

	ext := imageExtension(contentType)
	key := storageKeyJoin("avatars", strconv.FormatInt(institutionID, 10), fmt.Sprintf("%s-%d%s", userName, time.Now().Unix(), ext))
	url, err := store.Put(ctx, storage.Object{Key: key, Body: body, ContentType: contentType, Size: size})
	if err != nil {
		return "", bizerrors.Wrap(bizerrors.CodeServiceUnavailable, "store avatar failed", err)
	}

	user, err := s.users.GetByName(ctx, institutionID, userName)
	if err != nil {
		return "", err
	}
	previous := user.Avatar
	user.Avatar = url
	if err := s.users.Update(ctx, user); err != nil {
		return "", err
	}
	// 清理旧头像（同后端下才可删除，失败仅记日志）
	if previous != "" && strings.Contains(previous, "/avatars/"+strconv.FormatInt(institutionID, 10)+"/") {
		if idx := strings.Index(previous, "avatars/"); idx >= 0 {
			_ = store.Delete(ctx, previous[idx:])
		}
	}
	s.audits.Record(ctx, userName, institutionID, "user.update_avatar", userName, nil)
	return url, nil
}

// allowedUploadCategories 允许的通用上传分类（作为对象键前缀，避免路径穿越与乱用）。
var allowedUploadCategories = map[string]bool{
	"logos":       true,
	"favicons":    true,
	"avatars":     true,
	"application": true,
	"images":      true,
}

// Upload 通用上传：把文件写入组织存储并返回可访问 URL。
// category 作为对象键前缀（白名单校验），键形如 <category>/<institutionID>/<uuid><ext>。
func (s *StorageService) Upload(ctx context.Context, institutionID int64, category, fileName, contentType string, body io.Reader, size int64) (string, error) {
	if institutionID == 0 {
		return "", bizerrors.ErrInvalidParams.WithDetail("institution_id", "required")
	}
	category = strings.Trim(strings.ToLower(strings.TrimSpace(category)), "/")
	if !allowedUploadCategories[category] {
		return "", bizerrors.ErrInvalidParams.WithDetail("category", "unsupported")
	}
	if size <= 0 || size > UploadMaxBytes {
		return "", bizerrors.New(bizerrors.CodeFileSizeExceeded, "file exceeds size limit").
			WithDetail("max_bytes", UploadMaxBytes)
	}
	store, err := s.storeForInstitution(ctx, institutionID)
	if err != nil {
		return "", err
	}
	key := storageKeyJoin(category, strconv.FormatInt(institutionID, 10), uuid.NewString()+uploadExtension(fileName))
	url, err := store.Put(ctx, storage.Object{Key: key, Body: body, ContentType: contentType, Size: size})
	if err != nil {
		return "", bizerrors.Wrap(bizerrors.CodeServiceUnavailable, "upload failed", err)
	}
	return url, nil
}

// GetSettings 读取系统级存储配置（含密文 Secret Key，由 handler 脱敏）。
func (s *StorageService) GetSettings(ctx context.Context) (*repository.StorageSettings, error) {
	return s.settings.Get(ctx)
}

// UpdateSettings 更新系统级存储配置（密码/密钥 write-only：非空才覆盖）。
// `type` 为空表示**显式未配置**（清除平台默认存储）：整行归零落库，而不是被 normalizeStorageType
// 悄悄补成 local——历史上这里既过不了必填校验、又留着旧值，界面上「未配置」因此是个点不动的开关。
func (s *StorageService) UpdateSettings(ctx context.Context, actor string, in *repository.StorageSettings) error {
	kind := strings.ToLower(strings.TrimSpace(in.Type))
	if kind == "" {
		if err := s.settings.Update(ctx, &repository.StorageSettings{ID: 1}); err != nil {
			return err
		}
		s.audits.Record(ctx, actor, 0, "system.storage.clear", "storage", nil)
		return nil
	}
	if !isSupportedStorageType(kind) {
		return bizerrors.ErrInvalidParams.WithDetail("allowed", []string{"local", "s3", "minio"})
	}
	if err := validateStorageTarget(kind, in); err != nil {
		return err
	}
	current, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	in.ID = 1
	in.Type = kind
	if strings.TrimSpace(in.SecretKeyEnc) == "" {
		in.SecretKeyEnc = current.SecretKeyEnc
	} else if s.cipher != nil {
		encrypted, err := s.cipher.Encrypt(in.SecretKeyEnc)
		if err != nil {
			return bizerrors.Wrap(bizerrors.CodeInternalError, "加密存储密钥失败", err)
		}
		in.SecretKeyEnc = encrypted
	}
	if err := s.settings.Update(ctx, in); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, 0, "system.storage.update", "storage", map[string]any{"type": kind})
	return nil
}

// TestSettings 系统级存储连通性自检（写入并删除探测对象）。
func (s *StorageService) TestSettings(ctx context.Context) error {
	settings, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(settings.Type) == "" {
		return bizerrors.ErrProviderUnavailable.WithDetail("hint", "storage not configured")
	}
	store, err := s.platformStore(settings)
	if err != nil {
		return err
	}
	return store.Probe(ctx)
}

// ResolveLocalFile 解析本地存储中的文件（供公开静态路由使用）。
// 键形如 <category>/<institutionID>/<file>：据此定位组织，优先组织的本地提供商根目录，否则系统默认本地根目录。
func (s *StorageService) ResolveLocalFile(ctx context.Context, key string) (string, error) {
	clean := strings.TrimPrefix(path.Clean("/"+key), "/")
	segments := strings.Split(clean, "/")
	if len(segments) < 3 {
		return "", bizerrors.ErrNotFound
	}
	orgID, convErr := strconv.ParseInt(segments[1], 10, 64)
	if convErr != nil {
		return "", bizerrors.ErrNotFound
	}
	if provider, err := s.activeStorageProvider(ctx, orgID); err == nil {
		if normalizeStorageType(providerpkg.Kind(provider)) != storageTypeLocal {
			return "", bizerrors.ErrNotFound // 非本地后端由其自身域名提供访问
		}
		root := strings.TrimSuffix(provider.Endpoint, "/")
		if root == "" {
			return "", bizerrors.ErrNotFound
		}
		return root + "/" + clean, nil
	}
	// 组织无提供商 → 尝试系统默认本地存储
	settings, err := s.settings.Get(ctx)
	if err != nil || normalizeStorageType(settings.Type) != storageTypeLocal {
		return "", bizerrors.ErrNotFound
	}
	root := strings.TrimSuffix(settings.RootPath, "/")
	if root == "" {
		return "", bizerrors.ErrNotFound
	}
	return root + "/" + clean, nil
}

// Test 存储提供商自检（写入并删除探测对象）
func (s *StorageService) Test(ctx context.Context, provider *repository.Provider) error {
	store, err := s.storeFor(provider, "http://localhost")
	if err != nil {
		return err
	}
	return store.Probe(ctx)
}

// normalizeStorageType 归一化存储类型。
func normalizeStorageType(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "" {
		return storageTypeLocal
	}
	return k
}

// isSupportedStorageType 判断存储类型是否受支持。
func isSupportedStorageType(kind string) bool {
	switch kind {
	case storageTypeLocal, storageTypeS3, storageTypeMinIO:
		return true
	}
	return false
}

// validateStorageTarget 校验存储目标必填项。
func validateStorageTarget(kind string, in *repository.StorageSettings) error {
	if kind == storageTypeLocal {
		if strings.TrimSpace(in.RootPath) == "" {
			return bizerrors.ErrInvalidParams.WithDetail("required", "root_path")
		}
		return nil
	}
	if strings.TrimSpace(in.Bucket) == "" {
		return bizerrors.ErrInvalidParams.WithDetail("required", "bucket")
	}
	return nil
}

// normalizeImageContentType 依据显式类型或扩展名判定图片类型，非图片返回空串。
func normalizeImageContentType(contentType, fileName string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "image/png", "image/jpeg", "image/jpg", "image/gif", "image/webp":
		if contentType == "image/jpg" {
			return "image/jpeg"
		}
		return contentType
	}
	switch strings.ToLower(path.Ext(fileName)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

func imageExtension(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ".bin"
}

// uploadExtension 从文件名提取扩展名（保留下载类型；非法则空）。
func uploadExtension(fileName string) string {
	ext := strings.ToLower(path.Ext(fileName))
	if len(ext) > 16 {
		return ""
	}
	for _, r := range ext {
		if !(r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return ""
		}
	}
	return ext
}

// storageKeyJoin 拼接对象键路径（组织维度用 id 的数字串）。
func storageKeyJoin(elem ...string) string { return path.Join(elem...) }
