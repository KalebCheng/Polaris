package postgres

// userMatchStrategy 把「匹配系统字段」翻译为 users 表的 WHERE 条件。
// 占位符约定：$1 = 组织主键，$2 = 提供商字段值。
//
// 采用策略模式而不是 switch：新增一种本地匹配字段时只需注册一个策略，
// FindMatchCandidates 与 SQL 组装逻辑都无需改动。
type userMatchStrategy interface {
	where() string
}

type columnMatchStrategy struct {
	clause string
}

func (s columnMatchStrategy) where() string { return s.clause }

var userMatchStrategies = map[string]userMatchStrategy{
	"username": columnMatchStrategy{clause: `institution_id = $1 AND name = $2 AND is_deleted = FALSE`},
	"email":    columnMatchStrategy{clause: `institution_id = $1 AND lower(email) = lower($2) AND email <> '' AND is_deleted = FALSE`},
	"phone":    columnMatchStrategy{clause: `institution_id = $1 AND phone = $2 AND phone <> '' AND is_deleted = FALSE`},
}
