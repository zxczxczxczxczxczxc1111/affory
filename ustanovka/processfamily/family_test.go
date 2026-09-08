package afforyprocess

import (
	"slices"
	"testing"
)

func TestDescendantsIncludeNewChildrenAndKeepObservedParents(t *testing.T) {
	g := NewGraph()
	root := Process{PID: 10, Created: 10, Path: `C:\Steam\steam.exe`}
	child := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\Steam\helper.exe`}
	g.Observe([]Process{child, root}, 30)
	if !slices.Contains(g.Paths(child), root.Path) {
		t.Fatal("child lost root")
	}
	grandchild := Process{PID: 30, Parent: 20, Created: 40, Path: `C:\Games\game.exe`}
	g.Observe([]Process{child, grandchild}, 50)
	if !slices.Contains(g.Paths(grandchild), root.Path) {
		t.Fatal("root exit broke inheritance")
	}
}

func TestSameExecutableOutsideFamilyDoesNotMatch(t *testing.T) {
	g := NewGraph()
	root := Process{PID: 10, Created: 10, Path: `C:\Steam\steam.exe`}
	inside := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\shared\helper.exe`}
	outside := Process{PID: 30, Created: 30, Path: inside.Path}
	g.Observe([]Process{root, inside, outside}, 40)
	if slices.Contains(g.Paths(outside), root.Path) {
		t.Fatal("matched executable name instead of ancestry")
	}
}

func TestPIDReuseDoesNotInheritOldFamily(t *testing.T) {
	g := NewGraph()
	root := Process{PID: 10, Created: 10, Path: `C:\Steam\steam.exe`}
	old := Process{PID: 20, Parent: 10, Created: 20, Path: `C:\shared\helper.exe`}
	g.Observe([]Process{root, old}, 30)
	reused := Process{PID: 20, Created: 40, Path: old.Path}
	g.Observe([]Process{reused}, 50)
	if slices.Contains(g.Paths(reused), root.Path) {
		t.Fatal("reused child PID inherited old lineage")
	}
	orphan := Process{PID: 40, Parent: 10, Created: 15, Path: `C:\orphan.exe`}
	newParent := Process{PID: 10, Created: 60, Path: `C:\unrelated.exe`}
	g.Observe([]Process{orphan, newParent}, 70)
	if slices.Contains(g.Paths(orphan), newParent.Path) {
		t.Fatal("parent born after child was accepted")
	}
}

func TestUnknownExitedParentIsNotGuessed(t *testing.T) {
	g := NewGraph()
	root := Process{PID: 10, Created: 10, Path: `C:\Steam\steam.exe`}
	g.Observe([]Process{root}, 20)
	child := Process{PID: 30, Parent: 10, Created: 40, Path: `C:\helper.exe`}
	g.Observe([]Process{child}, 50)
	if slices.Contains(g.Paths(child), root.Path) {
		t.Fatal("unobserved parent lifetime was guessed")
	}
}
