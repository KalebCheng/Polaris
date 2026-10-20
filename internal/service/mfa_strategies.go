package service

import (
	"context"
	"fmt"
	"strings"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	providerpkg "polaris/internal/service/provider"
)

// mfaStrategy 每种 MFA 类型的绑定、发送与校验策略。
// 登录链路只解析策略，不再判断“是不是 TOTP/短信”。
type mfaStrategy interface {
	Begin(ctx context.Context, svc *MFAService, session *auth.Session, user *repository.User,
		provider *repository.Provider, cred *repository.UserMFACredential) (*MFAEnrollResult, error)
	VerifyEnrollment(ctx context.Context, svc *MFAService, session *auth.Session,
		provider *repository.Provider, cred *repository.UserMFACredential, code string) error
	Verify(ctx context.Context, svc *MFAService, session *auth.Session,
		provider *repository.Provider, cred *repository.UserMFACredential, code string) (bool, error)
	VerifyDirect(ctx context.Context, svc *MFAService, institutionID, userID int64,
		provider *repository.Provider, cred *repository.UserMFACredential, code string) (bool, error)
	Resend(ctx context.Context, svc *MFAService, session *auth.Session,
		provider *repository.Provider, cred *repository.UserMFACredential) error
	SupportsRecovery() bool
}

var mfaStrategies = map[string]mfaStrategy{
	providerpkg.TypeMFATOTP: totpMFAStrategy{},
	providerpkg.TypeMFASMS:  smsMFAStrategy{},
}

func mfaStrategyFor(provider *repository.Provider) (mfaStrategy, error) {
	kind := providerpkg.NormalizeType(providerpkg.CategoryMFA, provider.Type)
	strategy, ok := mfaStrategies[kind]
	if !ok {
		return nil, bizerrors.ErrInvalidParams.WithDetail("type", provider.Type)
	}
	return strategy, nil
}

type totpMFAStrategy struct{}

func (totpMFAStrategy) Begin(ctx context.Context, svc *MFAService, session *auth.Session, user *repository.User,
	provider *repository.Provider, cred *repository.UserMFACredential) (*MFAEnrollResult, error) {
	orgName := fmt.Sprintf("%d", session.InstitutionID)
	if org, err := svc.orgs.GetByID(ctx, session.InstitutionID); err == nil && org != nil && org.Name != "" {
		orgName = org.Name
	}
	secret, url, err := auth.GenerateTOTPSecretWithOptions(orgName, user.Name, totpOptions(provider))
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成 TOTP 密钥失败", err)
	}
	if svc.cipher == nil {
		return nil, bizerrors.New(bizerrors.CodeInternalError, "credential cipher not configured")
	}
	encrypted, err := svc.cipher.Encrypt(secret)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "加密 TOTP 密钥失败", err)
	}
	cred.SecretEncrypted = encrypted
	return &MFAEnrollResult{ProviderID: provider.ID, Type: providerpkg.TypeMFATOTP, Secret: secret, URL: url,
		TTLSeconds: provider.MfaCodeTTLSeconds}, nil
}

func (totpMFAStrategy) VerifyEnrollment(ctx context.Context, svc *MFAService, _ *auth.Session,
	provider *repository.Provider, cred *repository.UserMFACredential, code string) error {
	if !svc.verifyTOTP(ctx, cred, provider, code) {
		return bizerrors.ErrMfaInvalid
	}
	return nil
}

func (totpMFAStrategy) Verify(ctx context.Context, svc *MFAService, _ *auth.Session,
	provider *repository.Provider, cred *repository.UserMFACredential, code string) (bool, error) {
	if svc.verifyTOTP(ctx, cred, provider, code) {
		return false, nil
	}
	if svc.consumeRecoveryCode(ctx, cred, code) {
		return true, nil
	}
	return false, bizerrors.ErrMfaInvalid
}

func (totpMFAStrategy) VerifyDirect(ctx context.Context, svc *MFAService, _, _ int64,
	provider *repository.Provider, cred *repository.UserMFACredential, code string) (bool, error) {
	if svc.verifyTOTP(ctx, cred, provider, code) {
		return true, nil
	}
	return svc.consumeRecoveryCode(ctx, cred, code), nil
}

func (totpMFAStrategy) SupportsRecovery() bool { return true }

func (totpMFAStrategy) Resend(context.Context, *MFAService, *auth.Session, *repository.Provider, *repository.UserMFACredential) error {
	return bizerrors.ErrInvalidParams.WithDetail("type", providerpkg.TypeMFATOTP)
}

type smsMFAStrategy struct{}

func (smsMFAStrategy) Begin(ctx context.Context, svc *MFAService, session *auth.Session, user *repository.User,
	provider *repository.Provider, cred *repository.UserMFACredential) (*MFAEnrollResult, error) {
	if strings.TrimSpace(user.Phone) == "" {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, "user has no bound phone")
	}
	cred.Phone = user.Phone
	if err := svc.sendSMSCode(ctx, session, provider, user, false); err != nil {
		return nil, err
	}
	return &MFAEnrollResult{ProviderID: provider.ID, Type: providerpkg.TypeMFASMS, Sent: true,
		Phone: maskPhone(user.Phone), TTLSeconds: provider.MfaCodeTTLSeconds}, nil
}

func (smsMFAStrategy) VerifyEnrollment(ctx context.Context, svc *MFAService, session *auth.Session,
	provider *repository.Provider, _ *repository.UserMFACredential, code string) error {
	return svc.verifySMSCode(ctx, session, provider, code)
}

func (smsMFAStrategy) Verify(ctx context.Context, svc *MFAService, session *auth.Session,
	provider *repository.Provider, _ *repository.UserMFACredential, code string) (bool, error) {
	return false, svc.verifySMSCode(ctx, session, provider, code)
}

func (smsMFAStrategy) VerifyDirect(ctx context.Context, svc *MFAService, institutionID, userID int64,
	provider *repository.Provider, _ *repository.UserMFACredential, code string) (bool, error) {
	if svc.codes == nil {
		return false, bizerrors.ErrProviderUnavailable.WithDetail("reason", "code store not configured")
	}
	stored, err := svc.codes.Take(ctx, mfaCodeKey(institutionID, userID, provider.ID))
	if err != nil {
		return false, err
	}
	return stored != "" && strings.TrimSpace(stored) == strings.TrimSpace(code), nil
}

func (smsMFAStrategy) SupportsRecovery() bool { return false }

func (smsMFAStrategy) Resend(ctx context.Context, svc *MFAService, session *auth.Session,
	provider *repository.Provider, _ *repository.UserMFACredential) error {
	user, err := svc.users.GetByID(ctx, session.UserID)
	if err != nil {
		return bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	return svc.sendSMSCode(ctx, session, provider, user, true)
}
