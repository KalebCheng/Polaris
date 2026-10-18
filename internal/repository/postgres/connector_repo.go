package postgres

import (
	"context"
	"time"

	"polaris/internal/repository"
	"polaris/pkg/dbclient"
)

// 存储列清单：类型与认证相关的配置落在 config / secrets（与入方向同构）。
const connectorColumns = `t.id, t.institution_id, t.application_id, t.name, t.display_name, t.type, t.config, t.secrets,
	t.state, t.last_run_time, t.last_status, t.last_message, t.created_at, t.updated_at`

// connectorSelect：读取时把主键解析回应用标识（code）；存储只认主键（§3.8）
const connectorSelect = `SELECT ` + connectorColumns + `,
	COALESCE((SELECT a.code FROM applications a WHERE a.id = t.application_id), '') AS application
	FROM connectors t`

// ConnectorRepository 下游应用连接器仓库（PostgreSQL 实现）
type ConnectorRepository struct{ db *dbclient.Client }

// NewConnectorRepository 创建连接器仓库
func NewConnectorRepository(db *dbclient.Client) *ConnectorRepository {
	return &ConnectorRepository{db: db}
}

func scanConnector(row interface{ Scan(...interface{}) error }) (*repository.Connector, error) {
	var c repository.Connector
	var config, secrets []byte
	err := row.Scan(&c.ID, &c.InstitutionID, &c.ApplicationID, &c.Name, &c.DisplayName, &c.Type,
		&config, &secrets, &c.State,
		&c.LastRunTime, &c.LastStatus, &c.LastMessage, &c.CreatedAt, &c.UpdatedAt, &c.Application)
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
//
// 返回**待序列化的 Go 值**，序列化由 encodeJSONB 统一做一次（理由见 shrinkSyncer 的注释：
// 先 Marshal 再交给 jsonb 编解码器会二次编码成 base64 字符串，凭据随即读不回来）。
func shrinkConnector(c *repository.Connector) (map[string]string, map[string]string) {
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
	return config, secrets
}

// Create 创建连接器
func (r *ConnectorRepository) Create(ctx context.Context, c *repository.Connector) (int64, error) {
	config, secrets := shrinkConnector(c)
	var id int64
	err := r.db.QueryRow(ctx, `INSERT INTO connectors (institution_id, application_id, name, display_name,
		type, config, secrets, state)
		VALUES ($1,$2,$3,$4,COALESCE(NULLIF($5,''),'http'),$6,$7,COALESCE(NULLIF($8,''),'active'))
		RETURNING id`,
		c.InstitutionID, c.ApplicationID, c.Name, c.DisplayName, c.Type,
		encodeJSONB(config), encodeJSONB(secrets), c.State,
	).Scan(&id)
	if err != nil {
		return 0, mapError(err, "connector not found")
	}
	c.ID = id
	return id, nil
}

// GetByID 按 ID 查询
func (r *ConnectorRepository) GetByID(ctx context.Context, id int64) (*repository.Connector, error) {
	c, err := scanConnector(r.db.QueryRow(ctx, connectorSelect+` WHERE t.id=$1`, id))
	if err != nil {
		return nil, mapError(err, "connector not found")
	}
	return c, nil
}

// GetByName 按租户 + 名称查询
func (r *ConnectorRepository) GetByName(ctx context.Context, institutionID int64, name string) (*repository.Connector, error) {
	c, err := scanConnector(r.db.QueryRow(ctx,
		connectorSelect+` WHERE t.institution_id=$1 AND t.name=$2`, institutionID, name))
	if err != nil {
		return nil, mapError(err, "connector not found")
	}
	return c, nil
}

// Update 更新连接器（不含运行态字段）
func (r *ConnectorRepository) Update(ctx context.Context, c *repository.Connector) error {
	config, secrets := shrinkConnector(c)
	const q = `UPDATE connectors SET display_name=$3, application_id=$4, type=COALESCE(NULLIF($5,''),'http'),
		config=$6, secrets=$7, state=$8, updated_at=NOW()
		WHERE institution_id=$1 AND name=$2`
	return mapError(r.db.Exec(ctx, q, c.InstitutionID, c.Name, c.DisplayName, c.ApplicationID, c.Type,
		encodeJSONB(config), encodeJSONB(secrets), defaultStr(c.State, "active")), "connector not found")
}

// Delete 删除连接器
func (r *ConnectorRepository) Delete(ctx context.Context, id int64) error {
	return mapError(r.db.Exec(ctx, `DELETE FROM connectors WHERE id=$1`, id), "connector not found")
}

// List 分页列出连接器
func (r *ConnectorRepository) List(ctx context.Context, q repository.ListQuery) ([]*repository.Connector, error) {
	b := &queryBuilder{}
	if q.InstitutionID != 0 {
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.name ILIKE ` + p + ` OR t.display_name ILIKE ` + p + `)`)
	}
	sql := connectorSelect + b.where() + b.orderLimit("t.name ASC", q.Offset, q.Limit)
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
		b.add(`t.institution_id=` + b.arg(q.InstitutionID))
	}
	if q.Field != "" && q.Value != "" {
		b.add(`t.` + q.Field + `=` + b.arg(idFilterValue(q.Field, q.Value)))
	} else if q.Keyword != "" {
		p := b.arg(like(q.Keyword))
		b.add(`(t.name ILIKE ` + p + ` OR t.display_name ILIKE ` + p + `)`)
	}
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM connectors t`+b.where(), b.args...).Scan(&n)
	return n, mapError(err, "")
}

// ListEnabled 返回指定应用下启用的连接器（applicationID 为 0 时返回租户内全部启用连接器）
func (r *ConnectorRepository) ListEnabled(ctx context.Context, institutionID int64, applicationID int64) ([]*repository.Connector, error) {
	sql := connectorSelect + ` WHERE t.institution_id=$1 AND t.state='active'`
	args := []interface{}{institutionID}
	if applicationID != 0 {
		args = append(args, applicationID)
		sql += ` AND t.application_id=$2`
	}
	sql += ` ORDER BY t.name ASC`
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
