package service

import (
	"context"
	"strconv"

	bizerrors "polaris/internal/errors"
	"polaris/internal/logger"
	"polaris/internal/repository"
	"polaris/internal/service/provisioner"
)

// 对齐检查（Reconcile）结果状态
const (
	// ReconcileOK 下游确认存在
	ReconcileOK = "ok"
	// ReconcileMissing 下游确认不存在（可安全补建号）
	ReconcileMissing = "missing"
	// ReconcileUnknown 无法判定（下游查询能力受限/返回未映射错误码）——**不可当作不存在**
	ReconcileUnknown = "unknown"
)

// ReconcileItem 单个账号的对齐结果
type ReconcileItem struct {
	User   string `json:"user"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// ReconcileReport 对齐检查报告
type ReconcileReport struct {
	Application string `json:"application"`
	Connector   string `json:"connector"`
	DryRun      bool   `json:"dry_run"`
	Total       int    `json:"total"`
	OK          int    `json:"ok"`
	Missing     int    `json:"missing"`
	Unknown     int    `json:"unknown"`
	Queued      int    `json:"queued"`
	// Truncated 本轮达到单次核对上限（还有账号未检查，下一轮继续）
	Truncated bool            `json:"truncated"`
	Items     []ReconcileItem `json:"items"`
}

// reconcileMaxAccounts 单次对齐检查最多核对的账号数。
// 对齐检查是"逐个查询下游"的串行操作，必须在有界时间内返回（避免把 HTTP 请求拖成长任务）；
// 超过阈值的部分留待下一轮，并在报告里明确告知。
const reconcileMaxAccounts = 100

// Reconcile 出方向对齐检查：逐个核对"本地已有应用账号"在下游是否真实存在。
//
// 设计取舍（安全优先）：
//   - 只做**保守方向**——本地有账号、下游确认缺失 → 补建号；**不做"下游有、本地没有就删除"**，
//     因为那需要可靠地全量枚举下游账号，一旦枚举不全就是误删；
//   - 判定只有三种结果，**"无法判定"绝不当作"不存在"**（否则会把权限不足误判成缺失而重复建号）；
//   - 默认 dry-run（只报告、不入队），只有显式传 dry_run=false 才补建号。
func (s *ProvisioningService) Reconcile(
	ctx context.Context, actor string, institutionID int64, connectorName string, dryRun bool,
) (*ReconcileReport, error) {
	conn, err := s.connectors.GetByName(ctx, institutionID, connectorName)
	if err != nil {
		return nil, bizerrors.ErrNotFound.WithDetail("connector", connectorName)
	}
	prov, err := provisioner.Get(connectorProvisionerType(conn))
	if err != nil {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams, err.Error())
	}
	reporter, ok := prov.(provisioner.ExistsReporter)
	if !ok {
		return nil, bizerrors.New(bizerrors.CodeInvalidParams,
			"connector does not support reconcile (cannot query downstream accounts)")
	}
	if conn.ApplicationID == nil {
		return nil, bizerrors.ErrInvalidParams.WithDetail("application_id", "required")
	}

	// 多取一条用于判断"是否被截断"（下一轮继续核对剩下的账号）
	accounts, err := s.accounts.List(ctx, repository.ListQuery{
		InstitutionID: institutionID,
		Field:         "application_id",
		Value:         strconv.FormatInt(*conn.ApplicationID, 10),
		Limit:         reconcileMaxAccounts + 1,
	})
	if err != nil {
		return nil, err
	}
	truncated := len(accounts) > reconcileMaxAccounts
	if truncated {
		accounts = accounts[:reconcileMaxAccounts]
	}

	cfg := s.connectorConfig(conn)
	items := make([]ReconcileItem, 0, len(accounts))
	missingUsers := make([]string, 0, len(accounts))
	for _, account := range accounts {
		username := firstNonEmptyStr(account.RelatedUsername, account.Username)
		result, err := reporter.ExistsUser(ctx, cfg, username)
		if err != nil {
			// 查询本身失败（网络/鉴权）：记为无法判定并保留原因，绝不当作缺失
			items = append(items, ReconcileItem{User: username, Status: ReconcileUnknown, Detail: err.Error()})
			continue
		}
		switch result.State {
		case provisioner.ExistsYes:
			items = append(items, ReconcileItem{User: username, Status: ReconcileOK})
		case provisioner.ExistsNo:
			items = append(items, ReconcileItem{User: username, Status: ReconcileMissing})
			missingUsers = append(missingUsers, account.User)
		default:
			items = append(items, ReconcileItem{User: username, Status: ReconcileUnknown, Detail: result.Detail})
		}
	}

	report := buildReconcileReport(conn.Application, conn.Name, dryRun, items)
	report.Truncated = truncated
	if !dryRun {
		// 补建号：复用供给任务（单飞键 = 机构×应用×用户），因此重复点击不会重复建号
		for _, user := range missingUsers {
			s.enqueue(ctx, provisionTask{
				InstitutionID: institutionID, Application: conn.Application,
				User: user, Action: ProvisionActionCreate,
			})
			report.Queued++
		}
		logger.Info("reconcile executed", "connector", conn.Name, "missing", report.Missing, "queued", report.Queued)
	}
	s.audits.Record(ctx, actor, institutionID, "connector.reconcile", conn.Name, map[string]interface{}{
		"dry_run": dryRun, "total": report.Total, "missing": report.Missing,
		"unknown": report.Unknown, "queued": report.Queued,
	})
	return report, nil
}

// buildReconcileReport 汇总对齐结果（纯函数，便于测试）
func buildReconcileReport(application, connector string, dryRun bool, items []ReconcileItem) *ReconcileReport {
	report := &ReconcileReport{Application: application, Connector: connector, DryRun: dryRun, Items: items}
	report.Total = len(items)
	for _, item := range items {
		switch item.Status {
		case ReconcileOK:
			report.OK++
		case ReconcileMissing:
			report.Missing++
		default:
			report.Unknown++
		}
	}
	return report
}
