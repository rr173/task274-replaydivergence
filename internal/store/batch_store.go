package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

const batchCols = "id, name, ref_trail_name, test_trail_name, status, seq_min, seq_max, hash_algo, created_at, updated_at"

// BatchStore 封装 replay_batches 表。
type BatchStore struct {
	db *sql.DB
}

// NewBatchStore 构造批次仓储。
func NewBatchStore(db *sql.DB) *BatchStore {
	return &BatchStore{db: db}
}

// Create 插入批次并回填自增 ID。
func (bs *BatchStore) Create(b *model.ReplayBatch) error {
	res, err := bs.db.Exec(
		`INSERT INTO replay_batches(name, ref_trail_name, test_trail_name, status, seq_min, seq_max, hash_algo, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		b.Name, b.RefTrailName, b.TestTrailName, string(b.Status),
		b.SeqMin, b.SeqMax, b.HashAlgo, b.CreatedAt.Format(timeFmt), b.UpdatedAt.Format(timeFmt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	b.ID = id
	return nil
}

// Get 按 ID 查询批次，不存在返回 ErrNotFound。
func (bs *BatchStore) Get(id int64) (*model.ReplayBatch, error) {
	row := bs.db.QueryRow(`SELECT `+batchCols+` FROM replay_batches WHERE id=?`, id)
	return scanBatch(row)
}

// List 列出全部批次（按创建时间倒序）。
func (bs *BatchStore) List() ([]*model.ReplayBatch, error) {
	rows, err := bs.db.Query(`SELECT ` + batchCols + ` FROM replay_batches ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ReplayBatch{}
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Update 更新批次全部可变字段。
func (bs *BatchStore) Update(b *model.ReplayBatch) error {
	_, err := bs.db.Exec(
		`UPDATE replay_batches SET name=?, status=?, seq_min=?, seq_max=?, updated_at=? WHERE id=?`,
		b.Name, string(b.Status), b.SeqMin, b.SeqMax, b.UpdatedAt.Format(timeFmt), b.ID)
	return err
}

// UpdateTx 在事务内更新批次。
func (bs *BatchStore) UpdateTx(tx *sql.Tx, b *model.ReplayBatch) error {
	_, err := tx.Exec(
		`UPDATE replay_batches SET name=?, status=?, seq_min=?, seq_max=?, updated_at=? WHERE id=?`,
		b.Name, string(b.Status), b.SeqMin, b.SeqMax, b.UpdatedAt.Format(timeFmt), b.ID)
	return err
}

// CountEventsByStatus 统计某侧轨迹各状态事件数量。
func (bs *BatchStore) CountEventsByStatus(batchID int64, side model.TrailSide) (map[model.EventStatus]int64, error) {
	rows, err := bs.db.Query(
		`SELECT status, COUNT(*) FROM exec_events WHERE batch_id=? AND side=? GROUP BY status`,
		batchID, string(side))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[model.EventStatus]int64{}
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[model.EventStatus(st)] = n
	}
	return out, rows.Err()
}

type batchScanner interface {
	Scan(dest ...interface{}) error
}

func scanBatch(sc batchScanner) (*model.ReplayBatch, error) {
	var b model.ReplayBatch
	var status, created, updated string
	if err := sc.Scan(&b.ID, &b.Name, &b.RefTrailName, &b.TestTrailName, &status,
		&b.SeqMin, &b.SeqMax, &b.HashAlgo, &created, &updated); err != nil {
		return nil, mapErr(err)
	}
	b.Status = model.BatchStatus(status)
	b.CreatedAt = parseTime(created)
	b.UpdatedAt = parseTime(updated)
	return &b, nil
}
