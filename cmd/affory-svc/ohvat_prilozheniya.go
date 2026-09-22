package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sagernet/sing-box/common/afforyprocess"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"golang.org/x/sys/windows"
)

type zapuskPrilozheniya struct {
	PID           uint32                   `json:"pid"`
	Created       string                   `json:"created"`
	Put           string                   `json:"put"`
	Imya          string                   `json:"imya"`
	Cherez        []string                 `json:"cherez"`
	Marshrut      protokol.MarshrutTrafika `json:"marshrut"`
	PraviloPut    string                   `json:"pravilo_put"`
	PraviloImya   string                   `json:"pravilo_imya"`
	Pereopredelen bool                     `json:"pereopredelen"`
}
type ohvatPrilozheniya struct {
	Pravilo       protokol.PraviloPrilozheniya `json:"pravilo"`
	Reviziya      string                       `json:"reviziya_pravil"`
	Trebuet       bool                         `json:"trebuet_podyoma"`
	Vremya        time.Time                    `json:"vremya"`
	Fayl          string                       `json:"fayl"`
	Samo          int                          `json:"samo"`
	Vsego         int                          `json:"vsego"`
	Zapushchennye []zapuskPrilozheniya         `json:"zapushchennye"`
	Neizvestno    int                          `json:"neizvestno"`
	Neizvestnye   []string                     `json:"neizvestnye"`
	Ogranichen    bool                         `json:"ogranichen"`
}

func sameProgram(a, b string) bool { return strings.ToLower(a) == strings.ToLower(b) }

// Same precedence as the generated core rules: exact executable, then the
// nearest configured program that includes the programs it launches.
func praviloZapushchennogo(paths []string, rules []protokol.PraviloPrilozheniya) *protokol.PraviloPrilozheniya {
	for depth, path := range paths {
		for i := range rules {
			if (depth == 0 || rules[i].Potomki) && sameProgram(path, rules[i].Put) {
				return &rules[i]
			}
		}
	}
	return nil
}

func sobratOhvat(root protokol.PraviloPrilozheniya, rules []protokol.PraviloPrilozheniya, views []afforyprocess.ProcessView) ohvatPrilozheniya {
	o := ohvatPrilozheniya{Pravilo: root, Vremya: time.Now(), Zapushchennye: []zapuskPrilozheniya{}, Neizvestnye: []string{}}
	views = slices.Clone(views)
	depth := func(p afforyprocess.ProcessView) int {
		paths := p.Paths
		if len(paths) == 0 {
			paths = []string{p.Path}
		}
		index := slices.IndexFunc(paths, func(path string) bool { return sameProgram(path, root.Put) })
		if index < 0 {
			return 1000
		}
		return index
	}
	slices.SortFunc(views, func(a, b afforyprocess.ProcessView) int {
		if depth(a) != depth(b) {
			return depth(a) - depth(b)
		}
		if c := strings.Compare(strings.ToLower(a.Path), strings.ToLower(b.Path)); c != 0 {
			return c
		}
		if a.PID < b.PID {
			return -1
		}
		if a.PID > b.PID {
			return 1
		}
		return 0
	})
	budget := 0
	unknownPaths := map[string]bool{}
	for _, p := range views {
		paths := p.Paths
		if len(paths) == 0 {
			paths = []string{p.Path}
		}
		index := slices.IndexFunc(paths, func(path string) bool { return sameProgram(path, root.Put) })
		if index < 0 || (index > 0 && !root.Potomki) {
			if root.Potomki && index < 0 && !p.Complete {
				o.Neizvestno++
				key := strings.ToLower(p.Path)
				if !unknownPaths[key] && len(o.Neizvestnye) < 10 && len(p.Path) < 32768 {
					encoded, _ := json.Marshal(p.Path)
					if budget+len(encoded) <= 256*1024 {
						o.Neizvestnye = append(o.Neizvestnye, p.Path)
						unknownPaths[key] = true
						budget += len(encoded)
					} else {
						o.Ogranichen = true
					}
				}
			}
			continue
		}
		o.Vsego++
		if index == 0 {
			o.Samo++
		}
		row := zapuskPrilozheniya{PID: p.PID, Created: strconv.FormatUint(p.Created, 10), Put: p.Path, Imya: filepath.Base(p.Path), Cherez: slices.Clone(paths[:index+1])}
		slices.Reverse(row.Cherez)
		if winner := praviloZapushchennogo(paths, rules); winner != nil {
			row.Marshrut = winner.Marshrut
			row.PraviloPut = winner.Put
			row.PraviloImya = winner.Imya
			row.Pereopredelen = !sameProgram(winner.Put, root.Put)
		}
		data, _ := json.Marshal(row)
		if len(o.Zapushchennye) >= 200 || budget+len(data) > 256*1024 {
			o.Ogranichen = true
			continue
		}
		budget += len(data)
		o.Zapushchennye = append(o.Zapushchennye, row)
	}
	return o
}

func (s *Sluzhba) inspectApplication(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var q struct {
		Put string `json:"put"`
	}
	if err := json.Unmarshal(k.Telo, &q); err != nil || q.Put == "" || len(q.Put) > 32768 {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "Нужен путь приложения из сохранённых правил")
	}
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	if n.Pravila.Trafik == nil {
		return otkaz(k.Id, k.Imya, protokol.KodPraviloNegodno, "Правила приложений ещё не загружены")
	}
	rules := n.Pravila.Trafik.Prilozheniya
	index := slices.IndexFunc(rules, func(r protokol.PraviloPrilozheniya) bool { return sameProgram(r.Put, q.Put) })
	if index < 0 {
		return otkaz(k.Id, k.Imya, protokol.KodPraviloNegodno, "Правило приложения изменилось. Обнови список правил")
	}
	pid := kanal.DopuskIz(ctx).Pid
	var session uint32
	if pid == 0 {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "Не удалось определить текущий сеанс Windows")
	}
	if err := windows.ProcessIdToSessionId(pid, &session); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodVnutrennyayaOshibka, "Не удалось определить текущий сеанс Windows")
	}
	s.mu.Lock()
	tracker := s.processTracker
	s.mu.Unlock()
	if tracker == nil {
		return otkaz(k.Id, k.Imya, protokol.KodYadroNeOtvechaet, "Наблюдение за приложениями недоступно. Подтвердить охват сейчас нельзя")
	}
	views, err := tracker.watcher.Views(session)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodYadroNeOtvechaet, fmt.Sprintf("Не удалось проверить запущенные программы: %v", err))
	}
	o := sobratOhvat(rules[index], rules, views)
	o.Reviziya = reviziyaPravil(n.Pravila)
	o.Trebuet = s.pravilaOzhidayut(n.Pravila)
	o.Fayl = proveritFaylPravila(ctx, rules[index].Put)
	return otvet(k.Id, k.Imya, o)
}

var proverkiFaylov = make(chan struct{}, 4)

func proveritFaylPravila(ctx context.Context, path string) string {
	if ctx.Err() != nil {
		return "ne_proveren"
	}
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return "ne_proveren"
	}
	root := filepath.VolumeName(path) + `\`
	ptr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "ne_proveren"
	}
	if windows.GetDriveType(ptr) != windows.DRIVE_FIXED {
		return "ne_proveren"
	}
	select {
	case proverkiFaylov <- struct{}{}:
	default:
		return "ne_proveren"
	}
	result := make(chan string, 1)
	go func() {
		defer func() { <-proverkiFaylov }()
		stat, err := os.Stat(path)
		state := "est"
		switch {
		case errors.Is(err, os.ErrNotExist):
			state = "net"
		case err != nil:
			state = "nedostupen"
		case stat.IsDir():
			state = "papka"
		}
		result <- state
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		return "ne_proveren"
	case <-timer.C:
		return "ne_proveren"
	}
}
