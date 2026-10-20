package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient 连接器共用的 HTTP 客户端
var httpClient = &http.Client{Timeout: 20 * time.Second}

// RequestJSON 发送任意方法的 JSON 请求并解析响应。
// 供 syncer 连接器与下游供给（provisioner）共用：统一 20s 超时、8MiB 响应上限、
// 非 2xx 归一为错误、out 为 nil 时表示"只发请求不解析"。
func RequestJSON(ctx context.Context, method, url string, headers map[string]string, body interface{}, out interface{}) error {
	req, err := newJSONRequest(ctx, method, url, headers, body)
	if err != nil {
		return err
	}
	return doJSON(req, out)
}

// postJSON 发送 JSON 请求并解析响应
func postJSON(ctx context.Context, url string, headers map[string]string, body interface{}, out interface{}) error {
	return RequestJSON(ctx, http.MethodPost, url, headers, body, out)
}

// getJSON 发送 GET 请求并解析响应
func getJSON(ctx context.Context, url string, headers map[string]string, out interface{}) error {
	return RequestJSON(ctx, http.MethodGet, url, headers, nil, out)
}

// RequestJSONRaw 发送 JSON 请求并返回原始状态码与响应体，**不做 2xx 判定、不做解析**。
//
// 面向"业务错误码在响应体里、HTTP 状态只是传输层信号"的下游：飞书即为典型——
// 应用少了部门通讯录权限时返回的是 HTTP 403 + body {"code":40004,...}，
// 若按 2xx 判定就会把它压成一句"外部数据源返回 403"，丢掉可诊断的业务码。
func RequestJSONRaw(ctx context.Context, method, url string, headers map[string]string, body interface{}) (int, []byte, error) {
	req, err := newJSONRequest(ctx, method, url, headers, body)
	if err != nil {
		return 0, nil, err
	}
	return doRaw(req)
}

func newJSONRequest(ctx context.Context, method, url string, headers map[string]string, body interface{}) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func doRaw(req *http.Request) (int, []byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("请求外部数据源失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("读取响应失败: %w", err)
	}
	return resp.StatusCode, data, nil
}

func doJSON(req *http.Request, out interface{}) error {
	status, data, err := doRaw(req)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("外部数据源返回 %d: %s", status, truncate(string(data), 300))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
