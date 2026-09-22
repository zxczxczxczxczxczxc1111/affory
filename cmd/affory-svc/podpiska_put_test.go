package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Стратегия загрузки подписки (A8): общий бюджет, два названных пути и
// запасной заход, ограниченный по смыслу.
//
// Проверяется НАСТОЯЩАЯ стратегия: подменяется только сеть под ней
// (s.zagruzitCherez), а выбор пути, срок и запись в журнал остаются свои.

type zahod struct {
	proksi  string
	popytok int
	srok    time.Duration
}

// sluzhbaSPutyami заводит службу, у которой сеть отвечает по списку ответов:
// i-й заход получает i-й ответ, лишние заходы получают последний.
func sluzhbaSPutyami(t *testing.T, otvety ...error) (*Sluzhba, *[]zahod, *bytes.Buffer) {
	t.Helper()
	s := podstavnaya(t, nil)
	var b bytes.Buffer
	s.zhurnalDiag = diagnostika.NovyyZhurnal(&b)
	zahody := make([]zahod, 0, 4)
	s.zagruzitCherez = func(ctx context.Context, adres, proksi string, popytok int) (ssylki.Razbor, error) {
		z := zahod{proksi: proksi, popytok: popytok}
		if dl, est := ctx.Deadline(); est {
			z.srok = time.Until(dl)
		}
		zahody = append(zahody, z)
		otvet := otvety[len(otvety)-1]
		if len(zahody) <= len(otvety) {
			otvet = otvety[len(zahody)-1]
		}
		if otvet != nil {
			return ssylki.Razbor{}, otvet
		}
		return ssylki.Razbor{Servery: []protokol.Server{serverProby()}}, nil
	}
	return s, &zahody, &b
}

func podnyatyyTunnel(s *Sluzhba, port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sost = protokol.SostPodnyat
	s.portProksiNash = port
}

// nedostupna строит отказ загрузки с названным шагом - ровно такой, какой
// приезжает из ssylki.
func nedostupna(vid sboi.Vid) error {
	return ssylki.OtkazSVidom(vid, ssylki.ErrPodpiskaNedostupna)
}

func TestPriOpushchennomTunneleDorogaOdna(t *testing.T) {
	s, zahody, _ := sluzhbaSPutyami(t, nil)
	if _, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub"); err != nil {
		t.Fatalf("загрузка: %v", err)
	}
	if len(*zahody) != 1 {
		t.Fatalf("заходов %d, ждали один: туннеля нет, второй дороге неоткуда взяться", len(*zahody))
	}
	z := (*zahody)[0]
	if z.proksi != "" {
		t.Errorf("прокси %q при опущенном туннеле", z.proksi)
	}
	if z.popytok != povtorovNapryamuyu {
		t.Errorf("повторов %d, ждали %d", z.popytok, povtorovNapryamuyu)
	}
	// Весь бюджет одной дороге: делить срок надвое там, где второй половине
	// некуда идти, значит вдвое сократить единственную попытку.
	if z.srok < budzhetPodpiski-5*time.Second {
		t.Errorf("срок дороги %v, а бюджет %v: половина съедена ни на что", z.srok, budzhetPodpiski)
	}
}

func TestPriPodnyatomTunneleSnachalaVpnPotomNapryamuyu(t *testing.T) {
	s, zahody, _ := sluzhbaSPutyami(t, nedostupna(sboi.TLS), nil)
	podnyatyyTunnel(s, 10809)
	r, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub")
	if err != nil {
		t.Fatalf("прямой путь не спас загрузку: %v", err)
	}
	if len(r.Servery) != 1 {
		t.Fatalf("серверов %d", len(r.Servery))
	}
	if len(*zahody) != 2 {
		t.Fatalf("заходов %d, ждали два", len(*zahody))
	}
	if got := (*zahody)[0].proksi; got != "127.0.0.1:10809" {
		t.Errorf("первый заход шёл через %q, ждали локальный вход ядра", got)
	}
	if got := (*zahody)[0].popytok; got != povtorovCherezVpn {
		t.Errorf("повторов через туннель %d, ждали %d", got, povtorovCherezVpn)
	}
	if got := (*zahody)[1].proksi; got != "" {
		t.Errorf("запасной заход шёл через %q, а он обязан быть прямым", got)
	}
	// Половина срока каждому: походу через туннель нельзя съедать время
	// прямого, иначе запасной путь существует только на бумаге.
	pervyy, vtoroy := (*zahody)[0].srok, (*zahody)[1].srok
	if pervyy > budzhetPodpiski*3/5 {
		t.Errorf("первому пути досталось %v из %v", pervyy, budzhetPodpiski)
	}
	if vtoroy < budzhetPodpiski/3 {
		t.Errorf("запасному пути осталось %v из %v", vtoroy, budzhetPodpiski)
	}
}

func TestOtkazPoPravuDostupaNeIshchetDrugoyDorogi(t *testing.T) {
	// Панель ответила и отказала: дело в ссылке, а не в дороге. Второй заход
	// вернул бы тот же ответ и лишний раз засветил бы пропуск в сети.
	s, zahody, _ := sluzhbaSPutyami(t, nedostupna(sboi.Dostup))
	podnyatyyTunnel(s, 10809)
	if _, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub"); err == nil {
		t.Fatal("отказ доступа не вернулся наружу")
	}
	if len(*zahody) != 1 {
		t.Fatalf("заходов %d, ждали один", len(*zahody))
	}
}

func TestSoderzhatelnyyOtvetNeIshchetDrugoyDorogi(t *testing.T) {
	// Пустая подписка это ответ, а не недоступность: другая дорога отдаст ровно
	// то же самое.
	s, zahody, _ := sluzhbaSPutyami(t, ssylki.ErrPodpiskaPusta)
	podnyatyyTunnel(s, 10809)
	if _, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub"); !errors.Is(err, ssylki.ErrPodpiskaPusta) {
		t.Fatalf("ошибка %v, ждали пустую подписку", err)
	}
	if len(*zahody) != 1 {
		t.Fatalf("заходов %d, ждали один", len(*zahody))
	}
}

func TestOtmenaNeZapuskaetZapasnoyPut(t *testing.T) {
	s, zahody, _ := sluzhbaSPutyami(t, nedostupna(sboi.TCP))
	podnyatyyTunnel(s, 10809)
	ctx, otmena := context.WithCancel(context.Background())
	otmena()
	if _, err := s.zagruzitPodpiskuStrategiey(ctx, "https://panel.example/sub"); err == nil {
		t.Fatal("отменённая загрузка ответила успехом")
	}
	// Ноль или один заход: сеть могла не успеть начаться вовсе. Двух быть не
	// может: запасной путь при отменённом сроке это враньё про бюджет.
	if len(*zahody) > 1 {
		t.Fatalf("заходов %d при отменённом контексте", len(*zahody))
	}
}

func TestZhurnalNazyvaetKazhdyyPut(t *testing.T) {
	s, _, b := sluzhbaSPutyami(t, nedostupna(sboi.TLS), nil)
	podnyatyyTunnel(s, 10809)
	if _, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub?token=tayna"); err != nil {
		t.Fatalf("загрузка: %v", err)
	}
	var puti, itogi, shagi []string
	for _, stroka := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if stroka == "" {
			continue
		}
		var sr diagnostika.Srez
		if err := json.Unmarshal([]byte(stroka), &sr); err != nil {
			t.Fatalf("строка журнала не разбирается: %v", err)
		}
		if sr.Sobytie != "операция zagruzka-podpiski" {
			continue
		}
		puti = append(puti, sr.OpPut)
		itogi = append(itogi, sr.OpItog)
		shagi = append(shagi, sr.OpShag)
	}
	if len(puti) != 2 || puti[0] != putCherezVpn || puti[1] != putNapryamuyu {
		t.Fatalf("пути в журнале %v", puti)
	}
	if itogi[0] != "otkaz" || itogi[1] != "ok" {
		t.Errorf("итоги %v", itogi)
	}
	if shagi[0] != string(sboi.TLS) {
		t.Errorf("шаг первого пути %q, ждали tls", shagi[0])
	}
	if strings.Contains(b.String(), "tayna") || strings.Contains(b.String(), "panel.example") {
		t.Errorf("адрес подписки доехал до журнала: %s", b.String())
	}
}

func TestObshchiyByudzhetOgranichivaetVseZahody(t *testing.T) {
	staryy := budzhetPodpiski
	budzhetPodpiski = 300 * time.Millisecond
	defer func() { budzhetPodpiski = staryy }()

	s := podstavnaya(t, nil)
	var zahodov int
	// Сеть, которая не отвечает никогда: без общего бюджета стратегия
	// досидела бы до конца срока каждой попытки, а их пять.
	//
	// Отвечает ГОЛЫМ ctx.Err(), как настоящий ZagruzitSPovtorami, когда срок
	// истекает между попытками: обёртки «подписка недоступна» на нём нет, и
	// стратегия обязана понимать это сама.
	s.zagruzitCherez = func(ctx context.Context, adres, proksi string, popytok int) (ssylki.Razbor, error) {
		zahodov++
		<-ctx.Done()
		return ssylki.Razbor{}, ctx.Err()
	}
	podnyatyyTunnel(s, 10809)
	nachalo := time.Now()
	if _, err := s.zagruzitPodpiskuStrategiey(context.Background(), "https://panel.example/sub"); err == nil {
		t.Fatal("молчащая сеть ответила успехом")
	}
	proshlo := time.Since(nachalo)
	if proshlo > budzhetPodpiski*2 {
		t.Fatalf("обновление заняло %v при бюджете %v", proshlo, budzhetPodpiski)
	}
	if zahodov != 2 {
		t.Errorf("заходов %d, ждали два: через туннель и запасной прямой", zahodov)
	}
}
