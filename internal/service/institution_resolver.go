package service

import (
	"context"
	"strings"

	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
)

// 租户解析（登录时用）：
// 管理端登录页不再强制用户先选组织——只要能唯一确定身份归属，就由后端自动解析。
// 解析顺序：
//  1. 调用方显式指定租户 → 直接使用；
//  2. 请求 Host 命中某租户域名（institutions.domain）→ 使用该租户（多租户按域名入口，ADR D3）；
//  3. 平台仅有一个租户 → 使用该租户（绝大多数私有化部署的情形）；
//  4. 跨租户按账号名 / 邮箱 / 手机号查找：唯一命中 → 使用其所属租户（优先于默认租户）；
//  5. 配置了默认租户（auth.oauth.default_institution）→ 使用该租户；
//  6. 命中多个 → 返回 INSTITUTION_REQUIRED，由前端提示用户选择组织。
const orgResolveLimit = 5

// chooseInstitution 纯函数：依据候选信息决定目标组织。
// matches 为跨组织查找到的候选数量，firstMatch 为第一个候选的组织名。
func chooseInstitution(requested string, orgTotal int64, defaultOrg string, matches int, firstMatch string) (string, bool) {
	if requested != "" {
		return requested, true
	}
	if orgTotal == 1 && firstMatch != "" {
		return firstMatch, true
	}
	// 唯一命中优先于默认组织：用户明确只属于某个组织时按其归属解析，
	// 否则多租户部署下非默认组织的用户会被默认组织"截胡"而无法免选登录。
	if matches == 1 {
		return firstMatch, true
	}
	if defaultOrg != "" {
		return defaultOrg, true
	}
	// 0 个命中无法定位；>1 个命中存在歧义
	return "", false
}

// normalizeHost 去除端口并小写化，用于域名匹配（域名场景下 IPv6 字面量无意义，仅做规范化）。
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	// IPv6 字面量（可能带方括号）：取括号内主机部分
	if strings.HasPrefix(host, "[") {
		if i := strings.Index(host, "]"); i > 0 {
			return host[1:i]
		}
	}
	// host:port → 去掉端口
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}

// resolveLoginInstitution 解析登录目标租户。
func (s *AuthService) resolveLoginInstitution(ctx context.Context, requestedID int64, requested, host, identifier string) (string, error) {
	// 0. 主键优先：公开面以 institution_id 为权威引用（§7.4 引用一律用主键 id）。
	//    给了 id 就必须按它解析——解析失败直接报错，**不回落**到自动解析，
	//    否则"传错组织"会静默登进别的组织，非常难排查。
	if requestedID != 0 {
		org, err := s.orgs.GetByID(ctx, requestedID)
		if err != nil || org == nil {
			return "", bizerrors.ErrNotFound.WithDetail("institution_id", requestedID)
		}
		return org.Name, nil
	}
	if requested != "" {
		return requested, nil
	}
	// 2. 按域名解析租户（多租户按域名入口）
	if h := normalizeHost(host); h != "" {
		if inst, err := s.orgs.GetByDomain(ctx, h); err == nil && inst.Name != "" {
			return inst.Name, nil
		}
	}
	total, err := s.orgs.Count(ctx, repository.ListQuery{})
	if err != nil {
		return "", err
	}
	defaultOrg := s.oauthCfg.DefaultInstitution
	if defaultOrg != "" {
		if _, err := s.orgs.GetByName(ctx, defaultOrg); err != nil {
			defaultOrg = ""
		}
	}

	// 跨组织按账号名 / 邮箱 / 手机号查找（取第一个命中的组织）
	matches := 0
	firstMatch := ""
	collect := func(users []*repository.User) {
		for _, user := range users {
			matches++
			if firstMatch == "" {
				// 命中用户的所属组织：按 id 取回组织名（对外仍是组织标识）
				if org, err := s.orgs.GetByID(ctx, user.InstitutionID); err == nil {
					firstMatch = org.Name
				}
			}
		}
	}
	if users, err := s.users.FindByNameAcrossInstitutions(ctx, identifier, orgResolveLimit); err == nil {
		collect(users)
	}
	if matches == 0 && strings.Contains(identifier, "@") {
		if users, err := s.users.FindByEmailAcrossInstitutions(ctx, identifier, orgResolveLimit); err == nil {
			collect(users)
		}
	}
	if matches == 0 {
		if users, err := s.users.FindByPhoneAcrossInstitutions(ctx, identifier, orgResolveLimit); err == nil {
			collect(users)
		}
	}

	org, ok := chooseInstitution("", total, defaultOrg, matches, firstMatch)
	if !ok || org == "" {
		return "", bizerrors.ErrInstitutionRequired
	}
	return org, nil
}
