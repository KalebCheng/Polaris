package postgres

import (
	"testing"

	"polaris/internal/repository"
)

// 回归：入方向配置的"扁平字段 ↔ JSONB"往返必须无损（存储换形态、契约不变）。
func TestSyncerConfigRoundTrip(t *testing.T) {
	original := &repository.Syncer{
		Host: "ldap.example.com", Port: 636, User: "cn=admin,dc=example,dc=com",
		Database: "dc=example,dc=com", Table: "(objectClass=person)", DatabaseType: "postgres",
		SSLMode: "require", AppID: "ding123", CorpID: "corp456",
		Password: "enc:bind-secret", AppSecret: "enc:app-secret",
		UserMapping: map[string]string{"name": "sAMAccountName", "email": "mail"},
	}
	config, secrets, fieldMapping := shrinkSyncer(original)

	restored := &repository.Syncer{}
	expandSyncer(restored, config, secrets, fieldMapping)

	if restored.Host != original.Host || restored.Port != original.Port || restored.User != original.User {
		t.Fatalf("连接配置丢失: %+v", restored)
	}
	if restored.Database != original.Database || restored.Table != original.Table {
		t.Fatalf("库名/过滤条件丢失: %+v", restored)
	}
	if restored.DatabaseType != original.DatabaseType || restored.SSLMode != original.SSLMode {
		t.Fatalf("数据库类型/SSL 丢失: %+v", restored)
	}
	if restored.AppID != original.AppID || restored.CorpID != original.CorpID {
		t.Fatalf("应用标识丢失: %+v", restored)
	}
	// 敏感值以密文形态原样搬运（加解密是服务层的事）
	if restored.Password != "enc:bind-secret" || restored.AppSecret != "enc:app-secret" {
		t.Fatalf("凭据密文被改写: %q / %q", restored.Password, restored.AppSecret)
	}
	if restored.UserMapping["name"] != "sAMAccountName" || restored.UserMapping["email"] != "mail" {
		t.Fatalf("字段映射丢失: %+v", restored.UserMapping)
	}
}

// 回归：空值不得写入 JSONB——否则 "未配置" 会被当成 "已配置"（has_* 会误报）。
func TestSyncerConfigSkipsEmptyValues(t *testing.T) {
	config, secrets, fieldMapping := shrinkSyncer(&repository.Syncer{Host: "db.local"})
	if string(config) != `{"host":"db.local"}` {
		t.Fatalf("config 应只含非空项，实际 %s", config)
	}
	if string(secrets) != `{}` {
		t.Fatalf("secrets 应为空对象，实际 %s", secrets)
	}
	if string(fieldMapping) != `{}` {
		t.Fatalf("field_mapping 应为空对象，实际 %s", fieldMapping)
	}
	emptyConfig, _, _ := shrinkSyncer(&repository.Syncer{})
	if got := string(emptyConfig); got != `{}` {
		t.Fatalf("全空配置应落 {}，实际 %s", got)
	}
}

// 回归：端口 0 不写入（连接器各有默认端口）；非法端口值展开为 0 而不是 panic。
func TestSyncerPortHandling(t *testing.T) {
	config, _, _ := shrinkSyncer(&repository.Syncer{Port: 0})
	if string(config) != `{}` {
		t.Fatalf("端口 0 不应写入，实际 %s", config)
	}
	restored := &repository.Syncer{}
	expandSyncer(restored, []byte(`{"port":"not-a-number"}`), nil, nil)
	if restored.Port != 0 {
		t.Fatalf("非法端口应回退为 0，实际 %d", restored.Port)
	}
}

// 回归：历史行可能没有配置（空 JSONB），展开必须安全且不 panic。
func TestSyncerExpandEmptyPayload(t *testing.T) {
	restored := &repository.Syncer{}
	expandSyncer(restored, nil, nil, nil)
	if restored.Config == nil || restored.Secrets == nil || restored.FieldMapping == nil {
		t.Fatalf("空载荷应得到空 map 而不是 nil: %+v", restored)
	}
	if restored.Host != "" || restored.Password != "" || restored.UserMapping != nil {
		t.Fatalf("空载荷不应产生任何配置: %+v", restored)
	}
}

// 回归：出方向的配置/凭据往返无损，且类型缺省为通用 HTTP。
// 键由类型目录声明（provider_url / app_id / 凭据等），仓储层只负责搬运，因此新增一种
// 下游连接器不需要改表、也不需要改实体。
func TestConnectorConfigRoundTrip(t *testing.T) {
	original := &repository.Connector{
		Type: "dingtalk",
		Values: map[string]string{
			"provider_url": "https://app.example.com/api",
			"principal":    "svc",
			"app_id":       "ding-app-key",
			"app_secret":   "enc:app-secret", // 密文（加密由服务层完成）
		},
		HasSecrets: map[string]bool{"app_secret": true},
	}
	config, secrets := shrinkConnector(original)
	restored := &repository.Connector{}
	expandConnector(restored, config, secrets)

	for _, key := range []string{"provider_url", "principal", "app_id"} {
		if restored.Values[key] != original.Values[key] {
			t.Fatalf("配置 %s 丢失: %q", key, restored.Values[key])
		}
	}
	if restored.Values["app_secret"] != "enc:app-secret" || !restored.HasSecrets["app_secret"] {
		t.Fatalf("凭据密文或存在性丢失: %+v", restored)
	}
	if restored.HasSecrets["app_id"] {
		t.Fatal("非敏感键不应被标记为敏感")
	}
	// 说明：type 是行上的列（不是配置），因此不在本次"配置往返"的断言范围内；
	// 这里只锁定"缺省回退 http"的历史数据兼容行为。

	// 历史行没有 type 时，展开必须回退为通用 HTTP（与 connectorProvisionerType 的兜底一致）
	legacy := &repository.Connector{}
	expandConnector(legacy, nil, nil)
	if legacy.Type != "http" {
		t.Fatalf("缺省类型应为 http，实际 %q", legacy.Type)
	}
}
