package store

import (
	"database/sql"

	"task274-replaydivergence/internal/model"
)

const fpCols = "id, batch_id, side, seq, scope, hash, algo, created_at"

// FingerprintStore 封装 state_fingerprints 表。
type FingerprintStore struct {
	db *sql.DB
}

// NewFingerprintStore 构造指纹仓储。
func NewFingerprintStore(db *sql.DB) *FingerprintStore {
	return &FingerprintStore{db: db}
}

// Upsert 以 (batch_id, side, seq, scope) 为键幂等写入指纹。
func (fs *FingerprintStore) Upsert(f *model.StateFingerprint) error {
	_, err := fs.db.Exec(
		`INSERT INTO state_fingerprints(batch_id, side, seq, scope, hash, algo, created_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(batch_id, side, seq, scope) DO UPDATE SET hash=excluded.hash, algo=excluded.algo`,
		f.BatchID, string(f.Side), f.Seq, string(f.Scope), f.Hash, f.Algo,
		f.CreatedAt.Format(timeFmt))
	return err
}

// GetPair 返回同一 (seq, scope) 的双侧指纹对；任一侧缺失（未扫描或仅单侧写入）
// 返回 ErrFingerprintMissing，由调用方据此跳过或中止比较。
func (fs *FingerprintStore) GetPair(batchID int64, seq int64, scope model.FingerprintScope) (*model.FingerprintPair, error) {
	var pair model.FingerprintPair
	pair.Seq = seq
	pair.Scope = scope

	var refHash, testHash string
	var refFound, testFound bool
	rows, err := fs.db.Query(
		`SELECT side, hash FROM state_fingerprints WHERE batch_id=? AND seq=? AND scope=?`,
		batchID, seq, string(scope))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var side, hash string
		if err := rows.Scan(&side, &hash); err != nil {
			return nil, err
		}
		switch model.TrailSide(side) {
		case model.TrailReference:
			refHash, refFound = hash, true
		case model.TrailUnderTest:
			testHash, testFound = hash, true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !refFound || !testFound {
		// 任一侧指纹缺失：必须返回可识别错误而非 nil，否则上层 Compare 会拿到
		// nil pair + nil err，继续解引用 pair 触发空指针崩溃。
		return nil, model.ErrFingerprintMissing
	}
	pair.RefHash = refHash
	pair.TestHash = testHash
	return &pair, nil
}

// CountByBatch 统计批次指纹数量。
func (fs *FingerprintStore) CountByBatch(batchID int64) (int64, error) {
	var n int64
	err := fs.db.QueryRow(`SELECT COUNT(*) FROM state_fingerprints WHERE batch_id=?`, batchID).Scan(&n)
	return n, err
}

// ListByBatch 列出批次全部指纹。
func (fs *FingerprintStore) ListByBatch(batchID int64) ([]*model.StateFingerprint, error) {
	rows, err := fs.db.Query(
		`SELECT `+fpCols+` FROM state_fingerprints WHERE batch_id=? ORDER BY seq ASC, side ASC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.StateFingerprint{}
	for rows.Next() {
		var f model.StateFingerprint
		var side, scope, created string
		if err := rows.Scan(&f.ID, &f.BatchID, &side, &f.Seq, &scope, &f.Hash, &f.Algo, &created); err != nil {
			return nil, err
		}
		f.Side = model.TrailSide(side)
		f.Scope = model.FingerprintScope(scope)
		f.CreatedAt = parseTime(created)
		out = append(out, &f)
	}
	return out, rows.Err()
}
