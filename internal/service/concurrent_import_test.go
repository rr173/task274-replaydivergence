package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

// newTestStore 打开一个临时 SQLite Store，返回服务与清理函数。
func newTestStore(t *testing.T) (*Service, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "rd-test-*")
	if err != nil {
		t.Fatalf("tmpdir: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("open: %v", err)
	}
	return NewService(st), func() {
		st.Close()
		os.RemoveAll(dir)
	}
}

// makeEventInput 构造单条写入 rax 的 mov 事件。
func makeEventInput(seq int64) trace.EventInput {
	return trace.EventInput{
		Seq:    seq,
		PC:     fmt.Sprintf("0x%x", seq*4),
		Opcode: "mov",
		Writes: []string{"reg:rax"},
		Values: map[string]string{"reg:rax": "0x1"},
	}
}

// TestConcurrentImportDistinctSeqs 验证：二十位验证工程师并发导入不同序号片段，
// 全部成功落库、互不踩踏、无遗漏、无崩溃。
func TestConcurrentImportDistinctSeqs(t *testing.T) {
	svc, cleanup := newTestStore(t)
	defer cleanup()

	b, err := svc.CreateBatch("concurrent", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	const workers = 20
	const perWorker = 50
	const total = workers * perWorker

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			inputs := make([]trace.EventInput, 0, perWorker)
			for i := 0; i < perWorker; i++ {
				inputs = append(inputs, makeEventInput(int64(w*perWorker + i)))
			}
			<-start
			res, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, inputs)
			if err != nil {
				errs <- fmt.Errorf("worker %d import failed: %w", w, err)
				return
			}
			if res.Accepted != perWorker {
				errs <- fmt.Errorf("worker %d expected %d accepted, got %d", w, perWorker, res.Accepted)
			}
		}(w)
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("%v", err)
	}

	// 落库校验：总事件数必须等于并发导入总量，每个序号恰好一条。
	min, max, err := svc.Events().SeqRange(b.ID, model.TrailReference)
	if err != nil {
		t.Fatalf("seq range: %v", err)
	}
	if min != 0 || max != total-1 {
		t.Fatalf("seq range expected [0,%d], got [%d,%d]", total-1, min, max)
	}
	events, err := svc.Events().ListBySide(b.ID, model.TrailReference, "")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != total {
		t.Fatalf("expected %d events, got %d", total, len(events))
	}
	seen := make(map[int64]bool, total)
	for _, e := range events {
		if e.Seq < 0 || e.Seq >= total {
			t.Fatalf("unexpected seq %d", e.Seq)
		}
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d in database", e.Seq)
		}
		seen[e.Seq] = true
	}
}

// TestConcurrentImportIdempotent 验证：并发重复导入同一批序号不崩溃、不报错，
// 第二轮全部被幂等跳过，数据库内无重复行。
func TestConcurrentImportIdempotent(t *testing.T) {
	svc, cleanup := newTestStore(t)
	defer cleanup()

	b, err := svc.CreateBatch("idempotent", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	const workers = 10
	const seqs = 40
	shared := make([]trace.EventInput, seqs)
	for i := 0; i < seqs; i++ {
		shared[i] = makeEventInput(int64(i))
	}

	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, shared); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent idempotent import failed: %v", err)
	}

	events, err := svc.Events().ListBySide(b.ID, model.TrailReference, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != seqs {
		t.Fatalf("expected exactly %d distinct events, got %d (duplicates leaked)", seqs, len(events))
	}
}

// TestConcurrentImportMixedSides 验证：同一批次两侧轨迹并发导入，
// 彼此独立落库、序号互不干扰。
func TestConcurrentImportMixedSides(t *testing.T) {
	svc, cleanup := newTestStore(t)
	defer cleanup()

	b, err := svc.CreateBatch("mixed", "ref", "test", "")
	if err != nil {
		t.Fatalf("create batch: %v", err)
	}

	const n = 100
	refInputs := make([]trace.EventInput, n)
	testInputs := make([]trace.EventInput, n)
	for i := 0; i < n; i++ {
		refInputs[i] = makeEventInput(int64(i))
		testInputs[i] = makeEventInput(int64(i))
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for side, inputs := range map[model.TrailSide][]trace.EventInput{
		model.TrailReference: refInputs,
		model.TrailUnderTest: testInputs,
	} {
		wg.Add(1)
		go func(side model.TrailSide, inputs []trace.EventInput) {
			defer wg.Done()
			if _, err := svc.ImportEvents(context.Background(), b.ID, side, inputs); err != nil {
				errs <- fmt.Errorf("side %s: %w", side, err)
			}
		}(side, inputs)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("%v", err)
	}

	for _, side := range model.ValidTrailSides() {
		events, err := svc.Events().ListBySide(b.ID, side, "")
		if err != nil {
			t.Fatalf("list %s: %v", side, err)
		}
		if len(events) != n {
			t.Fatalf("side %s expected %d events, got %d", side, n, len(events))
		}
	}
}
