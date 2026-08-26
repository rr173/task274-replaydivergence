package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

const snapCols = "id, batch_id, name, status, config_json, summary_json, created_at, published_at, superseded_by"

// SnapshotStore 封装 loc_snapshots 表。
type SnapshotStore struct {
	db *sql.DB
}

// NewSnapshotStore 构造快照仓储。
func NewSnapshotStore(db *sql.DB) *SnapshotStore {
	return &SnapshotStore{db: db}
}

// Create 插入快照。
func (ss *SnapshotStore) Create(s *model.LocSnapshot) error {
	res, err := ss.db.Exec(
		`INSERT INTO loc_snapshots(batch_id, name, status, config_json, summary_json, created_at, published_at, superseded_by)
		 VALUES(?,?,?,?,?,?,?,?)`,
		s.BatchID, s.Name, string(s.Status), s.ConfigJSON, s.SummaryJSON,
		s.CreatedAt.Format(timeFmt), nil, s.SupersededBy)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

// Get 按 ID 查询快照。
func (ss *SnapshotStore) Get(snapID int64) (*model.LocSnapshot, error) {
	row := ss.db.QueryRow(`SELECT `+snapCols+` FROM loc_snapshots WHERE id=?`, snapID)
	return scanSnapshot(row)
}

// ListByBatch 列出批次快照。
func (ss *SnapshotStore) ListByBatch(batchID int64) ([]*model.LocSnapshot, error) {
	rows, err := ss.db.Query(
		`SELECT `+snapCols+` FROM loc_snapshots WHERE batch_id=? ORDER BY id ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.LocSnapshot{}
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListByBatchTx 在事务内列出批次快照。
func (ss *SnapshotStore) ListByBatchTx(tx *sql.Tx, batchID int64) ([]*model.LocSnapshot, error) {
	rows, err := tx.Query(
		`SELECT `+snapCols+` FROM loc_snapshots WHERE batch_id=? ORDER BY id ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.LocSnapshot{}
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Update 更新快照状态、摘要、发布时间与替代关系。
func (ss *SnapshotStore) Update(s *model.LocSnapshot) error {
	return ss.updateExec(ss.db, s)
}

// UpdateTx 在事务内更新快照。
func (ss *SnapshotStore) UpdateTx(tx *sql.Tx, s *model.LocSnapshot) error {
	return ss.updateExec(tx, s)
}

type snapExec interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func (ss *SnapshotStore) updateExec(ex snapExec, s *model.LocSnapshot) error {
	var pub interface{}
	if s.PublishedAt != nil {
		pub = s.PublishedAt.Format(timeFmt)
	}
	_, err := ex.Exec(
		`UPDATE loc_snapshots SET summary_json=?, published_at=? WHERE id=?`,
		s.SummaryJSON, pub, s.ID)
	_ = s.Status
	_ = s.SupersededBy
	return err
}

type snapScanner interface {
	Scan(dest ...interface{}) error
}

func scanSnapshot(sc snapScanner) (*model.LocSnapshot, error) {
	var s model.LocSnapshot
	var status, created string
	var pub sql.NullString
	if err := sc.Scan(&s.ID, &s.BatchID, &s.Name, &status, &s.ConfigJSON, &s.SummaryJSON,
		&created, &pub, &s.SupersededBy); err != nil {
		return nil, mapErr(err)
	}
	s.Status = model.SnapshotStatus(status)
	s.CreatedAt = parseTime(created)
	s.PublishedAt = scanNullableTime(pub)
	return &s, nil
}
