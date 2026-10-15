// Package tracing 提供基于 OpenTelemetry 的链路追踪能力。
// 采用 OTel 全局注册方式（§3.1-1 的明确例外），在 app 组装时调用一次 Setup。
package tracing

import (
	"context"
	"net/http"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var (
	setupOnce sync.Once
	provider  *sdktrace.TracerProvider
)

// Setup 初始化 OpenTelemetry 全局（幂等）：
//  1. 设置全局 TracerProvider（无 exporter，AlwaysSample）；
//  2. 设置全局 W3C Trace Context 传播器；
//  3. 给 http.DefaultTransport 包一层 otelhttp——此后所有走默认 client 的出向请求
//     都会自动创建子 span 并注入 traceparent。
func Setup() *sdktrace.TracerProvider {
	setupOnce.Do(func() {
		provider = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
		otel.SetTracerProvider(provider)
		otel.SetTextMapPropagator(propagation.TraceContext{})
		http.DefaultTransport = otelhttp.NewTransport(http.DefaultTransport)
	})
	return provider
}

// TraceID 从 context 提取当前 span 的 trace_id（无有效 span 时返回空字符串）。
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanID 从 context 提取当前 span 的 span_id（无有效 span 时返回空字符串）。
func SpanID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}
