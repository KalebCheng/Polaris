package errors

import "testing"

// 回归：包级错误是**共享实例**，WithDetail/WithError 必须返回副本。
// 曾出现存储上传的响应里同时挂着登录链路写入的 category 与本地写入的 hint——
// 两个互不相干的调用点把细节写进了同一个错误对象。
func TestWithDetailDoesNotPolluteSharedError(t *testing.T) {
	first := ErrProviderUnavailable.WithDetail("category", "storage")
	second := ErrProviderUnavailable.WithDetail("hint", "no system default")

	if _, leaked := first.Details["hint"]; leaked {
		t.Fatalf("第一个错误的细节被后续调用污染: %+v", first.Details)
	}
	if _, leaked := second.Details["category"]; leaked {
		t.Fatalf("第二个错误继承了他处写入的细节: %+v", second.Details)
	}
	if len(ErrProviderUnavailable.Details) != 0 {
		t.Fatalf("包级共享实例被写入细节: %+v", ErrProviderUnavailable.Details)
	}
	if first == second {
		t.Fatal("WithDetail 必须返回新的错误实例")
	}
	// 业务码与文案不受影响（调用点仍按 code 分支）
	if first.Code != CodeProviderUnavailable || first.Message != CodeProviderUnavailable.Message() {
		t.Fatalf("副本的业务码/文案被改动: %+v", first)
	}
}

func TestWithErrorDoesNotPolluteSharedError(t *testing.T) {
	wrapped := Wrap(CodeProviderUnavailable, "provider check failed", errStub{})
	if ErrProviderUnavailable.Err != nil {
		t.Fatal("Wrap 不应污染包级实例的底层错误")
	}
	if wrapped.Unwrap() == nil {
		t.Fatal("Wrap 应保留底层错误（仅进日志）")
	}
}

type errStub struct{}

func (errStub) Error() string { return "boom" }
