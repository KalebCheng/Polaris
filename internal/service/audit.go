package service

import (
	"context"
	"net/http"
	"strings"

	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/requestmeta"
	"polaris/internal/tracing"
)

// AuditRecorder 审计记录器：把关键操作写入 records 表。
type AuditRecorder struct {
	records repository.RecordRepository
	// users 可选：用于把操作者名称**尽力**解析为主键（审计不因此失败，解析不到就只留名称快照）
	users repository.UserRepository
}

// NewAuditRecorder 创建审计记录器
func NewAuditRecorder(records repository.RecordRepository) *AuditRecorder {
	return &AuditRecorder{records: records}
}

// SetUserResolver 注入用户仓库（装配期调用）：审计写入时尽力补 user_id，供"按人精确检索"。
func (a *AuditRecorder) SetUserResolver(users repository.UserRepository) {
	if a != nil {
		a.users = users
	}
}

// Record 写入审计记录（失败仅记日志，不影响主流程）。
func (a *AuditRecorder) Record(ctx context.Context, actor string, institutionID int64, action, object string, detail map[string]interface{}) {
	if a == nil || a.records == nil {
		return
	}
	rec := &repository.Record{
		InstitutionID: institutionID,
		StatusCode:    auditStatusCode(action),
		User:          actor,
		UserID:        a.resolveUserID(ctx, institutionID, actor),
		Action:        action,
		Object:        object,
		TraceID:       tracing.TraceID(ctx),
	}
	request := requestmeta.From(ctx)
	rec.ClientIP = request.ClientIP
	rec.Method = request.Method
	rec.RequestURI = request.RequestURI
	if detail != nil {
		rec.Detail = marshalDetail(detail)
	}
	if _, err := a.records.Create(ctx, rec); err != nil {
		logger.Warn("failed to write audit record", "error", err, "action", action)
	}
}

func marshalDetail(detail map[string]interface{}) string {
	if len(detail) == 0 {
		return ""
	}
	b, err := jsonMarshal(detail)
	if err != nil {
		return ""
	}
	return string(b)
}

// auditStatusCode 审计记录的「结果」状态码。
//
// 业务审计在操作**完成后**写入，因此默认 200（成功）；失败类动作（目前只有登录失败）
// 记 401。新增失败类审计动作时在此集中维护，避免前端"结果"列出现空值。
func auditStatusCode(action string) int {
	if strings.HasSuffix(action, "_failed") {
		return http.StatusUnauthorized
	}
	return http.StatusOK
}

// resolveUserID 把操作者名称尽力解析为主键（审计是旁路：解析失败只留名称快照，绝不阻断主流程）。
func (a *AuditRecorder) resolveUserID(ctx context.Context, institutionID int64, name string) *int64 {
	if a.users == nil || name == "" || institutionID == 0 {
		return nil
	}
	u, err := a.users.GetByName(ctx, institutionID, name)
	if err != nil || u == nil {
		return nil
	}
	return &u.ID
}
