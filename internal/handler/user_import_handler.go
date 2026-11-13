package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"polaris/internal/auth"
	bizerrors "polaris/internal/errors"
	"polaris/internal/repository"
	"polaris/internal/service"
	"polaris/internal/service/importer"
)

// UserImportHandler 用户导入处理器（控制台「导入用户」：模板 / 上传即导入 / 导入记录）。
type UserImportHandler struct {
	svc     *service.UserImportService
	limiter repository.RateLimitRepository
}

// NewUserImportHandler 创建导入处理器。limiter 用于导入次数限流（nil = 不限流）。
func NewUserImportHandler(svc *service.UserImportService, limiter repository.RateLimitRepository) *UserImportHandler {
	return &UserImportHandler{svc: svc, limiter: limiter}
}

// importRateLimitKey 导入次数计数键（每组织每小时）
func importRateLimitKey(institutionID int64) string {
	return fmt.Sprintf("polaris:user_import:%d", institutionID)
}

// Template 下载导入模板（lang=zh|en）
func (h *UserImportHandler) Template(c *gin.Context) {
	lang := "zh"
	if c.Query("lang") == "en" {
		lang = "en"
	}
	content, err := importer.BuildTemplate(lang)
	if err != nil {
		bizerrors.Fail(c, bizerrors.Wrap(bizerrors.CodeInternalError, "build template failed", err))
		return
	}
	fileName := "polaris-user-import-template-zh.xlsx"
	if lang == "en" {
		fileName = "polaris-user-import-template-en.xlsx"
	}
	writeAttachment(c, fileName, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

// Upload 上传即导入（multipart/form-data：file + 选项）。返回完整报告。
func (h *UserImportHandler) Upload(c *gin.Context) {
	cfg := h.svc.Config()
	institutionID, err := tenantInstitution(c)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	if err := h.checkRateLimit(c, institutionID); err != nil {
		bizerrors.Fail(c, err)
		return
	}
	extendImportDeadlines(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, cfg.MaxFileBytes+64<<10)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			bizerrors.Fail(c, bizerrors.NewWithCode(bizerrors.CodeFileSizeExceeded).WithDetail("max_bytes", cfg.MaxFileBytes))
			return
		}
		bizerrors.FailInvalidParams(c, err)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, cfg.MaxFileBytes+1))
	if err != nil {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	if int64(len(content)) > cfg.MaxFileBytes {
		bizerrors.Fail(c, bizerrors.NewWithCode(bizerrors.CodeFileSizeExceeded).WithDetail("max_bytes", cfg.MaxFileBytes))
		return
	}
	authCtx := auth.GetAuthContext(c)
	opts := service.UserImportOptions{
		InstitutionID:   institutionID,
		Operator:        currentUser(c),
		CanAssignRoles:  authCtx != nil && authCtx.Enforce("role", "update"),
		Delimiter:       strings.TrimSpace(c.PostForm("delimiter")),
		AutoCreateDepts: parseBoolForm(c.PostForm("auto_create_departments")),
	}
	if raw := strings.TrimSpace(c.PostForm("default_job_level_id")); raw != "" {
		id, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil || id <= 0 {
			bizerrors.FailInvalidParams(c, convErr)
			return
		}
		opts.DefaultJobLevel = &id
	}
	report, err := h.svc.Import(c.Request.Context(), opts, fileHeader.Filename, content)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	c.Header("X-Polaris-Import-Id", strconv.FormatInt(report.ImportID, 10))
	bizerrors.RenderSuccess(c, report)
}

// checkRateLimit 每组织每小时 N 次；超限返回 TOO_MANY_REQUESTS。
func (h *UserImportHandler) checkRateLimit(c *gin.Context, institutionID int64) error {
	if h.limiter == nil || institutionID == 0 {
		return nil
	}
	count, err := h.limiter.Incr(c.Request.Context(), importRateLimitKey(institutionID), time.Hour)
	if err != nil {
		return nil // 计数不可用时放行：限流是保护而非准入条件
	}
	if count > int64(h.svc.Config().RunsPerHour) {
		return bizerrors.NewWithCode(bizerrors.CodeTooManyRequests).WithDetail("runs_per_hour", h.svc.Config().RunsPerHour)
	}
	return nil
}

// List 导入记录分页
func (h *UserImportHandler) List(c *gin.Context) {
	offset, limit, page, size := pageParams(c)
	institutionID, err := tenantInstitution(c)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), repository.UserImportListQuery{
		InstitutionID: institutionID, Offset: offset, Limit: limit,
	})
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// Detail 导入记录 + 逐行结果（上传进行中前端用它轮询进度）
func (h *UserImportHandler) Detail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	institutionID, err := tenantInstitution(c)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	rec, rows, err := h.svc.Detail(c.Request.Context(), institutionID, id)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	bizerrors.RenderSuccess(c, gin.H{"import": rec, "rows": rows})
}

// Report CSV 下载未成功的行（失败 + 跳过），可修改后重导。
func (h *UserImportHandler) Report(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		bizerrors.FailInvalidParams(c, err)
		return
	}
	institutionID, err := tenantInstitution(c)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	rec, rows, err := h.svc.Detail(c.Request.Context(), institutionID, id)
	if err != nil {
		bizerrors.Fail(c, err)
		return
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"row", "name", "email", "status", "errors"})
	for _, row := range rows {
		if row.Status == repository.UserImportRowCreated {
			continue
		}
		issues := make([]string, 0, len(row.Errors)+len(row.Warnings))
		for _, issue := range row.Errors {
			issues = append(issues, issueColumn(issue)+": "+issue.Message)
		}
		for _, issue := range row.Warnings {
			issues = append(issues, issueColumn(issue)+": "+issue.Message)
		}
		_ = writer.Write([]string{
			strconv.Itoa(row.RowNumber), csvSafe(row.UserName), csvSafe(row.Email), row.Status,
			csvSafe(strings.Join(issues, " | ")),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		bizerrors.Fail(c, bizerrors.Wrap(bizerrors.CodeInternalError, "build report failed", err))
		return
	}
	baseName := strings.TrimSuffix(strings.TrimSuffix(rec.FileName, ".xlsx"), ".csv")
	name := fmt.Sprintf("%s-report-%d.csv", baseName, rec.ID)
	writeAttachment(c, name, "text/csv; charset=utf-8", buf.Bytes())
}

func issueColumn(issue repository.UserImportRowIssue) string {
	if issue.Column == "" {
		return "row"
	}
	return issue.Column
}

// csvSafe 防止表格软件把单元格当公式执行（CSV 注入）。
func csvSafe(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

func parseBoolForm(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

func writeAttachment(c *gin.Context, fileName, contentType string, content []byte) {
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		"polaris-export", url.PathEscape(fileName)))
	c.Data(http.StatusOK, contentType, content)
}

// extendImportDeadlines keeps the Go HTTP server's 30s read/write deadlines
// from cutting off a synchronous batch import that is still making progress.
func extendImportDeadlines(c *gin.Context) {
	deadline := time.Now().Add(service.UserImportRunTimeout + time.Minute)
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(deadline)
	_ = controller.SetWriteDeadline(deadline)
}
