package provider

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// 本文件负责"目录声明 ↔ 实体列"的双向映射：
//   - Extract：把实体列读成对外契约（config 非敏感 / policy 策略 / 密钥只留 has_secrets）；
//   - Bind：把对外契约按目录声明写回实体列，未知键与"敏感键混入 config"一律拒绝。
//
// 之所以用反射而不是手写两份映射：字段会随类型增加而增加，手写清单必然漂移
// （AGENTS.md §3.6 的历史教训：列清单漂移三次）。

var providerColumnIndex = func() map[string]int {
	index := map[string]int{}
	typ := reflect.TypeOf(repository.Provider{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		index[strings.Split(tag, ",")[0]] = i
	}
	return index
}()

// topLevelFields 由响应体的一级字段承载（name / display_name / state），不再重复进 config，
// 避免"同一个值有两个来源"。
var topLevelFields = map[string]bool{"name": true, "display_name": true, "state": true}

// Extract 按目录把实体读成 config 与 policy 两个非敏感桶。
func Extract(item *repository.Provider, info TypeInfo) (config, policy map[string]any) {
	config, policy = map[string]any{}, map[string]any{}
	if item == nil {
		return config, policy
	}
	for _, field := range info.Fields {
		if field.Secret || topLevelFields[field.Key] {
			continue
		}
		value := readColumn(item, field.Column())
		if field.Policy {
			policy[field.Key] = value
			continue
		}
		config[field.Key] = value
	}
	return config, policy
}

// BindOptions 绑定选项
type BindOptions struct {
	// KeepSecretsOnEmpty 编辑态：secrets 缺省/空串表示"保持原值"（必填校验随之跳过）
	KeepSecretsOnEmpty bool
}

// Bind 把对外契约按目录写回实体。
//
// 校验（"配置即生效"的前置条件）：
//  1. 类型必须存在于目录（未知类型拒绝）；
//  2. config / policy / secrets 的键必须由该类型的目录声明，未知键拒绝；
//  3. 敏感字段必须走 secrets，出现在 config 里一律拒绝（避免明文密钥落进非敏感桶）；
//  4. 必填字段在创建时必须提供。
func Bind(item *repository.Provider, info TypeInfo, config, policy map[string]any, secrets map[string]string, opts BindOptions) error {
	for key := range config {
		field, ok := LookupField(info, key)
		if !ok {
			return bizerrors.ErrInvalidParams.WithDetail("unknown_config_key", key)
		}
		if field.Secret {
			return bizerrors.ErrInvalidParams.WithDetail("secret_in_config", key)
		}
		if field.Policy {
			return bizerrors.ErrInvalidParams.WithDetail("policy_in_config", key)
		}
	}
	for key := range policy {
		field, ok := LookupField(info, key)
		if !ok || !field.Policy {
			return bizerrors.ErrInvalidParams.WithDetail("unknown_policy_key", key)
		}
	}
	for key := range secrets {
		field, ok := LookupField(info, key)
		if !ok || !field.Secret {
			return bizerrors.ErrInvalidParams.WithDetail("unknown_secret_key", key)
		}
	}

	for _, field := range info.Fields {
		switch {
		case field.Secret:
			plain := strings.TrimSpace(secrets[field.Key])
			if plain == "" {
				continue // 留空 = 保持原值（Create 时表示未配置）
			}
			if err := writeColumn(item, field.Column(), plain); err != nil {
				return err
			}
		case field.Policy:
			value, present := policy[field.Key]
			if !present {
				continue
			}
			if err := writeColumn(item, field.Column(), value); err != nil {
				return err
			}
		default:
			value, present := config[field.Key]
			if !present {
				continue
			}
			if err := writeColumn(item, field.Column(), value); err != nil {
				return err
			}
		}
	}

	if err := validateOptions(info, config, policy); err != nil {
		return err
	}
	return validateRequired(item, info, opts)
}

// validateOptions 声明了 Options 的字段（select / multiselect）只接受清单内的取值。
//
// 意义：像"模板用到的变量"这种封闭集合，非法取值在**落库前**就被拒绝——
// 否则会变成"配了但运行时取不到值 → 发送失败"，排查起来很绕。
func validateOptions(info TypeInfo, config, policy map[string]any) error {
	for _, field := range info.Fields {
		if len(field.Options) == 0 {
			continue
		}
		bucket := config
		if field.Policy {
			bucket = policy
		}
		raw, present := bucket[field.Key]
		if !present {
			continue
		}
		allowed := make(map[string]bool, len(field.Options))
		for _, option := range field.Options {
			allowed[option.Value] = true
		}
		switch typed := raw.(type) {
		case string:
			if typed != "" && !allowed[typed] {
				return bizerrors.ErrInvalidParams.WithDetail("field", field.Key).WithDetail("value", typed)
			}
		case []any:
			for _, item := range typed {
				value := toString(item)
				if value != "" && !allowed[value] {
					return bizerrors.ErrInvalidParams.WithDetail("field", field.Key).WithDetail("value", value)
				}
			}
		case []string:
			for _, value := range typed {
				if value != "" && !allowed[value] {
					return bizerrors.ErrInvalidParams.WithDetail("field", field.Key).WithDetail("value", value)
				}
			}
		}
	}
	return nil
}

func validateRequired(item *repository.Provider, info TypeInfo, opts BindOptions) error {
	for _, field := range info.Fields {
		if !field.Required {
			continue
		}
		if field.Secret && opts.KeepSecretsOnEmpty {
			continue // 编辑态：敏感字段留空表示保持原值
		}
		if isEmptyColumn(item, field.Column()) {
			return bizerrors.ErrInvalidParams.WithDetail("required", field.Key)
		}
	}
	return nil
}

func readColumn(item *repository.Provider, column string) any {
	index, ok := providerColumnIndex[column]
	if !ok {
		return nil
	}
	value := reflect.ValueOf(item).Elem().Field(index)
	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Slice:
		out := make([]string, 0, value.Len())
		for i := 0; i < value.Len(); i++ {
			out = append(out, fmt.Sprintf("%v", value.Index(i).Interface()))
		}
		return out
	default:
		return nil
	}
}

func writeColumn(item *repository.Provider, column string, raw any) error {
	index, ok := providerColumnIndex[column]
	if !ok {
		return bizerrors.ErrInvalidParams.WithDetail("unknown_column", column)
	}
	target := reflect.ValueOf(item).Elem().Field(index)
	switch target.Kind() {
	case reflect.String:
		target.SetString(toString(raw))
	case reflect.Bool:
		target.SetBool(toBool(raw))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number, err := toInt(raw)
		if err != nil {
			return bizerrors.ErrInvalidParams.WithDetail("field", column)
		}
		target.SetInt(number)
	case reflect.Slice:
		items, err := toStringSlice(raw)
		if err != nil {
			return bizerrors.ErrInvalidParams.WithDetail("field", column)
		}
		target.Set(reflect.ValueOf(items))
	default:
		return bizerrors.ErrInvalidParams.WithDetail("field", column)
	}
	return nil
}

func isEmptyColumn(item *repository.Provider, column string) bool {
	value := readColumn(item, column)
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case int64:
		return typed == 0
	case int:
		return typed == 0
	case []string:
		return len(typed) == 0
	default:
		return value == nil
	}
}

func toString(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case []string:
		return strings.Join(typed, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, toString(item))
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", raw))
	}
}

func toBool(raw any) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		value, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && value
	default:
		return false
	}
}

func toInt(raw any) (int64, error) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case float64:
		return int64(typed), nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, nil
		}
		return strconv.ParseInt(trimmed, 10, 64)
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("不可转换为整数: %T", raw)
	}
}

func toStringSlice(raw any) ([]string, error) {
	switch typed := raw.(type) {
	case nil:
		return []string{}, nil
	case []string:
		return normalizeStrings(typed), nil
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, toString(item))
		}
		return normalizeStrings(parts), nil
	case string:
		return normalizeStrings(strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == '\n' })), nil
	default:
		return nil, fmt.Errorf("不可转换为字符串数组: %T", raw)
	}
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
