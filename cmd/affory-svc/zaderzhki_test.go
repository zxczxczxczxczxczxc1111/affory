package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Команда measureDelays: пинг по каждому серверу одним числом (01.10.2026).
//
// Число то же, что строка «Задержка» на главном экране и пинг в Discord: один
// круг по прогретому соединению. Через какой сервер идти, называет логин
// входа замеров у временного ядра.

// pingovayaSluzhba это служба с двумя серверами и подставным ядром замера.
// ostanovleno считает остановки ядра: временное ядро обязано гаснуть всегда.
func pingovayaSluzhba(t *testing.T, isklyucheny map[string]bool) (s *Sluzhba, ostanovleno func() int) {
	t.Helper()
	s = podstavnaya(t, nil)
	// Ожидание строки ядра (dozhdatsyaZhalob) судит свой тест; остальным
	// секунда на каждый замер с отказом ни к чему.
	srokBylo := srokStrokiYadra
	srokStrokiYadra = 10 * time.Millisecond
	t.Cleanup(func() { srokStrokiYadra = srokBylo })
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = []protokol.Server{
			{Id: "a", Imya: "первый", Transport: "trojan", Host: "192.0.2.1", Port: 443, Parol: "p"},
			{Id: "b:2", Imya: "второй", Transport: "hy2", Host: "192.0.2.2", Port: 443, Parol: "p"},
		}
		n.Vybran = "a"
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	var mu sync.Mutex
	n := 0
	s.yadroZamera = func(context.Context) (vremennoeYadro, error) {
		return vremennoeYadro{
			zamer:       genkonfig.VhodZamera{Port: 10810, Parol: "sekret"},
			isklyucheny: isklyucheny,
			ostanovit: func() {
				mu.Lock()
				n++
				mu.Unlock()
			},
		}, nil
	}
	return s, func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

func izmerit(t *testing.T, s *Sluzhba) map[string]zamerZaderzhki {
	t.Helper()
	k := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "komanda", Id: 1, Imya: "measureDelays"})
	if k.Oshib != nil {
		t.Fatalf("замер отвергнут целиком: %+v", k.Oshib)
	}
	var o struct {
		Zamery []zamerZaderzhki `json:"zamery"`
	}
	if err := json.Unmarshal(k.Telo, &o); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	po := map[string]zamerZaderzhki{}
	for _, z := range o.Zamery {
		po[z.Id] = z
	}
	return po
}

// Главное свойство: каждый сервер меряется через СВОЙ логин входа замеров, с
// паролем этого ядра, и число приходит в ping_ms.
func TestPingIdyotCherezLoginSvoegoServera(t *testing.T) {
	s, ostanovleno := pingovayaSluzhba(t, nil)
	var mu sync.Mutex
	loginy := map[string]string{}
	s.zamerPinga = func(_ context.Context, _ string, proksi *url.URL) (time.Duration, error) {
		parol, _ := proksi.User.Password()
		mu.Lock()
		loginy[proksi.User.Username()] = parol + "@" + proksi.Host
		mu.Unlock()
		return 61 * time.Millisecond, nil
	}

	po := izmerit(t, s)
	for _, id := range []string{"a", "b:2"} {
		z := po[id]
		if z.PingMs == nil || *z.PingMs != 61 {
			t.Fatalf("%s: пинг %v, ждали 61", id, z.PingMs)
		}
		if loginy[genkonfig.PolzovatelZamera(id)] != "sekret@127.0.0.1:10810" {
			t.Fatalf("%s мерился не своим логином: %v", id, loginy)
		}
	}
	if n := ostanovleno(); n != 1 {
		t.Fatalf("временное ядро остановлено %d раз, ждали один", n)
	}
}

// Мёртвый сервер это отказ с причиной, а НЕ ноль: ноль поставил бы его первым
// по пингу, то есть ровно наверх списка.
func TestPingMyortvyyServerEtoOtkazANeNol(t *testing.T) {
	s, _ := pingovayaSluzhba(t, nil)
	s.zamerPinga = func(_ context.Context, _ string, proksi *url.URL) (time.Duration, error) {
		if proksi.User.Username() == genkonfig.PolzovatelZamera("a") {
			return 0, errors.New("замер не прошёл: EOF")
		}
		return 40 * time.Millisecond, nil
	}
	po := izmerit(t, s)
	if po["a"].PingMs != nil || po["a"].PingOtkaz == "" {
		t.Fatalf("мёртвый сервер: %+v", po["a"])
	}
	if po["b:2"].PingMs == nil {
		t.Fatal("отказ одного сервера унёс замер соседа")
	}
}

// Причину отказа знает только журнал ядра (02.10.2026): замер видит обрыв,
// а ядро пишет «x509: certificate has expired» строкой с тегом сервера.
// Спрашивается она у ТОГО ядра, которым мерили, после его остановки (иначе
// строка последнего сервера могла бы ещё не доехать), и только про отказавший
// сервер.
func TestPingNazyvaetPrichinuIzZhurnalaYadra(t *testing.T) {
	s, ostanovleno := pingovayaSluzhba(t, nil)
	prezhnee := s.yadroZamera
	s.yadroZamera = func(ctx context.Context) (vremennoeYadro, error) {
		y, err := prezhnee(ctx)
		y.konfig = `C:\x\sing-box.zamer.json`
		return y, err
	}
	s.zamerPinga = func(_ context.Context, _ string, proksi *url.URL) (time.Duration, error) {
		if proksi.User.Username() == genkonfig.PolzovatelZamera("b:2") {
			return 40 * time.Millisecond, nil
		}
		return 0, errors.New("замер не прошёл: EOF")
	}
	nachalo := time.Now()
	var sprosheno []string
	posleOstanovki := 0
	s.zhalobaVyhoda = func(konfig, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		sprosheno = append(sprosheno, teg)
		if konfig != `C:\x\sing-box.zamer.json` {
			t.Errorf("жалоба спрошена у конфига %q, а мерило другое ядро", konfig)
		}
		if posle.Before(nachalo) {
			t.Errorf("жалобы берутся с %v, раньше замера %v: прошлое выдалось бы за нынешнее", posle, nachalo)
		}
		if ostanovleno() == 1 {
			posleOstanovki++
		}
		// Жалоба есть и на второй сервер: она не должна перебить его число.
		return yadra.ZhalobaYadra{Prichina: sboi.SrokSertifikata, Vremya: posle}, true
	}
	po := izmerit(t, s)
	if z := po["a"]; z.PingMs != nil || z.PingPrichina != sboi.SrokSertifikata.Korotko() || z.PingOtkaz != sboi.SrokSertifikata.Tekst() {
		t.Fatalf("отказавший сервер: %+v", z)
	}
	if z := po["b:2"]; z.PingMs == nil || z.PingPrichina != "" || z.PingOtkaz != "" {
		t.Fatalf("ответивший сервер получил причину отказа: %+v", z)
	}
	for _, teg := range sprosheno {
		if teg != genkonfig.TegKandidata("a") {
			t.Fatalf("жалобы спрошены про %v, ждали только тег отказавшего", sprosheno)
		}
	}
	// До остановки жалобы можно только ждать: ответ берётся после неё, когда
	// вывод ядра дочитан.
	if posleOstanovki != 1 {
		t.Fatalf("после остановки ядра жалоба спрошена %d раз, ждали 1", posleOstanovki)
	}
	if n := ostanovleno(); n != 1 {
		t.Fatalf("временное ядро остановлено %d раз, ждали один", n)
	}
}

// Ядро сначала рвёт соединение и только потом пишет причину, а остановка это
// Kill (приёмка 1.9.3, 02.10.2026). Здесь строка приходит через 100 мс после
// обрыва и только если ядро к тому времени живо: гасить его сразу значило бы
// терять причину последнего отказавшего сервера.
func TestPingZhdyotStrokuYadraDoOstanovki(t *testing.T) {
	s, ostanovleno := pingovayaSluzhba(t, nil)
	srokStrokiYadra = time.Second
	var stroka atomic.Bool
	s.zamerPinga = func(_ context.Context, _ string, proksi *url.URL) (time.Duration, error) {
		if proksi.User.Username() == genkonfig.PolzovatelZamera("b:2") {
			return 40 * time.Millisecond, nil
		}
		go func() {
			time.Sleep(100 * time.Millisecond)
			if ostanovleno() == 0 {
				stroka.Store(true)
			}
		}()
		return 0, errors.New("замер не прошёл: EOF")
	}
	s.zhalobaVyhoda = func(_, teg string, posle time.Time) (yadra.ZhalobaYadra, bool) {
		if teg != genkonfig.TegKandidata("a") || !stroka.Load() {
			return yadra.ZhalobaYadra{}, false
		}
		return yadra.ZhalobaYadra{Prichina: sboi.ObryvRukopozhatiya, Vremya: time.Now()}, true
	}
	if z := izmerit(t, s)["a"]; z.PingPrichina != sboi.ObryvRukopozhatiya.Korotko() {
		t.Fatalf("строка ядра потеряна, ядро погашено раньше неё: %+v", z)
	}
}

// Ядро ничего узнаваемого не сказало: остаётся прежний текст отказа, а не
// пустая строка и не догадка.
func TestPingBezZhalobyOstavlyaetPrezhniyOtkaz(t *testing.T) {
	s, _ := pingovayaSluzhba(t, nil)
	s.zamerPinga = func(context.Context, string, *url.URL) (time.Duration, error) {
		return 0, errors.New("замер не прошёл: EOF")
	}
	s.zhalobaVyhoda = func(string, string, time.Time) (yadra.ZhalobaYadra, bool) {
		return yadra.ZhalobaYadra{}, false
	}
	z := izmerit(t, s)["a"]
	if z.PingOtkaz == "" || z.PingPrichina != "" {
		t.Fatalf("без жалобы ядра: %+v", z)
	}
}

// Сервер, которого ядро не приняло, не мерится и не прячется: человек видит
// причину, а не вечный «не измерен».
func TestPingIsklyuchyonnyyYadromServer(t *testing.T) {
	s, _ := pingovayaSluzhba(t, map[string]bool{"b:2": true})
	s.zamerPinga = func(context.Context, string, *url.URL) (time.Duration, error) {
		return 40 * time.Millisecond, nil
	}
	po := izmerit(t, s)
	if po["b:2"].PingMs != nil || !strings.Contains(po["b:2"].PingOtkaz, "ядро не принимает") {
		t.Fatalf("исключённый сервер: %+v", po["b:2"])
	}
}

// Часы Windows умеют отдать одно и то же время дважды. Ноль на экране это
// «мгновенно», поэтому меньше миллисекунды не бывает.
func TestPingNeBivaetNulevym(t *testing.T) {
	s, _ := pingovayaSluzhba(t, nil)
	s.zamerPinga = func(context.Context, string, *url.URL) (time.Duration, error) {
		return 300 * time.Microsecond, nil
	}
	if z := izmerit(t, s)["a"]; z.PingMs == nil || *z.PingMs != 1 {
		t.Fatalf("пинг меньше миллисекунды: %v", z.PingMs)
	}
}

// Сборка временного ядра пинга несёт вход замеров с портом и паролем, которые
// она же и вернула: иначе прибор стучался бы не туда. Проверке сервера этот
// вход не положен (TestKonfigProverkiServeraBezTunIBezSledov).
func TestSborkaPingaNesyotVhodZamerov(t *testing.T) {
	s := podstavnaya(t, nil)
	telo, _, _, _, zamer, err := s.sobratTunPolno(nil, true, true, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if zamer.Port <= 0 || zamer.Parol == "" {
		t.Fatalf("вход замеров не выдан: %+v", zamer)
	}
	var k struct {
		Inbounds []struct {
			Tag        string `json:"tag"`
			ListenPort int    `json:"listen_port"`
			Users      []struct {
				Password string `json:"password"`
			} `json:"users"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	if len(k.Inbounds) != 1 || k.Inbounds[0].Tag != genkonfig.TegZamerVhod || k.Inbounds[0].ListenPort != zamer.Port {
		t.Fatalf("входы сборки пинга: %+v", k.Inbounds)
	}
	if len(k.Inbounds[0].Users) == 0 || k.Inbounds[0].Users[0].Password != zamer.Parol {
		t.Fatal("пароль входа не тот, что вернула сборка")
	}
}

// Ядро для замера не поднялось: виноваты мы, а не серверы. Это отказ команды
// с причиной, а не список из одних мёртвых серверов.
func TestPingBezYadraEtoOtkazKomandy(t *testing.T) {
	s := podstavnaya(t, nil)
	k := s.Obrabotat(context.Background(), protokol.Kadr{Tip: "komanda", Id: 1, Imya: "measureDelays"})
	if k.Oshib == nil || k.Oshib.Kod != protokol.KodYadroNeOtvechaet {
		t.Fatalf("отказ ядра замера: %+v", k.Oshib)
	}
	if !strings.Contains(k.Oshib.Tekst, "пинг не измерен") {
		t.Fatalf("текст не говорит, что не вышло: %q", k.Oshib.Tekst)
	}
}

// Окно и консоль разом: временное ядро одно в каждый момент, потому что
// конфиг у него один файл, и оба замера доходят до чисел.
func TestPingDvaZameraRazomPoOcheredi(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.pravitNabor(func(n *Nabor) error {
		n.Servery = []protokol.Server{{Id: "a", Imya: "первый", Transport: "trojan", Host: "192.0.2.1", Port: 443, Parol: "p"}}
		n.Vybran = "a"
		return nil
	}); err != nil {
		t.Fatalf("набор не записан: %v", err)
	}
	var mu sync.Mutex
	zhivyh, bolshe := 0, 0
	s.yadroZamera = func(context.Context) (vremennoeYadro, error) {
		mu.Lock()
		zhivyh++
		bolshe = max(bolshe, zhivyh)
		mu.Unlock()
		return vremennoeYadro{
			zamer: genkonfig.VhodZamera{Port: 10810, Parol: "sekret"},
			ostanovit: func() {
				mu.Lock()
				zhivyh--
				mu.Unlock()
			},
		}, nil
	}
	s.zamerPinga = func(context.Context, string, *url.URL) (time.Duration, error) {
		time.Sleep(50 * time.Millisecond)
		return 40 * time.Millisecond, nil
	}

	// Кадры собираются в горутинах, а разбираются здесь: t.Fatalf из чужой
	// горутины тест не останавливает.
	var gruppa sync.WaitGroup
	kadry := make([]protokol.Kadr, 2)
	for i := range kadry {
		gruppa.Add(1)
		go func() {
			defer gruppa.Done()
			kadry[i] = s.Obrabotat(context.Background(), protokol.Kadr{Tip: "komanda", Id: uint64(i + 1), Imya: "measureDelays"})
		}()
	}
	gruppa.Wait()

	if bolshe != 1 {
		t.Fatalf("временных ядер разом %d, а конфиг у них один файл", bolshe)
	}
	for i, k := range kadry {
		if k.Oshib != nil {
			t.Fatalf("замер %d отвергнут: %+v", i, k.Oshib)
		}
		var o struct {
			Zamery []zamerZaderzhki `json:"zamery"`
		}
		if err := json.Unmarshal(k.Telo, &o); err != nil || len(o.Zamery) != 1 {
			t.Fatalf("замер %d не разбирается: %v, %s", i, err, k.Telo)
		}
		if z := o.Zamery[0]; z.PingMs == nil || *z.PingMs != 40 {
			t.Fatalf("замер %d не дошёл до числа: %+v", i, z)
		}
	}
}
