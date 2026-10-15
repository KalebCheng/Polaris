package service

import (
	"context"
	"testing"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

// stubConnectorRepo 仅实现测试所需的 ConnectorRepository 方法（其余经嵌入接口置为 nil）。
type stubConnectorRepo struct {
	repository.ConnectorRepository
	created *repository.Connector
}

func (r *stubConnectorRepo) GetByName(context.Context, int64, string) (*repository.Connector, error) {
	return nil, errStubNotFound
}

func (r *stubConnectorRepo) Create(_ context.Context, c *repository.Connector) (int64, error) {
	r.created = c
	return 1, nil
}

type stubInstitutionRepo struct {
	repository.InstitutionRepository
}

func (r *stubInstitutionRepo) GetByID(_ context.Context, id int64) (*repository.Institution, error) {
	return &repository.Institution{ID: id, Name: "built-in"}, nil
}

func (r *stubInstitutionRepo) GetByName(context.Context, string) (*repository.Institution, error) {
	return &repository.Institution{Name: "acme"}, nil
}

var errStubNotFound = &stubError{"not found"}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

func newTestProvisioningService(conns repository.ConnectorRepository) (*ProvisioningService, *auth.SecretCipher) {
	cipher, _ := auth.NewSecretCipher(auth.DeriveKey("test-key"))
	return &ProvisioningService{
		connectors: conns,
		orgs:       &stubInstitutionRepo{},
		cipher:     cipher,
	}, cipher
}

// 回归：敏感字段必须经 secrets 提交；把 `credentials` 混进 config 属于明文旁路，直接拒绝。
func TestCreateConnectorRejectsSecretInPlainConfig(t *testing.T) {
	repo := &stubConnectorRepo{}
	svc, _ := newTestProvisioningService(repo)

	_, err := svc.CreateConnector(context.Background(), "admin", ConnectorInput{
		Connector: &repository.Connector{InstitutionID: 1, Name: "c1", Type: "http", Application: "app-one"},
		Config:    map[string]string{"credentials": "plaintext-secret"},
	})
	if err == nil {
		t.Fatal("把敏感键放进 config 应被拒绝")
	}
	if repo.created != nil {
		t.Fatal("被拒绝的请求不应落库")
	}
}

// 回归：未声明的配置键必须拒绝（避免"前端多传一个键、后端悄悄存下但没人读"）。
func TestCreateConnectorRejectsUnknownConfigKey(t *testing.T) {
	repo := &stubConnectorRepo{}
	svc, _ := newTestProvisioningService(repo)

	_, err := svc.CreateConnector(context.Background(), "admin", ConnectorInput{
		Connector: &repository.Connector{InstitutionID: 1, Name: "c1", Type: "http", Application: "app-one"},
		Config:    map[string]string{"not_declared": "x"},
	})
	if err == nil {
		t.Fatal("未声明的配置键应被拒绝")
	}
}

// 回归：secrets 里的明文必须加密落库（可解密回明文），且标记为敏感键。
func TestCreateConnectorEncryptsPlaintextInput(t *testing.T) {
	repo := &stubConnectorRepo{}
	svc, cipher := newTestProvisioningService(repo)

	if _, err := svc.CreateConnector(context.Background(), "admin", ConnectorInput{
		Connector: &repository.Connector{InstitutionID: 1, Name: "c2", Type: "http", Application: "app-one"},
		Config:    map[string]string{"provider_url": "https://app.example.com/api"},
		Secrets:   map[string]string{"credentials": "s3cr3t"},
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	stored := repo.created.Values["credentials"]
	if stored == "s3cr3t" || stored == "" {
		t.Fatalf("落库值应为密文，实际 %q", stored)
	}
	if !repo.created.HasSecrets["credentials"] {
		t.Fatal("敏感键必须标记为已配置（响应以 has_secrets 表达）")
	}
	if repo.created.Values["provider_url"] != "https://app.example.com/api" {
		t.Fatalf("非敏感配置应原样落库: %+v", repo.created.Values)
	}
	plain, err := cipher.Decrypt(stored)
	if err != nil || plain != "s3cr3t" {
		t.Fatalf("密文应可解密回明文，实际 plain=%q err=%v", plain, err)
	}
}
