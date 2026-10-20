package service

import "testing"

// 回归：对齐报告的计数必须与条目状态一致（"无法判定"单独计数，绝不混入"缺失"）。
func TestBuildReconcileReport(t *testing.T) {
	items := []ReconcileItem{
		{User: "alice", Status: ReconcileOK},
		{User: "bob", Status: ReconcileMissing},
		{User: "carol", Status: ReconcileMissing},
		{User: "dave", Status: ReconcileUnknown, Detail: "errcode=60121 权限不足"},
	}
	report := buildReconcileReport("app-one", "conn-dingtalk", true, items)
	if report.Total != 4 || report.OK != 1 || report.Missing != 2 || report.Unknown != 1 {
		t.Fatalf("计数错误: %+v", report)
	}
	if !report.DryRun || report.Application != "app-one" || report.Connector != "conn-dingtalk" {
		t.Fatalf("报告元信息错误: %+v", report)
	}
	if report.Queued != 0 {
		t.Fatalf("试运行不应入队，实际 %d", report.Queued)
	}
	empty := buildReconcileReport("app", "conn", false, nil)
	if empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatalf("空报告应为零值集合: %+v", empty)
	}
}
