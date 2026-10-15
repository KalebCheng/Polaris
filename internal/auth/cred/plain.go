package cred

// plainManager 明文实现（仅用于兼容外部导入数据；平台默认不启用）。
type plainManager struct{}

func (plainManager) Type() string { return TypePlain }

func (plainManager) Hash(password, _ string) (string, error) {
	return password, nil
}

func (plainManager) Verify(password, hash, _ string) bool {
	return hash != "" && hash == password
}
