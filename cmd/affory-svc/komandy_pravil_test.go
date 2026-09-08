package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Задача 5.4: команды правил. Хранение в наборе рядом с серверами, запись
// через sohranitIPeresobrat, чтобы заслон Ш7-4 действовал и на правила.

type pravilaOtvet struct {
	Protsessy      []string `json:"protsessy"`
	Domeny         []string `json:"domeny"`
	TrebuetPodyoma bool     `json:"trebuet_podyoma"`
	SpisokIzmenyon bool     `json:"spisok_izmenyon"`
	BezRuSpiska    bool     `json:"bez_ru_spiska"`
}

func razobratPravila(t *testing.T, k protokol.Kadr) pravilaOtvet {
	t.Helper()
	if k.Oshib != nil {
		t.Fatalf("отказ %s: %s", k.Oshib.Kod, k.Oshib.Tekst)
	}
	var p pravilaOtvet
	if err := json.Unmarshal(k.Telo, &p); err != nil {
		t.Fatalf("тело не разбирается: %v (%s)", err, k.Telo)
	}
	return p
}

func faylProby(t *testing.T) string {
	t.Helper()
	put := filepath.Join(t.TempDir(), "Steam", "steam.exe")
	if err := os.MkdirAll(filepath.Dir(put), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return put
}

func TestListRulesOtdayotPustyeSpiskiANeNull(t *testing.T) {
	// An empty list is `[]`, never `null`: the screen counts rows, and null
	// is "did not measure" in the fallback contract, not "zero".
	s := podstavnaya(t, nil)
	o := s.Obrabotat(context.Background(), protokol.Kadr{Id: 1, Imya: "listRules"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %s", o.Oshib.Kod)
	}
	if !strings.Contains(string(o.Telo), `"protsessy":[]`) || !strings.Contains(string(o.Telo), `"domeny":[]`) {
		t.Fatalf("пустые списки не пустые массивы: %s", o.Telo)
	}
}

func TestSetRulesNormalizuetPutISohranyaet(t *testing.T) {
	s := podstavnaya(t, nil)
	put := faylProby(t)
	telo, _ := json.Marshal(map[string]any{
		"protsessy": []string{strings.ToLower(put)},
		"domeny":    []string{" Example.ORG. "},
	})
	o := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))
	if len(o.Protsessy) != 1 || o.Protsessy[0] != put {
		t.Fatalf("путь сохранён как %v, ждали нормализованный %q", o.Protsessy, put)
	}
	if len(o.Domeny) != 1 || o.Domeny[0] != "example.org" {
		t.Fatalf("домен сохранён как %v, ждали example.org", o.Domeny)
	}
	if o.TrebuetPodyoma {
		t.Fatal("туннель выключен, а ответ требует подъёма")
	}
	// Persisted: listRules reads it back from the set, not from memory.
	p := razobratPravila(t, s.Obrabotat(context.Background(), protokol.Kadr{Id: 2, Imya: "listRules"}))
	if len(p.Protsessy) != 1 || len(p.Domeny) != 1 {
		t.Fatalf("после записи listRules отдал %+v", p)
	}
}

func TestSetRulesOtvergaetNesushchestvuyushchiyPutINeTrogaetNabor(t *testing.T) {
	s := podstavnaya(t, nil)
	telo, _ := json.Marshal(map[string]any{
		"protsessy": []string{filepath.Join(t.TempDir(), "net.exe")},
		"domeny":    []string{},
	})
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodPraviloNegodno {
		t.Fatalf("несуществующий путь принят или отказ не тем кодом: %+v", o.Oshib)
	}
	p := razobratPravila(t, s.Obrabotat(context.Background(), protokol.Kadr{Id: 2, Imya: "listRules"}))
	if len(p.Protsessy) != 0 {
		t.Fatalf("отвергнутое правило всё же записано: %v", p.Protsessy)
	}
}

func TestSetRulesOtvergaetKrivoyDomen(t *testing.T) {
	s := podstavnaya(t, nil)
	for _, d := range []string{"https://example.org", "exa mple.org", "example", ""} {
		telo, _ := json.Marshal(map[string]any{"protsessy": []string{}, "domeny": []string{d}})
		o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo})
		if o.Oshib == nil || o.Oshib.Kod != protokol.KodPraviloNegodno {
			t.Errorf("домен %q принят или отказ не тем кодом: %+v", d, o.Oshib)
		}
	}
}

func TestSetRulesPriPodnyatomTunneleGovoritProPodyom(t *testing.T) {
	// The core has no hot reload of route rules: the answer must say the
	// change waits for the next connect, exactly like setRouteMode.
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём: %v", err)
	}
	telo, _ := json.Marshal(map[string]any{"protsessy": []string{}, "domeny": []string{"example.org"}})
	o := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))
	if !o.TrebuetPodyoma {
		t.Fatal("туннель поднят, а ответ молчит про подъём")
	}
}

func TestPravilaPopadayutVKonfigIIschezayutPodRezhimom(t *testing.T) {
	s := podstavnaya(t, nil)
	put := faylProby(t)
	telo, _ := json.Marshal(map[string]any{"protsessy": []string{put}, "domeny": []string{"example.org"}})
	razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))

	k, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Not "domain_suffix": the DNS block carries one for .local at all times.
	// The domain itself and the file name are the exclusions' own marks.
	if !strings.Contains(string(k), `"example.org"`) || !strings.Contains(string(k), filepath.Base(put)) {
		t.Fatal("правила не дошли до конфига туннеля")
	}
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()
	k, _, _, err = s.sobratTun(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(k), `"example.org"`) || strings.Contains(string(k), filepath.Base(put)) {
		t.Fatal("в режиме «весь трафик» исключения остались в конфиге")
	}
}

// faylSImenem кладёт настоящий файл с заданным именем и отдаёт путь.
func faylSImenem(t *testing.T, imya string) string {
	t.Helper()
	put := filepath.Join(t.TempDir(), imya)
	if err := os.WriteFile(put, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return put
}

func zadatPravila(t *testing.T, s *Sluzhba, puti ...string) pravilaOtvet {
	t.Helper()
	telo, err := json.Marshal(map[string]any{"protsessy": puti, "domeny": []string{}})
	if err != nil {
		t.Fatal(err)
	}
	return razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))
}

// Правило A ссылается на файл, которого на диске уже нет: игру удалили.
// Человек удаляет правило B. Прежде setRules прогонял нормализацию по ВСЕМУ
// списку и отвергал его целиком кодом rule-invalid, то есть удалить нельзя
// было ничего вообще, и выхода из этого состояния не было.
func TestUdaleniyePravilaRabotaetKogdaChuzhoyFaylPropal(t *testing.T) {
	s := podstavnaya(t, nil)
	a := faylSImenem(t, "igra.exe")
	b := faylSImenem(t, "launcher.exe")

	o := zadatPravila(t, s, a, b)
	if len(o.Protsessy) != 2 {
		t.Fatalf("записано %v, ожидались оба", o.Protsessy)
	}
	na, nb := o.Protsessy[0], o.Protsessy[1]

	// Игру снесли с диска.
	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}

	// Человек убирает ВТОРОЕ правило. Первое остаётся в списке как есть.
	posle := zadatPravila(t, s, na)
	if len(posle.Protsessy) != 1 || posle.Protsessy[0] != na {
		t.Fatalf("после удаления в списке %v, ожидалось только %q", posle.Protsessy, na)
	}
	if posle.Protsessy[0] == nb {
		t.Fatal("удалено не то правило")
	}
}

// Зеркало: правило, добавляемое ВПЕРВЫЕ, обязано отказать честно и назвать
// конкретный путь. Без него починка выше сводится к «принимаем что угодно».
func TestNovoePraviloNaPropavshiyFaylOtvergaetsyaSImenem(t *testing.T) {
	s := podstavnaya(t, nil)
	net := filepath.Join(t.TempDir(), "net.exe")
	telo, _ := json.Marshal(map[string]any{"protsessy": []string{net}, "domeny": []string{}})
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodPraviloNegodno {
		t.Fatalf("новое правило на пропавший файл принято: %+v", o.Oshib)
	}
	if !strings.Contains(o.Oshib.Tekst, net) {
		t.Fatalf("отказ не называет конкретное правило: %s", o.Oshib.Tekst)
	}
}

// Дедуп укорачивает список, и ответ обязан об этом СКАЗАТЬ. Молча укороченный
// список оставляет окно с лишними строками, а строки в нём нумерованы: индексы
// разъезжаются, и следующее удаление снимает не то правило.
func TestUkorochennyySpiskomOtvetSoobshchaetObIzmenenii(t *testing.T) {
	s := podstavnaya(t, nil)
	a := faylSImenem(t, "igra.exe")

	telo, _ := json.Marshal(map[string]any{"protsessy": []string{a, a}, "domeny": []string{}})
	o := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))
	if len(o.Protsessy) != 1 {
		t.Fatalf("дедуп не сработал: %v", o.Protsessy)
	}
	if !o.SpisokIzmenyon {
		t.Fatal("список укорочен молча: окно оставит на экране две строки вместо одной")
	}

	// Зеркало: список, принятый как есть, ничего менять не просит.
	rovno := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 2, Imya: "setRules", Telo: telo}))
	_ = rovno
	telo2, _ := json.Marshal(map[string]any{"protsessy": o.Protsessy, "domeny": []string{}})
	bez := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 3, Imya: "setRules", Telo: telo2}))
	if bez.SpisokIzmenyon {
		t.Fatalf("список не менялся, а ответ просит перерисовать: %v", bez.Protsessy)
	}
}

// Выключатель российского списка. Флаг едет тем же телом, что и списки, но
// отсутствие поля значит «не трогали», а не «выключено».
//
// Разница не теоретическая: `affory-cli rules set --in файл` и окно прошлой
// версии шлют ровно два списка. Считай служба молчание за false, и добавление
// одного правила из терминала вернуло бы российские домены мимо туннеля, ничего
// об этом не сказав.
func TestSetRulesBezPolyaNeTrogaetFlagRu(t *testing.T) {
	s := podstavnaya(t, nil)

	telo, _ := json.Marshal(map[string]any{"protsessy": []string{}, "domeny": []string{}, "bez_ru_spiska": true})
	o := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 1, Imya: "setRules", Telo: telo}))
	if !o.BezRuSpiska {
		t.Fatal("флаг прислали, а ответ его не подтвердил")
	}

	bezPolya, _ := json.Marshal(map[string]any{"protsessy": []string{}, "domeny": []string{}})
	p := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 2, Imya: "setRules", Telo: bezPolya}))
	if !p.BezRuSpiska {
		t.Fatal("молчание про поле сбросило флаг: правка списков вернула бы ru-набор молча")
	}

	l := razobratPravila(t, s.Obrabotat(context.Background(), protokol.Kadr{Id: 3, Imya: "listRules"}))
	if !l.BezRuSpiska {
		t.Fatal("listRules не отдаёт флаг: экран нарисует выключатель в другом положении")
	}

	// И обратно: явный false это явный false, иначе выключатель однонаправленный.
	nazad, _ := json.Marshal(map[string]any{"protsessy": []string{}, "domeny": []string{}, "bez_ru_spiska": false})
	v := razobratPravila(t, s.Obrabotat(ctxAdmina(), protokol.Kadr{Id: 4, Imya: "setRules", Telo: nazad}))
	if v.BezRuSpiska {
		t.Fatal("флаг не снимается: список нельзя вернуть")
	}
}
