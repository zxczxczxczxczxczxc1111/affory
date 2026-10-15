package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// sluzhbaSMyortvymServerom восстанавливает туннель, который не встаёт никогда.
func sluzhbaSMyortvymServerom(t *testing.T) (*Sluzhba, *atomic.Int32) {
	t.Helper()
	s := podstavnaya(t, nil)
	var podnimali atomic.Int32
	s.podnyatTunnel = func(context.Context) (set.Adapter, error) {
		podnimali.Add(1)
		return set.Adapter{}, errors.New("тест: сервер лёг")
	}
	s.otstupy = []time.Duration{time.Millisecond}
	s.redkiyOtstup = time.Hour
	s.postavit(protokol.SostNeNeset, nil)
	return s, &podnimali
}

// С8 аудита 1.6.1. Сервер лёг, а сеть жива: после десятка неудач подряд
// стучаться к нему раз в полминуты незачем, пауза вырастает до пяти минут.
func TestMyortvyyServerPriZhivoySetiRedkiePopytki(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "десяток попыток", func() bool { return podnimali.Load() >= neudachDoRedkih })
	time.Sleep(100 * time.Millisecond)
	if n := podnimali.Load(); n != neudachDoRedkih {
		t.Fatalf("попыток %d при живой сети и мёртвом сервере, ждали %d и паузу", n, neudachDoRedkih)
	}
}

// dozhdatsyaDlinnoyPauzy ждёт, пока после десятка неудач цикл встанет на
// длинную паузу и окно получит срок следующей попытки.
func dozhdatsyaDlinnoyPauzy(t *testing.T, s *Sluzhba, podnimali *atomic.Int32) {
	t.Helper()
	dozhdatsya(t, "длинная пауза", func() bool {
		return podnimali.Load() >= neudachDoRedkih && s.Status().SledPopytka != nil
	})
}

// Жалоба 15.10.2026. Серверы молчат, восстановление ушло в пятиминутную
// паузу, человек выбрал другой сервер. Прежде выбор только запоминался:
// попытка с ним ждала конца паузы, а окно всё это время показывало «снова
// через 5 мин».
func TestVyborServeraVPauzeVosstanovleniyaProbuetSrazu(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}})
	s.zapustitVosstanovlenie()
	dozhdatsyaDlinnoyPauzy(t, s, podnimali)

	if err := s.setServer(context.Background(), vtoroyServer().Id); err != nil {
		t.Fatalf("выбор сервера в паузе отказал: %v", err)
	}
	if st := s.Status(); st.SledPopytka != nil {
		t.Fatalf("сервер выбран, а окно по-прежнему ждёт попытки в %v", st.SledPopytka)
	}
	dozhdatsya(t, "попытка сразу после выбора", func() bool { return podnimali.Load() > neudachDoRedkih })
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Vybran != vtoroyServer().Id {
		t.Fatalf("выбран %q, а человек выбрал %q", n.Vybran, vtoroyServer().Id)
	}
}

// «Подключить» в паузе восстановления отдаётся циклу, а не идёт рядом с ним.
// Рядом оно поднимало туннель под чужим отсчётом, а цикл, проснувшись посреди
// такого подъёма, прекращался: неудачная попытка руками оставляла машину без
// восстановления.
func TestPodklyuchitVPauzeVosstanovleniyaOtdayotsyaTsiklu(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.zapustitVosstanovlenie()
	dozhdatsyaDlinnoyPauzy(t, s, podnimali)

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("«Подключить» в паузе ответило отказом: %v", err)
	}
	if st := s.Status(); st.SledPopytka != nil {
		t.Fatalf("нажато «Подключить», а окно по-прежнему ждёт попытки в %v", st.SledPopytka)
	}
	dozhdatsya(t, "попытка цикла после нажатия", func() bool { return podnimali.Load() > neudachDoRedkih })
	// Цикл живёт дальше: попытка не удалась, и он снова на паузе, а не
	// прекращён. Подъём ровно один: второго рядом с циклом не было.
	dozhdatsya(t, "цикл снова на паузе", func() bool { return s.Status().SledPopytka != nil })
	if n := podnimali.Load(); n != neudachDoRedkih+1 {
		t.Fatalf("подъёмов %d после одного нажатия, ждали %d", n, neudachDoRedkih+1)
	}
	if !s.vosstanovlenieIdyot() {
		t.Fatal("восстановление прекратилось после нажатия «Подключить»")
	}
}

// Тот же случай через «Подключиться к X» из списка, то есть connect с
// сервером. Выбор не теряется по дороге к циклу: попытка идёт сразу и с ним.
func TestPodklyuchitKServeruVPauzeVosstanovleniyaBeryotEgo(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}})
	s.zapustitVosstanovlenie()
	dozhdatsyaDlinnoyPauzy(t, s, podnimali)

	otvet := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "connect", Telo: []byte(`{"server":"` + vtoroyServer().Id + `"}`)})
	if otvet.Oshib != nil {
		t.Fatalf("«Подключиться к серверу» в паузе отказало: %+v", otvet.Oshib)
	}
	dozhdatsya(t, "попытка сразу после нажатия", func() bool { return podnimali.Load() > neudachDoRedkih })
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Vybran != vtoroyServer().Id {
		t.Fatalf("выбран %q, а человек выбрал %q", n.Vybran, vtoroyServer().Id)
	}
}

// Сеть мертва: пауза остаётся короткой, иначе вернувшуюся сеть человек ждал
// бы пять минут.
func TestMyortvayaSetNeZamedlyaetVosstanovlenie(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.sprositVyhod = func(context.Context, string, int) (string, error) { return "", errors.New("тест: сети нет") }
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "попытки после десятка при мёртвой сети", func() bool {
		return podnimali.Load() >= neudachDoRedkih+5
	})
}
