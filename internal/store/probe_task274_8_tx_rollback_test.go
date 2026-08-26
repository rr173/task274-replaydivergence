package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"task274-replaydivergence/internal/model"
)

func TestFailedTxDoesNotPersist(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	bs := NewBatchStore(st.DB())
	err = st.WithTx(func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(timeFmt)
		_, err := tx.Exec(
			`INSERT INTO replay_batches(name, ref_trail_name, test_trail_name, status, seq_min, seq_max, hash_algo, created_at, updated_at)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			"ghost", "ref", "test", string(model.BatchReceiving), 0, 0, "fnv1a-sorted", now, now)
		if err != nil {
			return err
		}
		return fmt.Errorf("boom")
	})
	if err == nil {
		t.Fatal("withtx must return the callback error")
	}
	all, err := bs.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("failed tx must roll back, found %d batches", len(all))
	}
	b := model.NewReplayBatch("ok", "ref", "test", "")
	if err := bs.Create(b); err != nil {
		t.Fatalf("subsequent write: %v", err)
	}
}
