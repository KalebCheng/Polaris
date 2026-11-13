package service

import (
	"testing"
	"time"

	"polaris/internal/repository"
)

func TestPasswordExpired(t *testing.T) {
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	if passwordExpired(&repository.Institution{PasswordExpireDays: 0}, &repository.User{PasswordUpdatedAt: &old}) {
		t.Fatal("过期天数为 0 时不应过期")
	}
	if !passwordExpired(&repository.Institution{PasswordExpireDays: 1}, &repository.User{PasswordUpdatedAt: &old}) {
		t.Fatal("超过 1 天的密码应过期")
	}
	if passwordExpired(&repository.Institution{PasswordExpireDays: 1}, &repository.User{PasswordUpdatedAt: &recent}) {
		t.Fatal("1 小时内更新的密码不应过期")
	}
	if passwordExpired(&repository.Institution{PasswordExpireDays: 1}, &repository.User{}) {
		t.Fatal("没有密码更新时间时不应判定过期")
	}
}
