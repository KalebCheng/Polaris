package service

import (
	"strings"
	"time"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

func validateAppScopes(app *repository.Application, requested string) error {
	if app == nil || len(app.Scopes) == 0 {
		return nil // 应用未限定 scope 时不校验
	}
	allowed := map[string]bool{"openid": true, "offline_access": true}
	for _, scope := range app.Scopes {
		allowed[scope.Name] = true
	}
	for _, scope := range strings.Fields(requested) {
		if !allowed[scope] {
			return bizerrors.New(bizerrors.CodeOAuthInvalidScope, "scope is not granted to the application").
				WithDetail("scope", scope)
		}
	}
	return nil
}

// tokenFieldCatalog access_token 可内嵌的用户字段白名单（唯一声明处）。
//
// 字段名与 users 实体 / 用户 DTO 对齐；`name` 是早期版本留下的兼容别名（= display_name）。
// 部门字段是数组：一个人可能同时在多个部门挂职（user_departments 多值关系）。
const (
	TokenFormatStandard = "standard"
	TokenFormatCustom   = "custom"

	TokenAttributeCategoryExisting = "existing"
	TokenAttributeCategoryStatic   = "static"
	TokenAttributeTypeString       = "string"
	TokenAttributeTypeArray        = "array"
)

// isCustomTokenFormat 判断应用是否使用自定义 claim 模式。
// 仅显式 standard 走标准模式；历史值（jwt / 空）继续按自定义处理，避免升级后静默变更。
func isCustomTokenFormat(app *repository.Application) bool {
	return app == nil || app.TokenFormat != TokenFormatStandard
}

var tokenFieldCatalog = map[string]struct{}{
	"username":       {},
	"uid":            {},
	"name":           {}, // 兼容别名：display_name
	"display_name":   {},
	"email":          {},
	"email_verified": {},
	"phone":          {},
	"phone_verified": {},
	"institution":    {},
	"department_ids": {},
	"departments":    {},
	"job_titles":     {},
	"title":          {},
	"is_admin":       {},
	"type":           {},
}

// tokenProfileClaims 把用户实体映射为可选 claim 全集（不查库的标量字段）。
func tokenProfileClaims(user *repository.User) map[string]interface{} {
	if user == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"username":       user.Name,
		"uid":            user.UID,
		"name":           user.DisplayName,
		"display_name":   user.DisplayName,
		"email":          user.Email,
		"email_verified": user.EmailVerified,
		"phone":          user.Phone,
		"phone_verified": user.PhoneVerified,
		"institution":    user.InstitutionID,
		"title":          user.Title,
		"is_admin":       user.IsAdmin,
		"type":           user.Type,
	}
}

// normalizeTokenAttributes 校验并规范化 access_token 的自定义 claim 映射。
// 类别 existing：值必须来自 tokenFieldCatalog；类别 static：值原样写入，按 Type 转成字符串或数组。
func normalizeTokenAttributes(attributes []repository.TokenAttribute) ([]repository.TokenAttribute, error) {
	out := make([]repository.TokenAttribute, 0, len(attributes))
	seen := map[string]bool{}
	for _, item := range attributes {
		item.Name = strings.TrimSpace(item.Name)
		item.Category = strings.TrimSpace(item.Category)
		item.Value = strings.TrimSpace(item.Value)
		item.Type = strings.TrimSpace(item.Type)
		if item.Name == "" && item.Value == "" {
			continue
		}
		if item.Name == "" || !validClaimName(item.Name) {
			return nil, bizerrors.New(bizerrors.CodeInvalidParams, "invalid access token claim name").
				WithDetail("name", item.Name)
		}
		if seen[item.Name] {
			return nil, bizerrors.New(bizerrors.CodeInvalidParams, "duplicate access token claim").
				WithDetail("claim", item.Name)
		}
		seen[item.Name] = true

		if item.Category == "" {
			item.Category = TokenAttributeCategoryExisting
		}
		switch item.Category {
		case TokenAttributeCategoryExisting:
			if _, ok := tokenFieldCatalog[item.Value]; !ok {
				return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported access token field").
					WithDetail("field", item.Value)
			}
			// existing 字段沿用原生类型，Type 仅用于 UI 展示。
			if tokenFieldIsArray(item.Value) {
				item.Type = TokenAttributeTypeArray
			} else {
				item.Type = TokenAttributeTypeString
			}
		case TokenAttributeCategoryStatic:
			if item.Value == "" {
				return nil, bizerrors.New(bizerrors.CodeInvalidParams, "static token attribute requires a value").
					WithDetail("claim", item.Name)
			}
			if item.Type == "" {
				item.Type = TokenAttributeTypeString
			}
			if item.Type != TokenAttributeTypeString && item.Type != TokenAttributeTypeArray {
				return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported token attribute type").
					WithDetail("type", item.Type)
			}
		default:
			return nil, bizerrors.New(bizerrors.CodeInvalidParams, "unsupported token attribute category").
				WithDetail("category", item.Category)
		}
		out = append(out, item)
	}
	return out, nil
}

func tokenFieldIsArray(field string) bool {
	switch field {
	case "department_ids", "departments", "job_titles":
		return true
	default:
		return false
	}
}

// validClaimName 限制 claim 名为常见 JWT 名称字符，避免控制字符/空格进入 token。
func validClaimName(name string) bool {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}

// accessTokenTTL 解析应用级 access_token 有效期。
func accessTokenTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.ExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.ExpireInHours * float64(time.Hour))
}

// refreshTokenTTL 解析应用级 refresh_token 有效期。
func refreshTokenTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.RefreshExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.RefreshExpireInHours * float64(time.Hour))
}

// cookieTTL 解析应用级会话 Cookie 有效期；未配置时回退到全局会话 TTL。
func cookieTTL(app *repository.Application, fallback time.Duration) time.Duration {
	if app == nil || app.CookieExpireInHours <= 0 {
		return fallback
	}
	return time.Duration(app.CookieExpireInHours) * time.Hour
}

// ── 应用适配器（P4，D6）────────────────────────────────────────

// userAttrNames 自定义适配器可映射的用户属性白名单（本地属性 → 取值）。
// 未收录的属性名一律忽略，避免任意取值/越权暴露。
var userAttrNames = map[string]func(u *repository.User) interface{}{
	"name":           func(u *repository.User) interface{} { return u.Name },
	"username":       func(u *repository.User) interface{} { return u.Name },
	"uid":            func(u *repository.User) interface{} { return u.UID },
	"display_name":   func(u *repository.User) interface{} { return firstNonEmptyStr(u.DisplayName, u.Name) },
	"email":          func(u *repository.User) interface{} { return u.Email },
	"email_verified": func(u *repository.User) interface{} { return u.EmailVerified },
	"phone":          func(u *repository.User) interface{} { return u.Phone },
	"phone_verified": func(u *repository.User) interface{} { return u.PhoneVerified },
	"avatar":         func(u *repository.User) interface{} { return u.Avatar },
	"picture":        func(u *repository.User) interface{} { return u.Avatar },
	"language":       func(u *repository.User) interface{} { return u.Language },
	"locale":         func(u *repository.User) interface{} { return u.Language },
	"institutionID":  func(u *repository.User) interface{} { return u.InstitutionID },
	"institution":    func(u *repository.User) interface{} { return u.InstitutionID },
	// org 仅保留为适配器 claimMap 的历史属性名，不再作为 token 字段对外提供。
	"org":                func(u *repository.User) interface{} { return u.InstitutionID },
	"is_admin":           func(u *repository.User) interface{} { return u.IsAdmin },
	"type":               func(u *repository.User) interface{} { return u.Type },
	"first_name":         func(u *repository.User) interface{} { return u.FirstName },
	"last_name":          func(u *repository.User) interface{} { return u.LastName },
	"title":              func(u *repository.User) interface{} { return u.Title },
	"affiliation":        func(u *repository.User) interface{} { return u.Affiliation },
	"homepage":           func(u *repository.User) interface{} { return u.Homepage },
	"location":           func(u *repository.User) interface{} { return u.Location },
	"address":            func(u *repository.User) interface{} { return u.Address },
	"country_code":       func(u *repository.User) interface{} { return u.CountryCode },
	"gender":             func(u *repository.User) interface{} { return u.Gender },
	"birthday":           func(u *repository.User) interface{} { return u.Birthday },
	"education":          func(u *repository.User) interface{} { return u.Education },
	"bio":                func(u *repository.User) interface{} { return u.Bio },
	"signup_application": func(u *repository.User) interface{} { return u.SignupApplication },
}

// resolveUserAttr 按属性名取用户值；未知属性返回 (nil,false)。
func resolveUserAttr(u *repository.User, attr string) (interface{}, bool) {
	getter, ok := userAttrNames[attr]
	if !ok {
		return nil, false
	}
	return getter(u), true
}

// applyAdapter 依据应用的适配器配置覆盖 claims：
//   - adapter != "custom" 或无 claimMap：保持原样（标准 OIDC claims，default 行为）。
//   - custom：对 claimMap 中每个 <claim> → <userAttr>，用用户属性值覆盖/新增该 claim；
//     空值（零值字符串 / false）不写入，避免用空值覆盖默认 claim；未知属性忽略。
//
// 生效点：id_token 与 userinfo 的签发路径（配置即生效）。
func applyAdapter(app *repository.Application, user *repository.User, claims map[string]interface{}) {
	if app == nil || user == nil || app.Adapter != "custom" || app.AdapterConfig == nil {
		return
	}
	for claim, attr := range app.AdapterConfig.ClaimMap {
		claim = strings.TrimSpace(claim)
		attr = strings.TrimSpace(attr)
		if claim == "" || attr == "" {
			continue
		}
		value, ok := resolveUserAttr(user, attr)
		if !ok || isEmptyClaimValue(value) {
			continue
		}
		claims[claim] = value
	}
}

// isEmptyClaimValue 判定是否为空值（不覆盖既有 claim）。
func isEmptyClaimValue(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case bool:
		return !t
	default:
		return false
	}
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func orEmptyItems(items []repository.ProviderItem) []repository.ProviderItem {
	if items == nil {
		return []repository.ProviderItem{}
	}
	return items
}

func orEmptySignupItems(items []repository.SignupItem) []repository.SignupItem {
	if items == nil {
		return []repository.SignupItem{}
	}
	return items
}
