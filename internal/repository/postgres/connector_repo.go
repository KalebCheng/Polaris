package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 存储列清单：类型与认证相关的配置落在 config / secrets（与入方向同构）。
const connectorColumns = `id, institution_id, application, name, display_name, type, config, secrets,
	just_in_time, state, last_run_time, last_status, last_message, created_at, updated_at`

// ConnectorRepository 下游应用连接器仓库（PostgreSQL 实现）
type ConnectorRepository struct{ db *dbclient.Client }

// NewConnectorRepository 创建连接器仓库
func NewConnectorRepository(db *dbclient.Client) *ConnectorRepository {
	return &ConnectorRepository{db: db}
}

func scanConnector(row interface{ Scan(...interface{}) error }) (*repository.Connector, error) {
	var c repository.Connector
	var config, secrets []byte
	err := row.Scan(&c.ID, &c.InstitutionID, &c.Application, &c.Name, &c.DisplayName, &c.Type,
		&config, &secrets, &c.JustInTime, &c.State,
		&c.LastRunTime, &c.LastStatus, &c.LastMessage, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	expandConnector(&c, config, secrets)
	return &c, nil
}

// expandConnector 把 JSONB 展开为运行期扁平字段（供给链路与 DTO 仍读扁平字段）。
func expandConnector(c *repository.Connector, config, secrets []byte) {
	c.Config = decodeStringMap(config)
	c.Secrets = decodeStringMap(secrets)
	c.Values = map[string]string{}
	c.HasSecrets = map[string]bool{}
	for key, value := range c.Config {
		c.Values[key] = value
	}
	for key, cipherText := range c.Secrets { // 密文；解密由服务层负责
		c.Values[key] = cipherText
		c.HasSecrets[key] = true
	}
	if c.Type == "" {
		c.Type = "http"
	}
}

// shrinkConnector 把运行期扁平字段回收为 JSONB 载荷。
func shrinkConnector(c *repository.Connector) ([]byte, []byte) {
	config := map[string]string{}
	secrets := map[string]string{}
	for key, value := range c.Values {
		if value == "" {
			continue
		}
		if c.HasSecrets[key] {
			secrets[key] = value // 密文（加密由服务层完成）
			continue
		}
		config[key] = value
	}
	return jsonOrEmptyObject(config), jsonOrEmptyObject(secrets)
}

// Create 创建连接器
func (r *ConnectorRepository) Create(ctx context.Context, c *repository.Connector) (int64, error) {
	config, secrets := shrinkConnector(c)
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO connectors (institution_id, application, name, display_name,
		type, config, secrets, just_in_time, state)
		VALUES ($1,$2,$3,$4,COALESCE(NULLIF($5,''),'http'),$6,$7,$8,COALESCE(NULLIF($9,''),'active'))
		RETURNING id`,
		c.InstitutionID, c.Application, c.Name, c.DisplayName, c.Type,
		encodeJSONB(config), encodeJSONB(secrets), c.JustInTime, c.State,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "connector not found")
	}
	c.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *ConnectorRepository) GetByID(ctx context.Context, id int64) (*repository.Connector, error) {
	c, err := scanConnector(r.db.QueryRow(ctx, `SELECT `+connectorColumns+` FROM connectors WHERE id=$1`, id))
	if err != nil {
		return nil, mapError(err, "connector not found")
	}
	return c, nil
}

// GetByName 按租户 + 名称查询
func (r *ConnectorRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Connector, error) {
	c, err := scanConnector(r.db.QueryRow(ctx,
		`SELECT `+connectorColumns+` FROM connectors WHERE institution_id=$1 AND name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "connector not found")
	}
	return c, nil
}

// Update 更新连接器（不含运行态字段）
func (r *ConnectorRepository) Update(ctx context.Context, c *repository.Connector) error {
	config, secrets := shrinkConnector(c)
	const q = `UPDATE connectors SET display_name=$3, application=$4, type=COALESCE(NULLIF($5,''),'http'),
		config=$6, secrets=$7, just_in_time=$8, state=$9, updated_at=NOW()
		WHERE institution_id=$1 AND name=$2`
	return mapError(r.db.Exec(ctx, q, c.InstitutionID, c.Name, c.DisplayName, c.Application, c.Type,
		encodeJSONB(config), encodeJSONB(secrets), c.JustInTime, defaultStr(c.State, "active")), "connector not found")
}

// Delete 删除连接器
func (r *ConnectorRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM connectors WHERE id=$1`, id), "connector not found")
}

// List 分页列出连接器
func (r *ConnectorRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Connector, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	sql := `SELECT ` + connectorColumns + ` FROM connectors` + b.where() + b.orderLimit("name ASC", q.Offset, q.Limit)
	rows, err := r.db.Query(ctx, sql, b.args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Connector
	for rows.Next() {
		c, err := scanConnector(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, c)
	}
	return result, mapError(rows.Err(), "")
}

// Count 统计连接器数量
func (r *ConnectorRepository) Count(ctx context.Context, q repository.ListQuery) (int64, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(columnExpr(q.Field) + `=` + b.arg(q.Value))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(name ILIKE ` + p + ` OR display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM connectors`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// ListEnabled 返回指定应用下启用的连接器（application 为空时返回租户内全部启用连接器）
func (r *ConnectorRepository) ListEnabled(ctx context.Context, institutionID int64, application string) ([]*repository.Connector, error) {
	sql := `SELECT ` + connectorColumns + ` FROM connectors WHERE institution_id=$1 AND state='active'`
	args := []interface{}{institutionID}
	if application != "" {
		args = append(args, application)
		sql += ` AND application=$2`
	}
	sql += ` ORDER BY name ASC`
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, mapError(err, "")
	}
	defer rows.Close()
	var result []*repository.Connector
	for rows.Next() {
		c, err := scanConnector(rows)
		if err != nil {
			return nil, mapError(err, "")
		}
		result = append(result, c)
	}
	return result, mapError(rows.Err(), "")
}

// UpdateRunResult 写回最近一次供给状态
func (r *ConnectorRepository) UpdateRunResult(ctx context.Context, id int64, status, message string, at time.Time) error {
	return mapError(r.db.Exec(ctx, `UPDATE connectors SET last_status=$2, last_message=$3,
		last_run_time=$4, updated_at=NOW() WHERE id=$1`, id, status, message, at), "connector not found")
}

var _ repository.ConnectorRepository = (*ConnectorRepository)(nil)
