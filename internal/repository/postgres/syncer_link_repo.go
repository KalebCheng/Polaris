package postgres

import (
	"context"
	"fmt"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 外部身份映射的"对象类别"（决定表名与本地外键列）
type linkKind struct {
	table    string
	localCol string
}

var linkKinds = map[string]linkKind{
	"user":       {table: "syncer_user_links", localCol: "user_id"},
	"department": {table: "syncer_department_links", localCol: "department_id"},
	"job_title":  {table: "syncer_job_title_links", localCol: "job_title_id"},
	"job_level":  {table: "syncer_job_level_links", localCol: "job_level_id"},
}

// SyncerLinkRepository 外部身份映射仓库（Spec 第六阶段 FR-1）：
// 键为「连接 + 源端唯一标识」，同步据此判定"新增 / 更新"，本地对象改名不再产生新对象。
type SyncerLinkRepository struct{ db *dbclient.Client }

// NewSyncerLinkRepository 创建仓库
func NewSyncerLinkRepository(db *dbclient.Client) *SyncerLinkRepository {
	return &SyncerLinkRepository{db: db}
}

func linkKindFor(kind string) (linkKind, error) {
	k, ok := linkKinds[kind]
	if !ok {
		return linkKind{}, fmt.Errorf("unknown link kind: %s", kind)
	}
	return k, nil
}

// LoadExternalIndex 返回「源端标识 → 本地对象 ID」索引（单连接内）
func (r *SyncerLinkRepository) LoadExternalIndex(ctx context.Context, kind string, syncerID int64) (map[string]int64, error) {
	k, err := linkKindFor(kind)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, fmt.Sprintf(
		`SELECT external_id, %s FROM %s WHERE syncer_id=$1`, k.localCol, k.table), syncerID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var external string
		var local int64
		if err := rows.Scan(&external, &local); err != nil {
			return nil, mapError(err, "")
		}
		out[external] = local
	}
	return out, mapError(rows.Err(), "")
}

// Upsert 写入/更新一条映射（同一连接内源端标识唯一）
func (r *SyncerLinkRepository) Upsert(
	ctx context.Context, kind string, institutionID, syncerID, localID int64, externalID string, raw []byte,
) error {
	k, err := linkKindFor(kind)
	if err != nil {
		return err
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	sql := fmt.Sprintf(`INSERT INTO %s (institution_id, syncer_id, %s, external_id, raw)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (syncer_id, external_id) DO UPDATE
		SET %s = EXCLUDED.%s, raw = EXCLUDED.raw, updated_at = NOW()`,
		k.table, k.localCol, k.localCol, k.localCol)
	return mapError(r.db.Exec(ctx, sql, institutionID, syncerID, localID, externalID, raw), "")
}

// Prune 删除本连接中"源端已不存在"的映射行，返回被清理的本地对象 ID。
// 调用方据此决定是否停用/软删本地对象（本阶段只清理映射，不自动删本地对象）。
func (r *SyncerLinkRepository) Prune(ctx context.Context, kind string, syncerID int64, externalIDs []string) ([]int64, error) {
	k, err := linkKindFor(kind)
	if err != nil {
		return nil, err
	}
	if len(externalIDs) == 0 {
		rows, err := r.db.Query(ctx, fmt.Sprintf(`DELETE FROM %s WHERE syncer_id=$1 RETURNING %s`, k.table, k.localCol), syncerID)
		if err != nil {
			return nil, mapError(err, "")
		}
		defer rows.Close()
		return scanIDs(rows)
	}
	rows, err := r.db.Query(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE syncer_id=$1 AND external_id <> ALL($2) RETURNING %s`, k.table, k.localCol), syncerID, externalIDs)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	return scanIDs(rows)
}

func scanIDs(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}) ([]int64, error) {
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, id)
	}
	return out, mapError(rows.Err(), "")
}

// ListForLocal 返回某个本地对象的外部身份明细（用户/部门详情页展示源端报文，用于排障）
func (r *SyncerLinkRepository) ListForLocal(ctx context.Context, kind string, localID int64) ([]*repository.ExternalIdentityRef, error) {
	k, err := linkKindFor(kind)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, fmt.Sprintf(`SELECT l.syncer_id, COALESCE(s.display_name, s.name), s.type, l.external_id, l.raw, l.updated_at
		FROM %s l JOIN syncers s ON s.id = l.syncer_id
		WHERE l.%s = $1 ORDER BY l.syncer_id`, k.table, k.localCol), localID)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	out := []*repository.ExternalIdentityRef{}
	for rows.Next() {
		ref := &repository.ExternalIdentityRef{}
		if err := rows.Scan(&ref.SyncerID, &ref.SyncerName, &ref.SyncerType, &ref.ExternalID, &ref.Raw, &ref.UpdatedAt); err != nil {
			return nil, mapError(err, "")
		}
		out = append(out, ref)
	}
	return out, mapError(rows.Err(), "")
}

var _ repository.SyncerLinkRepository = (*SyncerLinkRepository)(nil)
