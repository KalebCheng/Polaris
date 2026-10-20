package syncer

import "testing"

// 回归：TLS 模式解析（明文 / StartTLS / LDAPS）——默认端口随之变化，避免"配了 LDAPS 却连 389"。
func TestLDAPTLSMode(t *testing.T) {
	cases := []struct {
		in          string
		wantMode    string
		wantDefault int
	}{
		{"", "plain", 389},
		{"plain", "plain", 389},
		{"starttls", "starttls", 389},
		{"StartTLS", "starttls", 389},
		{"ldaps", "ldaps", 636},
		{"tls", "ldaps", 636},
	}
	for _, tc := range cases {
		mode, port := LDAPTLSMode(tc.in)
		if mode != tc.wantMode || port != tc.wantDefault {
			t.Errorf("LDAPTLSMode(%q) = (%s,%d)，期望 (%s,%d)", tc.in, mode, port, tc.wantMode, tc.wantDefault)
		}
	}
}

// 回归：由 DN 推导父节点（组织树层级还原）
func TestParentDN(t *testing.T) {
	cases := map[string]string{
		"CN=张三,OU=技术部,DC=example,DC=com": "OU=技术部,DC=example,DC=com",
		"OU=技术部,DC=example,DC=com":       "DC=example,DC=com",
		"DC=example,DC=com":              "DC=com",
		"DC=com":                         "",
		"":                               "",
		"  CN=x  ":                       "",
	}
	for in, want := range cases {
		if got := ParentDN(in); got != want {
			t.Errorf("ParentDN(%q) = %q，期望 %q", in, got, want)
		}
	}
}
