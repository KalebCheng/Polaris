// Package auth 提供认证与授权的核心能力：会话、JWT、密码、策略、MFA、OAuth/OIDC。
package auth

import "time"

// PermissionRule 单条有效权限规则（resource + action + effect）。
type PermissionRule struct {
	Name     string `json:"name,omitempty"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Effect   string `json:"effect"` // Allow | Deny
}

// Session 会话数据（Redis 持久化，承载完整认证信息；JWT 仅携带 SessionID）。
type Session struct {
	ID            string           `json:"id"`
	UserID        int64            `json:"user_id"`
	InstitutionID int64            `json:"institution_id"`
	Name          string           `json:"name"`
	DisplayName   string           `json:"display_name,omitempty"`
	Email         string           `json:"email,omitempty"`
	IsAdmin       bool             `json:"is_admin"`
	Roles         []string         `json:"roles"`
	Permissions   []PermissionRule `json:"permissions"`
	Application   string           `json:"application,omitempty"`
	// MfaPending 主认证已通过但 MFA 尚未完成：会话受限，只可访问 MFA 必要接口。
	MfaPending bool `json:"mfa_pending,omitempty"`
	// MfaProviderIDs 本次登录方式允许的 MFA 提供商集合。
	MfaProviderIDs []int64 `json:"mfa_provider_ids,omitempty"`
	// MfaRequired 本次挑战是否强制完成（false 时未绑定用户可跳过绑定提示）。
	MfaRequired bool `json:"mfa_required,omitempty"`
	// MfaCanSkip 仅当“尚未绑定且非强制”时为 true；已绑定用户不接受跳过。
	MfaCanSkip bool `json:"mfa_can_skip,omitempty"`
	// MfaMethod 触发 MFA 的登录方式（Password / EmailCode / SMSCode / MagicLink / Provider）。
	MfaMethod string `json:"mfa_method,omitempty"`
	// MfaRemember 本次主认证请求是否要求记住设备（仅在校验成功后签发信任令牌）。
	MfaRemember bool `json:"mfa_remember,omitempty"`
	// MfaTrustProviderID 命中旧订阅信任令牌时对应的 provider（用于免二次校验）。
	MfaTrustProviderID int64  `json:"mfa_trust_provider_id,omitempty"`
	IP                 string `json:"ip,omitempty"`
	UserAgent          string `json:"user_agent,omitempty"`
	CreatedAt          int64  `json:"created_at"`
	ExpiresAt          int64  `json:"expires_at"`
}

// AuthContext 请求级认证上下文（由中间件装配）。
type AuthContext struct {
	SessionID     string
	UserID        int64
	InstitutionID int64
	Name          string
	DisplayName   string
	Email         string
	IsAdmin       bool
	Roles         []string
	Permissions   []PermissionRule
	ExpiresAt     int64
	// MfaPending 见 Session.MfaPending
	MfaPending         bool
	MfaProviderIDs     []int64
	MfaRequired        bool
	MfaCanSkip         bool
	MfaMethod          string
	MfaRemember        bool
	MfaTrustProviderID int64
}

// IsAuthenticated 是否已认证
func (a *AuthContext) IsAuthenticated() bool { return a != nil && a.UserID > 0 }

// HasRole 判断是否拥有指定角色
func (a *AuthContext) HasRole(name string) bool {
	for _, r := range a.Roles {
		if r == name {
			return true
		}
	}
	return false
}

// Enforce 判断是否允许对资源执行动作。管理员直通；Deny 优先于 Allow；支持 * 通配。
func (a *AuthContext) Enforce(resource, action string) bool {
	if a == nil {
		return false
	}
	if a.IsAdmin {
		return true
	}
	allowed := false
	for _, p := range a.Permissions {
		if p.Resource != "*" && p.Resource != resource {
			continue
		}
		if p.Action != "*" && p.Action != action {
			continue
		}
		if p.Effect == "Deny" {
			return false
		}
		allowed = true
	}
	return allowed
}

// LoginRequest 密码登录请求。
// Institution 可选：未指定时由后端自动解析（唯一组织 / 默认组织 / 账号唯一命中），
// 只有账号在多个组织都存在时才要求显式指定（返回 ORGANIZATION_REQUIRED）。
type LoginRequest struct {
	// Institution 组织标识（组织 name）：兼容旧客户端与既有链接
	Institution string `json:"institution"`
	// InstitutionID 组织主键：公开面的**权威引用**（与全仓"引用一律用主键 id"一致）
	InstitutionID int64  `json:"institution_id"`
	Application   string `json:"application"`
	// ClientID 应用驱动入口的**标准应用上下文**（登录页 URL 的 client_id），优先于 Application。
	// 两者都为空 = "登录平台"，不做应用准入判定（见 Spec 第六阶段 FR-3 判定作用域）。
	ClientID string `json:"client_id"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	// IdentifierType 密码登录标识类型：username | email | phone；留空时按应用允许集合自动匹配。
	IdentifierType string `json:"identifier_type"`
	MfaCode        string `json:"mfa_code"`
	From           string `json:"from"`
	// RememberMfa 本次二次校验通过后是否「记住此设备」（组织配置了记住时长时生效）
	RememberMfa bool `json:"remember_mfa"`
	// MfaTrust 已有的「信任设备」令牌（由 handler 从 Cookie 注入，非请求体字段）
	MfaTrust string `json:"-"`

	// 人机校验（组织启用验证码提供商时必填）
	CaptchaID    string `json:"captcha_id"`
	CaptchaCode  string `json:"captcha_code"`
	CaptchaToken string `json:"captcha_token"`
}

// LoginSession 登录会话元信息
type LoginSession struct {
	ExpiresAt int64  `json:"expires_at"`
	From      string `json:"from,omitempty"`
}

// LoginResponse 登录响应（Cookie 模式，data 不含 token 明文）
type LoginResponse struct {
	Session        *LoginSession `json:"session"`
	User           *UserView     `json:"user"`
	Roles          []string      `json:"roles,omitempty"`
	Mfa            *MfaChallenge `json:"mfa,omitempty"`
	IdentityLinked bool          `json:"identity_linked,omitempty"`

	// MfaTrustToken / MfaTrustMaxAge：内部字段——登录通过且需「记住此设备」时，
	// 由 handler 读取并下发可信 Cookie，不进入响应体。
	MfaTrustToken  string `json:"-"`
	MfaTrustMaxAge int    `json:"-"`
}

// MfaChallenge MFA 二次校验挑战
type MfaChallenge struct {
	// Required 是否必须完成 MFA（false 表示“提示绑定，可跳过”）。
	Required bool   `json:"required"`
	Type     string `json:"type"` // totp | sms | select
	// Enroll 用户当前没有允许集合内的已绑定凭证：前端应引导绑定。
	Enroll bool `json:"enroll,omitempty"`
	// CanSkip 未绑定用户是否可跳过绑定提示。
	CanSkip bool `json:"can_skip,omitempty"`
	// RememberHours 当前允许的「记住此设备」最大时长（小时）；0 表示不支持。
	RememberHours int `json:"remember_hours,omitempty"`
}

// UserView 登录态用户信息（不含敏感字段）
type UserView struct {
	InstitutionID int64  `json:"institution_id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Avatar        string `json:"avatar"`
	IsAdmin       bool   `json:"is_admin"`
	MfaEnabled    bool   `json:"mfa_enabled"`
}

// AuthStatus 认证状态响应
type AuthStatus struct {
	Authenticated bool             `json:"authenticated"`
	MfaPending    bool             `json:"mfa_pending,omitempty"`
	InstitutionID int64            `json:"institution_id,omitempty"`
	Name          string           `json:"name,omitempty"`
	DisplayName   string           `json:"display_name,omitempty"`
	Email         string           `json:"email,omitempty"`
	IsAdmin       bool             `json:"is_admin"`
	Roles         []string         `json:"roles,omitempty"`
	Permissions   []PermissionRule `json:"permissions,omitempty"`
	ExpiresAt     int64            `json:"expires_at,omitempty"`
	// InstitutionName 组织展示名（顶栏账号菜单与个人资料页展示用；不直接用组织主键）
	InstitutionName    string `json:"institution_name,omitempty"`
	InstitutionLogo    string `json:"institution_logo,omitempty"`
	InstitutionFavicon string `json:"institution_favicon,omitempty"`
	// Avatar 用户头像地址（可能为空；前端回退到姓名首字母）
	Avatar string `json:"avatar,omitempty"`
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	// OldPassword 仅在账号已设置密码时必填；首次设密（如第三方登录自动建号）可省略。
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ResetPasswordRequest 管理员重置密码请求
type ResetPasswordRequest struct {
	NewPassword string `json:"new_password" binding:"required"`
}

// OAuthProviderInfo 外部身份提供方信息（登录页用）
type OAuthProviderInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	// Type 提供商类型（dingtalk / wechat / feishu / lark / oidc）：
	// 登录页据此渲染对应的**品牌 logo**（此前只给名称，登录页只能用文字按钮）。
	Type string `json:"type"`
	// Icon 图标 key（当前与 Type 同值；由提供商目录声明，前端按 key 选素材）
	Icon string `json:"icon"`
}

// nowUnix 当前 Unix 时间戳（便于测试替换）
var nowUnix = func() int64 { return time.Now().Unix() }

// nowTime 返回当前时间（便于测试替换）
var nowTime = func() time.Time { return time.Now() }
