package postgres

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"polaris/internal/repository"
)

// jsonbText 模拟"参数经 jsonb 列往返后读回"的文本形态——仓储层的编码缺陷只在这一步暴露，
// 因此凡涉及配置编解码的断言都必须走这里，而不是直接拿 shrink 的返回值喂给 expand。
func jsonbText(v interface{}) []byte { return []byte(encodeJSONB(v)) }

// jsonbTypeOf 返回 JSON 文本的顶层类型（等价于 PostgreSQL 的 jsonb_typeof(col)）。
func jsonbTypeOf(t *testing.T, payload []byte) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(payload, &v); err != nil {
		t.Fatalf("JSONB 文本不是合法 JSON: %s", payload)
	}
	switch v.(type) {
	case map[string]interface{}:
		return "object"
	case string:
		return "string"
	case []interface{}:
		return "array"
	default:
		return "other"
	}
}

// 回归：入方向配置的"扁平字段 ↔ JSONB"往返必须无损（存储换形态、契约不变）。
func TestSyncerConfigRoundTrip(t *testing.T) {
	original := &repository.Syncer{
		Host: "ldap.example.com", Port: 636, User: "cn=admin,dc=example,dc=com",
		Database: "dc=example,dc=com", Table: "(objectClass=person)", DatabaseType: "postgres",
		SSLMode: "require", AppID: "ding123", CorpID: "corp456",
		Password: "enc:bind-secret", AppSecret: "enc:app-secret",
		AccountMapping:    map[string]string{"name": "sAMAccountName", "email": "mail"},
		DepartmentMapping: map[string]string{"name": "name"},
		JobTitleMapping:   map[string]string{"name": "title"},
	}
	config, secrets, fieldMapping := shrinkSyncer(original)

	restored := &repository.Syncer{}
	expandSyncer(restored, jsonbText(config), jsonbText(secrets), jsonbText(fieldMapping))

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
	if restored.AccountMapping["name"] != "sAMAccountName" || restored.AccountMapping["email"] != "mail" {
		t.Fatalf("字段映射丢失: %+v", restored.AccountMapping)
	}
	if restored.DepartmentMapping["name"] != "name" || restored.JobTitleMapping["name"] != "title" {
		t.Fatalf("组织实体映射丢失: department=%+v job_title=%+v", restored.DepartmentMapping, restored.JobTitleMapping)
	}
}

// 回归：空值不得写入 JSONB——否则 "未配置" 会被当成 "已配置"（has_* 会误报）。
func TestSyncerConfigSkipsEmptyValues(t *testing.T) {
	config, secrets, fieldMapping := shrinkSyncer(&repository.Syncer{Host: "db.local"})
	if got := encodeJSONB(config); got != `{"host":"db.local"}` {
		t.Fatalf("config 应只含非空项，实际 %s", got)
	}
	if got := encodeJSONB(secrets); got != `{}` {
		t.Fatalf("secrets 应为空对象，实际 %s", got)
	}
	if got := encodeJSONB(fieldMapping); got != `{}` {
		t.Fatalf("field_mapping 应为空对象，实际 %s", got)
	}
	emptyConfig, _, _ := shrinkSyncer(&repository.Syncer{})
	if got := encodeJSONB(emptyConfig); got != `{}` {
		t.Fatalf("全空配置应落 {}，实际 %s", got)
	}
}

// 回归：端口 0 不写入（连接器各有默认端口）；非法端口值展开为 0 而不是 panic。
func TestSyncerPortHandling(t *testing.T) {
	config, _, _ := shrinkSyncer(&repository.Syncer{Port: 0})
	if got := encodeJSONB(config); got != `{}` {
		t.Fatalf("端口 0 不应写入，实际 %s", got)
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
	if restored.Host != "" || restored.Password != "" || restored.AccountMapping != nil {
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
	expandConnector(restored, jsonbText(config), jsonbText(secrets))

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

// 回归（飞书 10003 invalid param 的根因）：配置与凭据落库必须是 JSON **对象**。
//
// 历史缺陷：shrinkSyncer/shrinkConnector 返回的已是 json.Marshal 过的 []byte，
// Create/Update 又把它交给 encodeJSONB（再 Marshal 一次），于是列里存成
// base64 字符串（jsonb_typeof = 'string'）；读回时 Unmarshal 成 map 必然失败，
// config/secrets 变成空表 → 连接器的 app_id / app_secret 全空 → 飞书
// tenant_access_token 接口回 10003 invalid param（钉钉、企业微信同理）。
func TestSyncerJSONBPayloadIsObject(t *testing.T) {
	original := &repository.Syncer{
		AppID: "cli_aa5bc049ac785d2e", AppSecret: "enc:app-secret",
		Host: "ldap.example.com", AccountMapping: map[string]string{"name": "sAMAccountName"},
	}
	config, secrets, fieldMapping := shrinkSyncer(original)
	for _, payload := range [][]byte{jsonbText(config), jsonbText(secrets), jsonbText(fieldMapping)} {
		if kind := jsonbTypeOf(t, payload); kind != "object" {
			t.Fatalf("JSONB 载荷必须是对象，实际 %s: %s", kind, payload)
		}
	}

	restored := &repository.Syncer{}
	expandSyncer(restored, jsonbText(config), jsonbText(secrets), jsonbText(fieldMapping))
	if restored.AppID != original.AppID {
		t.Fatalf("应用标识读回为空（连接器会以空 app_id 调外部接口）: %q", restored.AppID)
	}
	if restored.AppSecret != original.AppSecret {
		t.Fatalf("应用密钥读回为空: %q", restored.AppSecret)
	}
	if restored.AccountMapping["name"] != "sAMAccountName" {
		t.Fatalf("字段映射读回为空: %+v", restored.AccountMapping)
	}
}

// 回归：出方向连接器同样必须是 JSON 对象（同一缺陷的另一个入口）。
func TestConnectorJSONBPayloadIsObject(t *testing.T) {
	original := &repository.Connector{
		Type:       "feishu",
		Values:     map[string]string{"app_id": "cli_aa5bc049ac785d2e", "app_secret": "enc:app-secret"},
		HasSecrets: map[string]bool{"app_secret": true},
	}
	config, secrets := shrinkConnector(original)
	for _, payload := range [][]byte{jsonbText(config), jsonbText(secrets)} {
		if kind := jsonbTypeOf(t, payload); kind != "object" {
			t.Fatalf("JSONB 载荷必须是对象，实际 %s: %s", kind, payload)
		}
	}
	restored := &repository.Connector{}
	expandConnector(restored, jsonbText(config), jsonbText(secrets))
	if restored.Values["app_id"] != "cli_aa5bc049ac785d2e" || !restored.HasSecrets["app_secret"] {
		t.Fatalf("出方向配置读回为空: %+v", restored)
	}
}

// 回归：缺陷版本已经写进库的行（jsonb 里是 base64 字符串）必须仍可读，
// 否则升级后存量连接的凭据会"凭空消失"，只能靠人工重新填写。
func TestExpandLegacyDoubleEncodedPayload(t *testing.T) {
	// 缺陷版本写入的形态：json.Marshal([]byte) → jsonb 编解码器再编码一次
	legacy := func(v interface{}) []byte {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("构造历史载荷失败: %v", err)
		}
		quoted, err := json.Marshal(base64.StdEncoding.EncodeToString(raw))
		if err != nil {
			t.Fatalf("构造历史载荷失败: %v", err)
		}
		return quoted
	}

	sy := &repository.Syncer{}
	expandSyncer(sy,
		legacy(map[string]string{"app_id": "cli_aa5bc049ac785d2e"}),
		legacy(map[string]string{"app_secret": "enc:app-secret"}),
		legacy(map[string]map[string]string{"account": {"name": "sAMAccountName"}}))
	if sy.AppID != "cli_aa5bc049ac785d2e" || sy.AppSecret != "enc:app-secret" {
		t.Fatalf("历史行的配置/凭据未还原: app_id=%q has_secret=%v", sy.AppID, sy.AppSecret != "")
	}
	if sy.AccountMapping["name"] != "sAMAccountName" {
		t.Fatalf("历史行的字段映射未还原: %+v", sy.AccountMapping)
	}

	// 出方向同理
	c := &repository.Connector{}
	expandConnector(c, legacy(map[string]string{"app_id": "cli_x"}), legacy(map[string]string{"app_secret": "enc:s"}))
	if c.Values["app_id"] != "cli_x" || !c.HasSecrets["app_secret"] {
		t.Fatalf("历史行的出方向配置未还原: %+v", c)
	}

	// 非 base64 的普通字符串（脏数据）不得 panic，退化为空配置即可
	dirty := &repository.Syncer{}
	expandSyncer(dirty, []byte(`"not-base64"`), nil, nil)
	if dirty.AppID != "" || dirty.Config == nil {
		t.Fatalf("脏数据应退化为空配置: %+v", dirty)
	}
}
