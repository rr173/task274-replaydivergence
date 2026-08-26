package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

const cpCols = "id, batch_id, seq, kind, values_json, status, reason, created_at"

// CheckpointStore 封装 checkpoints 表。
type CheckpointStore struct {
	db *sql.DB
}

// NewCheckpointStore 构造检查点仓储。
func NewCheckpointStore(db *sql.DB) *CheckpointStore {
	return &CheckpointStore{db: db}
}

// Create 插入检查点（同批次同 seq 唯一，冲突返回 ErrConflict）。
func (cs *CheckpointStore) Create(c *model.Checkpoint) error {
	res, err := cs.db.Exec(
		`INSERT INTO checkpoints(batch_id, seq, kind, values_json, status, reason, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		c.BatchID, c.Seq, string(c.Kind), string(c.ValuesJSON()), c.Status, c.Reason,
		c.CreatedAt.Format(timeFmt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

// ListByBatch 列出批次全部检查点。
func (cs *CheckpointStore) ListByBatch(batchID int64) ([]*model.Checkpoint, error) {
	rows, err := cs.db.Query(
		`SELECT `+cpCols+` FROM checkpoints WHERE batch_id=? ORDER BY seq ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Checkpoint{}
	for rows.Next() {
		c, err := scanCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get 按 ID 查询检查点。
func (cs *CheckpointStore) Get(batchID, cpID int64) (*model.Checkpoint, error) {
	row := cs.db.QueryRow(
		`SELECT `+cpCols+` FROM checkpoints WHERE batch_id=? AND id=?`, batchID, cpID)
	return scanCheckpoint(row)
}

// UpdateStatus 更新可信状态与原因。
func (cs *CheckpointStore) UpdateStatus(c *model.Checkpoint) error {
	_, err := cs.db.Exec(
		`UPDATE checkpoints SET status=?, reason=? WHERE id=?`,
		c.Status, c.Reason, c.ID)
	return err
}

type cpScanner interface {
	Scan(dest ...interface{}) error
}

func scanCheckpoint(sc cpScanner) (*model.Checkpoint, error) {
	var c model.Checkpoint
	var kind, status, created string
	var valuesJSON string
	if err := sc.Scan(&c.ID, &c.BatchID, &c.Seq, &kind, &valuesJSON, &status, &c.Reason, &created); err != nil {
		return nil, mapErr(err)
	}
	c.Kind = model.CheckpointKind(kind)
	c.Status = status
	c.CreatedAt = parseTime(created)
	vals, err := parseJSONMap(valuesJSON)
	if err != nil {
		return nil, err
	}
	c.Values = vals
	return &c, nil
}
