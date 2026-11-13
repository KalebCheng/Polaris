package provider

import "testing"

func TestTemplateScenesDeclareRequiredPlaceholders(t *testing.T) {
	scenes := TemplateScenes()
	if len(scenes) != 4 {
		t.Fatalf("场景数量 = %d, want 4", len(scenes))
	}
	for _, scene := range scenes {
		required := 0
		for _, variable := range scene.Variables {
			if !IsKnownTemplateVar(variable.Key) {
				t.Fatalf("%s/%s 声明了未知占位符 %s", scene.Channel, scene.Scene, variable.Key)
			}
			if variable.Required {
				required++
			}
		}
		if required != 1 {
			t.Fatalf("%s/%s 必须且只能声明一个必填占位符，实际 %d", scene.Channel, scene.Scene, required)
		}
		if got := RequiredTemplateVar(scene.Channel, scene.Scene); got == "" {
			t.Fatalf("%s/%s 缺少必填占位符", scene.Channel, scene.Scene)
		}
		if !TemplateSceneAllowed(scene.Channel, scene.Scene) {
			t.Fatalf("声明的场景未被识别: %s/%s", scene.Channel, scene.Scene)
		}
	}
}

func TestTemplateVarsAreScopedByScene(t *testing.T) {
	if !TemplateVarAllowed("email", "login_code", TemplateVarCode) {
		t.Fatal("邮件登录验证码应允许 {{code}}")
	}
	if TemplateVarAllowed("email", "login_code", TemplateVarLink) {
		t.Fatal("邮件登录验证码不应允许 {{link}}")
	}
	if !TemplateVarAllowed("email", "magic_link", TemplateVarLink) {
		t.Fatal("魔法链接场景应允许 {{link}}")
	}
	if TemplateVarAllowed("email", "magic_link", TemplateVarCode) {
		t.Fatal("魔法链接场景不应允许 {{code}}")
	}
	if !TemplateVarAllowed("sms", "mfa_code", TemplateVarPhone) {
		t.Fatal("短信 MFA 场景应允许 {{phone}}")
	}
	if TemplateVarAllowed("sms", "mfa_code", TemplateVarLink) {
		t.Fatal("短信 MFA 场景不应允许 {{link}}")
	}
}
