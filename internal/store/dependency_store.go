package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

// DependencyStore 封装 rw_dependencies 表。
type DependencyStore struct {
	db *sql.DB
}

// NewDependencyStore 构造依赖仓储。
func NewDependencyStore(db *sql.DB) *DependencyStore {
	return &DependencyStore{db: db}
}

// ReplaceAll 事务内清空并重建某批次的依赖边（回溯前固化依赖图快照）。
func (ds *DependencyStore) ReplaceAll(tx *sql.Tx, batchID int64, edges []*model.RWEdge) error {
	if _, err := tx.Exec(`DELETE FROM rw_dependencies WHERE batch_id=?`, batchID); err != nil {
		return err
	}
	for _, e := range edges {
		if _, err := tx.Exec(
			`INSERT INTO rw_dependencies(batch_id, resource, writer_seq, writer_pc, writer_op, reader_seq, reader_pc, reader_op)
			 VALUES(?,?,?,?,?,?,?,?)`,
			e.BatchID, e.Resource, e.WriterSeq, e.WriterPC, e.WriterOp,
			e.ReaderSeq, e.ReaderPC, e.ReaderOp); err != nil {
			return err
		}
	}
	return nil
}

// ListByBatch 列出批次全部依赖边。
func (ds *DependencyStore) ListByBatch(batchID int64) ([]*model.RWEdge, error) {
	rows, err := ds.db.Query(
		`SELECT id, batch_id, resource, writer_seq, writer_pc, writer_op, reader_seq, reader_pc, reader_op
		 FROM rw_dependencies WHERE batch_id=? ORDER BY id ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.RWEdge{}
	for rows.Next() {
		var e model.RWEdge
		if err := rows.Scan(&e.ID, &e.BatchID, &e.Resource, &e.WriterSeq, &e.WriterPC, &e.WriterOp,
			&e.ReaderSeq, &e.ReaderPC, &e.ReaderOp); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// CountByBatch 统计批次依赖边数量。
func (ds *DependencyStore) CountByBatch(batchID int64) (int64, error) {
	var n int64
	err := ds.db.QueryRow(`SELECT COUNT(*) FROM rw_dependencies WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}
