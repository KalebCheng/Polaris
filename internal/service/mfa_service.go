package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service/notify"
	providerpkg "polaris/internal/service/provider"
)

const mfaSendRateScope = "polaris:mfa_send:"
const mfaCodeKeyScope = "polaris:mfa_code:"

// MFAService 多因素认证运行时不变量：
//   - 登录方式只声明允许的 provider 集合；
//   - 用户凭证按 provider 绑定；
//   - pending 会话只能访问 MFA 必要接口。
type MFAService struct {
	providers   repository.ProviderRepository
	credentials repository.UserMFACredentialRepository
	users       repository.UserRepository
	orgs        repository.InstitutionRepository
	templates   *TemplateService
	runtime     *providerpkg.Runtime
	codes       repository.CodeStore
	rateLimiter repository.RateLimitRepository
	sessions    *auth.SessionManager
	jwt         *auth.JWTManager
	audits      *AuditRecorder
	cipher      *auth.SecretCipher
}

// NewMFAService 创建 MFA 运行时。
func NewMFAService(
	providers repository.ProviderRepository,
	credentials repository.UserMFACredentialRepository,
	users repository.UserRepository,
	orgs repository.InstitutionRepository,
	templates *TemplateService,
	runtime *providerpkg.Runtime,
	codes repository.CodeStore,
	rateLimiter repository.RateLimitRepository,
	sessions *auth.SessionManager,
	jwt *auth.JWTManager,
	audits *AuditRecorder,
	cipher *auth.SecretCipher,
) *MFAService {
	return &MFAService{
		providers: providers, credentials: credentials, users: users, orgs: orgs,
		templates: templates, runtime: runtime, codes: codes, rateLimiter: rateLimiter,
		sessions: sessions, jwt: jwt, audits: audits, cipher: cipher,
	}
}

// MFAProviderOption 登录页可选择的 MFA 提供商。
type MFAProviderOption struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// MFABoundOption 用户已绑定的 MFA 凭证。
type MFABoundOption struct {
	ProviderID  int64  `json:"provider_id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// MFAOptions pending 会话可用的 MFA 选项。
type MFAOptions struct {
	Required      bool                `json:"required"`
	CanSkip       bool                `json:"can_skip"`
	Providers     []MFAProviderOption `json:"providers"`
	Bound         []MFABoundOption    `json:"bound"`
	RememberHours int                 `json:"remember_hours"`
}

// MFAEnrollResult 开始绑定 MFA 的返回。
type MFAEnrollResult struct {
	ProviderID int64  `json:"provider_id"`
	Type       string `json:"type"`
	Secret     string `json:"secret,omitempty"`
	URL        string `json:"url,omitempty"`
	Sent       bool   `json:"sent,omitempty"`
	Phone      string `json:"phone,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// MFAVerifyResult 校验成功后的内部结果（信任令牌由 handler 下发 Cookie）。
type MFAVerifyResult struct {
	TrustToken    string   `json:"-"`
	TrustMaxAge   int      `json:"-"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

// Options 返回当前 pending 会话的 MFA options。
func (s *MFAService) Options(ctx context.Context, session *auth.Session) (*MFAOptions, error) {
	if session == nil {
		return nil, bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	var providers []*repository.Provider
	var err error
	if session.MfaPending {
		providers, err = s.allowedProviders(ctx, session)
	} else {
		// 个人资料页复用同一端点：列出组织内全部启用 MFA 提供商与已绑定凭证。
		providers, err = s.runtime.ListActive(ctx, session.InstitutionID, providerpkg.CategoryMFA)
	}
	if err != nil {
		return nil, err
	}
	options := &MFAOptions{Required: session.MfaRequired, Providers: make([]MFAProviderOption, 0, len(providers))}
	allowed := map[int64]bool{}
	remember := 0
	for _, p := range providers {
		allowed[p.ID] = true
		options.Providers = append(options.Providers, MFAProviderOption{
			ID: p.ID, Type: providerpkg.NormalizeType(providerpkg.CategoryMFA, p.Type),
			Name: p.Name, DisplayName: firstNonEmptyStr(p.DisplayName, p.Name),
		})
		if p.MfaRememberHours > remember {
			remember = p.MfaRememberHours
		}
	}
	creds, err := s.credentials.ListByUser(ctx, session.InstitutionID, session.UserID)
	if err != nil {
		return nil, err
	}
	for _, cred := range creds {
		if cred.Status != repository.MfaCredentialActive || !allowed[cred.ProviderID] {
			continue
		}
		p := providersByID(providers, cred.ProviderID)
		if p == nil {
			continue
		}
		options.Bound = append(options.Bound, MFABoundOption{
			ProviderID: cred.ProviderID, Type: cred.Type,
			Name: p.Name, DisplayName: firstNonEmptyStr(p.DisplayName, p.Name),
		})
	}
	// 可跳过只覆盖“尚未绑定”的绑定提示；已绑定用户仍必须校验。
	options.CanSkip = session.MfaPending && !session.MfaRequired && len(options.Bound) == 0
	options.RememberHours = remember
	return options, nil
}

// BeginEnrollment 开始绑定指定 MFA 提供商。
func (s *MFAService) BeginEnrollment(ctx context.Context, session *auth.Session, providerID int64) (*MFAEnrollResult, error) {
	provider, err := s.providerForSession(ctx, session, providerID)
	if err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil || user.InstitutionID != session.InstitutionID {
		return nil, bizerrors.New(bizerrors.CodeUserNotFound, "user not found")
	}
	strategy, err := mfaStrategyFor(provider)
	if err != nil {
		return nil, err
	}
	kind := providerpkg.NormalizeType(providerpkg.CategoryMFA, provider.Type)
	cred := &repository.UserMFACredential{
		InstitutionID: session.InstitutionID, UserID: session.UserID, ProviderID: provider.ID,
		Type: kind, Status: repository.MfaCredentialPending,
	}
	result, err := strategy.Begin(ctx, s, session, user, provider, cred)
	if err != nil {
		return nil, err
	}
	if _, err := s.credentials.Save(ctx, cred); err != nil {
		return nil, err
	}
	return result, nil
}

// resendEnrollmentCode 重新发送短信 MFA 验证码（绑定页“重新发送”）。
func (s *MFAService) ResendEnrollmentCode(ctx context.Context, session *auth.Session, providerID int64) error {
	provider, err := s.providerForSession(ctx, session, providerID)
	if err != nil {
		return err
	}
	cred, err := s.credentials.Get(ctx, session.InstitutionID, session.UserID, providerID)
	if err != nil || cred.Status != repository.MfaCredentialPending {
		return bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	strategy, err := mfaStrategyFor(provider)
	if err != nil {
		return err
	}
	return strategy.Resend(ctx, s, session, provider, cred)
}

// VerifyEnrollment 校验绑定验证码并激活凭证。
func (s *MFAService) VerifyEnrollment(ctx context.Context, session *auth.Session, providerID int64, code string) (*MFAVerifyResult, error) {
	provider, err := s.providerForSession(ctx, session, providerID)
	if err != nil {
		return nil, err
	}
	cred, err := s.credentials.Get(ctx, session.InstitutionID, session.UserID, providerID)
	if err != nil || cred.Status != repository.MfaCredentialPending {
		return nil, bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	strategy, err := mfaStrategyFor(provider)
	if err != nil {
		return nil, err
	}
	if err := strategy.VerifyEnrollment(ctx, s, session, provider, cred, code); err != nil {
		return nil, err
	}
	codes := []string{}
	if strategy.SupportsRecovery() {
		plain, genErr := auth.GenerateRecoveryCodes(8)
		if genErr != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "生成恢复码失败", genErr)
		}
		codes = plain
		cred.RecoveryCodesHashed = hashRecoveryCodes(plain)
	}
	cred.Status = repository.MfaCredentialActive
	if err := s.credentials.Update(ctx, cred); err != nil {
		return nil, err
	}
	_ = s.syncLegacyMFA(ctx, session.InstitutionID, session.UserID)
	if session.MfaPending {
		if err := s.finishPending(ctx, session); err != nil {
			return nil, err
		}
	}
	s.audits.Record(ctx, session.Name, session.InstitutionID, "user.mfa_enable", session.Name,
		map[string]interface{}{"provider": provider.Name, "type": providerpkg.NormalizeType(providerpkg.CategoryMFA, provider.Type)})
	result := &MFAVerifyResult{RecoveryCodes: codes}
	if session.MfaRemember && provider.MfaRememberHours > 0 {
		if token, tokenErr := s.jwt.GenerateMfaTrust(session.InstitutionID, session.Name, provider.ID,
			time.Duration(provider.MfaRememberHours)*time.Hour); tokenErr == nil {
			result.TrustToken, result.TrustMaxAge = token, provider.MfaRememberHours*3600
		}
	}
	return result, nil
}

// Verify 校验已有凭证；成功返回可选恢复码（消费恢复码时不返回新码）。
func (s *MFAService) Verify(ctx context.Context, session *auth.Session, providerID int64, code string, remember bool) (*MFAVerifyResult, error) {
	provider, err := s.providerForSession(ctx, session, providerID)
	if err != nil {
		return nil, err
	}
	cred, err := s.credentials.Get(ctx, session.InstitutionID, session.UserID, providerID)
	if err != nil || cred.Status != repository.MfaCredentialActive {
		return nil, bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	strategy, err := mfaStrategyFor(provider)
	if err != nil {
		return nil, err
	}
	recoveryUsed, err := strategy.Verify(ctx, s, session, provider, cred, code)
	if err != nil {
		return nil, err
	}
	if err := s.finishPending(ctx, session); err != nil {
		return nil, err
	}
	s.audits.Record(ctx, session.Name, session.InstitutionID, "auth.mfa_verify", session.Name,
		map[string]interface{}{"provider": provider.Name, "recovery": recoveryUsed})
	result := &MFAVerifyResult{}
	if remember && provider.MfaRememberHours > 0 {
		if token, tokenErr := s.jwt.GenerateMfaTrust(session.InstitutionID, session.Name, provider.ID,
			time.Duration(provider.MfaRememberHours)*time.Hour); tokenErr == nil {
			result.TrustToken, result.TrustMaxAge = token, provider.MfaRememberHours*3600
		}
	}
	return result, nil
}

// Skip 可选模式下跳过“未绑定”提示。
func (s *MFAService) Skip(ctx context.Context, session *auth.Session) error {
	if session == nil || !session.MfaPending {
		return bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	options, err := s.Options(ctx, session)
	if err != nil {
		return err
	}
	if !options.CanSkip {
		return bizerrors.ErrMfaRequired
	}
	return s.finishPending(ctx, session)
}

// ResetUser 删除用户全部 MFA 凭证（管理员重置）。
func (s *MFAService) ResetUser(ctx context.Context, institutionID, userID int64) error {
	items, err := s.credentials.ListByUser(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.credentials.Delete(ctx, institutionID, userID, item.ProviderID); err != nil {
			return err
		}
	}
	_ = s.syncLegacyMFA(ctx, institutionID, userID)
	return nil
}

// syncLegacyMFA 同步展示用旧列；认证链路不再读取 users.mfa_*。
func (s *MFAService) syncLegacyMFA(ctx context.Context, institutionID, userID int64) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	items, err := s.credentials.ListByUser(ctx, institutionID, userID)
	if err != nil {
		return err
	}
	enabled := false
	types := []string{}
	secret := ""
	for _, item := range items {
		if item.Status != repository.MfaCredentialActive {
			continue
		}
		enabled = true
		if !contains(types, item.Type) {
			types = append(types, item.Type)
		}
		if secret == "" && item.Type == providerpkg.TypeMFATOTP {
			secret = item.SecretEncrypted
		}
	}
	user.MfaEnabled = enabled
	user.MfaType = strings.Join(types, ",")
	user.MfaSecret = secret
	if !enabled {
		user.MfaRecoveryCodes = nil
	}
	return s.users.Update(ctx, user)
}

// CredentialSummary 返回用户 MFA 概览（个人资料 / 管理员安全视图）。
func (s *MFAService) CredentialSummary(ctx context.Context, institutionID, userID int64) (enabled bool, types []string, recoveryLeft int, err error) {
	items, err := s.credentials.ListByUser(ctx, institutionID, userID)
	if err != nil {
		return false, nil, 0, err
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Status != repository.MfaCredentialActive {
			continue
		}
		enabled = true
		if !seen[item.Type] {
			seen[item.Type] = true
			types = append(types, item.Type)
		}
		if len(item.RecoveryCodesHashed) > recoveryLeft {
			recoveryLeft = len(item.RecoveryCodesHashed)
		}
	}
	return enabled, types, recoveryLeft, nil
}

// Disable 关闭某个 MFA 凭证。
func (s *MFAService) Disable(ctx context.Context, actor string, session *auth.Session, providerID int64) error {
	if session == nil || session.MfaPending {
		return bizerrors.ErrUnauthorized
	}
	provider, err := s.providerByID(ctx, session.InstitutionID, providerID)
	if err != nil {
		return err
	}
	if err := s.credentials.Delete(ctx, session.InstitutionID, session.UserID, providerID); err != nil {
		return err
	}
	_ = s.syncLegacyMFA(ctx, session.InstitutionID, session.UserID)
	s.audits.Record(ctx, actor, session.InstitutionID, "user.mfa_disable", session.Name,
		map[string]interface{}{"provider": provider.Name})
	return nil
}

// Bound 返回用户已绑定凭证（个人资料页展示）。
func (s *MFAService) Bound(ctx context.Context, institutionID, userID int64) ([]*repository.UserMFACredential, error) {
	return s.credentials.ListByUser(ctx, institutionID, userID)
}

// ActiveCredentials 返回允许集合内状态为 active 的凭证（登录主链路用于判断“已绑定/未绑定”）。
func (s *MFAService) ActiveCredentials(ctx context.Context, institutionID, userID int64, providerIDs []int64) ([]*repository.UserMFACredential, error) {
	allowed := make(map[int64]bool, len(providerIDs))
	for _, id := range providerIDs {
		allowed[id] = true
	}
	items, err := s.credentials.ListByUser(ctx, institutionID, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*repository.UserMFACredential, 0, len(items))
	for _, item := range items {
		if item.Status == repository.MfaCredentialActive && allowed[item.ProviderID] {
			out = append(out, item)
		}
	}
	return out, nil
}

// ProviderByIDForUser 返回 MFA 提供商配置（密钥已解密；仅 category=mfa 且启用）。
func (s *MFAService) ProviderByIDForUser(ctx context.Context, institutionID, providerID int64) (*repository.Provider, error) {
	return s.providerByID(ctx, institutionID, providerID)
}

// VerifyCredentialDirect 供旧客户端在登录请求中直接携带 mfa_code 时使用：
// 校验已有凭证但不要求 pending 会话（调用方随后建立完整会话或复用 pending 会话）。
func (s *MFAService) VerifyCredentialDirect(ctx context.Context, institutionID, userID, providerID int64, code string) (bool, error) {
	provider, err := s.providerByID(ctx, institutionID, providerID)
	if err != nil {
		return false, err
	}
	cred, err := s.credentials.Get(ctx, institutionID, userID, providerID)
	if err != nil || cred.Status != repository.MfaCredentialActive {
		return false, nil
	}
	strategy, err := mfaStrategyFor(provider)
	if err != nil {
		return false, err
	}
	return strategy.VerifyDirect(ctx, s, institutionID, userID, provider, cred, code)
}

func (s *MFAService) finishPending(ctx context.Context, session *auth.Session) error {
	if err := s.sessions.SetMfaPending(ctx, session.ID, false); err != nil {
		return err
	}
	session.MfaPending = false
	session.MfaProviderIDs = nil
	session.MfaRequired = false
	session.MfaCanSkip = false
	session.MfaMethod = ""
	return nil
}

func (s *MFAService) allowedProviders(ctx context.Context, session *auth.Session) ([]*repository.Provider, error) {
	out := make([]*repository.Provider, 0, len(session.MfaProviderIDs))
	for _, id := range session.MfaProviderIDs {
		item, err := s.providerByID(ctx, session.InstitutionID, id)
		if err != nil {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("category", string(providerpkg.CategoryMFA))
	}
	return out, nil
}

func (s *MFAService) providerForSession(ctx context.Context, session *auth.Session, providerID int64) (*repository.Provider, error) {
	if session == nil {
		return nil, bizerrors.New(bizerrors.CodeMfaNotBound, "multi-factor authentication not bound")
	}
	if session.MfaPending {
		allowed := false
		for _, id := range session.MfaProviderIDs {
			if id == providerID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, bizerrors.ErrInvalidParams.WithDetail("provider_id", providerID)
		}
	}
	return s.providerByID(ctx, session.InstitutionID, providerID)
}

func (s *MFAService) providerByID(ctx context.Context, institutionID, providerID int64) (*repository.Provider, error) {
	item, err := s.providers.GetByID(ctx, providerID)
	if err != nil || item == nil || item.InstitutionID != institutionID ||
		providerpkg.NormalizeCategory(item.Category) != string(providerpkg.CategoryMFA) ||
		!strings.EqualFold(item.State, "active") {
		return nil, bizerrors.ErrProviderUnavailable.WithDetail("provider_id", providerID)
	}
	s.runtime.DecryptSecrets(item)
	return item, nil
}

func (s *MFAService) verifyTOTP(ctx context.Context, cred *repository.UserMFACredential, provider *repository.Provider, code string) bool {
	if s.cipher == nil || cred.SecretEncrypted == "" {
		return false
	}
	secret, err := s.cipher.Decrypt(cred.SecretEncrypted)
	if err != nil || secret == "" {
		return false
	}
	return auth.VerifyTOTPWithOptions(secret, code, totpOptions(provider))
}

func (s *MFAService) sendSMSCode(ctx context.Context, session *auth.Session, mfaProvider *repository.Provider, user *repository.User, resend bool) error {
	if s.templates == nil || s.codes == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("reason", "mfa dependencies not configured")
	}
	rateKey := fmt.Sprintf("%s%d:%d:%d", mfaSendRateScope, session.InstitutionID, user.ID, mfaProvider.ID)
	if resend && s.rateLimiter != nil {
		if n, _ := s.rateLimiter.Get(ctx, rateKey); n > 0 {
			return bizerrors.ErrVerifyCodeTooMany.WithDetail("retry_after_seconds", 60)
		}
	}
	if s.rateLimiter != nil {
		if _, err := s.rateLimiter.Incr(ctx, rateKey, time.Minute); err != nil {
			return err
		}
	}
	smsProvider, tpl, err := s.templates.ResolveNotification(ctx, session.InstitutionID,
		mfaProvider.MfaSMSProviderID, mfaProvider.MfaSMSTemplateID, repository.TemplateSceneMfaCode, providerpkg.CategorySMS)
	if err != nil {
		return err
	}
	sender, err := notificationSender(smsProvider, tpl)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "notification provider unavailable", err)
	}
	code, err := generateNumericCode(6)
	if err != nil {
		return bizerrors.Wrap(bizerrors.CodeInternalError, "生成验证码失败", err)
	}
	ttl := time.Duration(defaultInt(mfaProvider.MfaCodeTTLSeconds, 300)) * time.Second
	if err := s.codes.Save(ctx, mfaCodeKey(session.InstitutionID, user.ID, mfaProvider.ID), code, ttl); err != nil {
		return err
	}
	vars := templateVars(user, code, ttl, user.Phone, "", "")
	if err := sender.Send(ctx, notify.Message{To: user.Phone, Code: code, Vars: vars}); err != nil {
		return bizerrors.Wrap(bizerrors.CodeProviderUnavailable, "send mfa code failed", err)
	}
	s.audits.Record(ctx, user.Name, session.InstitutionID, "auth.mfa_send_code", user.Name,
		map[string]interface{}{"provider": mfaProvider.Name})
	return nil
}

func (s *MFAService) verifySMSCode(ctx context.Context, session *auth.Session, mfaProvider *repository.Provider, code string) error {
	if s.codes == nil {
		return bizerrors.ErrProviderUnavailable.WithDetail("reason", "code store not configured")
	}
	stored, err := s.codes.Take(ctx, mfaCodeKey(session.InstitutionID, session.UserID, mfaProvider.ID))
	if err != nil {
		return err
	}
	if stored == "" || subtle.ConstantTimeCompare([]byte(stored), []byte(strings.TrimSpace(code))) != 1 {
		return bizerrors.ErrMfaInvalid
	}
	return nil
}

func (s *MFAService) consumeRecoveryCode(ctx context.Context, cred *repository.UserMFACredential, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	hash := hashRecoveryCode(code)
	legacy := false
	if v, ok := cred.Metadata["legacy_recovery_plain"].(bool); ok {
		legacy = v
	}
	for i, stored := range cred.RecoveryCodesHashed {
		matched := subtle.ConstantTimeCompare([]byte(stored), []byte(hash)) == 1
		if legacy && !matched {
			matched = subtle.ConstantTimeCompare([]byte(stored), []byte(code)) == 1
		}
		if !matched {
			continue
		}
		remaining := append([]string(nil), cred.RecoveryCodesHashed[:i]...)
		remaining = append(remaining, cred.RecoveryCodesHashed[i+1:]...)
		if legacy {
			hashed := make([]string, 0, len(remaining))
			for _, value := range remaining {
				hashed = append(hashed, hashRecoveryCode(value))
			}
			remaining = hashed
			if cred.Metadata == nil {
				cred.Metadata = map[string]any{}
			}
			delete(cred.Metadata, "legacy_recovery_plain")
		}
		cred.RecoveryCodesHashed = remaining
		_ = s.credentials.Update(ctx, cred)
		return true
	}
	return false
}

func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func hashRecoveryCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		out = append(out, hashRecoveryCode(code))
	}
	return out
}

func totpOptions(p *repository.Provider) auth.TOTPOptions {
	return auth.TOTPOptions{Issuer: p.MfaIssuer, Digits: p.MfaDigits, Period: p.MfaPeriod, Algorithm: p.MfaAlgorithm}
}

func providersByID(items []*repository.Provider, id int64) *repository.Provider {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func mfaCodeKey(institutionID, userID, providerID int64) string {
	return fmt.Sprintf("%s%d:%d:%d", mfaCodeKeyScope, institutionID, userID, providerID)
}

func defaultInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) <= 4 {
		return phone
	}
	return strings.Repeat("*", len(phone)-4) + phone[len(phone)-4:]
}

func notificationSender(p *repository.Provider, tpl *repository.ProviderTemplate) (notify.Sender, error) {
	cfg := notify.Config{
		Name: p.Name, Type: p.Type,
		Host: p.Host, Port: p.Port, User: p.ClientID, Password: p.ClientSecret,
		SslMode: p.SslMode, FromAddress: p.FromAddress, Title: tpl.Subject,
		Content: tpl.Content, RegionID: p.RegionID, SignName: p.SignName,
		TemplateCode: tpl.TemplateCode, AppID: p.ClientID, AppSecret: p.ClientSecret,
		Endpoint: p.Endpoint, TemplateParams: tpl.TemplateParams,
	}
	return notify.New(p.Category, cfg)
}

func templateVars(user *repository.User, code string, ttl time.Duration, phone, institution, application string) map[string]string {
	minutes := int(ttl.Minutes())
	if minutes <= 0 {
		minutes = 1
	}
	return map[string]string{
		providerpkg.TemplateVarCode:        code,
		providerpkg.TemplateVarMinutes:     fmt.Sprintf("%d", minutes),
		providerpkg.TemplateVarPhone:       firstNonEmptyStr(phone, user.Phone),
		providerpkg.TemplateVarInstitution: institution,
		providerpkg.TemplateVarApplication: application,
	}
}
