package syncer

import (
	"context"
	"fmt"
	"net/url"

	"polaris/internal/repository"
)

// FeishuConnector 飞书（Lark）通讯录连接器
type FeishuConnector struct{}

// Type 连接器类型
func (c *FeishuConnector) Type() string { return "lark" }

func (c *FeishuConnector) tenantToken(ctx context.Context, s *repository.Syncer) (string, error) {
	var resp struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
	}
	body := map[string]string{"app_id": s.AppID, "app_secret": s.AppSecret}
	if err := postJSON(ctx, "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal", nil, body, &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 || resp.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书获取 tenant_access_token 失败: %d %s", resp.Code, resp.Msg)
	}
	return resp.TenantAccessToken, nil
}

// FetchUsers 拉取飞书用户
func (c *FeishuConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	token, err := c.tenantToken(ctx, s)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	var users []ExternalUser
	pageToken := ""
	for {
		u := "https://open.feishu.cn/open-apis/contact/v3/users?page_size=50&department_id_type=open_department_id&user_id_type=open_id"
		if pageToken != "" {
			u += "&page_token=" + url.QueryEscape(pageToken)
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
				Items     []struct {
					OpenID     string `json:"open_id"`
					UnionID    string `json:"union_id"`
					UserID     string `json:"user_id"`
					Name       string `json:"name"`
					EnName     string `json:"en_name"`
					EmployeeNo string `json:"employee_no"` // 工号
					Email      string `json:"email"`
					Mobile     string `json:"mobile"`
					Avatar     struct {
						Avatar72 string `json:"avatar_72"`
					} `json:"avatar"`
					DepartmentIDs []string `json:"department_ids"`
					JobTitle      string   `json:"job_title"`
					City          string   `json:"city"`
				} `json:"items"`
			} `json:"data"`
		}
		if err := getJSON(ctx, u, headers, &resp); err != nil {
			return nil, err
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("飞书拉取用户失败: %d %s", resp.Code, resp.Msg)
		}
		for _, it := range resp.Data.Items {
			id := it.UnionID
			if id == "" {
				id = it.OpenID
			}
			raw := map[string]string{
				"user_id": it.UserID, "name": it.Name, "en_name": it.EnName,
				"mobile": it.Mobile, "email": it.Email, "avatar": it.Avatar.Avatar72,
				"title": it.JobTitle, "location": it.City, "job_number": it.EmployeeNo,
			}
			mapping := s.UserMapping
			users = append(users, ExternalUser{
				ID:            PickMapped(mapping, raw, "id", "user_id"),
				Username:      PickMapped(mapping, raw, "name", "user_id"),
				Name:          PickMapped(mapping, raw, "displayName", "name"),
				Email:         PickMapped(mapping, raw, "email", "email"),
				Phone:         PickMapped(mapping, raw, "phone", "mobile"),
				Avatar:        PickMapped(mapping, raw, "avatar", "avatar"),
				Title:         PickMapped(mapping, raw, "title", "title"),
				Location:      PickMapped(mapping, raw, "location", "location"),
				DepartmentIDs: it.DepartmentIDs,
			})
		}
		if !resp.Data.HasMore || resp.Data.PageToken == "" {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return users, nil
}

// FetchGroups 拉取飞书部门
func (c *FeishuConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	token, err := c.tenantToken(ctx, s)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Authorization": "Bearer " + token}
	var groups []ExternalGroup
	pageToken := ""
	for {
		u := "https://open.feishu.cn/open-apis/contact/v3/departments?page_size=50&department_id_type=open_department_id&fetch_child=true"
		if pageToken != "" {
			u += "&page_token=" + url.QueryEscape(pageToken)
		}
		var resp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				HasMore   bool   `json:"has_more"`
				PageToken string `json:"page_token"`
				Items     []struct {
					OpenDepartmentID   string `json:"open_department_id"`
					ParentDepartmentID string `json:"parent_department_id"`
					Name               string `json:"name"`
				} `json:"items"`
			} `json:"data"`
		}
		if err := getJSON(ctx, u, headers, &resp); err != nil {
			return nil, err
		}
		if resp.Code != 0 {
			return nil, fmt.Errorf("飞书拉取部门失败: %d %s", resp.Code, resp.Msg)
		}
		for _, it := range resp.Data.Items {
			groups = append(groups, ExternalGroup{
				ID: it.OpenDepartmentID, Name: it.Name, ParentID: it.ParentDepartmentID,
			})
		}
		if !resp.Data.HasMore || resp.Data.PageToken == "" {
			break
		}
		pageToken = resp.Data.PageToken
	}
	return groups, nil
}
