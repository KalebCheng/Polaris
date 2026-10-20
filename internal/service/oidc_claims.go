package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"polaris/internal/auth/tokenhash"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

func (s *OIDCService) tokenExtra(ctx context.Context, app *repository.Application, user *repository.User) (map[string]interface{}, error) {
	// 标准模式只保留注册 claim（iss/sub/aud/exp/iat/scope），自定义字段与映射全部忽略。
	if !isCustomTokenFormat(app) {
		return map[string]interface{}{}, nil
	}
	attributes := app.TokenAttributes
	if len(attributes) == 0 {
		return map[string]interface{}{
			"username":    user.Name,
			"institution": user.InstitutionID,
		}, nil
	}
	all := tokenProfileClaims(user)
	needIDs := false
	needPaths := false
	needJobTitles := false
	for _, item := range attributes {
		if item.Category != TokenAttributeCategoryExisting {
			continue
		}
		switch item.Value {
		case "department_ids":
			needIDs = true
		case "departments":
			needPaths = true
		case "job_titles":
			needJobTitles = true
		}
	}
	if needIDs {
		if s.userDepts == nil {
			return nil, bizerrors.New(bizerrors.CodeInternalError, "department repositories are not configured")
		}
		ids, err := s.departmentIDs(ctx, user)
		if err != nil {
			return nil, err
		}
		all["department_ids"] = ids
	}
	if needPaths {
		if s.userDepts == nil || s.depts == nil {
			return nil, bizerrors.New(bizerrors.CodeInternalError, "department repositories are not configured")
		}
		paths, err := s.departmentPaths(ctx, user)
		if err != nil {
			return nil, err
		}
		all["departments"] = paths
	}
	if needJobTitles {
		if s.groups == nil {
			return nil, bizerrors.New(bizerrors.CodeInternalError, "job title repository is not configured")
		}
		titles, err := s.groups.JobTitlesForUser(ctx, user.InstitutionID, user.Name)
		if err != nil {
			return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "读取用户职务失败", err)
		}
		if titles == nil {
			titles = []string{}
		}
		all["job_titles"] = titles
	}
	out := make(map[string]interface{}, len(attributes))
	for _, item := range attributes {
		if item.Category == TokenAttributeCategoryStatic {
			if item.Type == TokenAttributeTypeArray {
				out[item.Name] = splitStaticList(item.Value)
			} else {
				out[item.Name] = item.Value
			}
			continue
		}
		if value, ok := all[item.Value]; ok {
			out[item.Name] = value
		}
	}
	return out, nil
}

func splitStaticList(value string) []string {
	items := strings.Split(value, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (s *OIDCService) idTokenClaims(ctx context.Context, app *repository.Application, user *repository.User, scope string) (map[string]interface{}, error) {
	custom := isCustomTokenFormat(app)
	claims := standardClaims(user, scope)
	if custom {
		if hasScope(scope, "profile") {
			roles, _ := s.roles.RolesForUser(ctx, user.InstitutionID, user.Name)
			if len(roles) > 0 {
				claims["roles"] = roles
			}
		}
		// groups 与部门同源：Dex 只透传 groups，NetBird 因此能按部门建组。
		// 自定义模式恒下发，不依赖 profile scope——NetBird 连接器可能只申请 openid/email。
		groups, _ := s.departmentPaths(ctx, user)
		if len(groups) > 0 {
			claims["groups"] = groups
		}
		if s.groups != nil {
			jobTitles, _ := s.groups.JobTitlesForUser(ctx, user.InstitutionID, user.Name)
			if len(jobTitles) > 0 {
				claims["job_titles"] = jobTitles
			}
		}
		claims["institution"] = user.InstitutionID
		claims["is_admin"] = user.IsAdmin
		// 应用适配器：仅自定义模式按 claimMap 覆盖/新增 claim
		applyAdapter(app, user, claims)
	}
	return claims, nil
}

// standardClaims 严格按 OIDC Core 的 scope 语义生成标准 claim。
// 自定义模式在此基础上再叠加业务 claim；标准模式只返回这里的结果。
func standardClaims(user *repository.User, scope string) map[string]interface{} {
	claims := map[string]interface{}{}
	if hasScope(scope, "profile") {
		setNonEmptyClaim(claims, "name", firstNonEmptyStr(user.DisplayName, user.Name))
		setNonEmptyClaim(claims, "given_name", user.FirstName)
		setNonEmptyClaim(claims, "family_name", user.LastName)
		setNonEmptyClaim(claims, "preferred_username", user.Name)
		setNonEmptyClaim(claims, "picture", user.Avatar)
		setNonEmptyClaim(claims, "website", user.Homepage)
		setNonEmptyClaim(claims, "gender", user.Gender)
		setNonEmptyClaim(claims, "birthdate", user.Birthday)
		setNonEmptyClaim(claims, "locale", user.Language)
		if !user.UpdatedAt.IsZero() {
			claims["updated_at"] = user.UpdatedAt.Unix()
		}
	}
	// 与 rc6 行为保持一致：只要客户端申请了 email scope，就必须下发 email claim，
	// 即使本地用户没有邮箱也下发空字符串。NetBird 内置 Dex 用 claim 是否存在判断
	// （scope 含 email 但 claim 缺失会直接拒绝登录），因此不能在这里按非空裁剪。
	if hasScope(scope, "email") {
		claims["email"] = user.Email
		claims["email_verified"] = user.EmailVerified
	}
	if hasScope(scope, "phone") && user.Phone != "" {
		claims["phone_number"] = user.Phone
		claims["phone_number_verified"] = user.PhoneVerified
	}
	if hasScope(scope, "address") {
		address := map[string]interface{}{}
		if user.Address != "" {
			address["street_address"] = user.Address
		}
		if user.Location != "" {
			address["locality"] = user.Location
		}
		if user.CountryCode != "" {
			address["country"] = user.CountryCode
		}
		if len(address) > 0 {
			claims["address"] = address
		}
	}
	return claims
}

func setNonEmptyClaim(claims map[string]interface{}, key, value string) {
	if strings.TrimSpace(value) != "" {
		claims[key] = value
	}
}

// departmentPaths 返回用户全部部门的展示路径（与 user_departments 顺序一致）。
// 部门仓库未装配时返回空集合：标准模式/未启用部门字段的调用方不应因此失败。
func (s *OIDCService) departmentPaths(ctx context.Context, user *repository.User) ([]string, error) {
	if s.userDepts == nil || s.depts == nil {
		return nil, nil
	}
	ids, err := s.departmentIDs(ctx, user)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{}, nil
	}
	labels := map[int64]string{}
	byID := map[int64]*repository.Department{}
	items, err := s.depts.List(ctx, repository.ListQuery{InstitutionID: user.InstitutionID, Limit: 10000})
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "读取部门目录失败", err)
	}
	for _, dept := range items {
		label := firstNonEmptyStr(dept.DisplayName, dept.Name)
		labels[dept.ID] = label
		byID[dept.ID] = dept
	}
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		dept := byID[id]
		if dept == nil {
			continue
		}
		// 显式沿 parent_id_chain（根→父）拼部门名称，最后追加自身。
		parts := make([]string, 0, len(dept.ParentIDChain)+1)
		for _, ancestorID := range dept.ParentIDChain {
			if label := labels[ancestorID]; label != "" {
				parts = append(parts, label)
			}
		}
		if label := labels[id]; label != "" {
			parts = append(parts, label)
		}
		if len(parts) > 0 {
			paths = append(paths, strings.Join(parts, " / "))
		}
	}
	return paths, nil
}

// departmentIDs 返回用户部门主键（唯一真相：user_departments）。
func (s *OIDCService) departmentIDs(ctx context.Context, user *repository.User) ([]int64, error) {
	if s.userDepts == nil {
		return nil, nil
	}
	ids, err := s.userDepts.DepartmentIDsForUser(ctx, user.ID)
	if err != nil {
		return nil, bizerrors.Wrap(bizerrors.CodeInternalError, "读取用户部门归属失败", err)
	}
	if ids == nil {
		ids = []int64{}
	}
	return ids, nil
}

func (s *OIDCService) storeToken(ctx context.Context, app *repository.Application, user *repository.User, name, scope, accessToken, refreshToken, idToken string, exp time.Time, grantType string) error {
	return s.storeTokenWithRefresh(ctx, app, user, name, scope, accessToken, refreshToken, idToken, exp, nil, grantType)
}

func (s *OIDCService) storeTokenWithRefresh(ctx context.Context, app *repository.Application, user *repository.User, name, scope, accessToken, refreshToken, idToken string, exp time.Time, refreshExp *time.Time, grantType string) error {
	rec := tokenRecord(app, user, name, scope, accessToken, refreshToken, idToken, "", exp, refreshExp, grantType)
	_, err := s.tokens.Create(ctx, rec)
	return err
}

func tokenRecord(app *repository.Application, user *repository.User, name, scope, accessToken, refreshToken, idToken, sessionID string, exp time.Time, refreshExp *time.Time, grantType string) *repository.Token {
	var userID *int64
	org := app.InstitutionID
	if user != nil {
		org = user.InstitutionID
		userID = &user.ID
	}
	rec := &repository.Token{
		InstitutionID: org,
		ApplicationID: &app.ID,
		UserID:        userID,
		Name:          name,
		ExpiresIn:     int(time.Until(exp).Seconds()),
		Scope:         scope,
		TokenType:     "Bearer",
		GrantType:     grantType,
		ExpireTime:    refreshExp,
		SessionID:     sessionID,
	}
	if accessToken != "" {
		rec.AccessTokenHash = tokenhash.Hash(accessToken)
	}
	if refreshToken != "" {
		rec.RefreshTokenHash = tokenhash.Hash(refreshToken)
	}
	if idToken != "" {
		rec.IDTokenHash = tokenhash.Hash(idToken)
	}
	return rec
}

// UserInfo 返回 access_token 对应的用户信息。
func (s *OIDCService) UserInfo(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	claims, err := s.issuer.VerifyToken(accessToken)
	if err != nil {
		return nil, bizerrors.ErrInvalidToken
	}
	sub, _ := claims["sub"].(string)
	username, _ := claims["username"].(string)
	// institution claim 是组织主键 id（数字）；兼容旧令牌里的 org / institutionID。
	orgID := int64(0)
	orgClaim := claims["institution"]
	if orgClaim == nil {
		orgClaim = claims["institutionID"]
	}
	if orgClaim == nil {
		orgClaim = claims["org"]
	}
	switch v := orgClaim.(type) {
	case float64:
		orgID = int64(v)
	case string:
		if v != "" {
			parsed, parseErr := strconv.ParseInt(v, 10, 64)
			if parseErr == nil {
				orgID = parsed
			} else if o, orgErr := s.orgs.GetByName(ctx, v); orgErr == nil {
				orgID = o.ID
			}
		}
	}
	var user *repository.User
	if username != "" && orgID != 0 && s.users != nil {
		user, _ = s.users.GetByName(ctx, orgID, username)
	}
	// 标准模式 access_token 不带业务 claim：回退到按 sub（UID）解析用户。
	if user == nil && sub != "" && s.users != nil {
		user, _ = s.users.GetByUID(ctx, sub)
	}
	if user == nil {
		return map[string]interface{}{"sub": sub}, nil
	}
	scope, _ := claims["scope"].(string)
	// access_token 的 aud 即 client_id：据此判定标准/自定义模式。
	custom := true
	var app *repository.Application
	if aud, _ := claims["aud"].(string); aud != "" && s.apps != nil {
		if found, appErr := s.apps.GetByClientID(ctx, aud); appErr == nil {
			app = found
			custom = isCustomTokenFormat(app)
		}
	}
	result := standardClaims(user, scope)
	result["sub"] = user.UID
	if custom {
		if hasScope(scope, "profile") {
			roles, _ := s.roles.RolesForUser(ctx, user.InstitutionID, user.Name)
			result["roles"] = roles
		}
		groups, _ := s.departmentPaths(ctx, user)
		if len(groups) > 0 {
			result["groups"] = groups
		}
		if s.groups != nil {
			jobTitles, _ := s.groups.JobTitlesForUser(ctx, user.InstitutionID, user.Name)
			if len(jobTitles) > 0 {
				result["job_titles"] = jobTitles
			}
		}
		result["institution"] = user.InstitutionID
		// 应用适配器：仅自定义模式覆盖 claim
		applyAdapter(app, user, result)
	}
	return result, nil
}

// Introspect 令牌内省。
func (s *OIDCService) Introspect(ctx context.Context, token string) map[string]interface{} {
	claims, err := s.issuer.VerifyToken(token)
	if err != nil {
		return map[string]interface{}{"active": false}
	}
	claims["active"] = true
	return claims
}

// ── 辅助 ──────────────────────────────────────────────

func appAllowsRedirect(app *repository.Application, redirectURI string) bool {
	if redirectURI == "" {
		return false
	}
	for _, r := range app.RedirectURIs {
		if r == redirectURI {
			return true
		}
		// 支持通配子路径（如 https://app.example.com/*）
		if strings.HasSuffix(r, "/*") && strings.HasPrefix(redirectURI, strings.TrimSuffix(r, "*")) {
			return true
		}
	}
	return false
}

func normalizeScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "openid profile email"
	}
	if !contains(strings.Fields(scope), "openid") {
		scope = "openid " + scope
	}
	return scope
}

// hasScope 判断以空格分隔的 scope 串中是否包含指定 scope。
func hasScope(scope, target string) bool {
	return contains(strings.Fields(scope), target)
}

// shouldIssueRefresh 判定是否签发 refresh_token：
// 应用须允许刷新（RefreshExpireInHours > 0），且请求 scope 含 offline_access。
func shouldIssueRefresh(app *repository.Application, scope string) bool {
	return app != nil && app.RefreshExpireInHours > 0 && hasScope(scope, "offline_access")
}

func contains(items []string, target string) bool {
	for _, it := range items {
		if it == target {
			return true
		}
	}
	return false
}

func verifyPKCE(challenge, method, verifier string) bool {
	if verifier == "" {
		return false
	}
	switch method {
	case "", "plain":
		return challenge == verifier
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		return challenge == base64.RawURLEncoding.EncodeToString(sum[:])
	default:
		return false
	}
}
