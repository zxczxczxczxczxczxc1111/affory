package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Обновление (задача 6.5, усечённый объём): installUpdate{path} к архиву, который
// уже лежит на диске. Служба сверяет sha256, распаковывает рядом, опускает
// туннель и запускает подменщика: копию ТЕКУЩЕГО affory-svc.exe во временном
// каталоге в режиме swap. Подменщик останавливает службу, подменяет файлы,
// ставит заново и ждёт ответа по каналу; не дождался за 20 секунд, возвращает
// прежнюю версию. Исход ложится в obnovlenie.json и показывается один раз.
//
// Подменщик именно текущий, а не новый: первый живой прогон 03.09.2026 запускал
// новый, и заведомо битая сборка (не служба вовсе) молча не сделала ничего,
// то есть откат не проверялся тем самым случаем, ради которого он написан.

const rezhimPodmeny = "swap"

func (s *Sluzhba) installUpdate(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Put string `json:"path"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, err.Error())
		}
	}
	if telo.Put == "" {
		return otkaz(k.Id, k.Imya, protokol.KodArhivNegoden, "нужен путь к архиву сборки")
	}
	return s.ustanovitArhiv(k, telo.Put)
}

// ustanovitArhiv это общая часть installUpdate и downloadUpdate: сверка,
// распаковка, опускание туннеля, подменщик.
func (s *Sluzhba) ustanovitArhiv(k protokol.Kadr, put string) protokol.Kadr {
	if err := obnovlenie.Sverit(put); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodArhivNegoden, err.Error())
	}
	novaya := filepath.Join(s.dirProgrammy, obnovlenie.KatalogNovoy)
	if err := obnovlenie.Raspakovat(put, novaya); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodArhivNegoden, err.Error())
	}
	if _, err := os.Stat(filepath.Join(novaya, "affory-svc.exe")); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodArhivNegoden, "в архиве нет affory-svc.exe")
	}
	// Туннель опускается ДО подмены: правила и политика снимаются штатно, а
	// не остаются сиротами от службы, которую сейчас убьют.
	s.Otklyuchit()
	if st := s.Status(); st.Oshib != nil {
		return otkaz(k.Id, k.Imya, st.Oshib.Kod, st.Oshib.Tekst)
	}
	if err := s.zapustitPodmenshchika(s.dirProgrammy, novaya); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodUpdateRollback, "подменщик не запущен: "+err.Error())
	}
	return otvet(k.Id, k.Imya, map[string]any{"zapushchena": true, "srok_s": int(obnovlenie.SrokPodyoma.Seconds())})
}

// zapustitPodmenshchikaVTemp копирует ТЕКУЩУЮ службу во временный каталог и
// запускает её в режиме swap отвязанным процессом: служба, которая сейчас
// отвечает, через секунды будет остановлена вместе со всеми своими детьми.
func zapustitPodmenshchikaVTemp(prog, novaya string) error {
	svoy, err := os.Executable()
	if err != nil {
		return err
	}
	vrem := filepath.Join(os.TempDir(), "affory-podmena")
	if err := os.MkdirAll(vrem, 0o700); err != nil {
		return err
	}
	b, err := os.ReadFile(svoy)
	if err != nil {
		return err
	}
	exe := filepath.Join(vrem, "affory-svc.exe")
	if err := os.WriteFile(exe, b, 0o700); err != nil {
		return err
	}
	cmd := exec.Command(exe, rezhimPodmeny, prog, novaya)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	return cmd.Start()
}

// podmenit это режим swap: тело подменщика. Пишет в log\obnovlenie.log: у
// отвязанного процесса нет ни консоли, ни родителя, которому жаловаться.
func podmenit(prog, novaya string) {
	if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "obnovlenie.log"); err == nil {
		log.SetOutput(zh)
		defer zh.Close()
	}
	log.Printf("подмена начата: %s <- %s", prog, novaya)
	p := obnovlenie.Podmena{
		KatalogProgrammy: prog,
		Novaya:           novaya,
		Ostanovit:        ostanovitSluzhbu,
		Ustanovit: func() error {
			out, err := exec.Command(filepath.Join(prog, "affory-svc.exe"), "install").CombinedOutput()
			if err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		},
		ZhdatOtveta: zhdatOtvetaSluzhby,
		Srok:        obnovlenie.SrokPodyoma,
	}
	itog := p.Vypolnit()
	log.Printf("подмена закончена: ok=%v %s %s", itog.Ok, itog.Kod, itog.Tekst)
	if err := obnovlenie.ZapisatItog(sostoyanie.KatalogDannyh(), itog); err != nil {
		log.Printf("исход обновления не записан: %v", err)
	}
}

func ostanovitSluzhbu() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(imyaSluzhby)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return err
	}
	return zhdatSostoyaniya(s, svc.Stopped)
}

// zhdatOtvetaSluzhby ждёт не «Running» у диспетчера, а ОТВЕТ по каналу: служба,
// висящая в Running с мёртвым каналом, это ровно тот случай, ради которого
// откат и заведён.
func zhdatOtvetaSluzhby(srok time.Duration) error {
	do := time.Now().Add(srok)
	var posledn error
	for time.Now().Before(do) {
		k, err := kanal.Podklyuchitsya()
		if err == nil {
			ctxOtv, otm := context.WithTimeout(context.Background(), 3*time.Second)
			_, err = zvatHello(ctxOtv, k)
			otm()
			_ = k.Zakryt()
			if err == nil {
				return nil
			}
		}
		posledn = err
		time.Sleep(500 * time.Millisecond)
	}
	return posledn
}

func zvatHello(ctx context.Context, k *kanal.Klient) (protokol.Kadr, error) {
	tip := make(chan struct{})
	var kadr protokol.Kadr
	var err error
	go func() {
		kadr, err = k.Zvat("hello", map[string]int{"protocol": protokol.Versiya})
		close(tip)
	}()
	select {
	case <-tip:
		return kadr, err
	case <-ctx.Done():
		return protokol.Kadr{}, ctx.Err()
	}
}

// pokazatItogObnovleniya забирает исход прошлой подмены и показывает его в
// статусе. Успех идёт только в журнал. Зовётся на каждом status: исход
// появляется ПОСЛЕ старта новой службы, когда подменщик дождался её ответа.
func (s *Sluzhba) pokazatItogObnovleniya() {
	i, est, err := obnovlenie.ProchitatItog(s.dirDannyh)
	if err != nil {
		log.Printf("исход обновления не прочитан: %v", err)
		return
	}
	if !est {
		return
	}
	if i.Ok {
		log.Printf("обновление прошло, новая версия отвечает")
		return
	}
	log.Printf("обновление откачено: %s %s", i.Kod, i.Tekst)
	s.mu.Lock()
	s.oshib = &protokol.Oshibka{Kod: i.Kod, Tekst: i.Tekst}
	s.mu.Unlock()
}

// ubratHvostyPodmeny стирает *.ubrat в каталоге программы: файл работавшего
// окна, отодвинутый подменой, удалить в тот момент было нельзя.
func ubratHvostyPodmeny() {
	hvosty, _ := filepath.Glob(filepath.Join(sostoyanie.KatalogProgrammy(), "*.ubrat"))
	for _, h := range hvosty {
		_ = os.Remove(h)
	}
}
