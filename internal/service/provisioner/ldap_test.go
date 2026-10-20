package provisioner

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/go-ldap/ldap/v3"
)

// ldapAttributeAlias 保持测试可读性（等价于 ldap.Attribute）
type ldapAttributeAlias = ldap.Attribute

func ldapConfig(directoryType string) Config {
	values := map[string]string{
		"host": "ldap.example.com", "port": "389", "ssl_mode": "ldaps",
		"user": "cn=admin,dc=example,dc=com", "password": "bind-secret",
		"base_dn": "OU=Users,DC=example,DC=com",
	}
	if directoryType != "" {
		values["directory_type"] = directoryType
	}
	return Config{Type: "ldap", Values: values}
}

// 回归：AD 支持停用（userAccountControl），OpenLDAP 不支持——能力必须按配置区分。
func TestLDAPCapabilitiesByDirectoryType(t *testing.T) {
	ad := (&LDAPProvisioner{}).Capabilities(ldapConfig("ad"))
	if !ad.Disable {
		t.Fatal("AD 应支持停用账号")
	}
	if !ad.Password || !ad.Update || !ad.Delete {
		t.Fatalf("目录连接器应支持口令/更新/删除: %+v", ad)
	}
	openldap := (&LDAPProvisioner{}).Capabilities(ldapConfig("openldap"))
	if openldap.Disable {
		t.Fatal("OpenLDAP 没有停用语义，不应声明支持停用")
	}
	// 且运行时也必须拒绝（双保险）
	err := (&LDAPProvisioner{}).Disable(context.Background(), ldapConfig("openldap"), &Account{Username: "alice"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("OpenLDAP 停用应返回 ErrUnsupported，实际 %v", err)
	}
}

// 回归：账号 DN 的组装——属性随目录类型变化，用户名需转义
func TestLDAPUserDN(t *testing.T) {
	openldap := userDN(ldapConfig("openldap"), "alice")
	if openldap != "uid=alice,OU=Users,DC=example,DC=com" {
		t.Fatalf("OpenLDAP DN 组装错误: %s", openldap)
	}
	ad := userDN(ldapConfig("ad"), "alice")
	if ad != "cn=alice,OU=Users,DC=example,DC=com" {
		t.Fatalf("AD 默认 RDN 应为 cn: %s", ad)
	}
	cfg := ldapConfig("openldap")
	cfg.Values["rdn_attr"] = "sAMAccountName"
	if got := userDN(cfg, "alice"); !strings.HasPrefix(got, "sAMAccountName=alice,") {
		t.Fatalf("应使用配置的 RDN 属性: %s", got)
	}
	// 含逗号的用户名必须转义，否则 DN 会被拆成两段
	escaped := userDN(ldapConfig("openldap"), "smith, john")
	if !strings.Contains(escaped, `smith\, john`) && !strings.Contains(escaped, `smith\2C john`) {
		t.Fatalf("用户名中的逗号应被转义: %s", escaped)
	}
}

// 回归：对象类与属性集随目录类型变化
func TestLDAPObjectClasses(t *testing.T) {
	if got := objectClasses(ldapConfig("openldap")); !contains(got, "inetOrgPerson") {
		t.Fatalf("OpenLDAP 应使用 inetOrgPerson: %v", got)
	}
	ad := objectClasses(ldapConfig("ad"))
	if !contains(ad, "user") || contains(ad, "inetOrgPerson") {
		t.Fatalf("AD 应使用 user 对象类: %v", ad)
	}
	attrs := ldapAttributes(ldapConfig("ad"), &Account{Username: "alice", DisplayName: "Alice", Email: "a@b.com"})
	if !hasAttr(attrs, "sAMAccountName") {
		t.Fatalf("AD 必须写入 sAMAccountName: %+v", attrs)
	}
}

// 回归：AD 口令必须走 unicodePwd（UTF-16LE、双引号包裹），且必须加密连接
func TestLDAPPasswordAttribute(t *testing.T) {
	if _, err := passwordAttribute(ldapConfig("ad"), "Secret123"); err != nil {
		t.Fatalf("AD + LDAPS 应允许设置口令: %v", err)
	}
	plainAD := ldapConfig("ad")
	plainAD.Values["ssl_mode"] = "plain"
	if _, err := passwordAttribute(plainAD, "Secret123"); err == nil {
		t.Fatal("AD 明文连接下设置口令必须被拒绝（否则口令会走明文）")
	}
	openldap, err := passwordAttribute(ldapConfig("openldap"), "Secret123")
	if err != nil || openldap.Type != "userPassword" {
		t.Fatalf("OpenLDAP 应使用 userPassword: %+v err=%v", openldap, err)
	}

	encoded := encodeADPassword("Secret123")
	decoded := utf16.Decode(bytesToUint16([]byte(encoded)))
	if string(decoded) != `"Secret123"` {
		t.Fatalf("AD 口令编码应为双引号包裹的 UTF-16LE，实际 %q", string(decoded))
	}
}

func bytesToUint16(raw []byte) []uint16 {
	out := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		out = append(out, binary.LittleEndian.Uint16(raw[i:i+2]))
	}
	return out
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func hasAttr(attrs []ldapAttributeAlias, want string) bool {
	for _, attr := range attrs {
		if attr.Type == want {
			return true
		}
	}
	return false
}
