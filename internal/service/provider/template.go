package provider

// 短信模板变量（少数几个"平台确实能取到值"的内置变量）。
//
// 为什么需要它：阿里云对 TemplateParam 的键与模板内容里的变量是**严格匹配**的
// （多传、少传、名字对不上都会被直接拒绝），通用 HTTP 网关同理。此前发送侧写死
// `{"code": ...}`，于是任何带 `${minutes}` 之类变量的模板都发不出去。
// 现在由模板**声明**它用到哪些变量，发送时按声明装配。
//
// 变量集是封闭的：只有这里列出的名字可取到值，未列出的名字在落库时就被拒绝，
// 避免"配了但运行时取不到值 → 发送失败"。
type TemplateVar struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

// TemplateVars 内置变量全集（顺序即界面展示顺序）
func TemplateVars() []TemplateVar {
	return []TemplateVar{
		{Key: TemplateVarCode, Label: "验证码", Hint: "本次发送的验证码"},
		{Key: TemplateVarMinutes, Label: "有效分钟数", Hint: "取应用配置的验证码有效期"},
		{Key: TemplateVarPhone, Label: "接收号码", Hint: "收件手机号（含国家码按用户填写原样）"},
		{Key: TemplateVarInstitution, Label: "组织名称", Hint: "当前登录目标组织的展示名"},
		{Key: TemplateVarApplication, Label: "应用名称", Hint: "发起登录的应用展示名"},
	}
}

// 内置变量 key
const (
	TemplateVarCode        = "code"
	TemplateVarMinutes     = "minutes"
	TemplateVarPhone       = "phone"
	TemplateVarInstitution = "institution"
	TemplateVarApplication = "application"
)

// DefaultTemplateVars 未声明时的默认变量（与历史行为一致：只发验证码）
func DefaultTemplateVars() []string { return []string{TemplateVarCode} }

// IsKnownTemplateVar 变量名是否可取到值
func IsKnownTemplateVar(key string) bool {
	for _, item := range TemplateVars() {
		if item.Key == key {
			return true
		}
	}
	return false
}

// TemplateVarOptions 变量集转成表单下拉选项（目录声明与前端渲染共用）
func TemplateVarOptions() []FieldOption {
	vars := TemplateVars()
	options := make([]FieldOption, 0, len(vars))
	for _, item := range vars {
		options = append(options, FieldOption{Value: item.Key, Label: item.Label})
	}
	return options
}
