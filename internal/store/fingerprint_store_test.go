package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"task274-replaydivergence/internal/model"
)

// newTestStore 打开一个临时 SQLite Store，专供测试。
func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestGetPairMissingReturnsErrFingerprintMissing 防止回归：
// 指纹尚未扫描或仅单侧写入时，GetPair 必须返回 ErrFingerprintMissing，
// 而非 (nil, nil)。否则上层 Compare 会拿到 nil pair + nil err，
// 继续解引用 pair.Matched() 触发空指针 panic，导致服务崩溃。
func TestGetPairMissingReturnsErrFingerprintMissing(t *testing.T) {
	st := newTestStore(t)
	fs := NewFingerprintStore(st.DB())
	now := time.Now().UTC()

	// 场景 1：两侧指纹均不存在（尚未扫描）。
	_, err := fs.GetPair(1, 4, model.ScopeFull)
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("missing both sides: expected ErrFingerprintMissing, got %v", err)
	}

	// 场景 2：仅写入参考侧，待测侧缺失。
	if err := fs.Upsert(&model.StateFingerprint{
		BatchID: 1, Side: model.TrailReference, Seq: 4,
		Scope: model.ScopeFull, Hash: "aaaa", Algo: "fnv1a-sorted", CreatedAt: now,
	}); err != nil {
		t.Fatalf("upsert ref: %v", err)
	}
	_, err = fs.GetPair(1, 4, model.ScopeFull)
	if !errors.Is(err, model.ErrFingerprintMissing) {
		t.Fatalf("missing test side: expected ErrFingerprintMissing, got %v", err)
	}

	// 场景 3：双侧写入后应正常返回成对指纹。
	if err := fs.Upsert(&model.StateFingerprint{
		BatchID: 1, Side: model.TrailUnderTest, Seq: 4,
		Scope: model.ScopeFull, Hash: "bbbb", Algo: "fnv1a-sorted", CreatedAt: now,
	}); err != nil {
		t.Fatalf("upsert test: %v", err)
	}
	pair, err := fs.GetPair(1, 4, model.ScopeFull)
	if err != nil {
		t.Fatalf("both sides present: unexpected err: %v", err)
	}
	if pair.RefHash != "aaaa" || pair.TestHash != "bbbb" {
		t.Fatalf("unexpected pair: ref=%s test=%s", pair.RefHash, pair.TestHash)
	}
}
