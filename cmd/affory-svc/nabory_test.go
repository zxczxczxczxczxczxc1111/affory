package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/netip"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// naborVoVremennom подменяет желаемые наборы одним, чей файл лежит во
// временном каталоге теста, а не в живом ProgramData.
func naborVoVremennom(t *testing.T, s *Sluzhba) genkonfig.NaborPravil {
	t.Helper()
	n := genkonfig.NaborPravil{
		Teg:  "ru",
		URL:  "https://nabory.example/geosite-category-ru.srs",
		Fayl: filepath.Join(t.TempDir(), "nabory", "ru.srs"),
	}
	s.naboryZhelaemye = func() []genkonfig.NaborPravil { return []genkonfig.NaborPravil{n} }
	return n
}

func TestKonfigTunnelyaSoderzhitNabor(t *testing.T) {
	s := podstavnaya(t, nil)
	n := naborVoVremennom(t, s)
	if err := os.MkdirAll(filepath.Dir(n.Fayl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(n.Fayl, []byte("srs"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.skachatNabor = func(context.Context, string) ([]byte, error) {
		t.Fatal("файл на месте, а служба пошла качать")
		return nil, nil
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	for _, kusok := range []string{`"rule_set"`, n.URL, `"http_client"`, `"cache_file"`, `kesh.db`} {
		if !strings.Contains(string(telo), kusok) {
			t.Errorf("в конфиге туннеля нет %s", kusok)
		}
	}
}

func TestNaborSkachivaetsyaVFaylPeredPodyomom(t *testing.T) {
	// Cold start: no file yet. The service downloads the set itself, before
	// the tunnel, so sing-box always finds initial_path on disk. Otherwise a
	// failed first download inside the core would refuse the whole config.
	s := podstavnaya(t, nil)
	n := naborVoVremennom(t, s)
	s.skachatNabor = func(_ context.Context, adres string) ([]byte, error) {
		if adres != n.URL {
			t.Errorf("качают %s вместо %s", adres, n.URL)
		}
		return []byte("srs-telo"), nil
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	b, err := os.ReadFile(n.Fayl)
	if err != nil {
		t.Fatalf("набор не записан в initial_path: %v", err)
	}
	if string(b) != "srs-telo" {
		t.Fatalf("в файле %q, а качали srs-telo", b)
	}
	if !strings.Contains(string(telo), `"rule_set"`) {
		t.Fatal("скачанный набор не попал в конфиг")
	}
}

func TestNaborBezFaylaINeSkachannyyNeVhoditVKonfig(t *testing.T) {
	// The spec's rule for every download: a failure never touches the tunnel.
	// No file and no network means a tunnel without domain sets, not no tunnel.
	s := podstavnaya(t, nil)
	naborVoVremennom(t, s)
	s.skachatNabor = func(context.Context, string) ([]byte, error) {
		return nil, errors.New("сети нет")
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("неудача загрузки набора уронила подъём: %v", err)
	}
	if strings.Contains(string(telo), `"rule_set"`) {
		t.Fatal("набор без файла попал в конфиг: ядро упадёт на первой же загрузке")
	}
}

func TestHostyNaborovVhodyatVAdresaKandidatov(t *testing.T) {
	// The loop rule and the firewall allow-list come from one collector; the
	// set hosts have to reach it, or the download goes into the tunnel.
	s := podstavnaya(t, nil)
	n := naborVoVremennom(t, s)
	var poluchil []string
	s.sobratAdresaSet = func(_ []protokol.Server, _ string, zagruzki ...string) ([]netip.Addr, error) {
		poluchil = zagruzki
		return nil, nil
	}
	if _, err := s.adresaKandidatov(); err != nil {
		t.Fatal(err)
	}
	if len(poluchil) != 1 || poluchil[0] != n.URL {
		t.Fatalf("сборщику адресов ушло %v, ждали адрес набора", poluchil)
	}
}

// Выключатель российского списка (просьба владельца 08.09.2026). Набор ru был
// зашит намертво: человек видел его действие, но выключить не мог ничем.

func TestBezRuSpiskaUbiraetNaborIzKonfiga(t *testing.T) {
	s := podstavnaya(t, nil)
	n := naborVoVremennom(t, s)
	if err := os.MkdirAll(filepath.Dir(n.Fayl), 0o755); err != nil {
		t.Fatal(err)
	}
	// Файл НА МЕСТЕ намеренно: набор выпадает из конфига по решению человека,
	// а не потому, что его не удалось скачать. Это разные причины, и вторая
	// прикрыла бы первую.
	if err := os.WriteFile(n.Fayl, []byte("srs"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.skachatNabor = func(context.Context, string) ([]byte, error) {
		t.Error("список выключен, а служба пошла его качать")
		return nil, nil
	}
	if err := s.pravitNabor(func(nb *Nabor) error { nb.Pravila.BezRuSpiska = true; return nil }); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}

	telo, _, _, err := s.sobratTun(nil)
	if err != nil {
		t.Fatalf("конфиг туннеля не собран: %v", err)
	}
	if strings.Contains(string(telo), `"rule_set"`) {
		t.Fatalf("список выключен, а набор в конфиге: российские домены остались мимо туннеля\n%s", telo)
	}
}

func TestBezRuSpiskaUbiraetAdresNaboraIzKandidatov(t *testing.T) {
	// Адрес набора стоит в правиле петли и в разрешающих правилах брандмауэра.
	// Ненужный набор не должен оставлять за собой дырку в обоих.
	s := podstavnaya(t, nil)
	naborVoVremennom(t, s)
	if err := s.pravitNabor(func(nb *Nabor) error { nb.Pravila.BezRuSpiska = true; return nil }); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	var poluchil []string
	s.sobratAdresaSet = func(_ []protokol.Server, _ string, zagruzki ...string) ([]netip.Addr, error) {
		poluchil = zagruzki
		return nil, nil
	}
	if _, err := s.adresaKandidatov(); err != nil {
		t.Fatal(err)
	}
	if len(poluchil) != 0 {
		t.Fatalf("список выключен, а его адрес всё ещё разрешают: %v", poluchil)
	}
}
