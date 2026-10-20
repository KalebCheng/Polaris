package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 回归：目录声明的每个字段都必须有中英词条，且词条**必须是字符串**。
//
// 两个真实教训：
//  1. 曾出现 `providers.field.name` 被写成对象（`{name, nameHint}`），i18next 直接报
//     "returned an object instead of string"，界面把报错原文当成字段标签显示；
//  2. 词条缺失时前端会回退到目录里的**中文** label，英文界面因此混入中文。
//
// 之所以放在后端测试里：目录是词条 key 的声明处（catalog ↔ locale 必须同步），
// 且前端测试若用 `i18n.t(key)` 断言，坏掉时两边会拿到同一个报错字符串而"自证通过"。
func TestCatalogLabelsHaveI18nEntries(t *testing.T) {
	locales := map[string]string{
		"zh": "../../../frontend/web/src/shared/i18n/locales/zh.json",
		"en": "../../../frontend/web/src/shared/i18n/locales/en.json",
	}
	for locale, path := range locales {
		root := readLocale(t, path)
		providers, ok := root["providers"].(map[string]any)
		if !ok {
			t.Fatalf("%s: 缺少 providers 命名空间", locale)
		}
		categories, _ := providers["category"].(map[string]any)
		fields, _ := providers["field"].(map[string]any)
		if fields == nil {
			t.Fatalf("%s: 缺少 providers.field 命名空间", locale)
		}
		for _, category := range Categories() {
			requireString(t, locale, "providers.category."+string(category.Key), categories[string(category.Key)])
			for _, info := range category.Types {
				// 类型专属命名空间（如 providers.field.local）：有则其中每一项都必须是字符串
				scoped, _ := fields[info.Key].(map[string]any)
				for key, value := range scoped {
					requireString(t, locale, "providers.field."+info.Key+"."+key, value)
				}
				for _, field := range info.Fields {
					// 与前端同一套回退顺序：类型专属 → 通用（两者都没有则界面会回退目录中文）
					if scoped != nil {
						if _, ok := scoped[field.Key]; ok {
							continue
						}
					}
					requireString(t, locale, "providers.field."+field.Key, fields[field.Key])
					if strings.TrimSpace(field.Hint) != "" {
						if _, ok := scoped[field.Key+"Hint"]; ok {
							continue
						}
						requireString(t, locale, "providers.field."+field.Key+"Hint", fields[field.Key+"Hint"])
					}
					if field.Required && strings.TrimSpace(field.Label) == "" {
						t.Errorf("类型 %s 的必填字段 %s 缺少 label（目录声明错误）", info.Key, field.Key)
					}
				}
			}
		}
	}
}

func requireString(t *testing.T, locale, key string, value any) {
	t.Helper()
	if value == nil {
		t.Errorf("%s: 缺少词条 %s（界面会回退目录中文或显示原始 key）", locale, key)
		return
	}
	text, ok := value.(string)
	if !ok {
		t.Errorf("%s: 词条 %s 必须是字符串，实际 %T（i18next 会报 returned an object instead of string）", locale, key, value)
		return
	}
	if strings.TrimSpace(text) == "" {
		t.Errorf("%s: 词条 %s 为空", locale, key)
	}
}

func readLocale(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("读取词条失败（%s）: %v", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("解析词条失败（%s）: %v", path, err)
	}
	return root
}
