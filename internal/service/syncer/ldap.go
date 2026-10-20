package syncer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"polaris/internal/repository"
)

// netDialer LDAP 拨号器
var netDialer = net.Dialer{Timeout: 10 * time.Second}

// ldapPageSize 分页大小（AD 默认 1000 条上限，取 500 留出余量）
const ldapPageSize = 500

// LDAPConnector LDAP / Active Directory 连接器。
// Syncer.Database 作为 Base DN，Syncer.Table 作为过滤条件（可选）。
type LDAPConnector struct{}

// Type 连接器类型
func (c *LDAPConnector) Type() string { return "ldap" }

// LDAPTLSMode 解析 TLS 模式（ssl_mode 字段；兼容历史留空 = 明文）。
//
//	plain    → ldap://（默认端口 389）
//	starttls → ldap:// + StartTLS（默认端口 389）
//	ldaps    → ldaps://（默认端口 636）
func LDAPTLSMode(sslMode string) (mode string, defaultPort int) {
	switch strings.ToLower(strings.TrimSpace(sslMode)) {
	case "ldaps", "tls":
		return "ldaps", 636
	case "starttls":
		return "starttls", 389
	default:
		return "plain", 389
	}
}

// DialLDAP 建立并绑定 LDAP/AD 连接（入方向同步与出方向供给共用同一套 TLS 语义）。
//
// tlsMode 取值见 LDAPTLSMode：plain（明文）/ starttls / ldaps。
// 注意：StartTLS 一定在 Bind 之前完成，避免口令走明文。
func DialLDAP(ctx context.Context, host string, port int, bindDN, bindPassword, tlsMode string) (*ldap.Conn, error) {
	mode, defaultPort := LDAPTLSMode(tlsMode)
	if port == 0 {
		port = defaultPort
	}
	address := fmt.Sprintf("%s:%d", host, port)
	scheme := "ldap"
	if mode == "ldaps" {
		scheme = "ldaps"
	}
	conn, err := ldap.DialURL(fmt.Sprintf("%s://%s", scheme, address),
		ldap.DialWithDialer(&netDialer), ldap.DialWithTLSConfig(&tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}))
	if err != nil {
		return nil, fmt.Errorf("连接 LDAP 失败: %w", err)
	}
	if mode == "starttls" {
		if err := conn.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("StartTLS 失败: %w", err)
		}
	}
	if bindDN != "" {
		if err := conn.Bind(bindDN, bindPassword); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("LDAP 绑定失败: %w", err)
		}
	}
	return conn, nil
}

// dial 连接器内部使用：从同步器配置取连接参数
func (c *LDAPConnector) dial(ctx context.Context, s *repository.Syncer) (*ldap.Conn, error) {
	return DialLDAP(ctx, s.Host, s.Port, s.User, s.Password, s.SSLMode)
}

// FetchUsers 搜索 LDAP 用户
func (c *LDAPConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	conn, err := c.dial(ctx, s)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	filter := s.Table
	if filter == "" {
		filter = "(objectClass=person)"
	}
	baseDN := s.Database
	attrs := []string{"uid", "sAMAccountName", "cn", "displayName", "mail", "mobile", "telephoneNumber", "title", "l", "memberOf"}
	// 按映射规则串收集需要抓取的源属性（固定值不产生属性；备选字段都要抓）
	attrs = append(attrs, MappingSourceFields(s.AccountMapping)...)
	search := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter, attrs, nil)
	// 分页搜索：AD 默认单次最多返回 1000 条，必须用分页控件才能取全量
	result, err := conn.SearchWithPaging(search, ldapPageSize)
	if err != nil {
		return nil, fmt.Errorf("LDAP 查询失败: %w", err)
	}

	users := make([]ExternalUser, 0, len(result.Entries))
	for _, e := range result.Entries {
		// 源字段集合：字段映射的候选来源（键名与目录里声明的 source_fields 一致）。
		// memberOf 是多值属性，join 后进 raw，才能被字段映射真实取到（此前是硬编码赋值）。
		raw := map[string]string{
			"uid": e.GetAttributeValue("uid"), "sAMAccountName": e.GetAttributeValue("sAMAccountName"),
			"cn": e.GetAttributeValue("cn"), "displayName": e.GetAttributeValue("displayName"),
			"mail": e.GetAttributeValue("mail"), "mobile": e.GetAttributeValue("mobile"),
			"telephoneNumber": e.GetAttributeValue("telephoneNumber"),
			"title":           e.GetAttributeValue("title"), "l": e.GetAttributeValue("l"),
			"memberOf":          strings.Join(e.GetAttributeValues("memberOf"), ","),
			"distinguishedName": e.DN,
		}
		get := func(attrsList []string) string {
			for _, a := range attrsList {
				if v := raw[a]; v != "" {
					return v
				}
			}
			return ""
		}
		// 先按字段映射（支持"备选源字段"与"固定值"），未配置或取空则回退到内置属性候选
		accountMapping := s.AccountMapping
		jobTitleMapping := s.JobTitleMapping
		mapped := func(localKey string, fallback []string) string {
			if v, ok := ResolveMapping(accountMapping, localKey, func(f string) string { return raw[f] }); ok {
				return v
			}
			return get(fallback)
		}
		id := mapped("id", []string{"uid", "sAMAccountName"})
		username := mapped("name", []string{"uid", "sAMAccountName", "cn"})
		display := mapped("displayName", []string{"displayName", "cn"})
		if id == "" {
			id = e.DN
		}
		users = append(users, ExternalUser{
			Raw:           raw,
			ID:            e.DN,
			Username:      firstNonEmpty(username, id),
			Name:          firstNonEmpty(display, username),
			Email:         mapped("email", []string{"mail"}),
			Phone:         mapped("phone", []string{"mobile", "telephoneNumber"}),
			Title:         PickMapped(accountMapping, raw, "title", "title"),
			JobTitle:      PickJobTitle(accountMapping, jobTitleMapping, raw, "title"),
			Location:      mapped("location", []string{"l"}),
			DepartmentIDs: PickDepartments(raw, "memberOf"),
		})
	}
	return users, nil
}

// FetchGroups 搜索 LDAP 用户组
func (c *LDAPConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	conn, err := c.dial(ctx, s)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	search := ldap.NewSearchRequest(s.Database, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(|(objectClass=groupOfNames)(objectClass=group)(objectClass=posixGroup))", []string{"cn", "member", "uniqueMember", "memberUid"}, nil)
	result, err := conn.SearchWithPaging(search, ldapPageSize)
	if err != nil {
		return nil, fmt.Errorf("LDAP 查询用户组失败: %w", err)
	}
	groups := make([]ExternalGroup, 0, len(result.Entries))
	for _, e := range result.Entries {
		members := append(e.GetAttributeValues("member"), e.GetAttributeValues("uniqueMember")...)
		members = append(members, e.GetAttributeValues("memberUid")...)
		// DN 自带层级：父节点 = 去掉最左侧 RDN
		parent := ParentDN(e.DN)
		raw := map[string]string{
			"cn": e.GetAttributeValue("cn"), "distinguishedName": e.DN,
			"memberOf": strings.Join(members, ","),
		}
		groups = append(groups, ExternalGroup{
			ID: e.DN, Name: e.GetAttributeValue("cn"), ParentID: parent, MemberIDs: members, Raw: raw,
		})
	}
	return groups, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ParentDN 由 DN 推导父节点 DN（去掉最左侧 RDN）；无父节点时返回空串。
func ParentDN(dn string) string {
	parts := strings.SplitN(strings.TrimSpace(dn), ",", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
