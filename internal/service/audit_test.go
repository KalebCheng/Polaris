package service

import (
	"context"
	"testing"

	"polaris/internal/repository"
	"polaris/internal/requestmeta"
)

type auditRecordRepo struct {
	repository.RecordRepository
	created *repository.Record
}

func (r *auditRecordRepo) Create(_ context.Context, rec *repository.Record) (int64, error) {
	copy := *rec
	r.created = &copy
	rec.ID = 1
	return 1, nil
}

func TestAuditRecorderCapturesRequestMetadata(t *testing.T) {
	repo := &auditRecordRepo{}
	recorder := NewAuditRecorder(repo)
	ctx := requestmeta.With(context.Background(), requestmeta.Info{
		ClientIP:   "203.0.113.7",
		Method:     "POST",
		RequestURI: "/api/v1/users?source=test",
	})

	recorder.Record(ctx, "admin", 1, "user.create", "alice", nil)

	if repo.created == nil {
		t.Fatal("audit record was not created")
	}
	if repo.created.ClientIP != "203.0.113.7" ||
		repo.created.Method != "POST" ||
		repo.created.RequestURI != "/api/v1/users?source=test" {
		t.Fatalf("audit request metadata not captured: %+v", repo.created)
	}
}
