package syncer

import (
	"context"
	"fmt"
	"net/url"

	"polaris/internal/repository"
)

// DingTalkConnector 钉钉通讯录连接器
type DingTalkConnector struct{}

// Type 连接器类型
func (c *DingTalkConnector) Type() string { return "dingtalk" }

func (c *DingTalkConnector) accessToken(ctx context.Context, s *repository.Syncer) (string, error) {
	u := fmt.Sprintf("https://oapi.dingtalk.com/gettoken?appkey=%s&appsecret=%s",
		url.QueryEscape(s.AppID), url.QueryEscape(s.AppSecret))
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := getJSON(ctx, u, nil, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 || resp.AccessToken == "" {
		return "", fmt.Errorf("钉钉获取 token 失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken, nil
}

// FetchUsers 拉取钉钉用户
func (c *DingTalkConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	token, err := c.accessToken(ctx, s)
	if err != nil {
		return nil, err
	}
	deptIDs, err := c.departments(ctx, token)
	if err != nil {
		return nil, err
	}
	var users []ExternalUser
	for _, dept := range deptIDs {
		deptID := dept.ID
		var cursor int64
		for {
			var resp struct {
				ErrCode int    `json:"errcode"`
				ErrMsg  string `json:"errmsg"`
				Result  struct {
					HasMore    bool  `json:"has_more"`
					NextCursor int64 `json:"next_cursor"`
					List       []struct {
						UserID     string  `json:"userid"`
						UnionID    string  `json:"unionid"`
						Name       string  `json:"name"`
						Mobile     string  `json:"mobile"`
						StateCode  string  `json:"state_code"` // 国际电话区号
						Telephone  string  `json:"telephone"`  // 分机号
						Email      string  `json:"email"`
						OrgEmail   string  `json:"org_email"` // 企业邮箱
						Avatar     string  `json:"avatar"`
						Title      string  `json:"title"`
						JobNumber  string  `json:"job_number"` // 工号
						WorkPlace  string  `json:"work_place"`
						DeptIDList []int64 `json:"dept_id_list"`
					} `json:"list"`
				} `json:"result"`
			}
			body := map[string]interface{}{"dept_id": deptID, "cursor": cursor, "size": 100}
			if err := postJSON(ctx, "https://oapi.dingtalk.com/topapi/v2/user/list?access_token="+url.QueryEscape(token), nil, body, &resp); err != nil {
				return nil, err
			}
			if resp.ErrCode != 0 {
				return nil, fmt.Errorf("钉钉拉取用户失败: %d %s", resp.ErrCode, resp.ErrMsg)
			}
			for _, u := range resp.Result.List {
				groupIDs := make([]string, 0, len(u.DeptIDList))
				for _, d := range u.DeptIDList {
					groupIDs = append(groupIDs, fmt.Sprintf("%d", d))
				}
				// 源字段集合：字段映射的候选来源（键名与目录里声明的 source_fields 一致）
				raw := map[string]string{
					"userid": u.UserID, "unionid": u.UnionID, "name": u.Name,
					"mobile": u.Mobile, "state_code": u.StateCode, "telephone": u.Telephone,
					"email": u.Email, "org_email": u.OrgEmail, "avatar": u.Avatar,
					"title": u.Title, "job_number": u.JobNumber, "work_place": u.WorkPlace,
				}
				mapping := s.UserMapping
				users = append(users, ExternalUser{
					ID:            PickMapped(mapping, raw, "id", "userid"),
					Username:      PickMapped(mapping, raw, "name", "userid"),
					Name:          PickMapped(mapping, raw, "displayName", "name"),
					Email:         PickMapped(mapping, raw, "email", "email"),
					Phone:         PickMapped(mapping, raw, "phone", "mobile"),
					Avatar:        PickMapped(mapping, raw, "avatar", "avatar"),
					Title:         PickMapped(mapping, raw, "title", "title"),
					Location:      PickMapped(mapping, raw, "location", "work_place"),
					DepartmentIDs: groupIDs,
				})
			}
			if !resp.Result.HasMore {
				break
			}
			cursor = resp.Result.NextCursor
		}
	}
	return users, nil
}

// deptNode 钉钉部门节点（遍历时天然知道父部门，用于还原组织层级）
type deptNode struct {
	ID       int64
	ParentID int64
	Name     string
}

func (c *DingTalkConnector) departments(ctx context.Context, token string) ([]deptNode, error) {
	nodes := []deptNode{{ID: 1, ParentID: 0, Name: "1"}}
	queue := []int64{1}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		var resp struct {
			ErrCode int    `json:"errcode"`
			ErrMsg  string `json:"errmsg"`
			Result  []struct {
				DeptID int64  `json:"dept_id"`
				Name   string `json:"name"`
			} `json:"result"`
		}
		body := map[string]interface{}{"dept_id": parent}
		if err := postJSON(ctx, "https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token="+url.QueryEscape(token), nil, body, &resp); err != nil {
			// 无权限时退化为根部门
			return nodes, nil
		}
		if resp.ErrCode != 0 {
			return nodes, nil
		}
		for _, d := range resp.Result {
			nodes = append(nodes, deptNode{ID: d.DeptID, ParentID: parent, Name: d.Name})
			queue = append(queue, d.DeptID)
		}
	}
	return nodes, nil
}

// FetchGroups 拉取钉钉部门
func (c *DingTalkConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	token, err := c.accessToken(ctx, s)
	if err != nil {
		return nil, err
	}
	deptIDs, err := c.departments(ctx, token)
	if err != nil {
		return nil, err
	}
	groups := make([]ExternalGroup, 0, len(deptIDs))
	for _, node := range deptIDs {
		parent := ""
		if node.ParentID > 0 {
			parent = fmt.Sprintf("%d", node.ParentID)
		}
		name := node.Name
		if name == "" {
			name = fmt.Sprintf("%d", node.ID)
		}
		groups = append(groups, ExternalGroup{
			ID: fmt.Sprintf("%d", node.ID), Name: name, ParentID: parent,
		})
	}
	return groups, nil
}
