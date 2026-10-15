// Package service 承载业务逻辑：编排仓库、校验规则、记录审计。
package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"polaris/internal/repository"
)

// randomHex 生成指定字节长度的随机十六进制串。
func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// randomBase64 生成 URL 安全的随机串。
func randomBase64(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
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
