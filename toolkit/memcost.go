package toolkit

import (
	"context"
	"fmt"
	"io"
	"time"

	. "github.com/iodesystems/mcpshell/runtime"
)

// Memory costs of the loadable toolkits, for the memory report (see
// runtime.MemCoster).

// MemCost reports the pages of the scratch database. SQLite allocates them
// outside the Go heap, so the memory limit does not count them; --db-max-mb
// caps them. Per-table sizes come from dbstat; without it the total stands.
func (st *dbState) MemCost() MemCost {
	mc := MemCost{OffHeap: true, Free: `Db.drop("table") or unload("db")`}
	if !st.mu.TryRLock() {
		mc.Detail = "busy"
		return mc
	}
	defer st.mu.RUnlock()
	if st.db == nil {
		return mc
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// A running command holds the only connection: use it, as tableList does.
	c := st.cur.Load()
	if c == nil {
		var err error
		if c, err = st.db.Conn(ctx); err != nil {
			return mc
		}
		defer c.Close()
	}
	used, _ := st.sizeBytes(ctx, c)
	mc.Bytes = used
	rows := c.QueryContext
	r, err := rows(ctx, `SELECT m.tbl_name, SUM(d.pgsize) FROM dbstat d JOIN sqlite_master m ON m.name = d.name
		WHERE m.tbl_name NOT LIKE 'sqlite_%' GROUP BY m.tbl_name ORDER BY 2 DESC`)
	if err != nil {
		return mc
	}
	defer r.Close()
	tables, top, topBytes := 0, "", int64(0)
	for r.Next() {
		var name string
		var n int64
		if r.Scan(&name, &n) != nil {
			break
		}
		if tables == 0 {
			top, topBytes = name, n
		}
		tables++
	}
	switch tables {
	case 0:
		mc.Detail = "no tables"
	case 1:
		mc.Detail = fmt.Sprintf("1 table %q", top)
		mc.Free = fmt.Sprintf(`Db.drop(%q) or unload("db")`, top)
	default:
		mc.Detail = fmt.Sprintf("%d tables; largest %q %s", tables, top, FormatBytes(topBytes))
		mc.Free = fmt.Sprintf(`Db.drop(%q) or unload("db")`, top)
	}
	return mc
}

// graphCloser is the closer of the graph loadable; it reports the store's cost.
type graphCloser struct{ gt *graphToolkit }

func (graphCloser) Close() error { return nil }

var _ io.Closer = graphCloser{}

const (
	graphNodeBytes = 200 // node struct, map slots, order and edge-list entries
	graphEdgeBytes = 250
)

// MemCost estimates the graph store: a flat cost per node and edge, and the
// properties by walking them.
func (g graphCloser) MemCost() MemCost {
	gt := g.gt
	mc := MemCost{Free: `unload("graph")`}
	if !gt.mu.TryLock() {
		mc.Detail = "busy"
		return mc
	}
	defer gt.mu.Unlock()
	st := gt.store
	var props []Value
	for _, n := range st.nodeMap {
		if n.props != nil {
			props = append(props, n.props)
		}
	}
	for _, e := range st.edgeMap {
		if e.props != nil {
			props = append(props, e.props)
		}
	}
	sizes, _ := EstimateSizes(props, 1_000_000)
	for _, s := range sizes {
		mc.Bytes += s
	}
	mc.Bytes += int64(len(st.nodeMap))*graphNodeBytes + int64(len(st.edgeMap))*graphEdgeBytes
	mc.Detail = fmt.Sprintf("%s nodes, %s edges", Count(len(st.nodeMap)), Count(len(st.edgeMap)))
	return mc
}
