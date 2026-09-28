package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Задача 6.5. Служба принимает архив, который уже лежит на диске, сверяет
// sha256 и запускает подменщика. Сам подменщик проверен в internal/obnovlenie.

func arhivSborki(t *testing.T, fayly map[string]string, pravilnyyHesh bool) string {
	t.Helper()
	d := t.TempDir()
	put := filepath.Join(d, "affory.zip")
	f, err := os.Create(put)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for imya, telo := range fayly {
		z, _ := w.Create(imya)
		z.Write([]byte(telo))
	}
	w.Close()
	f.Close()
	b, _ := os.ReadFile(put)
	h := sha256.Sum256(b)
	hesh := hex.EncodeToString(h[:])
	if !pravilnyyHesh {
		hesh = "00" + hesh[2:]
	}
	os.WriteFile(put+".sha256", []byte(hesh+"  affory.zip\n"), 0o600)
	return put
}

func TestInstallUpdateOtvergaetChuzhoyHesh(t *testing.T) {
	s := podstavnaya(t, nil)
	zapuskov := 0
	s.zapustitPodmenshchika = func(prog, novaya string, podnyat bool) error { zapuskov++; return nil }
	put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, false)
	telo, _ := json.Marshal(map[string]string{"path": put})
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodArhivNegoden {
		t.Fatalf("архив с чужим хешем принят: %+v", o)
	}
	if zapuskov != 0 {
		t.Fatal("подменщик запущен для негодного архива")
	}
}

// Н10 аудита 1.6.1: после самообновления туннель оставался опущенным, даже
// если человек до обновления сидел в VPN. Служба опускает его перед подменой,
// поэтому помнить, был ли он поднят, обязана она же и передать подменщику.
func TestObnovlenieZapominaetPodnyatyyTunnel(t *testing.T) {
	for _, podnimat := range []bool{false, true} {
		s := podstavnaya(t, nil)
		if podnimat {
			if err := s.Connect(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		var peredano *bool
		s.zapustitPodmenshchika = func(prog, novaya string, podnyat bool) error { peredano = &podnyat; return nil }
		put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, true)
		telo, _ := json.Marshal(map[string]string{"path": put})
		o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
		if o.Oshib != nil {
			t.Fatalf("обновление отвергнуто: %+v", o.Oshib)
		}
		if peredano == nil || *peredano != podnimat {
			t.Fatalf("туннель был поднят: %v, подменщику передано %v", podnimat, peredano)
		}
	}
}

// Флаг доезжает от подменщика до старта новой службы: install с ним стартует
// службу аргументом «поднять после обновления», а не install-idle.
func TestFlagPodnyatiyaDoezzhaetDoStartaSluzhby(t *testing.T) {
	if got := argumentyUstanovki(true); strings.Join(got, " ") != "install "+flagPodnyat {
		t.Fatalf("подменщик зовёт %v", got)
	}
	if got := argumentyUstanovki(false); strings.Join(got, " ") != "install" {
		t.Fatalf("подменщик без туннеля зовёт %v", got)
	}
	if !estFlagPodnyat([]string{"affory-svc.exe", "install", flagPodnyat}) {
		t.Fatal("install не видит флага")
	}
	if argumentStarta(true) != argumentPosleObnovleniya || argumentStarta(false) != argumentUstanovki {
		t.Fatal("служба стартует не тем аргументом")
	}
	args := []string{imyaSluzhby, argumentPosleObnovleniya}
	if !razreshenAvtopodyom(args) || !podnyatPosleObnovleniya(args) {
		t.Fatal("новая служба не поднимет туннель после обновления")
	}
	if podnyatPosleObnovleniya([]string{imyaSluzhby}) {
		t.Fatal("обычный старт принят за старт после обновления")
	}
}

// Новая служба поднимает туннель после обновления и без флага «подключать
// при старте»: человек его не выключал, его опустило обновление.
func TestPosleObnovleniyaTunnelPodnimaetsya(t *testing.T) {
	s := podstavnaya(t, nil)
	podnimali := 0
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podnimali++
		return set.Adapter{Indeks: 10, Imya: "tun0", Adresa: []netip.Addr{netip.MustParseAddr("172.19.0.1")}}, nil
	}
	s.prochitat = func() (sostoyanie.SostoyanieFayla, error) {
		return sostoyanie.SostoyanieFayla{Sostoyanie: protokol.SostVyklyuchen}, nil
	}
	s.zagruzitNastroyki()
	s.PomnitPodnyatPosleObnovleniya()
	s.PodklyuchitPriStarte(context.Background())
	if podnimali != 1 {
		t.Fatalf("подъёмов после обновления %d, ожидали 1", podnimali)
	}
}

func TestInstallUpdateBezPutiOtkazyvaet(t *testing.T) {
	s := podstavnaya(t, nil)
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodArhivNegoden {
		t.Fatalf("пустое тело принято: %+v", o)
	}
}

func TestInstallUpdateTrebuetAdmina(t *testing.T) {
	// Подмена файлов в Program Files это расширение области поражения: без
	// прав любой процесс от пользователя подложил бы свою службу.
	s := podstavnaya(t, nil)
	put := arhivSborki(t, map[string]string{"affory-svc.exe": "n"}, true)
	telo, _ := json.Marshal(map[string]string{"path": put})
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("без прав: %+v", o)
	}
}

func TestStatusPokazyvaetIshodObnovleniyaIZabiraetFayl(t *testing.T) {
	// Исход пишет подменщик ПОСЛЕ того, как новая служба уже поднялась и
	// ответила ему, поэтому читать файл на старте поздно: он появляется через
	// секунду после старта. Читается при status, файл забирается один раз.
	s := podstavnaya(t, nil)
	s.dirDannyh = t.TempDir()
	if err := obnovlenie.ZapisatItog(s.dirDannyh, obnovlenie.Itog{Kod: obnovlenie.KodOtkat, Tekst: "новая служба молчит"}); err != nil {
		t.Fatal(err)
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"})
	var st protokol.StatusOtvet
	if err := json.Unmarshal(o.Telo, &st); err != nil {
		t.Fatal(err)
	}
	if st.Oshib == nil || st.Oshib.Kod != obnovlenie.KodOtkat {
		t.Fatalf("статус не показал откат: %+v", st.Oshib)
	}
	if _, est, _ := obnovlenie.ProchitatItog(s.dirDannyh); est {
		t.Fatal("файл исхода пережил показ")
	}
}

// Б1 аудита 1.6.1. Архив ставился любой версии, и старая сборка молча
// откатывала машину на исправленные дефекты. Номер берётся из ресурсов
// affory-ui.exe в архиве: запускать новый exe от SYSTEM ради номера нельзя.
func TestInstallUpdateNePonizhaetVersiyu(t *testing.T) {
	sluchai := []struct {
		ustanovlena, vArhive string
		prochitana, prinyat  bool
	}{
		{"1.7.0", "1.6.2", true, false},
		{"1.7.0", "1.7.0", true, true},
		{"1.7.0", "1.7.1", true, true},
		{"1.7.0", "", false, true},
		{"dev", "1.0.0", true, true},
	}
	for _, sl := range sluchai {
		byla := versiyaProgrammy
		versiyaProgrammy = sl.ustanovlena
		byloChtenie := versiyaSborki
		versiyaSborki = func(string) (string, bool) { return sl.vArhive, sl.prochitana }
		s := podstavnaya(t, nil)
		zapuskov := 0
		s.zapustitPodmenshchika = func(prog, novaya string, podnyat bool) error { zapuskov++; return nil }
		put := arhivSborki(t, map[string]string{"affory-svc.exe": "n", "affory-ui.exe": "u"}, true)
		telo, _ := json.Marshal(map[string]string{"path": put})
		o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "installUpdate", Telo: telo})
		versiyaProgrammy, versiyaSborki = byla, byloChtenie
		if sl.prinyat && (o.Oshib != nil || zapuskov != 1) {
			t.Errorf("%s поверх %s: ждали установку, получили %+v, запусков %d", sl.vArhive, sl.ustanovlena, o.Oshib, zapuskov)
		}
		if !sl.prinyat && (o.Oshib == nil || o.Oshib.Kod != protokol.KodArhivNegoden || !strings.Contains(o.Oshib.Tekst, "старее") || zapuskov != 0) {
			t.Errorf("%s поверх %s: ждали отказ «старее», получили %+v, запусков %d", sl.vArhive, sl.ustanovlena, o.Oshib, zapuskov)
		}
	}
}

// Номер из ресурсов настоящего exe. notepad.exe есть на любой Windows и несёт
// VERSIONINFO; файл без ресурсов номера не даёт.
func TestVersiyaIzResursovExe(t *testing.T) {
	v, ok := versiyaIzResursov(filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe"))
	if !ok {
		t.Fatal("номер notepad.exe не прочитан")
	}
	if _, razobrana := razobratVersiyu(v); !razobrana {
		t.Fatalf("номер %q не в виде X.Y.Z", v)
	}
	ne := filepath.Join(t.TempDir(), "ne.exe")
	if err := os.WriteFile(ne, []byte("не exe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, ok := versiyaIzResursov(ne); ok {
		t.Fatalf("у файла без ресурсов прочитан номер %q", v)
	}
}
