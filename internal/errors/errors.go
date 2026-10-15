package errors

import "fmt"

// Error 自定义业务错误类型。
// 携带业务码 + 面向客户端的英文消息 + 可选细节（details）+ 底层错误（仅服务端日志，绝不下发）。
type Error struct {
	Code    Code
	Message string
	Details map[string]interface{}
	Err     error
}

// New 创建业务错误
func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message, Details: make(map[string]interface{})}
}

// NewWithCode 使用业务码的默认消息创建错误
func NewWithCode(code Code) *Error { return New(code, code.Message()) }

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 返回底层错误
func (e *Error) Unwrap() error { return e.Err }

// WithDetail 添加详细错误信息并返回**副本**。
//
// 为什么要复制：包级错误（如 ErrProviderUnavailable、ErrNotFound）是**共享实例**，
// 直接在其上写 Details 会让不同请求、不同调用点互相污染——曾出现存储上传的响应里
// 同时挂着登录链路写入的 category 与本地写入的 hint。
func (e *Error) WithDetail(key string, value interface{}) *Error {
	clone := e.clone()
	details := make(map[string]interface{}, len(clone.Details)+1)
	for k, v := range clone.Details {
		details[k] = v
	}
	details[key] = value
	clone.Details = details
	return clone
}

// WithError 添加底层错误（仅用于日志/链路，不进入响应体），同样返回副本
func (e *Error) WithError(err error) *Error {
	clone := e.clone()
	clone.Err = err
	return clone
}

// clone 浅拷贝（Details 由调用方按需复制，避免无谓分配）
func (e *Error) clone() *Error {
	copied := *e
	return &copied
}

// Wrap 包装底层错误，使用自定义消息
func Wrap(code Code, message string, err error) *Error { return New(code, message).WithError(err) }

// WrapWithCode 包装底层错误，使用业务码默认消息
func WrapWithCode(code Code, err error) *Error { return NewWithCode(code).WithError(err) }

// Is 判断错误是否为指定业务码
func Is(err error, code Code) bool {
	if e, ok := err.(*Error); ok {
		return e.Code == code
	}
	return false
}

// As 提取 *Error
func As(err error) (*Error, bool) {
	e, ok := err.(*Error)
	return e, ok
}

// CommonError 预定义常用错误
var (
	ErrInvalidParams        = NewWithCode(CodeInvalidParams)
	ErrUnauthorized         = NewWithCode(CodeUnauthorized)
	ErrForbidden            = NewWithCode(CodeForbidden)
	ErrNotFound             = NewWithCode(CodeNotFound)
	ErrConflict             = NewWithCode(CodeConflict)
	ErrInternalError        = NewWithCode(CodeInternalError)
	ErrInvalidCredentials   = NewWithCode(CodeInvalidCredentials)
	ErrSessionExpired       = NewWithCode(CodeSessionExpired)
	ErrSessionNotFound      = NewWithCode(CodeSessionNotFound)
	ErrInvalidToken         = NewWithCode(CodeInvalidToken)
	ErrTokenExpired         = NewWithCode(CodeTokenExpired)
	ErrUserDisabled         = NewWithCode(CodeUserDisabled)
	ErrMfaRequired          = NewWithCode(CodeMfaRequired)
	ErrMfaInvalid           = NewWithCode(CodeMfaInvalid)
	ErrMfaEnrollRequired    = NewWithCode(CodeMfaEnrollRequired)
	ErrPermissionDenied     = NewWithCode(CodePermissionDenied)
	ErrRoleNotFound         = NewWithCode(CodeRoleNotFound)
	ErrProtectedObject      = NewWithCode(CodeProtectedObject)
	ErrDBNotFound           = NewWithCode(CodeDBNotFound)
	ErrDBDuplicate          = NewWithCode(CodeDBDuplicate)
	ErrTooManyRequests      = NewWithCode(CodeTooManyRequests)
	ErrOAuthInvalidClient   = NewWithCode(CodeOAuthInvalidClient)
	ErrOAuthInvalidGrant    = NewWithCode(CodeOAuthInvalidGrant)
	ErrOAuthInvalidRedirect = NewWithCode(CodeOAuthInvalidRedirect)
	ErrSSOStateInvalid      = NewWithCode(CodeSSOStateInvalid)
	ErrIdentityNotBound     = NewWithCode(CodeIdentityNotBound)
	ErrInstitutionRequired  = NewWithCode(CodeInstitutionRequired)
	ErrCaptchaRequired      = NewWithCode(CodeCaptchaRequired)
	ErrCaptchaInvalid       = NewWithCode(CodeCaptchaInvalid)
	ErrVerifyCodeInvalid    = NewWithCode(CodeVerifyCodeInvalid)
	ErrVerifyCodeTooMany    = NewWithCode(CodeVerifyCodeTooMany)
	ErrProviderUnavailable  = NewWithCode(CodeProviderUnavailable)
)
