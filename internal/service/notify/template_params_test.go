package notify

import (
	"testing"
)

// 回归：模板变量必须"按声明装配"。
//
// 阿里云 SendSms 的 TemplateParam 与模板内容里的 ${变量} 是严格匹配的：多一个、少一个、
// 名字对不上都会被直接拒绝。此前发送侧写死 {"code": ...}，导致带 ${minutes} 的模板发不出去。
func TestTemplateParamsFollowDeclaration(t *testing.T) {
	msg := Message{
		To: "13800000000", Code: "123456",
		Vars: map[string]string{"minutes": "5", "institution": "内置组织"},
	}

	// 未声明（历史数据）→ 只发 code，保持向后兼容
	legacy := templateParams(Config{}, msg)
	if len(legacy) != 1 || legacy["code"] != "123456" {
		t.Fatalf("未声明变量时应只发 code: %+v", legacy)
	}

	// 声明 code + minutes → 只发这两个，且取到真实值
	declared := templateParams(Config{TemplateParams: []string{"code", "minutes"}}, msg)
	if len(declared) != 2 || declared["code"] != "123456" || declared["minutes"] != "5" {
		t.Fatalf("按声明装配失败: %+v", declared)
	}
	if _, sent := declared["institution"]; sent {
		t.Fatal("未声明的变量不得出现在 TemplateParam 里（阿里云会拒绝）")
	}

	// 声明的变量在本次发送里没有取值 → 不放入（避免发出空值导致厂商报错）
	empty := templateParams(Config{TemplateParams: []string{"code", "application"}}, msg)
	if _, sent := empty["application"]; sent {
		t.Fatalf("取不到值的变量不应下发: %+v", empty)
	}
}

// 通用 HTTP 网关：固定字段保持历史契约，声明的额外变量追加进来。
func TestSMSGatewayPayloadAddsDeclaredVars(t *testing.T) {
	cfg := Config{SignName: "Polaris", TemplateCode: "SMS_1", TemplateParams: []string{"code", "minutes"}}
	msg := Message{To: "13800000000", Code: "123456", Vars: map[string]string{"minutes": "5"}}

	payload := smsGatewayPayload(cfg, msg)
	for key, want := range map[string]string{
		"phone": "13800000000", "code": "123456", "signName": "Polaris", "template": "SMS_1", "minutes": "5",
	} {
		if payload[key] != want {
			t.Fatalf("payload[%s] = %q, want %q（完整 payload: %+v）", key, payload[key], want, payload)
		}
	}
	if len(payload) != 5 {
		t.Fatalf("不应出现未声明的键: %+v", payload)
	}
}
