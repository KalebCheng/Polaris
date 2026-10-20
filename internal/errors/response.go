package errors

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"polaris/internal/logger"
	"polaris/internal/tracing"
)

// Response 统一响应体结构。
// 成功：code=OK，data 承载业务数据；错误：code=业务码，details 承载可选细节。
type Response struct {
	Code    Code                   `json:"code"`
	Message string                 `json:"message"`
	Data    interface{}            `json:"data,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// RenderSuccess 渲染成功响应。默认 HTTP 200；需要 201 时传入 http.StatusCreated。
func RenderSuccess(c *gin.Context, data interface{}, status ...int) {
	httpStatus := http.StatusOK
	if len(status) > 0 {
		httpStatus = status[0]
	}
	c.JSON(httpStatus, Response{Code: CodeOK, Message: CodeOK.Message(), Data: data})
}

// RenderError 渲染错误响应。HTTP 状态码由业务码映射；非业务错误兜底 500。
func RenderError(c *gin.Context, err error) {
	c.JSON(httpStatusOf(err), responseFromError(err))
}

// AbortError 中止请求并渲染错误响应（认证/权限/CSRF 等中间件使用）。
func AbortError(c *gin.Context, err error) {
	c.AbortWithStatusJSON(httpStatusOf(err), responseFromError(err))
}

// Fail 登记业务错误，交由统一响应中间件渲染（handler 内使用）。
func Fail(c *gin.Context, err error) { _ = c.Error(err) }

// FailInvalidParams 登记参数错误（INVALID_PARAMS）。
func FailInvalidParams(c *gin.Context, err error) {
	_ = c.Error(New(CodeInvalidParams, err.Error()))
}

// Middleware 统一响应中间件（Spring @ControllerAdvice 的 Go/Gin 等价物）。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.IsAborted() {
			return
		}
		if len(c.Errors) == 0 {
			return
		}
		RenderError(c, c.Errors.Last().Err)
	}
}

// responseFromError 将错误转换为统一响应体。
// 非业务错误（非 *Error）一律兜底为 INTERNAL_ERROR，绝不把底层错误细节下发给客户端。
func responseFromError(err error) Response {
	if e, ok := err.(*Error); ok {
		return Response{Code: e.Code, Message: e.Message, Details: e.Details}
	}
	return Response{Code: CodeInternalError, Message: CodeInternalError.Message()}
}

func httpStatusOf(err error) int {
	if e, ok := err.(*Error); ok {
		return e.Code.HTTPStatus()
	}
	return http.StatusInternalServerError
}

// RecoveryFunc 全局 panic 恢复函数。堆栈只记录到服务端日志，绝不返回给客户端。
func RecoveryFunc(c *gin.Context, recovered interface{}) {
	logger.Error("panic recovered",
		"error", recovered,
		"path", c.Request.URL.Path,
		"trace_id", tracing.TraceID(c.Request.Context()),
		"span_id", tracing.SpanID(c.Request.Context()),
		"stack", string(debug.Stack()),
	)
	AbortError(c, ErrInternalError)
}
