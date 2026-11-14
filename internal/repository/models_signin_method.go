package repository

import "time"

// InstitutionSigninMethod 组织级登录方式定义。
//
// 定义只存一份；应用通过 application_signin_methods 选择，不再保存任何参数。
type InstitutionSigninMethod struct {
	ID            int64    `db:"id" json:"id"`
	InstitutionID int64    `db:"institution_id" json:"institution_id"`
	Name          string   `db:"name" json:"name"`
	DisplayName   string   `db:"display_name" json:"display_name"`
	Identifiers   []string `db:"identifiers" json:"identifiers"`

	ProviderID      *int64 `db:"provider_id" json:"provider_id,omitempty"`
	EmailTemplateID *int64 `db:"email_template_id" json:"email_template_id,omitempty"`
	SMSTemplateID   *int64 `db:"sms_template_id" json:"sms_template_id,omitempty"`
	TTLSeconds      int    `db:"ttl_seconds" json:"ttl_seconds"`

	MfaEnabled  bool   `db:"mfa_enabled" json:"mfa_enabled"`
	MfaRequired bool   `db:"mfa_required" json:"mfa_required"`
	SortOrder   int    `db:"sort_order" json:"sort_order"`
	State       string `db:"state" json:"state"`

	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`

	// 关联装配字段（非持久化）。
	MfaProviderIDs      []int64 `db:"-" json:"mfa_provider_ids"`
	ProviderName        string  `db:"-" json:"provider_name,omitempty"`
	ProviderDisplayName string  `db:"-" json:"provider_display_name,omitempty"`
	ProviderState       string  `db:"-" json:"provider_state,omitempty"`
	TemplateName        string  `db:"-" json:"template_name,omitempty"`
	ApplicationCount    int     `db:"-" json:"application_count"`
}

// IsActive 返回组织方法是否启用。
func (m *InstitutionSigninMethod) IsActive() bool {
	return m != nil && m.State == "active"
}

// EffectiveSigninMethod 把组织方法转换为登录页/认证链路使用的 DTO。
func EffectiveSigninMethod(m *InstitutionSigninMethod) SigninMethod {
	if m == nil {
		return SigninMethod{}
	}
	out := SigninMethod{
		ID:          m.ID,
		Name:        m.Name,
		DisplayName: m.DisplayName,
		Rule:        "All",
		Identifiers: append([]string(nil), m.Identifiers...),
		TTLSeconds:  m.TTLSeconds,
		State:       m.State,
		MfaEnabled:  m.MfaEnabled,
		MfaRequired: m.MfaRequired,
	}
	if m.ProviderID != nil {
		out.ProviderID = *m.ProviderID
	}
	switch m.Name {
	case SigninMethodEmailCode, SigninMethodMagicLink:
		if m.EmailTemplateID != nil {
			out.TemplateID = *m.EmailTemplateID
		}
	case SigninMethodSMSCode:
		if m.SMSTemplateID != nil {
			out.TemplateID = *m.SMSTemplateID
		}
	}
	out.MfaProviderIDs = append([]int64(nil), m.MfaProviderIDs...)
	out.MarkMFAExplicit()
	if out.Name == SigninMethodPassword {
		out.Identifiers = out.ResolvePasswordIdentifiers()
	}
	return out
}
