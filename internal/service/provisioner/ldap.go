package provisioner

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/go-ldap/ldap/v3"

	"polaris/internal/service/syncer"
)

// LDAPProvisioner 目录服务（OpenLDAP / AD）下游连接器：在目录里创建与维护账号。
//
// 这是目前唯一**支持设置口令**的下游类型（目录本身就是凭证权威），因此口令下发链路
// 在这一条上真正生效（`has_password` 也才为真）。
//
// 配置项（与入方向同名，便于管理员理解）：
//
//	host / port / ssl_mode    连接与加密（明文 / starttls / ldaps）
//	user / password           绑定 DN 与口令（口令为密文，服务层已解密）
//	base_dn                   账号创建位置（如 OU=Users,DC=example,DC=com）
//	rdn_attr                  账号 RDN 属性（OpenLDAP 默认 uid；AD 建议 cn）
//	directory_type            openldap（默认）| ad —— 决定属性集与是否支持禁用
//
// 能力差异：AD 用 userAccountControl 的 ACCOUNTDISABLE 位实现"禁用/启用"；
// OpenLDAP 没有等价的停用语义，故 **AD 支持停用、OpenLDAP 不支持**（由能力声明显式区分）。
type LDAPProvisioner struct{}

// Type 连接器类型标识（与入方向一致）
func (l *LDAPProvisioner) Type() string { return "ldap" }

// AD 的 userAccountControl 常量
const (
	adUserAccountControlNormal  = 512 // NORMAL_ACCOUNT
	adUserAccountControlDisable = 514 // NORMAL_ACCOUNT | ACCOUNTDISABLE
)

// isAD 是否 AD 目录
func isAD(cfg Config) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.Value("directory_type")), "ad")
}

// Capabilities：AD 支持停用（userAccountControl），OpenLDAP 不支持；两者都支持口令与增删改。
func (l *LDAPProvisioner) Capabilities(cfg Config) Capabilities {
	return Capabilities{
		Password: true,
		Update:   true,
		Disable:  isAD(cfg),
		Delete:   true,
	}
}

// connect 建立并绑定连接（复用入方向的 TLS 语义：StartTLS 一定先于 Bind）
func (l *LDAPProvisioner) connect(ctx context.Context, cfg Config) (*ldap.Conn, error) {
	host := cfg.Value("host")
	if host == "" || cfg.Value("base_dn") == "" {
		return nil, fmt.Errorf("目录连接器缺少 host / base_dn")
	}
	port, _ := strconv.Atoi(cfg.Value("port"))
	return syncer.DialLDAP(ctx, host, port,
		cfg.Value("user"), cfg.Value("password"), cfg.Value("ssl_mode"))
}

// userDN 组装账号 DN：<rdn_attr>=<username>,<base_dn>
func userDN(cfg Config, username string) string {
	attr := strings.TrimSpace(cfg.Value("rdn_attr"))
	if attr == "" {
		attr = "uid"
	}
	if isAD(cfg) && strings.TrimSpace(cfg.Value("rdn_attr")) == "" {
		attr = "cn"
	}
	return fmt.Sprintf("%s=%s,%s", attr, ldap.EscapeDN(username), strings.TrimSpace(cfg.Value("base_dn")))
}

// ldapAttributes 组装账号属性（AD 与 OpenLDAP 的必填属性不同）
func ldapAttributes(cfg Config, account *Account) []ldap.Attribute {
	attrs := []ldap.Attribute{
		{Type: "cn", Vals: []string{firstNonEmpty(account.DisplayName, account.Username)}},
		{Type: "sn", Vals: []string{firstNonEmpty(account.DisplayName, account.Username)}},
	}
	if isAD(cfg) {
		// AD 的 user 对象类要求 sAMAccountName 唯一，且通常写入 userPrincipalName
		attrs = append(attrs,
			ldap.Attribute{Type: "sAMAccountName", Vals: []string{account.Username}},
			ldap.Attribute{Type: "displayName", Vals: []string{firstNonEmpty(account.DisplayName, account.Username)}},
		)
	}
	if account.Email != "" {
		attrs = append(attrs, ldap.Attribute{Type: "mail", Vals: []string{account.Email}})
	}
	if account.Phone != "" {
		attrs = append(attrs, ldap.Attribute{Type: "mobile", Vals: []string{account.Phone}})
		if !isAD(cfg) {
			attrs = append(attrs, ldap.Attribute{Type: "telephoneNumber", Vals: []string{account.Phone}})
		}
	}
	if title := account.Attributes["title"]; title != "" {
		attrs = append(attrs, ldap.Attribute{Type: "title", Vals: []string{title}})
	}
	return attrs
}

// objectClasses 对象类：OpenLDAP 用 inetOrgPerson，AD 用 user 及其父类
func objectClasses(cfg Config) []string {
	if isAD(cfg) {
		return []string{"top", "person", "organizationalPerson", "user"}
	}
	return []string{"top", "person", "organizationalPerson", "inetOrgPerson"}
}

// passwordAttribute 口令属性：AD 用 unicodePwd（UTF-16LE 且需 TLS），OpenLDAP 用 userPassword
func passwordAttribute(cfg Config, plain string) (ldap.Attribute, error) {
	if isAD(cfg) {
		if strings.ToLower(cfg.Value("ssl_mode")) == "plain" || cfg.Value("ssl_mode") == "" {
			return ldap.Attribute{}, fmt.Errorf("AD 设置口令必须使用加密连接（starttls 或 ldaps）")
		}
		return ldap.Attribute{Type: "unicodePwd", Vals: []string{encodeADPassword(plain)}}, nil
	}
	return ldap.Attribute{Type: "userPassword", Vals: []string{plain}}, nil
}

// encodeADPassword AD 口令要求：以双引号包裹的 UTF-16LE 字节串
func encodeADPassword(plain string) string {
	quoted := `"` + plain + `"`
	encoded := utf16.Encode([]rune(quoted))
	raw := make([]byte, 0, len(encoded)*2)
	for _, unit := range encoded {
		raw = binary.LittleEndian.AppendUint16(raw, unit)
	}
	return string(raw)
}

// Provision 在目录里创建账号（支持首次口令）
func (l *LDAPProvisioner) Provision(ctx context.Context, cfg Config, account *Account) (string, error) {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	request := ldap.NewAddRequest(userDN(cfg, account.Username), nil)
	request.Attribute("objectClass", objectClasses(cfg))
	for _, attr := range ldapAttributes(cfg, account) {
		request.Attribute(attr.Type, attr.Vals)
	}
	if account.Password != "" {
		password, err := passwordAttribute(cfg, account.Password)
		if err != nil {
			return "", err
		}
		request.Attribute(password.Type, password.Vals)
	}
	if err := conn.Add(request); err != nil {
		return "", fmt.Errorf("目录创建账号失败: %w", err)
	}
	return account.Username, nil
}

// Update 更新账号属性；带口令时一并重置
func (l *LDAPProvisioner) Update(ctx context.Context, cfg Config, account *Account) error {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	request := ldap.NewModifyRequest(userDN(cfg, account.Username), nil)
	for _, attr := range ldapAttributes(cfg, account) {
		request.Replace(attr.Type, attr.Vals)
	}
	if account.Password != "" {
		password, err := passwordAttribute(cfg, account.Password)
		if err != nil {
			return err
		}
		request.Replace(password.Type, password.Vals)
	}
	if len(request.Changes) == 0 {
		return nil
	}
	if err := conn.Modify(request); err != nil {
		return fmt.Errorf("目录更新账号失败: %w", err)
	}
	return nil
}

// Enable 启用账号（仅 AD：清除 ACCOUNTDISABLE 位）
func (l *LDAPProvisioner) Enable(ctx context.Context, cfg Config, account *Account) error {
	return l.setAccountControl(ctx, cfg, account, adUserAccountControlNormal)
}

// Disable 禁用账号（仅 AD；OpenLDAP 不支持，由能力声明提前拦截）
func (l *LDAPProvisioner) Disable(ctx context.Context, cfg Config, account *Account) error {
	if !isAD(cfg) {
		return ErrUnsupported
	}
	return l.setAccountControl(ctx, cfg, account, adUserAccountControlDisable)
}

// setAccountControl 写 AD 的 userAccountControl（停用/启用）
func (l *LDAPProvisioner) setAccountControl(ctx context.Context, cfg Config, account *Account, value int) error {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	request := ldap.NewModifyRequest(userDN(cfg, account.Username), nil)
	request.Replace("userAccountControl", []string{fmt.Sprintf("%d", value)})
	if err := conn.Modify(request); err != nil {
		return fmt.Errorf("目录设置账号状态失败: %w", err)
	}
	return nil
}

// Delete 删除账号
func (l *LDAPProvisioner) Delete(ctx context.Context, cfg Config, account *Account) error {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Del(ldap.NewDelRequest(userDN(cfg, account.Username), nil)); err != nil {
		return fmt.Errorf("目录删除账号失败: %w", err)
	}
	return nil
}

// ExistsUser 查询目录里是否存在该账号（按 RDN 直接在账号位置下搜索，判定可靠）。
func (l *LDAPProvisioner) ExistsUser(ctx context.Context, cfg Config, username string) (ExistsResult, error) {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return ExistsResult{}, err
	}
	defer func() { _ = conn.Close() }()
	attr := strings.TrimSpace(cfg.Value("rdn_attr"))
	if attr == "" {
		if isAD(cfg) {
			attr = "cn"
		} else {
			attr = "uid"
		}
	}
	request := ldap.NewSearchRequest(strings.TrimSpace(cfg.Value("base_dn")),
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1, 0, false,
		existsUserFilter(attr, username), []string{"dn"}, nil)
	result, err := conn.Search(request)
	if err != nil {
		return ExistsResult{}, fmt.Errorf("目录查询账号失败: %w", err)
	}
	if len(result.Entries) > 0 {
		return ExistsResult{State: ExistsYes}, nil
	}
	return ExistsResult{State: ExistsNo}, nil
}

// existsUserFilter 组装存在性查询过滤器（用户名需转义，避免 LDAP 注入）
func existsUserFilter(attr, username string) string {
	return fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(username))
}

// Ping 连通性自检：能绑定成功即视为可用
func (l *LDAPProvisioner) Ping(ctx context.Context, cfg Config) error {
	conn, err := l.connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return nil
}
