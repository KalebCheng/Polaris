// Package syncer 实现外部用户数据源连接器（对应 Casdoor 的 Syncer 能力）。
//
// 每个连接器负责从外部系统拉取"用户 / 用户组"的中立视图，由 SyncerService 统一写回本地。
package syncer

import (
	"context"
	"fmt"

	"polaris/internal/repository"
)

// ExternalUser 外部用户的中立视图。
// 说明：外部"组"不再写入本地用户组（用户组是权限载体，只由管理员维护），
// 用户归属由「所属组织（部门）」表达，因此这里只保留部门归属标识。
type ExternalUser struct {
	ID       string
	Username string
	Name     string
	Email    string
	Phone    string
	Avatar   string
	Title    string
	// JobTitle is the local job-title entity name. It falls back to Title when
	// no explicit job_title_mapping is configured.
	JobTitle string
	Location string
	// CountryCode 手机区号（如 86）
	CountryCode string
	// DepartmentIDs 所属外部组织（部门）标识；写入 user_departments（多值）
	DepartmentIDs []string
	// Raw 源端字段原文（键名与目录声明的 source_fields 一致）。
	// 落库到 syncer_user_links.raw 并在详情页展示——"同步结果对不上时能直接看到源端给了什么"。
	Raw map[string]string
}

// ExternalGroup 外部组织（部门）的中立视图。
// ParentID 为外部父节点标识：提供时按层级还原组织树；缺失时挂在「同步目标」节点之下（扁平）。
type ExternalGroup struct {
	ID       string
	Name     string
	ParentID string
	// IsRoot 源端根节点占位（钉钉 dept_id=1、企业微信 dept_id=1、飞书 0）。
	// 它**不落库**：否则部门树的顶层会变成"1"而不是组织本身（见 Spec Q4.5）。
	IsRoot    bool
	MemberIDs []string
	Raw       map[string]string
}

// Connector 外部数据源连接器
type Connector interface {
	Type() string
	// FetchUsers 拉取用户（含所属组 ID）
	FetchUsers(ctx context.Context, s *repository.Syncer) ([]ExternalUser, error)
	// FetchGroups 拉取用户组
	FetchGroups(ctx context.Context, s *repository.Syncer) ([]ExternalGroup, error)
}

var registry = map[string]Connector{}

// Register 注册连接器
func Register(c Connector) { registry[c.Type()] = c }

// Get 按类型获取连接器
func Get(connectorType string) (Connector, error) {
	c, ok := registry[connectorType]
	if !ok {
		return nil, fmt.Errorf("不支持的同步器类型: %s", connectorType)
	}
	return c, nil
}

// Types 返回已注册的连接器类型
func Types() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}

// init 注册内置连接器（跨切面基础设施之外的注册表：仅承载无状态连接器实现，故使用 init）。
func init() {
	Register(&DingTalkConnector{})
	Register(&FeishuConnector{})
	Register(&WeComConnector{})
	Register(&DatabaseConnector{})
	Register(&LDAPConnector{})
	Register(&SCIMConnector{})
}
