package cred

import "golang.org/x/crypto/bcrypt"

// bcryptManager bcrypt 实现（哈希自带随机盐，忽略 salt 参数）。
type bcryptManager struct{}

func (bcryptManager) Type() string { return TypeBcrypt }

func (bcryptManager) Hash(password, _ string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (bcryptManager) Verify(password, hash, _ string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
