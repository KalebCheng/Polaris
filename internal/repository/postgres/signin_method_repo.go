package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

const institutionSigninMethodTable = "institution_signin_methods"

type queryer interface {
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
}

// InstitutionSigninMethodRepository 组织级登录方式仓库（PostgreSQL 实现）。
type InstitutionSigninMethodRepository struct{ db *dbclient.Client }

// NewInstitutionSigninMethodRepository 创建组织登录方式仓库。
func NewInstitutionSigninMethodRepository(db *dbclient.Client) *InstitutionSigninMethodRepository {
	return &InstitutionSigninMethodRepository{db: db}
}

// Create 创建组织方法并写入 MFA 关联（单事务）。
func (r *InstitutionSigninMethodRepository) Create(ctx context.Context, method *repository.InstitutionSigninMethod) (int64, error) {
	var id int64
	err := r.db.WithTx(ctx, func(tx pgx.Tx) error {
		next, err := insertReturningID(ctx, tx, institutionSigninMethodTable, method)
		if err != nil {
			return err
		}
		id = next
		method.ID = next
		return replaceMFAProvidersTx(ctx, tx, method.InstitutionID, next, method.MfaProviderIDs)
	})
	return id, err
}

// GetByID 读取组织方法并装配 MFA/提供商/模板摘要。
func (r *InstitutionSigninMethodRepository) GetByID(ctx context.Context, institutionID, id int64) (*repository.InstitutionSigninMethod, error) {
	method := &repository.InstitutionSigninMethod{}
	if err := selectOne(ctx, r.db, institutionSigninMethodTable, method,
		"WHERE id = $1 AND institution_id = $2", id, institutionID); err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, []*repository.InstitutionSigninMethod{method}); err != nil {
		return nil, err
	}
	return method, nil
}

// List 列出组织全部方法。
func (r *InstitutionSigninMethodRepository) List(ctx context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	items, err := queryEntities[repository.InstitutionSigninMethod](ctx, r.db, institutionSigninMethodTable,
		"WHERE institution_id = $1", []interface{}{institutionID}, "sort_order ASC, id ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListActive 列出组织启用方法。
func (r *InstitutionSigninMethodRepository) ListActive(ctx context.Context, institutionID int64) ([]*repository.InstitutionSigninMethod, error) {
	items, err := queryEntities[repository.InstitutionSigninMethod](ctx, r.db, institutionSigninMethodTable,
		"WHERE institution_id = $1 AND state = 'active'", []interface{}{institutionID}, "sort_order ASC, id ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListByIDs 按主键批量读取组织方法。
func (r *InstitutionSigninMethodRepository) ListByIDs(ctx context.Context, institutionID int64, ids []int64) ([]*repository.InstitutionSigninMethod, error) {
	if len(ids) == 0 {
		return []*repository.InstitutionSigninMethod{}, nil
	}
	items, err := queryEntities[repository.InstitutionSigninMethod](ctx, r.db, institutionSigninMethodTable,
		"WHERE institution_id = $1 AND id = ANY($2)", []interface{}{institutionID, ids}, "sort_order ASC, id ASC", 0, 0)
	if err != nil {
		return nil, err
	}
	if err := r.enrich(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// ListByApplication 列出应用选中的组织方法（含 disabled，调用方按需过滤）。
func (r *InstitutionSigninMethodRepository) ListByApplication(ctx context.Context, institutionID, applicationID int64) ([]*repository.InstitutionSigninMethod, error) {
	ids, err := r.applicationMethodIDs(ctx, institutionID, applicationID)
	if err != nil {
		return nil, err
	}
	return r.ListByIDs(ctx, institutionID, ids)
}

// ListByApplications 批量列出多个应用的选择，返回 appID → methods。
func (r *InstitutionSigninMethodRepository) ListByApplications(ctx context.Context, institutionID int64, applicationIDs []int64) (map[int64][]*repository.InstitutionSigninMethod, error) {
	out := map[int64][]*repository.InstitutionSigninMethod{}
	if len(applicationIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT application_id, method_id
		FROM application_signin_methods
		WHERE institution_id = $1 AND application_id = ANY($2)
		ORDER BY application_id, method_id`, institutionID, applicationIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()

	appMethods := map[int64][]int64{}
	allIDs := make([]int64, 0)
	seen := map[int64]bool{}
	for rows.Next() {
		var appID, methodID int64
		if err := rows.Scan(&appID, &methodID); err != nil {
			return nil, mapError(err, "")
		}
		appMethods[appID] = append(appMethods[appID], methodID)
		if !seen[methodID] {
			seen[methodID] = true
			allIDs = append(allIDs, methodID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, mapError(err, "")
	}
	methods, err := r.ListByIDs(ctx, institutionID, allIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*repository.InstitutionSigninMethod, len(methods))
	for _, method := range methods {
		byID[method.ID] = method
	}
	for appID, ids := range appMethods {
		for _, id := range ids {
			if method := byID[id]; method != nil {
				out[appID] = append(out[appID], method)
			}
		}
	}
	return out, nil
}

// Update 更新组织方法与 MFA 关联（单事务）。
func (r *InstitutionSigninMethodRepository) Update(ctx context.Context, method *repository.InstitutionSigninMethod) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		if err := updateEntityTx(ctx, tx, institutionSigninMethodTable, method, "institution_id", "name"); err != nil {
			return err
		}
		return replaceMFAProvidersTx(ctx, tx, method.InstitutionID, method.ID, method.MfaProviderIDs)
	})
}

// SetState 启停组织方法。
func (r *InstitutionSigninMethodRepository) SetState(ctx context.Context, institutionID, id int64, state string) error {
	return mapError(r.db.Exec(ctx, `
		UPDATE institution_signin_methods
		SET state = $1, updated_at = NOW()
		WHERE id = $2 AND institution_id = $3`, state, id, institutionID), "sign-in method not found")
}

// Delete 删除组织方法（应用选择由外键 RESTRICT 保护）。
func (r *InstitutionSigninMethodRepository) Delete(ctx context.Context, institutionID, id int64) error {
	return mapError(r.db.Exec(ctx,
		`DELETE FROM institution_signin_methods WHERE id = $1 AND institution_id = $2`,
		id, institutionID), "sign-in method not found")
}

// ReplaceApplicationSelection 整体替换应用选择（单事务）。
func (r *InstitutionSigninMethodRepository) ReplaceApplicationSelection(ctx context.Context, institutionID, applicationID int64, methodIDs []int64) error {
	return r.db.WithTx(ctx, func(tx pgx.Tx) error {
		return replaceApplicationSelectionTx(ctx, tx, institutionID, applicationID, methodIDs)
	})
}

// CountApplications 统计引用该方法的应用数。
func (r *InstitutionSigninMethodRepository) CountApplications(ctx context.Context, institutionID, methodID int64) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM application_signin_methods
		WHERE institution_id = $1 AND method_id = $2`, institutionID, methodID).Scan(&count)
	return count, mapError(err, "")
}

// CountApplicationsByMethod 批量统计方法引用数。
func (r *InstitutionSigninMethodRepository) CountApplicationsByMethod(ctx context.Context, institutionID int64) (map[int64]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT method_id, COUNT(*) FROM application_signin_methods
		WHERE institution_id = $1 GROUP BY method_id`, institutionID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var methodID int64
		var count int
		if err := rows.Scan(&methodID, &count); err != nil {
			return nil, mapError(err, "")
		}
		out[methodID] = count
	}
	return out, mapError(rows.Err(), "")
}

// ListApplicationsByMethod 列出引用该方法的应（用于冲突详情）。
func (r *InstitutionSigninMethodRepository) ListApplicationsByMethod(ctx context.Context, institutionID, methodID int64) ([]*repository.Application, error) {
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.code, a.name
		FROM applications a
		JOIN application_signin_methods sm ON sm.application_id = a.id
		WHERE sm.institution_id = $1 AND sm.method_id = $2
		ORDER BY a.code`, institutionID, methodID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var out []*repository.Application
	for rows.Next() {
		app := &repository.Application{}
		if err := rows.Scan(&app.ID, &app.Code, &app.Name); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, app)
	}
	return out, mapError(rows.Err(), "")
}

func (r *InstitutionSigninMethodRepository) applicationMethodIDs(ctx context.Context, institutionID, applicationID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT method_id FROM application_signin_methods
		WHERE institution_id = $1 AND application_id = $2`, institutionID, applicationID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		ids = append(ids, id)
	}
	return ids, mapError(rows.Err(), "")
}

func (r *InstitutionSigninMethodRepository) enrich(ctx context.Context, methods []*repository.InstitutionSigninMethod) error {
	if len(methods) == 0 {
		return nil
	}
	methodIDs := make([]int64, 0, len(methods))
	providerIDs := map[int64]bool{}
	emailTemplateIDs := map[int64]bool{}
	smsTemplateIDs := map[int64]bool{}
	for _, method := range methods {
		methodIDs = append(methodIDs, method.ID)
		if method.ProviderID != nil {
			providerIDs[*method.ProviderID] = true
		}
		if method.EmailTemplateID != nil {
			emailTemplateIDs[*method.EmailTemplateID] = true
		}
		if method.SMSTemplateID != nil {
			smsTemplateIDs[*method.SMSTemplateID] = true
		}
	}
	mfa, err := loadMFAProviderIDs(ctx, r.db, methodIDs)
	if err != nil {
		return err
	}
	providers, err := r.providerInfo(ctx, keys(providerIDs))
	if err != nil {
		return err
	}
	emailTemplates, err := r.templateNames(ctx, "email", keys(emailTemplateIDs))
	if err != nil {
		return err
	}
	smsTemplates, err := r.templateNames(ctx, "sms", keys(smsTemplateIDs))
	if err != nil {
		return err
	}
	counts, err := r.CountApplicationsByMethod(ctx, methods[0].InstitutionID)
	if err != nil {
		return err
	}
	for _, method := range methods {
		method.MfaProviderIDs = mfa[method.ID]
		if method.MfaProviderIDs == nil {
			method.MfaProviderIDs = []int64{}
		}
		if method.ProviderID != nil {
			if info := providers[*method.ProviderID]; info != nil {
				method.ProviderName = info.name
				method.ProviderDisplayName = info.displayName
				method.ProviderState = info.state
			}
		}
		if method.EmailTemplateID != nil {
			method.TemplateName = emailTemplates[*method.EmailTemplateID]
		}
		if method.SMSTemplateID != nil {
			method.TemplateName = smsTemplates[*method.SMSTemplateID]
		}
		method.ApplicationCount = counts[method.ID]
	}
	return nil
}

type providerSummary struct {
	name        string
	displayName string
	state       string
}

func (r *InstitutionSigninMethodRepository) providerInfo(ctx context.Context, ids []int64) (map[int64]*providerSummary, error) {
	out := map[int64]*providerSummary{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, name, display_name, state FROM providers WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var info providerSummary
		if err := rows.Scan(&id, &info.name, &info.displayName, &info.state); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = &info
	}
	return out, mapError(rows.Err(), "")
}

func (r *InstitutionSigninMethodRepository) templateNames(ctx context.Context, channel string, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	table := "provider_email_templates"
	if channel == "sms" {
		table = "provider_sms_templates"
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, name FROM `+table+` WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, mapError(err, "")
		}
		out[id] = name
	}
	return out, mapError(rows.Err(), "")
}

func keys(set map[int64]bool) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func loadMFAProviderIDs(ctx context.Context, q queryer, methodIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(methodIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx,
		`SELECT method_id, provider_id FROM institution_signin_method_mfa_providers
		 WHERE method_id = ANY($1) ORDER BY method_id, provider_id`, methodIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	for rows.Next() {
		var methodID, providerID int64
		if err := rows.Scan(&methodID, &providerID); err != nil {
			return nil, mapError(err, "")
		}
		out[methodID] = append(out[methodID], providerID)
	}
	return out, mapError(rows.Err(), "")
}

func replaceMFAProvidersTx(ctx context.Context, tx pgx.Tx, institutionID, methodID int64, providerIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM institution_signin_method_mfa_providers WHERE method_id = $1`, methodID); err != nil {
		return mapError(err, "")
	}
	for _, providerID := range dedupeIDs(providerIDs) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO institution_signin_method_mfa_providers (institution_id, method_id, provider_id)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, institutionID, methodID, providerID); err != nil {
			return mapError(err, "")
		}
	}
	return nil
}

func replaceApplicationSelectionTx(ctx context.Context, tx pgx.Tx, institutionID, applicationID int64, methodIDs []int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM application_signin_methods WHERE application_id = $1`, applicationID); err != nil {
		return mapError(err, "")
	}
	for _, methodID := range dedupeIDs(methodIDs) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO application_signin_methods (institution_id, application_id, method_id)
			VALUES ($1, $2, $3)`, institutionID, applicationID, methodID); err != nil {
			return mapError(err, "")
		}
	}
	return nil
}

func dedupeIDs(values []int64) []int64 {
	out := make([]int64, 0, len(values))
	seen := map[int64]bool{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

var _ repository.InstitutionSigninMethodRepository = (*InstitutionSigninMethodRepository)(nil)
