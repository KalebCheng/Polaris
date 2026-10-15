package syncer

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"polaris/internal/repository"
)

// SCIMConnector SCIM 2.0 连接器（Base URL 取自 Syncer.Host，Bearer Token 取自 AppSecret）。
type SCIMConnector struct{}

// Type 连接器类型
func (c *SCIMConnector) Type() string { return "scim" }

func (c *SCIMConnector) baseURL(s *repository.Syncer) string {
	base := s.Host
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return strings.TrimSuffix(base, "/")
}

// FetchUsers 拉取 SCIM 用户
func (c *SCIMConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	headers := map[string]string{"Authorization": "Bearer " + s.AppSecret}
	startIndex := 1
	var users []ExternalUser
	for {
		u := fmt.Sprintf("%s/Users?startIndex=%d&count=100", c.baseURL(s), startIndex)
		var resp struct {
			TotalResults int `json:"totalResults"`
			ItemsPerPage int `json:"itemsPerPage"`
			Resources    []struct {
				ID       string `json:"id"`
				UserName string `json:"userName"`
				Name     struct {
					Formatted  string `json:"formatted"`
					GivenName  string `json:"givenName"`
					FamilyName string `json:"familyName"`
				} `json:"name"`
				DisplayName string `json:"displayName"`
				Emails      []struct {
					Value   string `json:"value"`
					Primary bool   `json:"primary"`
				} `json:"emails"`
				PhoneNumbers []struct {
					Value   string `json:"value"`
					Primary bool   `json:"primary"`
				} `json:"phoneNumbers"`
				Photos []struct {
					Value   string `json:"value"`
					Primary bool   `json:"primary"`
				} `json:"photos"`
				Title  string `json:"title"`
				Groups []struct {
					Value   string `json:"value"`
					Display string `json:"display"`
				} `json:"groups"`
			} `json:"Resources"`
		}
		if err := getJSON(ctx, u, headers, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Resources {
			email := ""
			for _, e := range r.Emails {
				if e.Value != "" && (e.Primary || email == "") {
					email = e.Value
				}
			}
			phone := ""
			for _, p := range r.PhoneNumbers {
				if p.Value != "" && (p.Primary || phone == "") {
					phone = p.Value
				}
			}
			avatar := ""
			for _, p := range r.Photos {
				if p.Value != "" && (p.Primary || avatar == "") {
					avatar = p.Value
				}
			}
			name := r.DisplayName
			if name == "" {
				name = r.Name.Formatted
			}
			if name == "" {
				name = strings.TrimSpace(r.Name.GivenName + " " + r.Name.FamilyName)
			}
			groupIDs := make([]string, 0, len(r.Groups))
			for _, g := range r.Groups {
				groupIDs = append(groupIDs, g.Value)
			}
			users = append(users, ExternalUser{
				ID: r.ID, Username: r.UserName, Name: name, Email: email, Phone: phone,
				Avatar: avatar, Title: r.Title, DepartmentIDs: groupIDs,
			})
		}
		if len(resp.Resources) == 0 || startIndex+len(resp.Resources) > resp.TotalResults {
			break
		}
		startIndex += len(resp.Resources)
	}
	return users, nil
}

// FetchGroups 拉取 SCIM 用户组
func (c *SCIMConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	headers := map[string]string{"Authorization": "Bearer " + s.AppSecret}
	u := fmt.Sprintf("%s/Groups?startIndex=1&count=100", c.baseURL(s))
	var resp struct {
		Resources []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Members     []struct {
				Value string `json:"value"`
			} `json:"members"`
		} `json:"Resources"`
	}
	if err := getJSON(ctx, u, headers, &resp); err != nil {
		return nil, err
	}
	groups := make([]ExternalGroup, 0, len(resp.Resources))
	for _, g := range resp.Resources {
		var members []string
		for _, m := range g.Members {
			members = append(members, m.Value)
		}
		groups = append(groups, ExternalGroup{ID: g.ID, Name: g.DisplayName, MemberIDs: members})
	}
	return groups, nil
}

// unused 保留 url 依赖（SCIM 分页参数拼接）
var _ = url.Values{}
