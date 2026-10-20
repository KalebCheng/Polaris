package postgres

import (
	"strings"
	"testing"

	"polaris/internal/repository"
)

// 回归（2026-10 多部门问题）：部门筛选必须走 user_departments 多值归属，
// 不能只看 users.department_id（主部门），否则第二部门的人会被漏掉。
func TestUserQueryDepartmentFilterUsesUserDepartments(t *testing.T) {
	b := userQuery(repository.ListQuery{InstitutionID: 1, DepartmentID: 42})
	where := b.where()
	if !strings.Contains(where, "user_departments") {
		t.Fatalf("部门筛选应查询 user_departments，实际 %s", where)
	}
	if strings.Contains(where, "users.department_id =") {
		t.Fatalf("不得退化为 users.department_id 单值过滤，实际 %s", where)
	}
	if len(b.args) != 2 || b.args[0] != int64(1) || b.args[1] != int64(42) {
		t.Fatalf("参数应为组织主键与部门主键，实际 %#v", b.args)
	}
}

// Identity-link matching is unavailable through the unscoped local-field helper.
func TestMatchCandidateWhereRejectsUnscopedIdentityLink(t *testing.T) {
	for _, field := range []string{"external_id", "uid"} {
		if _, err := matchCandidateWhere(field); err == nil {
			t.Fatalf("未声明或无连接范围的匹配字段 %q 应被拒绝", field)
		}
	}
}
