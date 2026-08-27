// Package store 提供 SQLite 持久化：建表迁移、事务边界与实体 CRUD。
package store

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"

	"task274-replaydivergence/internal/model"
)

// Store 持有数据库句柄，所有仓储共享同一个事务管理器。
// SQLite 单写者：互斥锁覆盖 Begin 到 Commit 的整个窗口。
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Open 打开（或创建）SQLite 数据库并执行建表迁移。
// 使用纯 Go 驱动 modernc.org/sqlite，CGO 无关，离线可构建。
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(32)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	return s.db.Close()
}

// DB 返回底层句柄（供事务使用）。
func (s *Store) DB() *sql.DB {
	return s.db
}

// migrate 幂等建表：所有表使用 IF NOT EXISTS，重启安全。
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS replay_batches (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			ref_trail_name TEXT NOT NULL,
			test_trail_name TEXT NOT NULL,
			status TEXT NOT NULL,
			seq_min INTEGER NOT NULL DEFAULT 0,
			seq_max INTEGER NOT NULL DEFAULT 0,
			hash_algo TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS exec_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			side TEXT NOT NULL,
			seq INTEGER NOT NULL,
			pc TEXT NOT NULL,
			opcode TEXT NOT NULL,
			stage TEXT NOT NULL,
			reads_json TEXT NOT NULL,
			writes_json TEXT NOT NULL,
			values_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(batch_id, side, seq)
		)`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			seq INTEGER NOT NULL,
			kind TEXT NOT NULL,
			values_json TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(batch_id, seq)
		)`,
		`CREATE TABLE IF NOT EXISTS state_fingerprints (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			side TEXT NOT NULL,
			seq INTEGER NOT NULL,
			scope TEXT NOT NULL,
			hash TEXT NOT NULL,
			algo TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(batch_id, side, seq, scope)
		)`,
		`CREATE TABLE IF NOT EXISTS rw_dependencies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			resource TEXT NOT NULL,
			writer_seq INTEGER NOT NULL,
			writer_pc TEXT NOT NULL,
			writer_op TEXT NOT NULL,
			reader_seq INTEGER NOT NULL,
			reader_pc TEXT NOT NULL,
			reader_op TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS divergences (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			checkpoint_seq INTEGER NOT NULL,
			status TEXT NOT NULL,
			first_divergent_seq INTEGER NOT NULL DEFAULT 0,
			first_divergent_pc TEXT NOT NULL DEFAULT '',
			first_divergent_op TEXT NOT NULL DEFAULT '',
			root_cause TEXT NOT NULL DEFAULT '',
			evidence TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS divergence_chain_edges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			divergence_id INTEGER NOT NULL,
			depth INTEGER NOT NULL,
			side TEXT NOT NULL,
			seq INTEGER NOT NULL,
			pc TEXT NOT NULL,
			opcode TEXT NOT NULL,
			resource TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS loc_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			config_json TEXT NOT NULL,
			summary_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			published_at TEXT,
			superseded_by INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_batch_side ON exec_events(batch_id, side)`,
		`CREATE INDEX IF NOT EXISTS idx_fp_batch ON state_fingerprints(batch_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_rwdep_batch ON rw_dependencies(batch_id, resource)`,
		`CREATE INDEX IF NOT EXISTS idx_chain_div ON divergence_chain_edges(divergence_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// WithTx 在单个事务中执行 fn，出错即回滚。
// 锁必须覆盖 Begin/fn/Commit 全程：SQLite 单写者，串行化写事务
// 才能避免并发写入相互踩踏 SQLite 写锁触发 BUSY 超时或死锁。
func (s *Store) WithTx(fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// mapErr 把 SQL 错误归一为领域错误。
func mapErr(err error) error {
	if err == sql.ErrNoRows {
		return model.ErrNotFound
	}
	return err
}
