package afforyprocess

import (
	"fmt"
	"slices"
)

type ProcessView struct {
	PID, Session uint32
	Created      uint64
	Path         string
	Paths        []string
	Complete     bool
}

// Views is a snapshot of currently running programs in one Windows session.
// Exited launchers can remain in Paths, but never become live rows themselves.
func (w *Watcher) Views(session uint32) ([]ProcessView, error) {
	select {
	case <-w.done:
		return nil, fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
	default:
	}
	if w.events != nil {
		if err := w.events.Err(); err != nil {
			return nil, err
		}
		w.events.flush()
		if err := w.events.Err(); err != nil {
			return nil, err
		}
	}
	at := clockTicks()
	processes, err := snapshotSession(w.graph, &session)
	if err != nil {
		return nil, err
	}
	w.graph.Observe(processes, at)
	select {
	case <-w.done:
		return nil, fmt.Errorf("%w: watcher stopped", ErrSharedUnavailable)
	default:
	}
	w.graph.mu.Lock()
	defer w.graph.mu.Unlock()
	result := make([]ProcessView, 0, len(processes))
	for _, p := range processes {
		r, ok := w.graph.records[key(p)]
		view := ProcessView{PID: p.PID, Session: p.Session, Created: p.Created, Path: p.Path, Paths: []string{p.Path}}
		if ok {
			view.Paths = slices.Clone(r.paths)
			view.Complete = r.unresolved == 0
		}
		result = append(result, view)
	}
	return result, nil
}
