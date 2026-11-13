package provider

// 短信模板变量（少数几个"平台确实能取到值"的内置变量）。
//
// 为什么需要它：阿里云对 TemplateParam 的键与模板内容里的变量是**严格匹配**的
// （多传、少传、名字对不上都会被直接拒绝），通用 HTTP 网关同理。此前发送侧写死
// `{"code": ...}`，于是任何带 `${minutes}` 之类变量的模板都发不出去。
// 现在由模板声明"厂商变量名 → 平台占位符"的映射，发送时按映射装配。
//
// 变量集是封闭的，并且按渠道/场景收敛；未列出或不属于当前场景的名字在落库时就被拒绝，
// 避免"配了但运行时取不到值 → 发送失败"。
type TemplateVar struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Hint     string `json:"hint,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// TemplateVars 内置变量全集（顺序即界面展示顺序）
func TemplateVars() []TemplateVar {
	return []TemplateVar{
		{Key: TemplateVarCode, Label: "验证码", Hint: "本次发送的验证码"},
		{Key: TemplateVarLink, Label: "魔法链接", Hint: "仅魔法链接场景可用：一次性登录地址"},
		{Key: TemplateVarMinutes, Label: "有效分钟数", Hint: "取应用配置的验证码有效期"},
		{Key: TemplateVarPhone, Label: "接收号码", Hint: "收件手机号（含国家码按用户填写原样）"},
		{Key: TemplateVarInstitution, Label: "组织名称", Hint: "当前登录目标组织的展示名"},
		{Key: TemplateVarApplication, Label: "应用名称", Hint: "发起登录的应用展示名"},
	}
}

// TemplateSceneInfo 声明一个真实发送场景可用的占位符集合。
//
// 占位符必须按场景收敛：例如登录验证码场景拿不到 link，魔法链接场景拿不到 code。
// 界面只展示这些集合，落库前也按同一份声明校验，避免"配了但运行时取不到值"。
type TemplateSceneInfo struct {
	Channel   string        `json:"channel"`
	Scene     string        `json:"scene"`
	Label     string        `json:"label"`
	Variables []TemplateVar `json:"variables"`
}

type templateSceneDefinition struct {
	channel  string
	scene    string
	label    string
	varKeys  []string
	required string
}

var templateSceneDefinitions = []templateSceneDefinition{
	{
		channel: string(CategoryEmail), scene: "login_code", label: "邮件登录验证码",
		varKeys:  []string{TemplateVarCode, TemplateVarMinutes, TemplateVarInstitution, TemplateVarApplication},
		required: TemplateVarCode,
	},
	{
		channel: string(CategoryEmail), scene: "magic_link", label: "邮件魔法链接",
		varKeys:  []string{TemplateVarLink, TemplateVarMinutes, TemplateVarInstitution, TemplateVarApplication},
		required: TemplateVarLink,
	},
	{
		channel: string(CategorySMS), scene: "login_code", label: "短信登录验证码",
		varKeys: []string{
			TemplateVarCode, TemplateVarMinutes, TemplateVarPhone,
			TemplateVarInstitution, TemplateVarApplication,
		},
		required: TemplateVarCode,
	},
	{
		channel: string(CategorySMS), scene: "mfa_code", label: "短信 MFA 验证码",
		varKeys: []string{
			TemplateVarCode, TemplateVarMinutes, TemplateVarPhone,
			TemplateVarInstitution, TemplateVarApplication,
		},
		required: TemplateVarCode,
	},
}

// TemplateScenes 返回全部渠道/场景及其可用占位符。
func TemplateScenes() []TemplateSceneInfo {
	out := make([]TemplateSceneInfo, 0, len(templateSceneDefinitions))
	for _, definition := range templateSceneDefinitions {
		vars := make([]TemplateVar, 0, len(definition.varKeys))
		for _, key := range definition.varKeys {
			item, ok := templateVarByKey(key)
			if !ok {
				continue
			}
			item.Required = key == definition.required
			vars = append(vars, item)
		}
		out = append(out, TemplateSceneInfo{
			Channel: definition.channel, Scene: definition.scene,
			Label: definition.label, Variables: vars,
		})
	}
	return out
}

// TemplateVarsFor 返回某渠道/场景可用的占位符；未知场景返回空集合。
func TemplateVarsFor(channel, scene string) []TemplateVar {
	for _, item := range TemplateScenes() {
		if item.Channel == channel && item.Scene == scene {
			return item.Variables
		}
	}
	return []TemplateVar{}
}

// TemplateSceneAllowed 判断渠道与场景的组合是否受支持。
func TemplateSceneAllowed(channel, scene string) bool {
	for _, item := range TemplateScenes() {
		if item.Channel == channel && item.Scene == scene {
			return true
		}
	}
	return false
}

// TemplateVarAllowed 判断占位符是否属于该渠道/场景的可取值集合。
func TemplateVarAllowed(channel, scene, key string) bool {
	for _, item := range TemplateVarsFor(channel, scene) {
		if item.Key == key {
			return true
		}
	}
	return false
}

// RequiredTemplateVar 返回该场景必须被引用的占位符。
func RequiredTemplateVar(channel, scene string) string {
	for _, definition := range templateSceneDefinitions {
		if definition.channel == channel && definition.scene == scene {
			return definition.required
		}
	}
	return ""
}

func templateVarByKey(key string) (TemplateVar, bool) {
	for _, item := range TemplateVars() {
		if item.Key == key {
			return item, true
		}
	}
	return TemplateVar{}, false
}

// 内置变量 key
const (
	TemplateVarCode        = "code"
	TemplateVarLink        = "link"
	TemplateVarMinutes     = "minutes"
	TemplateVarPhone       = "phone"
	TemplateVarInstitution = "institution"
	TemplateVarApplication = "application"
)

// IsKnownTemplateVar 变量名是否可取到值
func IsKnownTemplateVar(key string) bool {
	for _, item := range TemplateVars() {
		if item.Key == key {
			return true
		}
	}
	return false
}
