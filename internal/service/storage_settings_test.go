package service

import (
	"context"
	"testing"

	"polaris/internal/auth"
	"polaris/internal/repository"
)

// stubStorageSettingsRepo 内存版系统存储配置仓库（仅实现测试所需方法）。
type stubStorageSettingsRepo struct {
	repository.StorageSettingsRepository
	current *repository.StorageSettings
	saved   *repository.StorageSettings
}

func (r *stubStorageSettingsRepo) Get(context.Context) (*repository.StorageSettings, error) {
	if r.current == nil {
		return &repository.StorageSettings{ID: 1}, nil
	}
	return r.current, nil
}

func (r *stubStorageSettingsRepo) Update(_ context.Context, s *repository.StorageSettings) error {
	r.saved = s
	return nil
}

func newStorageSettingsService(repo *stubStorageSettingsRepo) *StorageService {
	cipher, _ := auth.NewSecretCipher(auth.DeriveKey("test-key"))
	return &StorageService{settings: repo, cipher: cipher}
}

// 回归：非法存储类型 / 缺必填项必须被拒绝（避免"配了但不生效"或半配置状态）。
func TestStorageSettingsUpdateValidation(t *testing.T) {
	cases := []struct {
		name string
		in   *repository.StorageSettings
	}{
		{"不支持的类型", &repository.StorageSettings{Type: "ftp"}},
		{"local 缺根目录", &repository.StorageSettings{Type: "local"}},
		{"s3 缺 bucket", &repository.StorageSettings{Type: "s3", Endpoint: "https://s3.example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newStorageSettingsService(&stubStorageSettingsRepo{})
			if err := svc.UpdateSettings(context.Background(), "admin", tc.in); err == nil {
				t.Fatal("期望校验失败，实际通过")
			}
		})
	}
}

// 回归：Secret Key 必须加密落库；请求留空时保留既有密钥（write-only 语义）。
func TestStorageSettingsUpdateEncryptsSecret(t *testing.T) {
	repo := &stubStorageSettingsRepo{}
	svc := newStorageSettingsService(repo)

	in := &repository.StorageSettings{
		Type: "s3", Endpoint: "https://s3.example.com", Bucket: "polaris",
		AccessKey: "AKIA", SecretKeyEnc: "super-secret-key",
	}
	if err := svc.UpdateSettings(context.Background(), "admin", in); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("未落库")
	}
	if repo.saved.SecretKeyEnc == "super-secret-key" || repo.saved.SecretKeyEnc == "" {
		t.Fatalf("Secret Key 未加密落库: %q", repo.saved.SecretKeyEnc)
	}
	plain, err := svc.cipher.Decrypt(repo.saved.SecretKeyEnc)
	if err != nil || plain != "super-secret-key" {
		t.Fatalf("密文无法解密回明文: %v %q", err, plain)
	}
}

func TestStorageSettingsUpdateKeepsExistingSecret(t *testing.T) {
	repo := &stubStorageSettingsRepo{current: &repository.StorageSettings{
		ID: 1, Type: "s3", SecretKeyEnc: "ENCRYPTED-EXISTING",
	}}
	svc := newStorageSettingsService(repo)

	// 请求未携带密钥（空）→ 应保留既有密文，不因编辑其它字段而被清空
	in := &repository.StorageSettings{Type: "s3", Bucket: "polaris"}
	if err := svc.UpdateSettings(context.Background(), "admin", in); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if repo.saved.SecretKeyEnc != "ENCRYPTED-EXISTING" {
		t.Fatalf("留空时应保留既有密钥，实际 %q", repo.saved.SecretKeyEnc)
	}
}

// 回归：type 为空 = 界面上显式选择「未配置」，必须把整行清零落库。
// 历史缺陷：空类型被 normalizeStorageType 补成 local，随后又要求 root_path 必填——
// 于是"未配置"既存不下去、旧配置也不会被清掉，界面上的开关等于摆设。
func TestStorageSettingsClearResetsRow(t *testing.T) {
	repo := &stubStorageSettingsRepo{current: &repository.StorageSettings{
		ID: 1, Type: "s3", Endpoint: "https://s3.example.com", Bucket: "polaris", SecretKeyEnc: "ENCRYPTED-EXISTING",
	}}
	svc := newStorageSettingsService(repo)

	if err := svc.UpdateSettings(context.Background(), "admin", &repository.StorageSettings{}); err != nil {
		t.Fatalf("清除配置失败: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("未落库")
	}
	if repo.saved.Type != "" || repo.saved.Endpoint != "" || repo.saved.Bucket != "" || repo.saved.SecretKeyEnc != "" {
		t.Fatalf("未清空整行: %+v", repo.saved)
	}
}
