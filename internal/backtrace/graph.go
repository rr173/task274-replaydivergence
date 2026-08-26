// Package backtrace 构建读写依赖图并沿依赖边回溯首次状态分歧。
package backtrace

import (
	"task274-replaydivergence/internal/model"
)

// Writer 是某资源的最近写者信息。
type Writer struct {
	Seq  int64
	PC   string
	Op   string
}

// GraphBuilder 从双轨迹构建读写依赖边。
// 规则：参考轨迹按序维护"资源 -> 最近写者"；待测轨迹每读一个资源，
// 若该资源存在参考写者，则生成一条 (writer=ref, reader=test) 依赖边。
// 这刻画了待测实现复用参考实现写入状态的事实依赖。
type GraphBuilder struct {
	batchID int64
	edges   []*model.RWEdge
	byRes   map[string][]*model.RWEdge
}

// NewGraphBuilder 构造依赖图构建器。
func NewGraphBuilder(batchID int64) *GraphBuilder {
	return &GraphBuilder{batchID: batchID, byRes: map[string][]*model.RWEdge{}}
}

// Build 基于双侧事件构建依赖图。
// refEvents / testEvents 按 seq 升序。
func (g *GraphBuilder) Build(refEvents, testEvents []*model.ExecEvent) ([]*model.RWEdge, error) {
	lastWriter := map[string]Writer{}
	for _, e := range refEvents {
		for _, w := range e.Writes {
			lastWriter[w.Key()] = Writer{Seq: e.Seq, PC: e.PC, Op: e.Opcode}
		}
	}
	for _, e := range testEvents {
		for _, rd := range e.Reads {
			w, ok := lastWriter[rd.Key()]
			if !ok {
				continue
			}
			edge := &model.RWEdge{
				BatchID:  g.batchID,
				Resource: rd.Key(),
				WriterSeq: w.Seq,
				WriterPC:  w.PC,
				WriterOp:  w.Op,
				ReaderSeq: e.Seq,
				ReaderPC:  e.PC,
				ReaderOp:  e.Opcode,
			}
			g.edges = append(g.edges, edge)
			g.byRes[rd.Key()] = append(g.byRes[rd.Key()], edge)
		}
	}
	return g.edges, nil
}

// WritersOf 返回读取指定资源的最近依赖边（按读者序号升序）。
func (g *GraphBuilder) WritersOf(resource string) []*model.RWEdge {
	return g.byRes[resource]
}
