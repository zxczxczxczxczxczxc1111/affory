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
	observed    uint64
}

type identity struct {
	pid     uint32
	created uint64
}
type record struct {
	process    Process
	paths      []string
	seen       uint64
	exited     uint64
	unresolved uint64
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
		unresolved := p.Created
		if p.Parent <= 4 {
			unresolved = 0
		}
		if known && len(r.paths) > 0 {
			paths = slices.Clone(r.paths)
			paths[0] = p.Path
			unresolved = r.unresolved
		}
		if len(paths) == 1 && p.Parent != 0 && p.Parent != p.PID {
			if parent, ok := g.parentAt(p.Parent, p.Created); ok {
				paths = append(paths, parent.paths[:min(len(parent.paths), maxDepth-1)]...)
				unresolved = parent.unresolved
			}
		}
		observed := at
		if p.observed != 0 {
			observed = min(observed, p.observed)
		}
		g.records[k] = record{process: p, paths: paths, seen: max(r.seen, observed, p.Created), exited: r.exited, unresolved: unresolved}
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

// A start event supplies identity and path, not a promise that the process is
// still alive when a delayed ETW buffer is delivered.
func (g *Graph) Started(p Process) {
	g.Observe([]Process{p}, p.Created)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.relink()
}
func (g *Graph) Exited(pid uint32, created, exited uint64) {
	if created == 0 || exited < created {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	k := identity{pid, created}
	r, ok := g.records[k]
	if !ok {
		if len(g.records) >= maxRecords {
			return
		}
		r = record{process: Process{PID: pid, Created: created}}
	}
	r.exited = exited
	r.seen = max(r.seen, exited)
	g.records[k] = r
	g.relink()
}
func (g *Graph) parentAt(pid uint32, created uint64) (record, bool) {
	valid := func(r record) bool {
		return len(r.paths) > 0 && r.process.Created <= created && r.seen >= created && (r.exited == 0 || r.exited >= created)
	}
	if r, ok := g.records[g.latest[pid]]; ok && valid(r) {
		return r, true
	}
	// Late events can refer to an older, already exited instance of this PID.
	var best record
	for k, r := range g.records {
		if k.pid == pid && valid(r) && r.process.Created > best.process.Created {
			best = r
		}
	}
	return best, len(best.paths) > 0
}
func (g *Graph) relink() {
	ordered := make([]identity, 0, len(g.records))
	for k := range g.records {
		ordered = append(ordered, k)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].created < ordered[j].created })
	for _, k := range ordered {
		r := g.records[k]
		if r.process.Path == "" {
			continue
		}
		if r.process.Parent == 0 || r.process.Parent == r.process.PID {
			continue
		}
		if parent, ok := g.parentAt(r.process.Parent, r.process.Created); ok {
			paths := append([]string{r.process.Path}, parent.paths[:min(len(parent.paths), maxDepth-1)]...)
			if len(paths) > len(r.paths) || (len(paths) == len(r.paths) && parent.unresolved < r.unresolved) {
				r.paths = paths
				r.unresolved = parent.unresolved
				g.records[k] = r
			}
		}
	}
}

// A gap predating the tracker cannot be repaired by its event stream. A newer
// gap may still have a queued exit event, even when one launcher is known.
func (g *Graph) Settled(p Process, since uint64) bool {
	_, settled := g.PathsSince(p, since)
	return settled
}

func (g *Graph) PathsSince(p Process, since uint64) ([]string, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r, ok := g.records[key(p)]
	if !ok {
		return []string{p.Path}, false
	}
	return slices.Clone(r.paths), r.unresolved == 0 || r.unresolved < since
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
