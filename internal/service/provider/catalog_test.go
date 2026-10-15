package provider

import (
	"reflect"
	"strings"
	"testing"

	"polaris/internal/repository"
)

// providerColumns 反射 providers 实体的持久化列（db tag），用于锁定"目录声明的字段都有对应列"。
func providerColumns() map[string]bool {
	columns := map[string]bool{}
	typ := reflect.TypeOf(repository.Provider{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		columns[strings.Split(tag, ",")[0]] = true
	}
	return columns
}

func TestCatalogShape(t *testing.T) {
	columns := providerColumns()
	categories := map[string]bool{}
	types := map[string]bool{}
	for _, category := range Categories() {
		if category.Key == "" || category.Name == "" || category.Description == "" || category.Icon == "" {
			t.Fatalf("类别缺少声明（含图标）: %+v", category)
		}
		key := string(category.Key)
		if categories[key] {
			t.Fatalf("类别 key 重复: %s", key)
		}
		categories[key] = true
		if len(category.Types) == 0 {
			t.Fatalf("类别 %s 没有类型", key)
		}
		for _, info := range category.Types {
			if info.Key == "" || info.Name == "" || info.Description == "" || info.Icon == "" {
				t.Fatalf("类型缺少声明: %+v", info)
			}
			// 类型 key 全局唯一：运行时要按 type 反查类型（FindByType）
			if types[info.Key] {
				t.Fatalf("类型 key 重复: %s", info.Key)
			}
			types[info.Key] = true
			if info.Category != category.Key {
				t.Fatalf("类型 %s 的类别归属与所在分组不一致", info.Key)
			}
			caps := info.Capabilities
			declared := 0
			for _, on := range []bool{caps.Signin, caps.Notify, caps.Captcha, caps.Storage} {
				if on {
					declared++
				}
			}
			if declared != 1 {
				t.Fatalf("类型 %s 的能力声明必须恰好命中一类（signin/notify/captcha/storage），实际 %d", info.Key, declared)
			}
			if !caps.Test {
				t.Fatalf("类型 %s 缺少连通性自检能力声明", info.Key)
			}
			// 每个声明的字段都必须有对应的持久化列（防止"声明了但列不存在"）
			seenKeys := map[string]bool{}
			seenColumns := map[string]bool{}
			groups := map[string]bool{}
			for _, field := range info.Fields {
				if field.Key == "" || field.Label == "" {
					t.Fatalf("类型 %s 存在无 key/label 的字段", info.Key)
				}
				if seenKeys[field.Key] {
					t.Fatalf("类型 %s 字段 key 重复: %s", info.Key, field.Key)
				}
				seenKeys[field.Key] = true
				groups[field.Group] = true
				switch field.Group {
				case GroupBasic, GroupConnection, GroupAdvanced:
				default:
					t.Fatalf("类型 %s 字段 %s 的分组非法: %s", info.Key, field.Key, field.Group)
				}
				column := field.Column()
				if !columns[column] {
					t.Fatalf("类型 %s 字段 %s 落库列 %s 不存在于实体", info.Key, field.Key, column)
				}
				if seenColumns[column] {
					t.Fatalf("类型 %s 多个字段落到同一列: %s", info.Key, column)
				}
				seenColumns[column] = true
				if field.Type == FieldMultiSelect && len(field.Options) == 0 {
					t.Fatalf("类型 %s 字段 %s 是受限多选，必须声明 Options", info.Key, field.Key)
				}
				for _, option := range field.Options {
					if field.Key == "template_params" && !IsKnownTemplateVar(option.Value) {
						t.Fatalf("模板变量 %s 取不到值（不属于内置变量集）", option.Value)
					}
				}
				if field.Secret && field.Type != FieldPassword {
					t.Fatalf("类型 %s 的敏感字段 %s 必须使用 password 控件", info.Key, field.Key)
				}
				if field.Secret && field.Policy {
					t.Fatalf("类型 %s 字段 %s 不能同时是敏感字段与策略字段", info.Key, field.Key)
				}
			}
			// 基本信息字段所有类型都必须有；连接配置字段允许为空
			// （例如本地图形验证码不需要任何凭据）
			if !groups[GroupBasic] {
				t.Fatalf("类型 %s 缺少基本信息字段", info.Key)
			}
			// 外部身份提供方必须有 SSO 信任策略（否则"能配登录但不能建号"）
			if caps.Signin && !groups[GroupAdvanced] {
				t.Fatalf("类型 %s 缺少高级与策略字段", info.Key)
			}
		}
	}
	for _, want := range []string{"oauth", "email", "sms", "captcha", "storage"} {
		if !categories[want] {
			t.Fatalf("缺少类别 %s", want)
		}
	}
	// 单例语义：运行时"取一个"的类别必须声明 Singleton，否则界面上会出现"配了多个，不知道用哪个"
	for _, category := range Categories() {
		want := string(category.Key) != string(CategoryOAuth)
		if category.Singleton != want {
			t.Fatalf("类别 %s 的 Singleton = %v, want %v", category.Key, category.Singleton, want)
		}
	}
}

func TestCatalogBuckets(t *testing.T) {
	// 敏感字段一律进 secrets 桶，策略字段一律进 policy 桶，其余进 config 桶
	for _, category := range Categories() {
		for _, info := range category.Types {
			for _, field := range info.Fields {
				want := "config"
				if field.Secret {
					want = "secrets"
				} else if field.Policy {
					want = "policy"
				}
				if got := field.Bucket(); got != want {
					t.Fatalf("类型 %s 字段 %s 的请求桶错误: got %s want %s", info.Key, field.Key, got, want)
				}
			}
		}
	}
	// 密钥列必须且只能由声明为 secret 的字段占用（否则会出现"密钥从 config 明文入库"）
	for _, category := range Categories() {
		for _, info := range category.Types {
			for _, field := range info.Fields {
				if field.Column() == "client_secret" && !field.Secret {
					t.Fatalf("类型 %s 的 client_secret 必须声明为敏感字段", info.Key)
				}
			}
		}
	}
}

func TestNormalizeCategoryAndType(t *testing.T) {
	cases := map[string]string{
		"OAuth": "oauth", "oauth": "oauth", "OIDC": "oauth", "Email": "email",
		"SMS": "sms", "Captcha": "captcha", "Storage": "storage", "unknown": "",
	}
	for input, want := range cases {
		if got := NormalizeCategory(input); got != want {
			t.Fatalf("NormalizeCategory(%q) = %q, want %q", input, got, want)
		}
	}
	if got := NormalizeType(CategoryCaptcha, "recaptcha_v2"); got != TypeCaptchaRecaptch {
		t.Fatalf("recaptcha 历史子类型未归一化: %s", got)
	}
	if got := NormalizeType(CategoryCaptcha, ""); got != TypeCaptchaImage {
		t.Fatalf("空类型应回落到类别默认值，实际 %s", got)
	}
	if got := NormalizeType(CategoryOAuth, "not-exists"); got != "" {
		t.Fatalf("未知类型应被拒绝，实际 %q", got)
	}
	if got := NormalizeType(CategoryStorage, "Local"); got != TypeStorageLocal {
		t.Fatalf("类型大小写未归一化: %s", got)
	}
}

func TestDefaultType(t *testing.T) {
	// 默认类型必须是"无需额外凭据、开箱即用"的那一个（否则空类型回落会产生不可用配置）
	want := map[Category]string{
		CategoryOAuth:   TypeOIDC,
		CategoryEmail:   TypeSMTP,
		CategorySMS:     TypeAliyun,
		CategoryCaptcha: TypeCaptchaImage,
		CategoryStorage: TypeStorageLocal,
	}
	for category, key := range want {
		if got := DefaultType(category); got != key {
			t.Fatalf("类别 %s 的默认类型 = %s, want %s", category, got, key)
		}
		if _, ok := Find(category, key); !ok {
			t.Fatalf("类别 %s 的默认类型 %s 不在目录里", category, key)
		}
	}
	// 默认类型不得需要连接配置字段（除 OIDC 需要 issuer 外）
	if fields := FieldsIn(mustFind(t, CategoryCaptcha, TypeCaptchaImage), GroupConnection); len(fields) != 0 {
		t.Fatalf("本地图形验证码不应要求连接配置字段，实际 %d 项", len(fields))
	}
}

func mustFind(t *testing.T, category Category, key string) TypeInfo {
	t.Helper()
	info, ok := Find(category, key)
	if !ok {
		t.Fatalf("目录缺少类型 %s/%s", category, key)
	}
	return info
}
