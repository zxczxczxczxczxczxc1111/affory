package obnovlenie

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Задача 6.5 в усечённом объёме (решение 03.09.2026): служба принимает
// installUpdate{path} к УЖЕ лежащему архиву, сверяет sha256 из соседнего
// файла .sha256, подменяет файлы и откатывает, если новая служба не ответила
// за 20 секунд. Откуда архив берётся, остаётся открытым: приватный репозиторий
// требовал бы токена на машине.

func arhiv(t *testing.T, dir string, fayly map[string]string) string {
	t.Helper()
	put := filepath.Join(dir, "affory.zip")
	f, err := os.Create(put)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for imya, telo := range fayly {
		z, err := w.Create(imya)
		if err != nil {
			t.Fatal(err)
		}
		z.Write([]byte(telo))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	h := sha256.New()
	b, _ := os.ReadFile(put)
	h.Write(b)
	// Формат sha256sum: хеш, два пробела, имя. Именно его пишут и люди, и CI.
	if err := os.WriteFile(put+".sha256", []byte(hex.EncodeToString(h.Sum(nil))+"  affory.zip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return put
}

func TestSverkaOtvergaetChuzhoyHesh(t *testing.T) {
	d := t.TempDir()
	put := arhiv(t, d, map[string]string{"affory-svc.exe": "novaya"})
	if err := Sverit(put); err != nil {
		t.Fatalf("свой хеш отвергнут: %v", err)
	}
	if err := os.WriteFile(put+".sha256", []byte("0000  affory.zip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Sverit(put); err == nil {
		t.Fatal("чужой хеш принят")
	}
	os.Remove(put + ".sha256")
	if err := Sverit(put); err == nil {
		t.Fatal("архив без .sha256 принят: сверять было не с чем")
	}
}

func TestRaspakovkaTolkoVerhnihFaylovBezPutey(t *testing.T) {
	// Запись в архиве с путём наружу («..\\x.exe») это классический zip slip.
	// Из архива берутся только имена файлов, каталоги игнорируются.
	d := t.TempDir()
	put := arhiv(t, d, map[string]string{"affory-svc.exe": "n", "sub/x.exe": "x", "../y.exe": "y"})
	kuda := filepath.Join(d, "novaya")
	if err := Raspakovat(put, kuda); err == nil {
		t.Fatal("архив с путём наружу принят")
	}
	put = arhiv(t, d, map[string]string{"affory-svc.exe": "n", "affory-ui.exe": "u"})
	if err := Raspakovat(put, kuda); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"affory-svc.exe", "affory-ui.exe"} {
		if _, err := os.Stat(filepath.Join(kuda, f)); err != nil {
			t.Fatalf("файл %s не распакован", f)
		}
	}
}

func podmena(t *testing.T) (Podmena, string) {
	t.Helper()
	d := t.TempDir()
	prog := filepath.Join(d, "prog")
	os.MkdirAll(prog, 0o700)
	os.WriteFile(filepath.Join(prog, "affory-svc.exe"), []byte("staraya"), 0o600)
	os.WriteFile(filepath.Join(prog, "affory-ui.exe"), []byte("staraya-ui"), 0o600)
	os.WriteFile(filepath.Join(prog, "sing-box.exe"), []byte("yadro"), 0o600)
	nov := filepath.Join(d, "novaya")
	os.MkdirAll(nov, 0o700)
	os.WriteFile(filepath.Join(nov, "affory-svc.exe"), []byte("novaya"), 0o600)
	os.WriteFile(filepath.Join(nov, "affory-ui.exe"), []byte("novaya-ui"), 0o600)
	return Podmena{
		KatalogProgrammy: prog,
		Novaya:           nov,
		Ostanovit:        func() error { return nil },
		Ustanovit:        func() error { return nil },
		ZhdatOtveta:      func(time.Duration) error { return nil },
		Srok:             time.Millisecond,
	}, prog
}

func soderzhimoe(t *testing.T, put string) string {
	t.Helper()
	b, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPodmenaStavitNovyeIOstavlyaetChuzhie(t *testing.T) {
	p, prog := podmena(t)
	itog := p.Vypolnit()
	if !itog.Ok {
		t.Fatalf("подмена не удалась: %+v", itog)
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-svc.exe")) != "novaya" {
		t.Fatal("служба не подменена")
	}
	// Файл, которого нет в новой сборке, остаётся: ядро едет отдельно.
	if soderzhimoe(t, filepath.Join(prog, "sing-box.exe")) != "yadro" {
		t.Fatal("файл вне сборки задет")
	}
	if soderzhimoe(t, filepath.Join(prog, "predydushchaya", "affory-svc.exe")) != "staraya" {
		t.Fatal("прежняя версия не сохранена рядом")
	}
}

// С 0.7.0 ядро едет В архиве обновления, а не только с установщиком. Подмена
// общая и различий между именами не знает, но контракт надо закрепить: иначе
// следующая правка «копировать только наши три exe» пройдёт зелёной и тихо
// вернёт ядро на установщик.
func TestYadroIzNovoySborkiPodmenyaetsya(t *testing.T) {
	p, prog := podmena(t)
	if err := os.WriteFile(filepath.Join(p.Novaya, "sing-box.exe"), []byte("novoe-yadro"), 0o600); err != nil {
		t.Fatal(err)
	}
	if itog := p.Vypolnit(); !itog.Ok {
		t.Fatalf("подмена не удалась: %+v", itog)
	}
	if soderzhimoe(t, filepath.Join(prog, "sing-box.exe")) != "novoe-yadro" {
		t.Fatal("ядро из новой сборки не подменило прежнее")
	}
	if soderzhimoe(t, filepath.Join(prog, "predydushchaya", "sing-box.exe")) != "yadro" {
		t.Fatal("прежнее ядро не сохранено, откатывать будет нечем")
	}
}

func TestNepodnyavshayasyaSluzhbaOtkatyvaetsya(t *testing.T) {
	// The one path that must work when everything else has already failed.
	p, prog := podmena(t)
	zapuskov := 0
	p.Ustanovit = func() error { zapuskov++; return nil }
	p.ZhdatOtveta = func(time.Duration) error {
		if zapuskov == 1 {
			return errors.New("новая служба молчит")
		}
		return nil
	}
	itog := p.Vypolnit()
	if itog.Ok {
		t.Fatal("провал подъёма выдан за успех")
	}
	if itog.Kod != KodOtkat {
		t.Fatalf("код %q, ожидался %q", itog.Kod, KodOtkat)
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-svc.exe")) != "staraya" {
		t.Fatal("после отката стоит не прежняя версия")
	}
	if soderzhimoe(t, filepath.Join(prog, "affory-ui.exe")) != "staraya-ui" {
		t.Fatal("интерфейс не откачен")
	}
	if zapuskov != 2 {
		t.Fatalf("установок %d, ожидалось 2: новая и откат", zapuskov)
	}
}

func TestOtkatSamNeUdalsyaEtoOtdelnyyIshod(t *testing.T) {
	p, _ := podmena(t)
	p.ZhdatOtveta = func(time.Duration) error { return errors.New("молчит") }
	itog := p.Vypolnit()
	if itog.Ok || itog.Kod != KodOtkatNeUdalsya {
		t.Fatalf("откат, после которого служба тоже молчит, дал %+v", itog)
	}
}

func TestItogPishetsyaIChitaetsya(t *testing.T) {
	d := t.TempDir()
	if err := ZapisatItog(d, Itog{Ok: false, Kod: KodOtkat, Tekst: "молчит"}); err != nil {
		t.Fatal(err)
	}
	i, est, err := ProchitatItog(d)
	if err != nil || !est || i.Kod != KodOtkat {
		t.Fatalf("итог не прочитан: %+v %v %v", i, est, err)
	}
	// Чтение забирает файл: итог показывается один раз, а не на каждом старте.
	if _, est, _ := ProchitatItog(d); est {
		t.Fatal("итог пережил чтение")
	}
}

// Достижимо без злого умысла: путь к архиву приходит телом кадра installUpdate,
// а .sha256 берётся рядом с ним. Оборванная запись даёт пустой файл, и первое
// поле пустого среза это index out of range, то есть смерть службы вместе с
// туннелем на команде, которая обязана была ответить отказом.
func TestSveritNePanikuetNaPustomFayleSummy(t *testing.T) {
	kat := t.TempDir()
	arhiv := filepath.Join(kat, "affory-0.7.0.zip")
	if err := os.WriteFile(arhiv, []byte("не важно"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Оборванная запись даёт пустой файл. Злого умысла для этого не нужно.
	if err := os.WriteFile(arhiv+".sha256", []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := Sverit(arhiv)
	if err == nil {
		t.Fatal("пустая сумма обязана быть отказом")
	}
	if !strings.Contains(err.Error(), "сум") {
		t.Errorf("текст отказа не называет причину: %v", err)
	}
}

// Вторая половина той же дыры: поле есть, но это не sha256. Без проверки длины
// сверка честно не сойдётся, только текст отказа скажет «хеш не совпал» там,
// где на деле файл суммы битый, и человек пойдёт перекачивать исправный архив.
func TestSveritOtvergaetSummuNeToyDliny(t *testing.T) {
	kat := t.TempDir()
	arhiv := filepath.Join(kat, "affory-0.7.0.zip")
	if err := os.WriteFile(arhiv, []byte("не важно"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, summa := range []string{
		"deadbeef",
		strings.Repeat("z", 64),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
	} {
		t.Run(summa[:8]+strconv.Itoa(len(summa)), func(t *testing.T) {
			if err := os.WriteFile(arhiv+".sha256", []byte(summa+"  affory.zip\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			err := Sverit(arhiv)
			if err == nil {
				t.Fatal("негодная сумма обязана быть отказом")
			}
			if !strings.Contains(err.Error(), "сум") {
				t.Errorf("текст отказа не называет причину: %v", err)
			}
		})
	}
}
