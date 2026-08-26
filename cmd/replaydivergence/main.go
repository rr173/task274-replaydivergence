// 处理器指令重放状态分歧定位服务入口。
// 支持 --addr 启动 HTTP 服务，或 --smoke-test 执行端到端自检（不驻留）。
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"task274-replaydivergence/internal/httpapi"
	"task274-replaydivergence/internal/model"
	"task274-replaydivergence/internal/service"
	"task274-replaydivergence/internal/store"
	"task274-replaydivergence/internal/trace"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	dbPath := flag.String("db", "task274-replaydivergence.db", "SQLite database path")
	smoke := flag.Bool("smoke-test", false, "run end-to-end smoke test and exit")
	flag.Parse()

	if *smoke {
		os.Exit(runSmokeTest())
	}

	logger := log.New(os.Stderr, "[replaydivergence] ", log.LstdFlags)
	st, err := store.Open(*dbPath)
	if err != nil {
		logger.Fatalf("open store: %v", err)
	}
	defer st.Close()
	svc := service.NewService(st)
	server := httpapi.NewServer(svc, logger)
	logger.Printf("listening on %s (db=%s)", *addr, *dbPath)
	if err := http.ListenAndServe(*addr, server.Handler()); err != nil {
		logger.Fatalf("serve: %v", err)
	}
}

// runSmokeTest 执行端到端自检：
// 创建批次 → 导入双轨迹 → 登记检查点 → 同步 → 指纹扫描 → 比较 → 依赖回溯 →
// 快照发布 → 关闭并重新打开数据库验证持久化与重启恢复。
// 任一断言失败返回非零退出码。
func runSmokeTest() int {
	logger := log.New(os.Stdout, "[smoke] ", log.LstdFlags)
	started := time.Now()
	dir, err := os.MkdirTemp("", "rd-smoke-*")
	if err != nil {
		logger.Printf("tempdir: %v", err)
		return 1
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "smoke.db")

	openStore := func() *store.Store {
		st, err := store.Open(dbPath)
		if err != nil {
			logger.Fatalf("open store: %v", err)
		}
		return st
	}
	fail := func(format string, args ...interface{}) int {
		logger.Printf("FAIL: "+format, args...)
		return 1
	}

	// --- 阶段 1：建批次与双轨迹导入 ---
	st := openStore()
	svc := service.NewService(st)

	b, err := svc.CreateBatch("smoke-replay", "ref-trail", "test-trail", "")
	if err != nil {
		return fail("create batch: %v", err)
	}
	logger.Printf("batch created id=%d status=%s", b.ID, b.Status)

	// 参考轨迹：rax=1 → rbx=1 → mem=1 → rcx=1。
	refInputs := []trace.EventInput{
		{Seq: 0, PC: "0x0000", Opcode: "mov", Reads: nil, Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x0004", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x1"}},
		{Seq: 2, PC: "0x0008", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x1"}},
		{Seq: 3, PC: "0x000c", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x1"}},
		{Seq: 4, PC: "0x0010", Opcode: "ret", Reads: nil, Writes: nil},
	}
	// 待测轨迹：seq1 写 rbx=0x2（分歧注入），其余同参考。
	testInputs := []trace.EventInput{
		{Seq: 0, PC: "0x0000", Opcode: "mov", Reads: nil, Writes: []string{"reg:rax"}, Values: map[string]string{"reg:rax": "0x1"}},
		{Seq: 1, PC: "0x0004", Opcode: "add", Reads: []string{"reg:rax"}, Writes: []string{"reg:rbx"}, Values: map[string]string{"reg:rbx": "0x2"}},
		{Seq: 2, PC: "0x0008", Opcode: "store", Reads: []string{"reg:rbx"}, Writes: []string{"mem:0x1000"}, Values: map[string]string{"mem:0x1000": "0x2"}},
		{Seq: 3, PC: "0x000c", Opcode: "load", Reads: []string{"mem:0x1000"}, Writes: []string{"reg:rcx"}, Values: map[string]string{"reg:rcx": "0x2"}},
		{Seq: 4, PC: "0x0010", Opcode: "ret", Reads: nil, Writes: nil},
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, refInputs); err != nil {
		return fail("import ref: %v", err)
	}
	if _, err := svc.ImportEvents(context.Background(), b.ID, model.TrailUnderTest, testInputs); err != nil {
		return fail("import test: %v", err)
	}
	// 幂等验证：重复导入同一片段应全被跳过。
	res2, err := svc.ImportEvents(context.Background(), b.ID, model.TrailReference, refInputs)
	if err != nil {
		return fail("re-import ref: %v", err)
	}
	if res2.Accepted != 0 {
		return fail("re-import should skip all, accepted=%d", res2.Accepted)
	}
	logger.Printf("trails imported (ref=5, test=5), idempotent re-import skipped=%d", len(res2.Skipped))

	// --- 阶段 2：检查点、同步、指纹 ---
	if _, err := svc.AddCheckpoint(b.ID, 4, model.CheckpointFull, map[string]string{
		"reg:rax": "0x1", "reg:rbx": "0x1", "reg:rcx": "0x1", "mem:0x1000": "0x1",
	}); err != nil {
		return fail("add checkpoint: %v", err)
	}
	syncRes, err := svc.Sync(b.ID)
	if err != nil {
		return fail("sync: %v", err)
	}
	if syncRes.Align.Aligned != 10 {
		return fail("align expected 10, got %d", syncRes.Align.Aligned)
	}
	logger.Printf("synced aligned=%d range=[%d,%d] status=%s",
		syncRes.Align.Aligned, syncRes.Align.SeqMin, syncRes.Align.SeqMax, syncRes.Batch.Status)
	scanned, err := svc.ScanFingerprints(context.Background(), b.ID)
	if err != nil {
		return fail("scan fingerprints: %v", err)
	}
	if scanned != 1 {
		return fail("scan fingerprints expected 1, got %d", scanned)
	}

	// --- 阶段 3：比较与回溯 ---
	comp, err := svc.Compare(context.Background(), b.ID)
	if err != nil {
		return fail("compare: %v", err)
	}
	if comp.Divergent != 1 {
		return fail("compare expected 1 divergence, got %d", comp.Divergent)
	}
	divs, err := svc.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs) != 1 {
		return fail("list divergences: n=%d err=%v", len(divs), err)
	}
	traceOut, err := svc.TraceDivergence(context.Background(), b.ID, divs[0].ID)
	if err != nil {
		return fail("trace divergence: %v", err)
	}
	d := traceOut.Divergence
	if d.FirstDivergentSeq != 1 {
		return fail("first divergent seq expected 1, got %d", d.FirstDivergentSeq)
	}
	if d.FirstDivergentOp != "add" {
		return fail("first divergent op expected add, got %s", d.FirstDivergentOp)
	}
	chain, err := svc.Divergences().ListChain(d.ID)
	if err != nil || len(chain) == 0 {
		return fail("chain empty or err: %v", err)
	}
	logger.Printf("compared divergent=%d; traced first_divergent=seq%d(%s @%s) chain_depth=%d status=%s",
		comp.Divergent, d.FirstDivergentSeq, d.FirstDivergentOp, d.FirstDivergentPC, len(chain), d.Status)

	// --- 阶段 4：快照发布与封存 ---
	snap, err := svc.CreateSnapshot(b.ID, "smoke-loc-snapshot")
	if err != nil {
		return fail("create snapshot: %v", err)
	}
	snap, err = svc.PublishSnapshot(context.Background(), b.ID, snap.ID)
	if err != nil {
		return fail("publish snapshot: %v", err)
	}
	cfg, err := snap.ParseConfig()
	if err != nil {
		return fail("parse snapshot config: %v", err)
	}
	if cfg.SeqMin != 0 || cfg.SeqMax != 4 || len(cfg.CheckpointSeqs) != 1 {
		return fail("snapshot config wrong: %+v", cfg)
	}
	if err := st.Close(); err != nil {
		return fail("close store: %v", err)
	}

	// --- 阶段 5：重启恢复验证 ---
	st2 := openStore()
	svc2 := service.NewService(st2)
	defer st2.Close()

	b2, err := svc2.Batches().Get(b.ID)
	if err != nil {
		return fail("reopen get batch: %v", err)
	}
	if b2.Status != model.BatchSealed {
		return fail("reopen batch status expected sealed, got %s", b2.Status)
	}
	divs2, err := svc2.Divergences().ListByBatch(b.ID)
	if err != nil || len(divs2) != 1 {
		return fail("reopen divergences: n=%d err=%v", len(divs2), err)
	}
	if divs2[0].FirstDivergentSeq != 1 || divs2[0].Status != model.DivConfirmed {
		return fail("reopen divergence lost: %+v", divs2[0])
	}
	snap2, err := svc2.Snapshots().Get(snap.ID)
	if err != nil {
		return fail("reopen snapshot: %v", err)
	}
	if snap2.Status != model.SnapPublished {
		return fail("reopen snapshot status expected published, got %s", snap2.Status)
	}
	sum, err := snap2.ParseSummary()
	if err != nil || sum.FirstDivergentSeq != 1 {
		return fail("reopen snapshot summary wrong: %+v err=%v", sum, err)
	}
	stats, err := svc2.Stats(b.ID)
	if err != nil {
		return fail("reopen stats: %v", err)
	}
	logger.Printf("recovery verified: batch=%s divergences=%d snapshot=%s chain_depth=%d fingerprints=%d",
		b2.Status, len(divs2), snap2.Status, sum.ChainDepth, stats.Fingerprints)
	logger.Printf("SMOKE TEST PASSED in %s", time.Since(started).Round(time.Millisecond))
	return 0
}
