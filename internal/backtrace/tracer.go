package backtrace

import (
	"fmt"
	"sort"

	"task274-replaydivergence/internal/model"
)

// TraceResult 是一次依赖回溯的定位结果。
type TraceResult struct {
	DivergenceID int64                    `json:"divergence_id"`
	FirstSeq     int64                    `json:"first_seq"`
	FirstPC      string                   `json:"first_pc"`
	FirstOp      string                   `json:"first_op"`
	RootCause    string                   `json:"root_cause"`
	Status       model.DivergenceStatus   `json:"status"`
	ChainNodes   []*model.ChainEdge       `json:"chain_nodes"`
}

// Tracer 沿依赖边回溯首次分歧：
// 1. 对齐双轨迹后逐指令比较写入值，第一个写入偏差的待测指令即首次分歧；
// 2. 沿该指令的读资源向参考轨迹上游展开依赖链，刻画差异如何传播；
// 3. 若差异资源在待测轨迹从未被写入（只存在于参考轨迹），判定证据不足。
type Tracer struct {
	batchID   int64
	edges     []*model.RWEdge
	events    map[int64]*model.ExecEvent // 待测事件按 seq 索引
	refEvents map[int64]*model.ExecEvent // 参考事件按 seq 索引
}

// NewTracer 构造回溯器。
func NewTracer(batchID int64, edges []*model.RWEdge, testEvents, refEvents []*model.ExecEvent) *Tracer {
	t := &Tracer{
		batchID:   batchID,
		edges:     edges,
		events:    map[int64]*model.ExecEvent{},
		refEvents: map[int64]*model.ExecEvent{},
	}
	for _, e := range testEvents {
		t.events[e.Seq] = e
	}
	for _, e := range refEvents {
		t.refEvents[e.Seq] = e
	}
	return t
}

// Trace 从差异资源集合回溯首次分歧。
// divergentResources: 检查点处状态不一致的资源键（reg:rax 等）。
// checkpointSeq: 触发差异的检查点序号。
func (t *Tracer) Trace(divergentResources []string, checkpointSeq int64, divergenceID int64) *TraceResult {
	res := &TraceResult{DivergenceID: divergenceID}
	if len(divergentResources) == 0 {
		res.Status = model.DivInsufficient
		res.RootCause = "no divergent resources identified"
		return res
	}
	// 逐指令比较写入值，定位第一个偏差指令（首次分歧）。
	first := t.firstDivergentEvent(checkpointSeq)
	if first == nil {
		res.Status = model.DivInsufficient
		res.RootCause = "divergent resources never written differently by under-test trail"
		return res
	}
	res.FirstSeq = first.Seq
	res.FirstPC = first.PC
	res.FirstOp = first.Opcode
	res.RootCause = fmt.Sprintf(
		"first divergent instruction: under-test seq=%d (%s) at %s writes a value differing from reference",
		first.Seq, first.Opcode, first.PC)
	res.Status = model.DivFirst
	res.ChainNodes = append(res.ChainNodes, &model.ChainEdge{
		DivergenceID: divergenceID,
		Depth:        0,
		Side:         string(model.TrailUnderTest),
		Seq:          first.Seq,
		PC:           first.PC,
		Opcode:       first.Opcode,
		Role:         "writer",
	})
	// 沿首次分歧指令的读资源向参考轨迹上游展开依赖传播链。
	res.ChainNodes = append(res.ChainNodes, t.expandChain(first, 1, map[int64]bool{first.Seq: true})...)
	res.Status = model.DivConfirmed
	return res
}

// firstDivergentEvent 返回按 seq 升序第一个写入值与参考轨迹不一致的待测指令。
func (t *Tracer) firstDivergentEvent(checkpointSeq int64) *model.ExecEvent {
	seqs := make([]int64, 0, len(t.events))
	for seq := range t.events {
		if seq <= checkpointSeq {
			seqs = append(seqs, seq)
		}
	}
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	for _, seq := range seqs {
		testE := t.events[seq]
		refE, ok := t.refEvents[seq]
		if !ok {
			continue // 该序号参考轨迹缺失，跳过
		}
		if writesDiffer(testE, refE) {
			return testE
		}
	}
	return nil
}

// writesDiffer 比较两侧指令的写入值：存在同资源值不同即判定偏差。
func writesDiffer(testE, refE *model.ExecEvent) bool {
	for _, w := range testE.Writes {
		testVal, testHas := testE.Values[w.Key()]
		refVal, refHas := refE.Values[w.Key()]
		if testHas && refHas && testVal != refVal {
			return true
		}
	}
	return false
}

// expandChain 从首次分歧指令的读资源出发，找参考轨迹写者并逐层展开依赖链。
func (t *Tracer) expandChain(writer *model.ExecEvent, depth int, visited map[int64]bool) []*model.ChainEdge {
	nodes := []*model.ChainEdge{}
	queue := []*model.ExecEvent{writer}
	queueDepth := []int{depth}
	for len(queue) > 0 {
		cur := queue[0]
		curDepth := queueDepth[0]
		queue = queue[1:]
		queueDepth = queueDepth[1:]
		for _, rd := range cur.Reads {
			// 参考轨迹中最近写入该资源的指令。
			if refW := t.latestRefWriter(rd.Key(), cur.Seq); refW != nil {
				if visited[refW.Seq] {
					continue
				}
				visited[refW.Seq] = true
				nodes = append(nodes, &model.ChainEdge{
					DivergenceID: 0, // 由调用方回填
					Depth:        curDepth,
					Side:         string(model.TrailReference),
					Seq:          refW.Seq,
					PC:           refW.PC,
					Opcode:       refW.Opcode,
					Resource:     rd.Key(),
					Role:         "reader",
				})
				queue = append(queue, refW)
				queueDepth = append(queueDepth, curDepth+1)
			}
		}
	}
	return nodes
}

// latestRefWriter 返回参考轨迹中 seq <= limit 且写入指定资源的最近指令。
func (t *Tracer) latestRefWriter(resource string, limit int64) *model.ExecEvent {
	var best *model.ExecEvent
	for seq, e := range t.refEvents {
		if seq > limit {
			continue
		}
		for _, w := range e.Writes {
			if w.Key() == resource {
				if best == nil || e.Seq > best.Seq {
					best = e
				}
			}
		}
	}
	return best
}

// SortEdges 按读者序号排序依赖边（结果展示稳定）。
func SortEdges(edges []*model.RWEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].ReaderSeq != edges[j].ReaderSeq {
			return edges[i].ReaderSeq < edges[j].ReaderSeq
		}
		return edges[i].Resource < edges[j].Resource
	})
}
