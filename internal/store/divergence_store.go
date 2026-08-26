package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

const divCols = "id, batch_id, checkpoint_seq, status, first_divergent_seq, first_divergent_pc, first_divergent_op, root_cause, evidence, created_at, updated_at"

// DivergenceStore 封装 divergences 与 divergence_chain_edges 表。
type DivergenceStore struct {
	db *sql.DB
}

// NewDivergenceStore 构造分歧仓储。
func NewDivergenceStore(db *sql.DB) *DivergenceStore {
	return &DivergenceStore{db: db}
}

// Create 插入候选分歧。
func (ds *DivergenceStore) Create(d *model.Divergence) error {
	res, err := ds.db.Exec(
		`INSERT INTO divergences(batch_id, checkpoint_seq, status, first_divergent_seq, first_divergent_pc, first_divergent_op, root_cause, evidence, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		d.BatchID, d.CheckpointSeq, string(d.Status), d.FirstDivergentSeq,
		d.FirstDivergentPC, d.FirstDivergentOp, d.RootCause, d.Evidence,
		d.CreatedAt.Format(timeFmt), d.UpdatedAt.Format(timeFmt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	d.ID = id
	return nil
}

// Get 按 ID 查询分歧。
func (ds *DivergenceStore) Get(batchID, divID int64) (*model.Divergence, error) {
	row := ds.db.QueryRow(
		`SELECT `+divCols+` FROM divergences WHERE batch_id=? AND id=?`, batchID, divID)
	return scanDivergence(row)
}

// ListByBatch 列出批次全部分歧。
func (ds *DivergenceStore) ListByBatch(batchID int64) ([]*model.Divergence, error) {
	rows, err := ds.db.Query(
		`SELECT `+divCols+` FROM divergences WHERE batch_id=? ORDER BY id ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Divergence{}
	for rows.Next() {
		d, err := scanDivergence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Update 更新分歧判定字段。
func (ds *DivergenceStore) Update(d *model.Divergence) error {
	_, err := ds.db.Exec(
		`UPDATE divergences SET status=?, first_divergent_seq=?, first_divergent_pc=?, first_divergent_op=?, root_cause=?, evidence=?, updated_at=?
		 WHERE id=?`,
		string(d.Status), d.FirstDivergentSeq, d.FirstDivergentPC, d.FirstDivergentOp,
		d.RootCause, d.Evidence, d.UpdatedAt.Format(timeFmt), d.ID)
	return err
}

// SaveChain 事务内保存依赖链节点（覆盖写，保证与分歧状态一致）。
func (ds *DivergenceStore) SaveChain(tx *sql.Tx, divID int64, nodes []*model.ChainEdge) error {
	if _, err := tx.Exec(`DELETE FROM divergence_chain_edges WHERE divergence_id=?`, divID); err != nil {
		return err
	}
	for _, n := range nodes {
		if _, err := tx.Exec(
			`INSERT INTO divergence_chain_edges(divergence_id, depth, side, seq, pc, opcode, resource, role)
			 VALUES(?,?,?,?,?,?,?,?)`,
			n.DivergenceID, n.Depth, n.Side, n.Seq, n.PC, n.Opcode, n.Resource, n.Role); err != nil {
			return err
		}
	}
	return nil
}

// ListChain 列出分歧的依赖链节点。
func (ds *DivergenceStore) ListChain(divID int64) ([]*model.ChainEdge, error) {
	rows, err := ds.db.Query(
		`SELECT id, divergence_id, depth, side, seq, pc, opcode, resource, role
		 FROM divergence_chain_edges WHERE divergence_id=? ORDER BY depth ASC`, divID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ChainEdge{}
	for rows.Next() {
		var n model.ChainEdge
		if err := rows.Scan(&n.ID, &n.DivergenceID, &n.Depth, &n.Side, &n.Seq, &n.PC, &n.Opcode, &n.Resource, &n.Role); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

// MaxDepth 返回分歧依赖链最大深度。
func (ds *DivergenceStore) MaxDepth(divID int64) (int, error) {
	var d int
	err := ds.db.QueryRow(`SELECT COALESCE(MAX(depth),0) FROM divergence_chain_edges WHERE divergence_id=?`, divID).Scan(&d)
	return d, err
}

type divScanner interface {
	Scan(dest ...interface{}) error
}

func scanDivergence(sc divScanner) (*model.Divergence, error) {
	var d model.Divergence
	var status, created, updated string
	if err := sc.Scan(&d.ID, &d.BatchID, &d.CheckpointSeq, &status, &d.FirstDivergentSeq,
		&d.FirstDivergentPC, &d.FirstDivergentOp, &d.RootCause, &d.Evidence, &created, &updated); err != nil {
		return nil, mapErr(err)
	}
	d.Status = model.DivergenceStatus(status)
	d.CreatedAt = parseTime(created)
	d.UpdatedAt = parseTime(updated)
	return &d, nil
}
