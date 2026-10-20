package service

import (
	"testing"
	"time"

	"polaris/internal/repository"
)

func TestCookieTTLUsesApplicationPolicy(t *testing.T) {
	fallback := 24 * time.Hour
	if got := cookieTTL(nil, fallback); got != fallback {
		t.Fatalf("nil application TTL = %s, want fallback", got)
	}
	app := &repository.Application{CookieExpireInHours: 720}
	if got := cookieTTL(app, fallback); got != 30*24*time.Hour {
		t.Fatalf("application cookie TTL = %s, want 720h", got)
	}
}
