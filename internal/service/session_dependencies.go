package service

import (
	"context"
	"errors"

	"polaris/internal/auth"
)

type sessionController interface {
	InvalidateUser(ctx context.Context, institutionID int64, user string) ([]string, error)
	EffectivePermissions(ctx context.Context, institutionID int64, user string) ([]auth.PermissionRule, error)
}

func invalidateUserSessions(ctx context.Context, sessions sessionController, institutionID int64, users []string) error {
	if sessions == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(users))
	var errs []error
	for _, user := range users {
		if user == "" {
			continue
		}
		if _, ok := seen[user]; ok {
			continue
		}
		seen[user] = struct{}{}
		if _, err := sessions.InvalidateUser(ctx, institutionID, user); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
