package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/common/afforyprocess"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestApplicationScopeShowsOverridesAndKeepsSeparateLaunches(t *testing.T) {
	root := protokol.PraviloPrilozheniya{Put: `C:\Launcher.exe`, Imya: "Launcher", Potomki: true, Marshrut: protokol.TrafikPryamo}
	child := protokol.PraviloPrilozheniya{Put: `C:\Game.exe`, Imya: "Game", Potomki: true, Marshrut: protokol.TrafikVPN}
	views := []afforyprocess.ProcessView{
		{PID: 1, Created: 134000000000000001, Path: root.Put, Paths: []string{root.Put}},
		{PID: 2, Created: 134000000000000002, Path: child.Put, Paths: []string{child.Put, root.Put}},
		{PID: 3, Path: child.Put, Paths: []string{child.Put, `C:\Other.exe`}, Complete: true},
		{PID: 4, Path: `C:\Unknown.exe`, Paths: []string{`C:\Unknown.exe`}},
	}
	got := sobratOhvat(root, []protokol.PraviloPrilozheniya{root, child}, views)
	if got.Samo != 1 || got.Vsego != 2 || got.Neizvestno != 1 || len(got.Zapushchennye) != 2 {
		t.Fatalf("incorrect scope: %+v", got)
	}
	row := got.Zapushchennye[1]
	if row.PID != 2 || row.Created != "134000000000000002" || !row.Pereopredelen || row.PraviloPut != child.Put || row.Marshrut != protokol.TrafikVPN || row.Cherez[0] != root.Put {
		t.Fatalf("incorrect winner: %+v", row)
	}
	root.Potomki = false
	got = sobratOhvat(root, []protokol.PraviloPrilozheniya{root, child}, views)
	if got.Vsego != 1 || got.Neizvestno != 0 {
		t.Fatal("exact-only rule included other programs")
	}
}

func TestApplicationScopeKeepsClosedLauncherAndBoundsResponse(t *testing.T) {
	root := protokol.PraviloPrilozheniya{Put: `C:\Launcher.exe`, Potomki: true, Marshrut: protokol.TrafikVPN}
	var views []afforyprocess.ProcessView
	for i := 0; i < 300; i++ {
		path := fmt.Sprintf(`C:\Worker%d.exe`, i)
		views = append(views, afforyprocess.ProcessView{PID: uint32(i + 10), Path: path, Paths: []string{path, root.Put}})
	}
	got := sobratOhvat(root, []protokol.PraviloPrilozheniya{root}, views)
	if got.Samo != 0 || got.Vsego != 300 || len(got.Zapushchennye) != 200 || !got.Ogranichen {
		t.Fatalf("scope limit: %+v", got)
	}
	for i := 0; i < 15; i++ {
		path := fmt.Sprintf(`C:\%d-%s.exe`, i, strings.Repeat("&", 30000))
		views = append(views, afforyprocess.ProcessView{PID: uint32(i + 500), Path: path, Paths: []string{path}})
	}
	got = sobratOhvat(root, []protokol.PraviloPrilozheniya{root}, views)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 512*1024 {
		t.Fatalf("response unbounded: %d", len(data))
	}
}

func TestApplicationFileStatusIsNotAPromiseThatItRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "program.exe")
	if got := proveritFaylPravila(context.Background(), path); got != "net" {
		t.Fatal(got)
	}
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := proveritFaylPravila(context.Background(), path); got != "est" {
		t.Fatal(got)
	}
	if got := proveritFaylPravila(context.Background(), dir); got != "papka" {
		t.Fatal(got)
	}
	if got := proveritFaylPravila(context.Background(), `\\invalid.example\share\program.exe`); got != "ne_proveren" {
		t.Fatal("network path was probed")
	}
}

func TestInspectApplicationUsesCallerSessionAndReturnsLiveSelf(t *testing.T) {
	s := podstavnaya(t, nil)
	n, err := novyyNablyudatelPrilozheniy()
	if err != nil {
		t.Fatal(err)
	}
	s.processTracker = n
	t.Cleanup(s.Zavershit)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Pravila.Trafik = &protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN, Prilozheniya: []protokol.PraviloPrilozheniya{{Put: self, Imya: "Self", Marshrut: protokol.TrafikPryamo}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"put": self})
	k := protokol.Kadr{Imya: "inspectApplication", Telo: body}
	if result := s.Obrabotat(context.Background(), k); result.Oshib == nil {
		t.Fatal("missing caller identity accepted")
	}
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{Pid: uint32(os.Getpid())})
	result := s.Obrabotat(ctx, k)
	if result.Oshib != nil {
		t.Fatal(result.Oshib)
	}
	var got ohvatPrilozheniya
	if err := json.Unmarshal(result.Telo, &got); err != nil {
		t.Fatal(err)
	}
	if got.Samo < 1 || got.Fayl != "est" || got.Reviziya == "" {
		t.Fatalf("self inspection failed: %+v", got)
	}
	if protokol.TeloMozhnoLogirovat(k.Imya) {
		t.Fatal("process list exposed to frame logging")
	}
}
