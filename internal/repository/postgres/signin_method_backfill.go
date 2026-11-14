package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

type legacyApplicationSignin struct {
	ID          int64
	Institution int64
	Code        string
	Methods     []repository.SigninMethod
	Providers   []repository.ProviderItem
	EmailTTL    int
	SMSTTL      int
	MagicTTL    int
}

type legacyInstitutionSignin struct {
	ID                 int64
	Name               string
	DisplayName        string
	DefaultApplication string
	Applications       []legacyApplicationSignin
}

type backfillProvider struct {
	ID          int64
	Name        string
	DisplayName string
	State       string
	Category    string
}

type backfillTemplate struct {
	ID         int64
	ProviderID int64
	Scene      string
	Name       string
}

type signinBackfiller struct {
	tx              pgx.Tx
	institutionID   int64
	methods         map[string]int64
	providers       map[string]*backfillProvider
	emailTPL        map[int64]*backfillTemplate
	smsTPL          map[int64]*backfillTemplate
	conflicts       int
	mfaWasAvailable bool
}

// BackfillLegacySigninMethods 把旧 applications.signin_methods/providers 回填到组织方法表。
//
// 幂等：已有组织方法/应用选择不会被重复插入；旧列在 008 删除后本函数自动跳过。
func BackfillLegacySigninMethods(ctx context.Context, db *dbclient.Client) error {
	var legacyExists bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'applications' AND column_name = 'signin_methods'
		)`).Scan(&legacyExists); err != nil {
		return mapError(err, "")
	}
	if !legacyExists {
		return nil
	}

	orgs, err := loadLegacyInstitutions(ctx, db)
	if err != nil {
		return err
	}
	apps, err := loadLegacyApplications(ctx, db)
	if err != nil {
		return err
	}
	byOrg := map[int64][]legacyApplicationSignin{}
	for _, app := range apps {
		byOrg[app.Institution] = append(byOrg[app.Institution], app)
	}
	for i := range orgs {
		orgs[i].Applications = byOrg[orgs[i].ID]
	}

	totalConflicts := 0
	for _, org := range orgs {
		if err := db.WithTx(ctx, func(tx pgx.Tx) error {
			b := &signinBackfiller{
				tx: tx, institutionID: org.ID,
				methods:   map[string]int64{},
				providers: map[string]*backfillProvider{},
				emailTPL:  map[int64]*backfillTemplate{},
				smsTPL:    map[int64]*backfillTemplate{},
			}
			if err := b.backfill(ctx, org); err != nil {
				return err
			}
			totalConflicts += b.conflicts
			return nil
		}); err != nil {
			return fmt.Errorf("回填组织 %s 登录方式失败: %w", org.Name, err)
		}
	}
	if len(orgs) > 0 {
		logger.Info("legacy sign-in methods backfilled", "institutions", len(orgs), "applications", len(apps), "conflicts", totalConflicts)
	}
	return nil
}

func loadLegacyInstitutions(ctx context.Context, db *dbclient.Client) ([]legacyInstitutionSignin, error) {
	rows, err := db.Query(ctx, `SELECT id, name, display_name, default_application FROM institutions ORDER BY id`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []legacyInstitutionSignin
	for rows.Next() {
		var org legacyInstitutionSignin
		if err := rows.Scan(&org.ID, &org.Name, &org.DisplayName, &org.DefaultApplication); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, org)
	}
	return out, mapError(rows.Err(), "")
}

func loadLegacyApplications(ctx context.Context, db *dbclient.Client) ([]legacyApplicationSignin, error) {
	rows, err := db.Query(ctx, `
		SELECT id, institution_id, code, signin_methods, providers,
		       email_code_ttl_seconds, sms_code_ttl_seconds, magic_link_ttl_seconds
		FROM applications ORDER BY institution_id, id`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []legacyApplicationSignin
	for rows.Next() {
		var app legacyApplicationSignin
		var methodsRaw, providersRaw []byte
		if err := rows.Scan(&app.ID, &app.Institution, &app.Code, &methodsRaw, &providersRaw,
			&app.EmailTTL, &app.SMSTTL, &app.MagicTTL); err != nil {
			return nil, mapError(err, "")
		}
		if len(methodsRaw) > 0 {
			if err := json.Unmarshal(methodsRaw, &app.Methods); err != nil {
				return nil, fmt.Errorf("解析应用 %s 的 signin_methods 失败: %w", app.Code, err)
			}
		}
		if len(providersRaw) > 0 {
			if err := json.Unmarshal(providersRaw, &app.Providers); err != nil {
				return nil, fmt.Errorf("解析应用 %s 的 providers 失败: %w", app.Code, err)
			}
		}
		out = append(out, app)
	}
	return out, mapError(rows.Err(), "")
}

func (b *signinBackfiller) backfill(ctx context.Context, org legacyInstitutionSignin) error {
	if err := b.ensureDefaultTOTP(ctx, org); err != nil {
		return err
	}
	baseline := b.baselineApp(org)

	passwordID, err := b.ensurePassword(ctx, baseline, org.Applications)
	if err != nil {
		return err
	}
	channelIDs := map[string]int64{}
	for _, kind := range []string{
		repository.SigninMethodEmailCode,
		repository.SigninMethodSMSCode,
		repository.SigninMethodMagicLink,
	} {
		id, ensureErr := b.ensureChannel(ctx, kind, baseline, org.Applications)
		if ensureErr != nil {
			return ensureErr
		}
		if id > 0 {
			channelIDs[kind] = id
		}
	}
	providerIDs, err := b.ensureProviders(ctx, org.Applications)
	if err != nil {
		return err
	}
	for _, app := range org.Applications {
		ids := b.selectionIDs(app, passwordID, channelIDs, providerIDs)
		if len(ids) == 0 {
			b.recordConflict(ctx, app.ID, "selection", map[string]any{
				"application": app.Code, "reason": "no executable sign-in method after mapping",
			})
			continue
		}
		for _, methodID := range ids {
			if _, err := b.tx.Exec(ctx, `
				INSERT INTO application_signin_methods (institution_id, application_id, method_id)
				VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				org.ID, app.ID, methodID); err != nil {
				return mapError(err, "")
			}
		}
	}
	return nil
}

func (b *signinBackfiller) baselineApp(org legacyInstitutionSignin) *legacyApplicationSignin {
	if len(org.Applications) == 0 {
		return nil
	}
	if org.DefaultApplication != "" {
		for i := range org.Applications {
			if org.Applications[i].Code == org.DefaultApplication {
				return &org.Applications[i]
			}
		}
	}
	return &org.Applications[0]
}

func (b *signinBackfiller) ensurePassword(ctx context.Context, baseline *legacyApplicationSignin, apps []legacyApplicationSignin) (int64, error) {
	var chosen *repository.SigninMethod
	for _, app := range apps {
		for i := range app.Methods {
			if app.Methods[i].Name != repository.SigninMethodPassword {
				continue
			}
			method := &app.Methods[i]
			if chosen == nil {
				chosen = method
			}
			if baseline != nil && app.ID == baseline.ID {
				chosen = &app.Methods[i]
			}
		}
	}
	identifiers := []string{}
	if chosen != nil {
		identifiers = append(identifiers, chosen.ResolvePasswordIdentifiers()...)
	}
	if len(identifiers) == 0 {
		seen := map[string]bool{}
		for _, app := range apps {
			for _, method := range app.Methods {
				if method.Name != repository.SigninMethodPassword {
					continue
				}
				for _, item := range method.ResolvePasswordIdentifiers() {
					if !seen[item] {
						seen[item] = true
						identifiers = append(identifiers, item)
					}
				}
			}
		}
	}
	if len(identifiers) == 0 {
		identifiers = []string{repository.PasswordIdentifierUsername, repository.PasswordIdentifierEmail}
	}
	mfaEnabled, mfaRequired, mfaIDs := b.mfaPolicy(ctx, apps, repository.SigninMethodPassword)
	if chosen == nil {
		// 组织从未配置过 Password 方法时，平台登录仍应保留密码入口；
		// 不默认启用 MFA，避免升级后把所有人挡在绑定流程里。
		mfaEnabled, mfaRequired, mfaIDs = false, false, nil
	}
	return b.ensureMethod(ctx, methodKey(repository.SigninMethodPassword, 0), repository.SigninMethodPassword,
		"Password", nil, nil, nil, 0, identifiers, mfaEnabled, mfaRequired, mfaIDs, 10)
}

func (b *signinBackfiller) ensureChannel(ctx context.Context, kind string, baseline *legacyApplicationSignin, apps []legacyApplicationSignin) (int64, error) {
	var chosen *legacyApplicationSignin
	var method *repository.SigninMethod
	for _, app := range apps {
		for i := range app.Methods {
			if app.Methods[i].Name != kind {
				continue
			}
			if baseline != nil && app.ID == baseline.ID {
				chosen = &app
				method = &app.Methods[i]
				break
			}
			if chosen == nil {
				chosen = &app
				method = &app.Methods[i]
			}
		}
		if baseline != nil && chosen != nil && chosen.ID == baseline.ID {
			break
		}
	}
	if chosen == nil || method == nil {
		return 0, nil
	}
	ttl := chosen.MagicTTL
	providerID := method.ProviderID
	templateID := method.TemplateID
	switch kind {
	case repository.SigninMethodEmailCode:
		ttl = chosen.EmailTTL
		if templateID > 0 {
			if item := b.emailTemplate(ctx, templateID); item == nil || item.ProviderID != providerID {
				b.recordConflict(ctx, chosen.ID, kind, map[string]any{"application": chosen.Code, "reason": "email provider/template mismatch"})
				return 0, nil
			}
		}
	case repository.SigninMethodSMSCode:
		ttl = chosen.SMSTTL
		if templateID > 0 {
			if item := b.smsTemplate(ctx, templateID); item == nil || item.ProviderID != providerID {
				b.recordConflict(ctx, chosen.ID, kind, map[string]any{"application": chosen.Code, "reason": "sms provider/template mismatch"})
				return 0, nil
			}
		}
	case repository.SigninMethodMagicLink:
		ttl = chosen.MagicTTL
		if templateID > 0 {
			if item := b.emailTemplate(ctx, templateID); item == nil || item.ProviderID != providerID {
				b.recordConflict(ctx, chosen.ID, kind, map[string]any{"application": chosen.Code, "reason": "email provider/template mismatch"})
				return 0, nil
			}
		}
	}
	if providerID == 0 || templateID == 0 {
		b.recordConflict(ctx, chosen.ID, kind, map[string]any{"application": chosen.Code, "reason": "provider/template is missing"})
		return 0, nil
	}
	if ttl <= 0 {
		switch kind {
		case repository.SigninMethodEmailCode, repository.SigninMethodSMSCode:
			ttl = 300
		default:
			ttl = 600
		}
	}
	mfaEnabled, mfaRequired, mfaIDs := b.mfaPolicy(ctx, apps, kind)
	display := method.DisplayName
	if display == "" {
		display = defaultBackfillDisplay(kind)
	}
	var emailTPL, smsTPL *int64
	if kind == repository.SigninMethodSMSCode {
		smsTPL = &templateID
	} else {
		emailTPL = &templateID
	}
	return b.ensureMethod(ctx, methodKey(kind, 0), kind, display, &providerID, emailTPL, smsTPL, ttl,
		nil, mfaEnabled, mfaRequired, mfaIDs, 20)
}

func (b *signinBackfiller) ensureProviders(ctx context.Context, apps []legacyApplicationSignin) (map[string]int64, error) {
	out := map[string]int64{}
	seen := map[string]bool{}
	for _, app := range apps {
		for _, item := range app.Providers {
			if !item.CanSignIn || strings.TrimSpace(item.Name) == "" || seen[item.Name] {
				continue
			}
			seen[item.Name] = true
			provider := b.provider(ctx, item.Name)
			if provider == nil {
				b.recordConflict(ctx, app.ID, "Provider", map[string]any{"application": app.Code, "provider": item.Name, "reason": "provider not found"})
				continue
			}
			mfaEnabled, mfaRequired, mfaIDs := b.providerMFAPolicy(ctx, apps, item.Name)
			id, err := b.ensureMethod(ctx, methodKey(repository.SigninMethodProvider, provider.ID), repository.SigninMethodProvider,
				firstNonEmpty(provider.DisplayName, provider.Name), &provider.ID, nil, nil, 0,
				nil, mfaEnabled, mfaRequired, mfaIDs, 30)
			if err != nil {
				return nil, err
			}
			out[item.Name] = id
		}
	}
	return out, nil
}

func (b *signinBackfiller) selectionIDs(app legacyApplicationSignin, passwordID int64, channels map[string]int64, providers map[string]int64) []int64 {
	ids := []int64{}
	if len(app.Methods) == 0 && passwordID > 0 {
		ids = append(ids, passwordID)
	}
	for _, method := range app.Methods {
		switch method.Name {
		case repository.SigninMethodPassword:
			if passwordID > 0 {
				ids = append(ids, passwordID)
			}
		case repository.SigninMethodEmailCode, repository.SigninMethodSMSCode, repository.SigninMethodMagicLink:
			if id := channels[method.Name]; id > 0 {
				ids = append(ids, id)
			}
		}
	}
	for _, item := range app.Providers {
		if !item.CanSignIn {
			continue
		}
		if id := providers[item.Name]; id > 0 {
			ids = append(ids, id)
		}
	}
	return dedupeIDs(ids)
}

func (b *signinBackfiller) mfaPolicy(ctx context.Context, apps []legacyApplicationSignin, name string) (bool, bool, []int64) {
	if !b.mfaWasAvailable {
		return false, false, nil
	}
	enabled, required := false, false
	ids := []int64{}
	for _, app := range apps {
		for _, method := range app.Methods {
			if method.Name != name {
				continue
			}
			enabled = enabled || method.MFAEnabled()
			required = required || method.MFARequired()
			ids = append(ids, method.MfaProviderIDs...)
		}
	}
	ids = b.validMFAIDs(ctx, ids)
	if enabled && len(ids) == 0 {
		if id := b.defaultTOTPID(ctx); id > 0 {
			ids = []int64{id}
		}
	}
	return enabled, required, ids
}

func (b *signinBackfiller) providerMFAPolicy(ctx context.Context, apps []legacyApplicationSignin, providerName string) (bool, bool, []int64) {
	if !b.mfaWasAvailable {
		return false, false, nil
	}
	enabled, required := false, false
	ids := []int64{}
	for _, app := range apps {
		uses := false
		for _, item := range app.Providers {
			if item.CanSignIn && item.Name == providerName {
				uses = true
				break
			}
		}
		if !uses {
			continue
		}
		for _, method := range app.Methods {
			if method.Name != repository.SigninMethodProvider {
				continue
			}
			enabled = enabled || method.MFAEnabled()
			required = required || method.MFARequired()
			ids = append(ids, method.MfaProviderIDs...)
		}
	}
	ids = b.validMFAIDs(ctx, ids)
	if enabled && len(ids) == 0 {
		if id := b.defaultTOTPID(ctx); id > 0 {
			ids = []int64{id}
		}
	}
	return enabled, required, ids
}

func (b *signinBackfiller) ensureMethod(ctx context.Context, key, name, display string,
	providerID, emailTemplateID, smsTemplateID *int64, ttl int, identifiers []string,
	mfaEnabled, mfaRequired bool, mfaIDs []int64, sortOrder int) (int64, error) {
	if id := b.methods[key]; id > 0 {
		return id, nil
	}
	var id int64
	query := `SELECT id FROM institution_signin_methods WHERE institution_id = $1 AND name = $2`
	args := []any{b.institutionID, name}
	if name == repository.SigninMethodProvider && providerID != nil {
		query += ` AND provider_id = $3`
		args = append(args, *providerID)
	}
	if err := b.tx.QueryRow(ctx, query, args...).Scan(&id); err == nil && id > 0 {
		b.methods[key] = id
		return id, nil
	}
	if sortOrder <= 0 {
		sortOrder = 30
	}
	if identifiers == nil {
		identifiers = []string{}
	}
	err := b.tx.QueryRow(ctx, `
		INSERT INTO institution_signin_methods
			(institution_id, name, display_name, identifiers, provider_id, email_template_id, sms_template_id,
			 ttl_seconds, mfa_enabled, mfa_required, sort_order, state)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'active')
		RETURNING id`,
		b.institutionID, name, display, identifiers, providerID, emailTemplateID, smsTemplateID,
		ttl, mfaEnabled, mfaRequired, sortOrder).Scan(&id)
	if err != nil {
		return 0, mapError(err, "create sign-in method")
	}
	for _, provider := range dedupeIDs(mfaIDs) {
		if _, err := b.tx.Exec(ctx, `
			INSERT INTO institution_signin_method_mfa_providers (institution_id, method_id, provider_id)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, b.institutionID, id, provider); err != nil {
			return 0, mapError(err, "")
		}
	}
	b.methods[key] = id
	return id, nil
}

func (b *signinBackfiller) ensureDefaultTOTP(ctx context.Context, org legacyInstitutionSignin) error {
	if err := b.tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM providers
			WHERE institution_id = $1 AND category = 'mfa' AND state = 'active'
		)`, org.ID).Scan(&b.mfaWasAvailable); err != nil {
		return mapError(err, "")
	}
	var id int64
	err := b.tx.QueryRow(ctx, `SELECT id FROM providers WHERE institution_id = $1 AND name = 'default-totp'`,
		org.ID).Scan(&id)
	if err == nil && id > 0 {
		b.providers["default-totp"] = &backfillProvider{ID: id, Name: "default-totp", State: "active", Category: "mfa"}
		return nil
	}
	issuer := firstNonEmpty(org.DisplayName, org.Name)
	if err := b.tx.QueryRow(ctx, `
		INSERT INTO providers (institution_id, name, display_name, category, type, state)
		VALUES ($1, 'default-totp', '虚拟 MFA', 'mfa', 'totp', 'active') RETURNING id`,
		org.ID).Scan(&id); err != nil {
		return mapError(err, "create default mfa provider")
	}
	if _, err := b.tx.Exec(ctx, `
		INSERT INTO provider_mfa (provider_id, institution_id, mfa_issuer, mfa_digits, mfa_period, mfa_algorithm, mfa_code_ttl_seconds, mfa_remember_hours)
		VALUES ($1, $2, $3, 6, 30, 'SHA1', 300, 0)`, id, org.ID, issuer); err != nil {
		return mapError(err, "create default mfa provider")
	}
	b.providers["default-totp"] = &backfillProvider{ID: id, Name: "default-totp", State: "active", Category: "mfa"}
	return nil
}

func (b *signinBackfiller) defaultTOTPID(ctx context.Context) int64 {
	if item := b.providers["default-totp"]; item != nil {
		return item.ID
	}
	var id int64
	if err := b.tx.QueryRow(ctx, `SELECT id FROM providers WHERE institution_id = $1 AND name = 'default-totp'`,
		b.institutionID).Scan(&id); err != nil {
		return 0
	}
	return id
}

func (b *signinBackfiller) validMFAIDs(ctx context.Context, ids []int64) []int64 {
	out := []int64{}
	for _, id := range dedupeIDs(ids) {
		var category, state string
		if err := b.tx.QueryRow(ctx, `SELECT category, state FROM providers WHERE id = $1 AND institution_id = $2`,
			id, b.institutionID).Scan(&category, &state); err != nil {
			continue
		}
		if strings.EqualFold(category, "mfa") && strings.EqualFold(state, "active") {
			out = append(out, id)
		}
	}
	return out
}

func (b *signinBackfiller) provider(ctx context.Context, name string) *backfillProvider {
	if item, ok := b.providers[name]; ok {
		return item
	}
	item := &backfillProvider{}
	if err := b.tx.QueryRow(ctx, `
		SELECT id, name, display_name, state, category FROM providers
		WHERE institution_id = $1 AND name = $2`, b.institutionID, name).
		Scan(&item.ID, &item.Name, &item.DisplayName, &item.State, &item.Category); err != nil {
		return nil
	}
	b.providers[name] = item
	return item
}

func (b *signinBackfiller) emailTemplate(ctx context.Context, id int64) *backfillTemplate {
	if item, ok := b.emailTPL[id]; ok {
		return item
	}
	item := &backfillTemplate{ID: id}
	if err := b.tx.QueryRow(ctx, `SELECT provider_id, scene, name FROM provider_email_templates WHERE id = $1 AND institution_id = $2`,
		id, b.institutionID).Scan(&item.ProviderID, &item.Scene, &item.Name); err != nil {
		return nil
	}
	b.emailTPL[id] = item
	return item
}

func (b *signinBackfiller) smsTemplate(ctx context.Context, id int64) *backfillTemplate {
	if item, ok := b.smsTPL[id]; ok {
		return item
	}
	item := &backfillTemplate{ID: id}
	if err := b.tx.QueryRow(ctx, `SELECT provider_id, scene, name FROM provider_sms_templates WHERE id = $1 AND institution_id = $2`,
		id, b.institutionID).Scan(&item.ProviderID, &item.Scene, &item.Name); err != nil {
		return nil
	}
	b.smsTPL[id] = item
	return item
}

func (b *signinBackfiller) recordConflict(ctx context.Context, appID int64, method string, detail map[string]any) {
	b.conflicts++
	raw, _ := json.Marshal(detail)
	_, _ = b.tx.Exec(ctx, `
		INSERT INTO signin_method_migration_conflicts (institution_id, application_id, method_name, detail)
		VALUES ($1, $2, $3, $4)`, b.institutionID, appID, method, raw)
}

func methodKey(name string, providerID int64) string {
	if name == repository.SigninMethodProvider {
		return fmt.Sprintf("Provider:%d", providerID)
	}
	return name
}

func defaultBackfillDisplay(kind string) string {
	switch kind {
	case repository.SigninMethodEmailCode:
		return "Email code"
	case repository.SigninMethodSMSCode:
		return "SMS code"
	case repository.SigninMethodMagicLink:
		return "Magic link"
	default:
		return kind
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
