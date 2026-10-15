package handler

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
)

// currentUser 返回当前登录用户名（未认证为空）。
func currentUser(c *gin.Context) string {
	if authCtx := auth.GetAuthContext(c); authCtx != nil {
		return authCtx.Name
	}
	return "anonymous"
}

// isPlatformAdmin 判断当前请求者是否具备平台级（跨组织）权限。
// 只有平台管理员（is_admin 或拥有通配权限）才能越过组织边界查看全平台数据。
func isPlatformAdmin(c *gin.Context) bool {
	authCtx := auth.GetAuthContext(c)
	if authCtx == nil || !authCtx.IsAuthenticated() {
		return false
	}
	return authCtx.IsAdmin || authCtx.Enforce("*", "*")
}

// platformScope 请求是否要求平台级（跨组织）范围，且请求者确有该权限。
// 非平台管理员携带 scope=platform 时不生效，退回其所在组织（不会越权）。
func platformScope(c *gin.Context) bool {
	return c.Query("scope") == "platform" && isPlatformAdmin(c)
}

// instID 解析目标组织主键 id。
// 0 表示"跨组织/平台级"——各列表仓库对 0 不过滤，因此可用于「角色 / 权限 / 证书 / 令牌 / 审计」
// 等平台级资源的跨组织列表；scope=platform 且请求者为平台管理员时才返回 0。
// 否则优先 query 的 institution（组织主键 id），其次当前登录用户的组织 id。
func instID(c *gin.Context) int64 {
	if platformScope(c) {
		return 0
	}
	if v := c.Query("institution"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	}
	if authCtx := auth.GetAuthContext(c); authCtx != nil {
		return authCtx.InstitutionID
	}
	return 0
}

// fmtSscan 解析字符串为整数（包装 fmt.Sscan）。
func fmtSscan(s string, out interface{}) (int, error) { return fmt.Sscan(s, out) }

// timeHour 小时转时间间隔。
func timeHour(h int) time.Duration { return time.Duration(h) * time.Hour }
