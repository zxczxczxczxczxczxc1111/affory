package afforyprocess

import (
	"slices"
	"sort"
	"sync"
)

const maxDepth = 64
const maxRecords = 16384
const retentionTicks = 30 * 10000000

type Process struct {
	PID, Parent uint32
	Created     uint64
	Path        string
}

type identity struct {
	pid     uint32
	created uint64
}
type record struct {
	process Process
	paths   []string
	seen    uint64
}

// Graph retains observed lineage, never an executable-name approximation.
type Graph struct {
	mu      sync.Mutex
	records map[identity]record
	latest  map[uint32]identity
}

func NewGraph() *Graph {
	return &Graph{records: make(map[identity]record), latest: make(map[uint32]identity)}
}

func key(p Process) identity { return identity{p.PID, p.Created} }

func (g *Graph) KnownPath(pid uint32, created uint64) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.records[identity{pid, created}].process.Path
}

func (g *Graph) Observe(processes []Process, at uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ordered := slices.Clone(processes)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Created < ordered[j].Created })
	for _, p := range ordered {
		if p.Created == 0 || p.Path == "" {
			continue
		}
		k := key(p)
		r, known := g.records[k]
		paths := []string{p.Path}
		if known {
			paths = slices.Clone(r.paths)
			paths[0] = p.Path
		}
		if len(paths) == 1 && p.Parent != 0 && p.Parent != p.PID {
			if parent, ok := g.records[g.latest[p.Parent]]; ok &&
				parent.process.Created <= p.Created && parent.seen >= p.Created {
				paths = append(paths, parent.paths[:min(len(parent.paths), maxDepth-1)]...)
			}
		}
		g.records[k] = record{process: p, paths: paths, seen: at}
		if old, exists := g.latest[p.PID]; !exists || old.created <= p.Created {
			g.latest[p.PID] = k
		}
	}
	for k, r := range g.records {
		if at > r.seen && at-r.seen > retentionTicks {
			delete(g.records, k)
			if g.latest[k.pid] == k {
				delete(g.latest, k.pid)
			}
		}
	}
	if len(g.records) > maxRecords {
		oldest := make([]identity, 0, len(g.records))
		for k := range g.records {
			oldest = append(oldest, k)
		}
		sort.Slice(oldest, func(i, j int) bool { return g.records[oldest[i]].seen < g.records[oldest[j]].seen })
		for _, k := range oldest[:len(oldest)-maxRecords] {
			delete(g.records, k)
			if g.latest[k.pid] == k {
				delete(g.latest, k.pid)
			}
		}
	}
}

func (g *Graph) Paths(p Process) []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.records[key(p)]; ok {
		return slices.Clone(r.paths)
	}
	if p.Path != "" {
		return []string{p.Path}
	}
	return nil
}
