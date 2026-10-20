// Package service 承载业务逻辑：编排仓库、校验规则、记录审计。
package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"polaris/internal/repository"
)

var secureRandomRead = rand.Read

// randomHex returns a cryptographically secure hexadecimal string.
// The process fails closed if the system random source is unavailable.
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := secureRandomRead(buf); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// randomBase64 returns a cryptographically secure URL-safe string.
func randomBase64(n int) string {
	buf := make([]byte, n)
	if _, err := secureRandomRead(buf); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// Page 归一化分页参数（page 从 1 开始）。
func Page(page, pageSize int) (offset, limit int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	return (page - 1) * pageSize, pageSize
}

// ListResult 通用分页结果
type ListResult struct {
	Items []interface{} `json:"items"`
	Total int64         `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

// clampLimit 归一化列表查询的 limit。
func clampLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 20
	}
	return limit
}

// institutionQuery 构造组织维度的查询参数。
func institutionQuery(institutionID int64, keyword string, offset, limit int) repository.ListQuery {
	return repository.ListQuery{InstitutionID: institutionID, Keyword: keyword, Offset: offset, Limit: clampLimit(limit)}
}
