package syncer

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"polaris/internal/repository"
)

// WeComConnector 企业微信通讯录连接器
type WeComConnector struct{}

// wecomUser 企业微信成员项（官方 /cgi-bin/user/list 的字段）。
type wecomUser struct {
	UserID         string `json:"userid"`
	Name           string `json:"name"`
	Alias          string `json:"alias"`
	Mobile         string `json:"mobile"`
	Telephone      string `json:"telephone"`
	Address        string `json:"address"`
	Email          string `json:"email"`
	Avatar         string `json:"avatar"`
	Position       string `json:"position"`
	Department     []int  `json:"department"`
	MainDepartment int    `json:"main_department"`
}

// wecomUserSourceFieldKeys 企业微信可映射源字段键全集（目录声明的对照基准）。
var wecomUserSourceFieldKeys = []string{
	"userid", "name", "alias", "mobile", "telephone", "email", "title",
	"avatar", "address", "department", "main_department",
}

// WeComUserSourceFieldKeys 返回可映射源字段键全集（目录侧测试据此双向锁定）。
func WeComUserSourceFieldKeys() []string {
	out := make([]string, len(wecomUserSourceFieldKeys))
	copy(out, wecomUserSourceFieldKeys)
	return out
}

// wecomUserRaw 把一条企业微信成员记录展开为映射取值的 raw 集合。
// 部门按官方字段名 department / main_department 进 raw（此前是硬编码赋值，选了也取不到）。
func wecomUserRaw(it wecomUser) map[string]string {
	deptIDs := make([]string, 0, len(it.Department))
	for _, d := range it.Department {
		deptIDs = append(deptIDs, fmt.Sprintf("%d", d))
	}
	return map[string]string{
		"userid": it.UserID, "name": it.Name, "alias": it.Alias, "mobile": it.Mobile,
		"telephone": it.Telephone, "address": it.Address, "email": it.Email,
		"avatar": it.Avatar, "title": it.Position,
		"department": strings.Join(deptIDs, ","), "main_department": fmt.Sprintf("%d", it.MainDepartment),
	}
}

// Type 连接器类型
func (c *WeComConnector) Type() string { return "wecom" }

func (c *WeComConnector) accessToken(ctx context.Context, s *repository.Syncer) (string, error) {
	corpID := s.CorpID
	if corpID == "" {
		corpID = s.AppID
	}
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s",
		url.QueryEscape(corpID), url.QueryEscape(s.AppSecret))
	var resp struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
	}
	if err := getJSON(ctx, u, nil, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 || resp.AccessToken == "" {
		return "", fmt.Errorf("企业微信获取 token 失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.AccessToken, nil
}

// FetchUsers 拉取企业微信成员
func (c *WeComConnector) FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error) {
	token, err := c.accessToken(ctx, s)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/user/list?access_token=%s&department_id=1&fetch_child=1",
		url.QueryEscape(token))
	var resp struct {
		ErrCode  int         `json:"errcode"`
		ErrMsg   string      `json:"errmsg"`
		UserList []wecomUser `json:"userlist"`
	}
	if err := getJSON(ctx, u, nil, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("企业微信拉取用户失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	users := make([]ExternalUser, 0, len(resp.UserList))
	for _, it := range resp.UserList {
		raw := wecomUserRaw(it)
		accountMapping := s.AccountMapping
		jobTitleMapping := s.JobTitleMapping
		users = append(users, ExternalUser{
			ID:            PickMapped(accountMapping, raw, "id", "userid"),
			Username:      PickMapped(accountMapping, raw, "name", "userid"),
			Name:          PickMapped(accountMapping, raw, "displayName", "name"),
			Email:         PickMapped(accountMapping, raw, "email", "email"),
			Phone:         PickMapped(accountMapping, raw, "phone", "mobile"),
			Avatar:        PickMapped(accountMapping, raw, "avatar", "avatar"),
			Title:         PickMapped(accountMapping, raw, "title", "title"),
			JobTitle:      PickJobTitle(accountMapping, jobTitleMapping, raw, "title"),
			DepartmentIDs: PickDepartments(raw, "department"),
		})
	}
	return users, nil
}

// FetchGroups 拉取企业微信部门
func (c *WeComConnector) FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error) {
	token, err := c.accessToken(ctx, s)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/department/list?access_token=%s", url.QueryEscape(token))
	var resp struct {
		ErrCode    int    `json:"errcode"`
		ErrMsg     string `json:"errmsg"`
		Department []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ParentID int    `json:"parentid"`
		} `json:"department"`
	}
	if err := getJSON(ctx, u, nil, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("企业微信拉取部门失败: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	groups := make([]ExternalGroup, 0, len(resp.Department))
	for _, d := range resp.Department {
		parent := ""
		if d.ParentID > 0 {
			parent = fmt.Sprintf("%d", d.ParentID)
		}
		// 企业微信根部门（id=1，parentid=0）是"企业本身"，不落库为部门（Spec Q4.5）
		groups = append(groups, ExternalGroup{
			ID: fmt.Sprintf("%d", d.ID), Name: d.Name, ParentID: parent, IsRoot: d.ID == 1 && d.ParentID == 0,
			Raw: map[string]string{"dept_id": fmt.Sprintf("%d", d.ID), "name": d.Name},
		})
	}
	return groups, nil
}
