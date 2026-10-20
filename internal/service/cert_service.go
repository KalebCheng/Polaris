package service

import (
	"context"
	"time"

	"polaris/internal/auth"
	"polaris/internal/auth/oidc"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// CertService 证书服务
type CertService struct {
	certs  repository.CertRepository
	orgs   repository.InstitutionRepository
	audits *AuditRecorder
	cipher *auth.SecretCipher
}

// NewCertService 创建证书服务
func NewCertService(certs repository.CertRepository, orgs repository.InstitutionRepository, audits *AuditRecorder, cipher *auth.SecretCipher) *CertService {
	return &CertService{certs: certs, orgs: orgs, audits: audits, cipher: cipher}
}

// Create 创建证书（未提供证书内容时自动生成自签名证书）
func (s *CertService) Create(ctx context.Context, actor string, c *repository.Cert) (*repository.Cert, error) {
	if c.InstitutionID == 0 || c.Name == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "institution_id and name are required")
	}
	if _, err := s.orgs.GetByID(ctx, c.InstitutionID); err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("institution_id", c.InstitutionID)
	}
	if _, err := s.certs.GetByName(ctx, c.InstitutionID, c.Name); err == nil {
		return nil, bizerrors.New(bizerrors.CodeConflict, "certificate already exists")
	}
	if c.Certificate == "" || c.PrivateKey == "" {
		bitSize := c.BitSize
		if bitSize == 0 {
			bitSize = 2048
		}
		years := c.ExpireInYears
		if years == 0 {
			years = 10
		}
		certPEM, keyPEM, _, err := oidc.GenerateCert(bitSize, years)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成证书失败", err)
		}
		c.Certificate, c.PrivateKey = certPEM, keyPEM
		c.BitSize, c.ExpireInYears = bitSize, years
		exp := time.Now().AddDate(years, 0, 0)
		c.ExpireTime = &exp
	}
	if c.Scope == "" {
		c.Scope = "JWT"
	}
	if c.Type == "" {
		c.Type = "x509"
	}
	if c.CryptoAlgorithm == "" {
		c.CryptoAlgorithm = "RSA"
	}
	protectedKey, err := s.protectPrivateKey(c.PrivateKey)
	if err != nil {
		return nil, err
	}
	c.PrivateKey = protectedKey
	id, err := s.certs.Create(ctx, c)
	if err != nil {
		return nil, err
	}
	c.ID = id
	s.audits.Record(ctx, actor, c.InstitutionID, "cert.create", c.Name, nil)
	return c, nil
}

// Get 查询证书
func (s *CertService) Get(ctx context.Context, institutionID int64, name string) (*repository.Cert, error) {
	c, err := s.certs.GetByName(ctx, institutionID, name)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("certificate", name)
	}
	return c, nil
}

// Update 更新证书（仅元数据与内容；名称/组织不变）
func (s *CertService) Update(ctx context.Context, actor string, c *repository.Cert) error {
	existing, err := s.certs.GetByName(ctx, c.InstitutionID, c.Name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("certificate", c.Name)
	}
	c.ID = existing.ID
	if c.Certificate == "" {
		c.Certificate = existing.Certificate
	}
	if c.PrivateKey == "" {
		c.PrivateKey = existing.PrivateKey
	} else {
		protectedKey, err := s.protectPrivateKey(c.PrivateKey)
		if err != nil {
			return err
		}
		c.PrivateKey = protectedKey
	}
	if err := s.certs.Update(ctx, c); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, c.InstitutionID, "cert.update", c.Name, nil)
	return nil
}

// protectPrivateKey encrypts a plaintext PEM key. Already encrypted values are
// returned unchanged so the method remains idempotent for maintenance paths.
func (s *CertService) protectPrivateKey(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if s.cipher == nil {
		return "", bizerrors.New(bizerrors.CodeInternalError, "certificate cipher is not configured")
	}
	if _, err := s.cipher.Decrypt(value); err == nil {
		return value, nil
	}
	encrypted, err := s.cipher.Encrypt(value)
	if err != nil {
		return "", bizerrors.Wrap(bizerrors.CodeInternalError, "加密证书私钥失败", err)
	}
	return encrypted, nil
}

// Delete 删除证书
func (s *CertService) Delete(ctx context.Context, actor string, institutionID int64, name string) error {
	c, err := s.certs.GetByName(ctx, institutionID, name)
	if err != nil {
		return bizerrors.ErrNotFound.WithDetail("certificate", name)
	}
	if c.IsSystem {
		return bizerrors.ErrProtectedObject.WithDetail("certificate", name)
	}
	if err := s.certs.Delete(ctx, c.ID); err != nil {
		return err
	}
	s.audits.Record(ctx, actor, institutionID, "cert.delete", name, nil)
	return nil
}

// List 分页列出证书
func (s *CertService) List(ctx context.Context, institutionID int64, keyword string, offset, limit int) ([]*repository.Cert, int64, error) {
	q := institutionQuery(institutionID, keyword, offset, limit)
	items, err := s.certs.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.certs.Count(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// NameByID 按主键解析资源标识（对外 HTTP 路径统一用 id；内部仍以 name 语义访问存储）。
// 同时校验租户归属，避免跨组织按 id 越权读取。
func (s *CertService) NameByID(ctx context.Context, institutionID int64, id int64) (string, error) {
	item, err := s.certs.GetByID(ctx, id)
	if err != nil || item == nil || (institutionID != 0 && item.InstitutionID != institutionID) {
		return "", bizerrors.ErrNotFound.WithDetail("id", id)
	}
	return item.Name, nil
}
