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
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
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

func doJSON(req *http.Request, out interface{}) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求外部数据源失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("外部数据源返回 %d: %s", resp.StatusCode, truncate(string(data), 300))
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
