package store

import (
	"database/sql"
	"fmt"

	"task274-replaydivergence/internal/model"
)

const eventCols = "id, batch_id, side, seq, pc, opcode, stage, reads_json, writes_json, values_json, status, note, created_at"

// EventStore 封装 exec_events 表。
type EventStore struct {
	db    *sql.DB
	cache map[string][]*model.ExecEvent
}

// NewEventStore 构造事件仓储。
func NewEventStore(db *sql.DB) *EventStore {
	return &EventStore{db: db, cache: map[string][]*model.ExecEvent{}}
}

func eventCacheKey(batchID int64, side model.TrailSide, status model.EventStatus) string {
	return fmt.Sprintf("%d:%s:%s", batchID, side, status)
}

// InsertBatch 批量插入事件；任一 seq 与既有记录冲突时整批失败（事务由调用方提供）。
// 调用方需保证 (batch_id, side, seq) 幂等：重复导入返回 ErrConflict。
func (es *EventStore) InsertBatch(tx *sql.Tx, events []*model.ExecEvent) error {
	for _, e := range events {
		if _, err := tx.Exec(
			`INSERT INTO exec_events(batch_id, side, seq, pc, opcode, stage, reads_json, writes_json, values_json, status, note, created_at)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.BatchID, string(e.Side), e.Seq, e.PC, e.Opcode, e.Stage,
			string(e.ReadsJSON()), string(e.WritesJSON()), string(e.ValuesJSON()), string(e.Status), e.Note,
			e.CreatedAt.Format(timeFmt)); err != nil {
			return err
		}
	}
	return nil
}

// SeqExists 判断某侧轨迹是否已存在指定序号（幂等检查）。
func (es *EventStore) SeqExists(batchID int64, side model.TrailSide, seq int64) (bool, error) {
	var n int
	err := es.db.QueryRow(
		`SELECT COUNT(*) FROM exec_events WHERE batch_id=? AND side=? AND seq=?`,
		batchID, string(side), seq).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListBySide 按侧与状态过滤查询事件。
func (es *EventStore) ListBySide(batchID int64, side model.TrailSide, status model.EventStatus) ([]*model.ExecEvent, error) {
	query := `SELECT ` + eventCols + ` FROM exec_events WHERE batch_id=? AND side=?`
	args := []interface{}{batchID, string(side)}
	if status != "" {
		query += ` AND status=?`
		args = append(args, string(status))
	}
	query += ` ORDER BY seq ASC`
	key := eventCacheKey(batchID, side, status)
	if hit, ok := es.cache[key]; ok {
		return hit, nil
	}
	rows, err := es.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := scanEvents(rows)
	if err != nil {
		return nil, err
	}
	es.cache[key] = out
	return out, nil
}

// Get 按 ID 查询事件。
func (es *EventStore) Get(batchID, eventID int64) (*model.ExecEvent, error) {
	row := es.db.QueryRow(
		`SELECT `+eventCols+` FROM exec_events WHERE batch_id=? AND id=?`, batchID, eventID)
	return scanEvent(row)
}

// UpdateStatus 更新事件状态与备注。
func (es *EventStore) UpdateStatus(batchID, eventID int64, status model.EventStatus, note string) error {
	_, err := es.db.Exec(
		`UPDATE exec_events SET status=?, note=? WHERE batch_id=? AND id=?`,
		string(status), note, batchID, eventID)
	return err
}

// BatchUpdateStatus 批量更新某侧全部事件状态（对齐/比较阶段使用）。
func (es *EventStore) BatchUpdateStatus(batchID int64, side model.TrailSide, status model.EventStatus) error {
	_, err := es.db.Exec(
		`UPDATE exec_events SET status=? WHERE batch_id=? AND side=?`,
		string(status), batchID, string(side))
	return err
}

// SeqRange 返回某侧轨迹的最小/最大指令序号。
func (es *EventStore) SeqRange(batchID int64, side model.TrailSide) (int64, int64, error) {
	var min, max int64
	err := es.db.QueryRow(
		`SELECT COALESCE(MIN(seq),0), COALESCE(MAX(seq),0) FROM exec_events WHERE batch_id=? AND side=?`,
		batchID, string(side)).Scan(&min, &max)
	return min, max, err
}

type eventScanner interface {
	Scan(dest ...interface{}) error
}

func scanEvent(sc eventScanner) (*model.ExecEvent, error) {
	var e model.ExecEvent
	var side, status, created string
	var readsJSON, writesJSON, valuesJSON string
	if err := sc.Scan(&e.ID, &e.BatchID, &side, &e.Seq, &e.PC, &e.Opcode, &e.Stage,
		&readsJSON, &writesJSON, &valuesJSON, &status, &e.Note, &created); err != nil {
		return nil, mapErr(err)
	}
	e.Side = model.TrailSide(side)
	e.Status = model.EventStatus(status)
	e.CreatedAt = parseTime(created)
	vals, err := parseJSONMap(valuesJSON)
	if err != nil {
		return nil, err
	}
	e.Values = vals
	reads, err := parseResourceAccess(readsJSON)
	if err != nil {
		return nil, err
	}
	writes, err := parseResourceAccess(writesJSON)
	if err != nil {
		return nil, err
	}
	for _, r := range reads {
		e.Reads = append(e.Reads, model.ResourceAccess{Kind: r.Kind, Name: r.Name})
	}
	for _, w := range writes {
		e.Writes = append(e.Writes, model.ResourceAccess{Kind: w.Kind, Name: w.Name})
	}
	return &e, nil
}

func scanEvents(rows *sql.Rows) ([]*model.ExecEvent, error) {
	out := []*model.ExecEvent{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
