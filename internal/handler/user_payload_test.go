package handler

import (
	"reflect"
	"testing"

	"polaris/internal/service"
)

// jsonTags 收集结构体的 JSON 字段名。
func jsonTags(v interface{}) map[string]bool {
	out := map[string]bool{}
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := tag
		for idx, ch := range tag {
			if ch == ',' {
				name = tag[:idx]
				break
			}
		}
		out[name] = true
	}
	return out
}

// 回归：新建 DTO 与编辑入参的**资料字段集合必须一致**。
//
// 事故形态：表单能填、后端静默丢弃（第四阶段"新增连接器配置字段被丢掉"同类）。
// 只要有人往一侧加了字段而忘了另一侧，这条测试就红。
func TestCreateUserRequestCoversProfileFields(t *testing.T) {
	createTags := jsonTags(createUserRequest{})
	updateTags := jsonTags(service.UpdateUserProfileInput{})

	// 创建独有的字段（标识 / 归属 / 初始凭证）
	createOnly := map[string]bool{
		"institution_id": true,
		"name":           true,
		"department_id":  true,
		"is_admin":       true,
		"password":       true,
	}

	for tag := range updateTags {
		if !createTags[tag] {
			t.Errorf("资料字段 %q 只出现在编辑入参里：新建表单填了也会被丢弃", tag)
		}
	}
	for tag := range createTags {
		if createOnly[tag] {
			continue
		}
		if !updateTags[tag] {
			t.Errorf("资料字段 %q 只出现在新建 DTO 里：创建后无法在编辑页修改", tag)
		}
	}
}
