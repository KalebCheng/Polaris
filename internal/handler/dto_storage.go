package handler

import (
	"time"

	"polaris/internal/repository"
)

// storageSettingsDTO 系统级存储配置响应 DTO。
// Secret Key 为 write-only：只回 has_secret 布尔，绝不回传密文/明文。
type storageSettingsDTO struct {
	Type       string    `json:"type"`
	Endpoint   string    `json:"endpoint,omitempty"`
	Region     string    `json:"region,omitempty"`
	Bucket     string    `json:"bucket,omitempty"`
	AccessKey  string    `json:"access_key,omitempty"`
	PathPrefix string    `json:"path_prefix,omitempty"`
	Domain     string    `json:"domain,omitempty"`
	RootPath   string    `json:"root_path,omitempty"`
	PublicBase string    `json:"public_base,omitempty"`
	PathStyle  bool      `json:"path_style"`
	HasSecret  bool      `json:"has_secret"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func toStorageSettingsDTO(s *repository.StorageSettings) *storageSettingsDTO {
	if s == nil {
		return nil
	}
	return &storageSettingsDTO{
		Type: s.Type, Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket,
		AccessKey: s.AccessKey, PathPrefix: s.PathPrefix, Domain: s.Domain,
		RootPath: s.RootPath, PublicBase: s.PublicBase, PathStyle: s.PathStyle,
		HasSecret: s.SecretKeyEnc != "", UpdatedAt: s.UpdatedAt,
	}
}
