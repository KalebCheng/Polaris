package service

import (
	"strings"

	"polaris/internal/auth/oauth"
)

// providerFieldStrategy 把目录声明的「匹配提供商字段」映射到归一化 UserInfo。
// 未识别的字段由 matchProviderValue 返回空串，调用方据此拒绝登录。
//
// 采用策略模式而不是 switch：新增可匹配字段时只注册一个提取策略，
// 登录供给链路与目录选项都不需要再加分支。
type providerFieldStrategy interface {
	value(info *oauth.UserInfo) string
}

type userInfoFieldStrategy struct {
	pick func(*oauth.UserInfo) string
}

func (s userInfoFieldStrategy) value(info *oauth.UserInfo) string {
	if info == nil {
		return ""
	}
	return s.pick(info)
}

func infoField(pick func(*oauth.UserInfo) string) providerFieldStrategy {
	return userInfoFieldStrategy{pick: pick}
}

var providerFieldStrategies = map[string]providerFieldStrategy{
	"unionid":            infoField(func(info *oauth.UserInfo) string { return info.UnionID }),
	"union_id":           infoField(func(info *oauth.UserInfo) string { return info.UnionID }),
	"openid":             infoField(func(info *oauth.UserInfo) string { return info.ProviderUserID }),
	"open_id":            infoField(func(info *oauth.UserInfo) string { return info.ProviderUserID }),
	"sub":                infoField(func(info *oauth.UserInfo) string { return info.ProviderUserID }),
	"email":              infoField(func(info *oauth.UserInfo) string { return info.Email }),
	"mobile":             infoField(func(info *oauth.UserInfo) string { return info.Phone }),
	"phone_number":       infoField(func(info *oauth.UserInfo) string { return info.Phone }),
	"nick":               infoField(func(info *oauth.UserInfo) string { return info.DisplayName }),
	"nickname":           infoField(func(info *oauth.UserInfo) string { return info.DisplayName }),
	"name":               infoField(func(info *oauth.UserInfo) string { return info.DisplayName }),
	"preferred_username": infoField(func(info *oauth.UserInfo) string { return info.Username }),
	"username":           infoField(func(info *oauth.UserInfo) string { return info.Username }),
	"avatar":             infoField(func(info *oauth.UserInfo) string { return info.Avatar }),
	"display_name":       infoField(func(info *oauth.UserInfo) string { return info.DisplayName }),
	"provider_user_id":   infoField(func(info *oauth.UserInfo) string { return info.ProviderUserID }),
}

// matchProviderValue 按策略提取提供商字段；空字段或未知字段返回空串。
func matchProviderValue(info *oauth.UserInfo, field string) string {
	strategy, ok := providerFieldStrategies[strings.ToLower(strings.TrimSpace(field))]
	if !ok {
		return ""
	}
	return strategy.value(info)
}

// provisionSource 解析「自动建号字段映射」的来源字段：配置为空时回退到默认字段。
func provisionSource(configured, fallback string) string {
	if trimmed := strings.TrimSpace(configured); trimmed != "" {
		return trimmed
	}
	return fallback
}

// providerIdentityAttributes 把提供商返回的身份字段归一化为 attributes（键名与目录字段一致）。
//
// 供"同源身份连接"的反向认领使用：同步侧按连接源字段取值后，用其中同名键在本地找回同一用户，
// 从而让"先登录建号"与"先同步建号"收敛到同一个人，而不是各建一条。
func providerIdentityAttributes(info *oauth.UserInfo) map[string]string {
	out := map[string]string{}
	if info == nil {
		return out
	}
	for _, key := range []string{
		"unionid", "openid", "sub", "email", "mobile", "phone_number",
		"nick", "name", "preferred_username", "avatar", "display_name", "provider_user_id",
	} {
		if value := matchProviderValue(info, key); value != "" {
			out[key] = value
		}
	}
	return out
}
