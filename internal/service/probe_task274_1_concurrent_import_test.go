package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func TestConcurrentTrailImportsSerialized(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewService(st)
	b, err := svc.CreateBatch("concurrent", "ref", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	const workers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			_, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, []trace.EventInput{{
				Seq: seq, PC: fmt.Sprintf("0x%x", seq*4), Opcode: "mov",
				Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": fmt.Sprintf("0x%x", seq+1)},
			}})
			if err != nil {
				errCh <- fmt.Errorf("seq %d: %w", seq, err)
			}
		}(int64(i))
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	events, err := svc.Events().ListBySide(b.ID, model.TrailReference, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != workers {
		t.Fatalf("want %d events, got %d", workers, len(events))
	}
}
