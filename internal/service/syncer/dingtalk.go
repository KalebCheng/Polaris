package syncer

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"polaris/internal/repository"
)

// DingTalkConnector 钉钉通讯录连接器
type DingTalkConnector struct{}

// dingtalkUser 钉钉用户项（官方 /topapi/v2/user/list 的字段）。
type dingtalkUser struct {
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
}

// dingTalkUserSourceFieldKeys 钉钉用户可映射源字段键全集（目录声明的对照基准）。
var dingTalkUserSourceFieldKeys = []string{
	"userid", "unionid", "name", "mobile", "state_code", "telephone",
	"email", "org_email", "avatar", "title", "job_number", "work_place", "dept_id_list",
}

// DingTalkUserSourceFieldKeys 返回可映射源字段键全集（目录侧测试据此双向锁定，杜绝"声明了取不到"）。
func DingTalkUserSourceFieldKeys() []string {
	out := make([]string, len(dingTalkUserSourceFieldKeys))
	copy(out, dingTalkUserSourceFieldKeys)
	return out
}

// dingtalkUserRaw 把一条钉钉用户记录展开为映射取值的 raw 集合：**每个候选键都存在**。
// 部门按官方字段名 dept_id_list 进 raw，才能被字段映射真实取到（此前是硬编码赋值）。
func dingtalkUserRaw(u dingtalkUser) map[string]string {
	deptIDs := make([]string, 0, len(u.DeptIDList))
	for _, d := range u.DeptIDList {
		deptIDs = append(deptIDs, fmt.Sprintf("%d", d))
	}
	return map[string]string{
		"userid": u.UserID, "unionid": u.UnionID, "name": u.Name,
		"mobile": u.Mobile, "state_code": u.StateCode, "telephone": u.Telephone,
		"email": u.Email, "org_email": u.OrgEmail, "avatar": u.Avatar,
		"title": u.Title, "job_number": u.JobNumber, "work_place": u.WorkPlace,
		"dept_id_list": strings.Join(deptIDs, ","),
	}
}

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
					HasMore    bool           `json:"has_more"`
					NextCursor int64          `json:"next_cursor"`
					List       []dingtalkUser `json:"list"`
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
				raw := dingtalkUserRaw(u)
				accountMapping := s.AccountMapping
				jobTitleMapping := s.JobTitleMapping
				users = append(users, ExternalUser{
					Raw:           raw,
					ID:            PickMapped(accountMapping, raw, "id", "userid"),
					Username:      PickMapped(accountMapping, raw, "name", "userid"),
					Name:          PickMapped(accountMapping, raw, "displayName", "name"),
					Email:         PickMapped(accountMapping, raw, "email", "email"),
					Phone:         PickMapped(accountMapping, raw, "phone", "mobile"),
					Avatar:        PickMapped(accountMapping, raw, "avatar", "avatar"),
					Title:         PickMapped(accountMapping, raw, "title", "title"),
					JobTitle:      PickJobTitle(accountMapping, jobTitleMapping, raw, "title"),
					Location:      PickMapped(accountMapping, raw, "location", "work_place"),
					DepartmentIDs: PickDepartments(raw, "dept_id_list"),
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
	// IsRoot 源端根部门占位（钉钉 dept_id=1、name="1"）：不落库为部门（Spec Q4.5）
	IsRoot bool
}

func (c *DingTalkConnector) departments(ctx context.Context, token string) ([]deptNode, error) {
	nodes := []deptNode{{ID: 1, ParentID: 0, Name: "1", IsRoot: true}}
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
			ID: fmt.Sprintf("%d", node.ID), Name: name, ParentID: parent, IsRoot: node.IsRoot,
			Raw: map[string]string{"dept_id": fmt.Sprintf("%d", node.ID), "name": node.Name},
		})
	}
	return groups, nil
}
