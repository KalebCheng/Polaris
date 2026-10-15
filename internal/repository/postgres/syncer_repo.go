package postgres

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 存储列清单：类型专属配置一律落在 config / secrets / field_mapping 三个 JSONB 列，
// 因此列清单不再随"新增一种接入方式"而变化（见 Spec §3.6 / §5.4）。
const syncerColumns = `id, institution_id, name, display_name, type, sub_type, is_enabled, schedule,
	config, secrets, field_mapping, sync_config, target_department_id,
	sync_users, sync_groups, soft_delete,
	last_run_time, last_status, last_message, last_counts, created_at, updated_at`

// SyncerRepository 同步器仓库（PostgreSQL 实现）
type SyncerRepository struct{ db *dbclient.Client }

// NewSyncerRepository 创建同步器仓库
func NewSyncerRepository(db *dbclient.Client) *SyncerRepository { return &SyncerRepository{db: db} }

func scanSyncer(row interface{ Scan(...interface{}) error }) (*repository.Syncer, error) {
	var s repository.Syncer
	var config, secrets, fieldMapping, syncConfig []byte
	err := row.Scan(&s.ID, &s.InstitutionID, &s.Name, &s.DisplayName, &s.Type, &s.SubType, &s.IsEnabled,
		&s.Schedule, &config, &secrets, &fieldMapping, &syncConfig, &s.TargetDepartmentID,
		&s.SyncUsers, &s.SyncGroups, &s.SoftDelete,
		&s.LastRunTime, &s.LastStatus, &s.LastMessage, &s.LastCounts, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	expandSyncer(&s, config, secrets, fieldMapping)
	if len(syncConfig) > 0 {
		_ = json.Unmarshal(syncConfig, &s.SyncConfig)
	}
	return &s, nil
}

// expandSyncer 把 JSONB 配置展开为运行期扁平字段。
// 连接器实现与 DTO 统一读扁平字段，因此"存储换形态、契约不变"（Spec §5.4）。
func expandSyncer(s *repository.Syncer, config, secrets, fieldMapping []byte) {
	s.Config = decodeStringMap(config)
	s.Secrets = decodeStringMap(secrets)
	s.FieldMapping = decodeFieldMapping(fieldMapping)

	s.Host = s.Config["host"]
	s.Port = atoiSafe(s.Config["port"])
	s.User = s.Config["user"]
	s.Database = s.Config["database"]
	s.Table = s.Config["table"]
	s.DatabaseType = s.Config["database_type"]
	s.SSLMode = s.Config["ssl_mode"]
	s.AppID = s.Config["app_id"]
	s.CorpID = s.Config["corp_id"]
	// 敏感字段保存的是密文（AES-GCM），解密仍由服务层负责
	s.Password = s.Secrets["password"]
	s.AppSecret = s.Secrets["app_secret"]
	s.UserMapping = s.FieldMapping["account"]
}

// shrinkSyncer 把运行期扁平字段回收为三个 JSONB 载荷；空值不写入（避免"空值被当成已配置"）。
func shrinkSyncer(s *repository.Syncer) ([]byte, []byte, []byte) {
	config := map[string]string{}
	putIfNotEmpty(config, "host", s.Host)
	if s.Port > 0 {
		config["port"] = strconv.Itoa(s.Port)
	}
	putIfNotEmpty(config, "user", s.User)
	putIfNotEmpty(config, "database", s.Database)
	putIfNotEmpty(config, "table", s.Table)
	putIfNotEmpty(config, "database_type", s.DatabaseType)
	putIfNotEmpty(config, "ssl_mode", s.SSLMode)
	putIfNotEmpty(config, "app_id", s.AppID)
	putIfNotEmpty(config, "corp_id", s.CorpID)

	secrets := map[string]string{}
	putIfNotEmpty(secrets, "password", s.Password)
	putIfNotEmpty(secrets, "app_secret", s.AppSecret)

	fieldMapping := map[string]map[string]string{}
	if len(s.UserMapping) > 0 {
		fieldMapping["account"] = s.UserMapping
	}

	return jsonOrEmptyObject(config), jsonOrEmptyObject(secrets), jsonOrEmptyObject(fieldMapping)
}

func putIfNotEmpty(m map[string]string, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func jsonOrEmptyObject(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// encodeJSONB 以文本形式传参，由 PostgreSQL 按列类型（jsonb）解析。
func encodeJSONB(v interface{}) string { return string(jsonOrEmptyObject(v)) }

func decodeStringMap(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func decodeFieldMapping(raw []byte) map[string]map[string]string {
	out := map[string]map[string]string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func atoiSafe(v string) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// Create 创建同步器
func (r *SyncerRepository) Create(ctx context.Context, s *repository.Syncer) (int64, error) {
	config, secrets, fieldMapping := shrinkSyncer(s)
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO syncers (institution_id, name, display_name, type, sub_type,
		is_enabled, schedule, config, secrets, field_mapping, sync_config, target_department_id,
		sync_users, sync_groups, soft_delete)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		s.InstitutionID, s.Name, s.DisplayName, s.Type, s.SubType, s.IsEnabled, s.Schedule,
		encodeJSONB(config), encodeJSONB(secrets), encodeJSONB(fieldMapping),
		encodeJSONB(s.SyncConfig), s.TargetDepartmentID,
		s.SyncUsers, s.SyncGroups, s.SoftDelete,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "syncer not found")
	}
	s.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *SyncerRepository) GetByID(ctx context.Context, id int64) (*repository.Syncer, error) {
	s, err := scanSyncer(r.db.QueryRow(ctx, `SELECT `+syncerColumns+` FROM syncers WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "syncer not found")
	}
	return s, nil
}

// GetByName 按组织 + 名称查询
func (r *SyncerRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Syncer, error) {
	s, err := scanSyncer(r.db.QueryRow(ctx,
		`SELECT `+syncerColumns+` FROM syncers WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "syncer not found")
	}
	return s, nil
}

// Update 更新同步器（不含运行态字段）
func (r *SyncerRepository) Update(ctx context.Context, s *repository.Syncer) error {
	config, secrets, fieldMapping := shrinkSyncer(s)
	return mapError(r.db.Exec(ctx, `UPDATE syncers SET display_name=$2, type=$3, sub_type=$4,
		is_enabled=$5, schedule=$6, config=$7, secrets=$8, field_mapping=$9,
		sync_config=$10, target_department_id=$11,
		sync_users=$12, sync_groups=$13, soft_delete=$14, updated_at=NOW() WHERE id=$1`,
		s.ID, s.DisplayName, s.Type, s.SubType, s.IsEnabled, s.Schedule,
		encodeJSONB(config), encodeJSONB(secrets), encodeJSONB(fieldMapping),
		encodeJSONB(s.SyncConfig), s.TargetDepartmentID,
		s.SyncUsers, s.SyncGroups, s.SoftDelete), "syncer not found")
}

// Delete 删除同步器
func (r *SyncerRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM syncers WHERE id=$1`, id), "syncer not found")
}

// List 分页列出同步器
func (r *SyncerRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Syncer, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + syncerColumns + ` FROM syncers` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Syncer
	for rows.Next() {
		s, err := scanSyncer(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, s)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计同步器数量
func (r *SyncerRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM syncers`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// ListEnabled 返回全部启用的同步器（调度遍历）
func (r *SyncerRepository) ListEnabled(ctx context.Context) ([]*repository.Syncer, error) {
	rows, err := r.db.Query(ctx, `SELECT `+syncerColumns+` FROM syncers WHERE is_enabled=TRUE ORDER BY id ASC`)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Syncer
	for rows.Next() {
		s, err := scanSyncer(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, s)
	}
	return result, mapError(rows.Err(), "")
}

// UpdateRunResult 写回最近一次同步状态
func (r *SyncerRepository) UpdateRunResult(ctx context.Context, id int64, status, message string, at time.Time, counts repository.SyncCounts) error {
	b, _ := json.Marshal(counts)
	return mapError(r.db.Exec(ctx, `UPDATE syncers SET last_status=$2, last_message=$3, last_run_time=$4,
		last_counts=$5, updated_at=NOW() WHERE id=$1`, id, status, message, at, b), "syncer not found")
}

// CreateRun 记录一次同步明细
func (r *SyncerRepository) CreateRun(ctx context.Context, run *repository.SyncerRun) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO syncer_runs (syncer_id, institution_id, status, trigger, message,
		counts, errors, duration_ms, started_at, finished_at)
		VALUES ($1,$2,$3,COALESCE(NULLIF($4,''),'manual'),$5,$6,COALESCE($7,'[]'::jsonb),$8,$9,$10) RETURNING id`,
		run.SyncerID, run.InstitutionID, run.Status, run.Trigger, run.Message,
		run.Counts, run.Errors, run.DurationMS, run.StartedAt, run.FinishedAt,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "")
	}
	run.ID = id
	return id, nil
}

// ListRuns 分页列出同步明细
func (r *SyncerRepository) ListRuns(ctx context.Context, syncerID int64, offset, limit int) ([]*repository.SyncerRun, error) {
	sql := `SELECT id, syncer_id, institution_id, status, trigger, message, counts, errors, duration_ms,
		started_at, finished_at
		FROM syncer_runs WHERE syncer_id=$1 ORDER BY started_at DESC`
	args := []interface{}{syncerID}
	if limit > 0 {
		args = append(args, limit, offset)
		sql += ` LIMIT $2 OFFSET $3`
	}
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.SyncerRun
	for rows.Next() {
		var run repository.SyncerRun
		if err := rows.Scan(&run.ID, &run.SyncerID, &run.InstitutionID, &run.Status, &run.Trigger,
			&run.Message, &run.Counts, &run.Errors, &run.DurationMS,
			&run.StartedAt, &run.FinishedAt); err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, &run)
	}
	return result, mapError(rows.Err(), "")
}

var _ repository.SyncerRepository = (*SyncerRepository)(nil)
