package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// podmenitDispetcher replaces the dispatch seam for one test and puts the real
// one back. Restoring is not politeness: the variable is shared by the whole
// package, and a test that leaves its own handler behind poisons every test
// that runs after it.
func podmenitDispetcher(t *testing.T, zamena func(*Sluzhba, context.Context, protokol.Kadr) protokol.Kadr) {
	t.Helper()
	nastoyashchiy := otdatDispetcheru
	t.Cleanup(func() { otdatDispetcheru = nastoyashchiy })
	otdatDispetcheru = zamena
}

// obsluzhitKadr гоняет кадр ровно тем путём, которым его гоняет служба: своя
// горутина на команду, ответ через otpravit. Прямой вызов Obrabotat проверил бы
// не тот код: горутина это ровно то место, где паника валила процесс.
func obsluzhitKadr(t *testing.T, s *Sluzhba, k protokol.Kadr) protokol.Kadr {
	t.Helper()
	otvety := make(chan protokol.Kadr, 1)
	otpravit := func(o protokol.Kadr) error {
		otvety <- o
		return nil
	}
	go s.obsluzhitKomandu(context.Background(), k, otpravit)
	select {
	case o := <-otvety:
		return o
	case <-time.After(10 * time.Second):
		t.Fatalf("на команду %q ответа не пришло", k.Imya)
		return protokol.Kadr{}
	}
}

// A panic in one command handler used to take the whole service down, tunnel
// included. The comment on dispetcher.go:34 promised it could not happen and
// nothing enforced that promise.
func TestPanikaVKomandeNeRonyaetSluzhbu(t *testing.T) {
	s := podstavnaya(t, nil)
	nastoyashchiy := otdatDispetcheru
	podmenitDispetcher(t, func(s *Sluzhba, ctx context.Context, k protokol.Kadr) protokol.Kadr {
		// Паникует ОДНА команда, а не все: иначе проверить, что служба жива,
		// было бы нечем, и тест доказывал бы только половину.
		if k.Imya == "status" {
			panic("тест: обработчик команды status")
		}
		return nastoyashchiy(s, ctx, k)
	})

	otvet := obsluzhitKadr(t, s, protokol.Kadr{Id: 1, Imya: "status"})

	if otvet.Oshib == nil {
		t.Fatal("паника обязана превратиться в отказ, а не в тишину")
	}
	// Служба жива: следующая команда отвечает нормально.
	if vtoroy := obsluzhitKadr(t, s, protokol.Kadr{Id: 2, Imya: "listServers"}); vtoroy.Oshib != nil {
		t.Errorf("после паники служба не обслуживает: %v", vtoroy.Oshib)
	}
}

// Отказ обязан нести код из словаря §9.1 и номер того кадра, на который он
// отвечает: клиент сопоставляет ответы по Id, и ответ с чужим номером он не
// заметит вовсе, то есть будет ждать до срока ровно так же, как ждал бы молча.
func TestOtkazPosleParnikiAdresovanTomuZheKadru(t *testing.T) {
	s := podstavnaya(t, nil)
	podmenitDispetcher(t, func(*Sluzhba, context.Context, protokol.Kadr) protokol.Kadr {
		panic("тест: обработчик команды")
	})

	otvet := obsluzhitKadr(t, s, protokol.Kadr{Id: 77, Imya: "listServers"})

	if otvet.Oshib == nil {
		t.Fatal("паника обязана превратиться в отказ")
	}
	if otvet.Id != 77 || otvet.Imya != "listServers" {
		t.Errorf("отказ адресован кадру %d/%q вместо 77/listServers", otvet.Id, otvet.Imya)
	}
	if otvet.Oshib.Kod == "" {
		t.Error("отказ без кода: экрану нечего показать")
	}
	if otvet.Oshib.Tekst == "" {
		t.Error("отказ без текста: в журнале останется пустая строка")
	}
}

// Рассылка событий это ВТОРАЯ горутина соединения, и паника в ней валила
// процесс ровно так же. Ронять её умеет сама отправка: соединение закрылось
// посреди кадра, запись пошла в никуда.
func TestPanikaVRassylkeSobytiyNeRonyaetSluzhbu(t *testing.T) {
	sob := make(chan protokol.Kadr, 1)
	sob <- protokol.Kadr{Tip: "sobytie", Imya: "status"}
	close(sob)

	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		rassylat(sob, func(protokol.Kadr) error { panic("тест: отправка события") })
	}()

	select {
	case <-gotovo:
	case <-time.After(10 * time.Second):
		t.Fatal("рассылка не завершилась")
	}
}

// Третья горутина: чтение кадров соединения. Паника здесь приходит из чужого
// кода под нами, и поймать её больше негде.
func TestPanikaNaChteniiSoedineniyaNeRonyaetSluzhbu(t *testing.T) {
	s := podstavnaya(t, nil)

	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		s.obsluzhitOdnogo(context.Background(), &panikuyushcheeSoedinenie{})
	}()

	select {
	case <-gotovo:
	case <-time.After(10 * time.Second):
		t.Fatal("обслуживание соединения не завершилось")
	}
}

// panikuyushcheeSoedinenie рушится на первом же чтении: так ведёт себя не наш
// код, и держать guard против него дешевле, чем доказывать, что чужой код
// паниковать не умеет.
type panikuyushcheeSoedinenie struct{ net.Conn }

func (*panikuyushcheeSoedinenie) Read([]byte) (int, error) {
	panic("тест: чтение кадра соединения")
}

func (*panikuyushcheeSoedinenie) Write(b []byte) (int, error) { return len(b), nil }

func (*panikuyushcheeSoedinenie) Close() error { return nil }

// Ворота против пустого guard: recover, который не пишет в журнал, прячет
// падение навсегда. Проверяется факт записи и её состав, а не формат строки.
func TestPanikaPopadaetVZhurnal(t *testing.T) {
	s := podstavnaya(t, nil)
	podmenitDispetcher(t, func(*Sluzhba, context.Context, protokol.Kadr) protokol.Kadr {
		panic("тест: обработчик команды")
	})

	zapisano := perehvatitZhurnal(t)
	obsluzhitKadr(t, s, protokol.Kadr{Id: 5, Imya: "listServers"})
	stroki := zapisano()

	if stroki == "" {
		t.Fatal("паника не попала в журнал: молча проглоченная паника невидима")
	}
	// Имя команды, текст паники и стек: без любого из трёх разбирать нечего.
	for _, chto := range []string{"listServers", "тест: обработчик команды", "obsluzhitKomandu"} {
		if !strings.Contains(stroki, chto) {
			t.Errorf("в журнале нет %q, а без него разбирать нечего:\n%s", chto, stroki)
		}
	}
}

// perehvatitZhurnal уводит вывод log в память на время теста и отдаёт
// накопленное. Через log, а не через свой шов: журнал службы это log, и
// проверять надо то, что реально уедет в файл.
func perehvatitZhurnal(t *testing.T) func() string {
	t.Helper()
	b := &bufferPodZamkom{}
	prezhniy := log.Writer()
	t.Cleanup(func() { log.SetOutput(prezhniy) })
	log.SetOutput(b)
	return b.Stroka
}

// Под замком, потому что пишет чужая горутина, а читает тестовая.
type bufferPodZamkom struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (z *bufferPodZamkom) Write(p []byte) (int, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.b.Write(p)
}

func (z *bufferPodZamkom) Stroka() string {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.b.String()
}

var _ io.Writer = (*bufferPodZamkom)(nil)

// Паника это НЕ «половинки разных версий».
//
// Полоса Г завела recover, а отвечать ей было нечем: строки «служба сломалась
// внутри» в §9.1 не было, и она поставила protocol-mismatch, честно написав это
// в комментарии. Цена: экран на этот код говорит «служба и программа разных
// версий, обнови программу». Для паники это прямая ложь, и человек идёт
// переустанавливать исправную программу вместо того, чтобы повторить команду.
func TestPanikaOtvechaetVnutrenneyOshibkoy(t *testing.T) {
	s := podstavnaya(t, nil)
	podmenitDispetcher(t, func(*Sluzhba, context.Context, protokol.Kadr) protokol.Kadr {
		panic("тест: обработчик команды")
	})

	otvet := obsluzhitKadr(t, s, protokol.Kadr{Id: 5, Imya: "listServers"})

	if otvet.Oshib == nil {
		t.Fatal("паника обязана превратиться в отказ")
	}
	if kod := otvet.Oshib.Kod; kod != protokol.KodVnutrennyayaOshibka {
		t.Errorf("код отказа %q, ожидали internal-error", kod)
	}
}

// Обратная сторона: законные точки protocol-mismatch остаются на месте.
//
// Кодом отвечают полтора десятка мест, где он ЗАКОННЫЙ: тело команды не
// разбирается, первый кадр не hello, версия протокола не та. Расползись починка
// на них, и человек с настоящим рассогласованием версий получил бы совет
// «повторить», который не поможет никогда.
func TestNerazbiraemoeTeloPoPrezhnemuProtocolMismatch(t *testing.T) {
	s := podstavnaya(t, nil)
	otvet := obsluzhitKadr(t, s, protokol.Kadr{
		Tip: "cmd", Id: 6, Imya: "setServer", Telo: []byte("не json"),
	})
	if otvet.Oshib == nil {
		t.Fatal("неразбираемое тело принято за годную команду")
	}
	if kod := otvet.Oshib.Kod; kod != protokol.KodProtocolMismatch {
		t.Errorf("код отказа %q, ожидали protocol-mismatch", kod)
	}
}
