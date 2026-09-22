package diagnostika

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const adresPodpiski = "https://panel.example/sub?token=s3cret-token-value"

func TestObezlichennyyAdresNeSoderzhitSekreta(t *testing.T) {
	otpechatok := Obezlichit(adresPodpiski)
	if otpechatok == "" {
		t.Fatal("отпечаток пуст")
	}
	if len(otpechatok) != 8 {
		t.Fatalf("длина отпечатка %d, ждали 8: %q", len(otpechatok), otpechatok)
	}
	// Ни адреса целиком, ни его узнаваемых кусков. Отпечаток заведён ровно для
	// того, чтобы журнал можно было отдать чужому человеку.
	for _, kusok := range []string{adresPodpiski, "panel.example", "s3cret-token-value", "token"} {
		if strings.Contains(otpechatok, kusok) {
			t.Fatalf("в отпечатке %q видно %q", otpechatok, kusok)
		}
	}
	if Obezlichit(adresPodpiski) != otpechatok {
		t.Fatal("два отпечатка одного адреса разошлись: строки одной подписки не свяжутся между собой")
	}
	if Obezlichit(adresPodpiski+"x") == otpechatok {
		t.Fatal("разные адреса дали один отпечаток")
	}
	if Obezlichit("") != "" {
		t.Fatalf("пустой адрес дал отпечаток %q, а пустое поле опускается", Obezlichit(""))
	}
}

func TestNomerOperatsiiRazlichaetPopytki(t *testing.T) {
	vidennye := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		n := NomerOperatsii()
		if n == "" || n == "neizvestno" {
			t.Fatalf("номер операции %q", n)
		}
		vidennye[n] = true
	}
	// Совпадения на 64 номерах из четырёх байт допустимы, но не десятками: если
	// номера перестанут различать попытки, строки журнала снова склеятся.
	if len(vidennye) < 60 {
		t.Fatalf("на 64 попытки различных номеров %d", len(vidennye))
	}
}

func TestStrokaOperatsiiNesyotVesKontekst(t *testing.T) {
	var b bytes.Buffer
	z := NovyyZhurnal(&b)
	z.seychas = func() time.Time { return time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC) }
	z.ZadatSborku("1.2.0+abc1234")
	if err := z.SobytieOperatsii(Operatsiya{
		Vid:        "podpiska",
		Pokolenie:  7,
		Dlitelnost: 1500 * time.Millisecond,
		Itog:       "subscription-timeout",
		Shag:       "srok",
		Istochnik:  Obezlichit(adresPodpiski),
	}); err != nil {
		t.Fatalf("запись операции: %v", err)
	}
	stroka := strings.TrimRight(b.String(), "\n")
	if strings.Contains(stroka, "panel.example") || strings.Contains(stroka, "s3cret-token-value") {
		t.Fatalf("адрес подписки доехал до журнала: %s", stroka)
	}
	var nazad Srez
	if err := json.Unmarshal([]byte(stroka), &nazad); err != nil {
		t.Fatalf("строка операции не разбирается обратно: %v", err)
	}
	// Все четыре вопроса разбора 22.09.2026 сразу: какая попытка, сколько
	// длилась, какое поколение, какая сборка. Проверяются вместе, потому что
	// поодиночке любое из полей бесполезно.
	if nazad.OpId == "" {
		t.Error("у строки операции нет номера: две попытки в журнале не различить")
	}
	if nazad.OpPokolenie != 7 {
		t.Errorf("поколение %d, ждали 7", nazad.OpPokolenie)
	}
	if nazad.OpDlitelnostMs != 1500 {
		t.Errorf("длительность %v мс, ждали 1500", nazad.OpDlitelnostMs)
	}
	if nazad.Sborka != "1.2.0+abc1234" {
		t.Errorf("сборка %q", nazad.Sborka)
	}
	if nazad.OpItog != "subscription-timeout" || nazad.OpShag != "srok" {
		t.Errorf("итог %q, шаг %q", nazad.OpItog, nazad.OpShag)
	}
	if nazad.OpIstochnik != Obezlichit(adresPodpiski) {
		t.Errorf("отпечаток источника %q", nazad.OpIstochnik)
	}
	if nazad.Sobytie != "операция podpiska" {
		t.Errorf("событие %q", nazad.Sobytie)
	}
}

func TestPosekundnyySrezNeNesyotPoleyOperatsii(t *testing.T) {
	var b bytes.Buffer
	z := NovyyZhurnal(&b)
	z.ZadatSborku("1.2.0+abc1234")
	if err := z.Pisat(Srez{Vremya: time.Now(), DeskriptorovSluzhby: 300}); err != nil {
		t.Fatalf("запись среза: %v", err)
	}
	// Журнал пишет строку в секунду сутками. Восемь пустых полей операции в
	// каждой строке это треть объёма файла ни за чем.
	stroka := b.String()
	for _, pole := range []string{"op_id", "op_pokolenie", "op_dlitelnost_ms", "op_itog", "op_shag", "op_istochnik", "op_uzel", "sborka"} {
		if strings.Contains(stroka, pole) {
			t.Errorf("в посекундном срезе есть поле операции %q: %s", pole, stroka)
		}
	}
}

func TestSborkaNazyvaetReviziyuIPravlenoeDerevo(t *testing.T) {
	// Три случая на чистой функции: под `go test` ревизии в build info нет
	// вовсе, и проверить их на настоящем чтении нечем. Что ревизия доезжает до
	// собранного бинаря, проверено отдельно: `go version -m affory-svc.exe`.
	if got := sobratSborku("1.2.0", "", false); got != "1.2.0" {
		t.Errorf("сборка без ревизии %q, ждали одну версию", got)
	}
	if got := sobratSborku("1.2.0", "42bc514", false); got != "1.2.0+42bc514" {
		t.Errorf("сборка из чистого дерева %q", got)
	}
	// Ревизия правленого дерева называет не тот код, что работает.
	if got := sobratSborku("1.2.0", "42bc514", true); got != "1.2.0+42bc514-izmeneno" {
		t.Errorf("сборка из правленого дерева %q", got)
	}
}

func TestSborkaNesyotVersiyu(t *testing.T) {
	s := Sborka("1.2.0")
	if !strings.HasPrefix(s, "1.2.0") {
		t.Fatalf("сборка %q не начинается с версии", s)
	}
	// Ответ фиксируется первым вызовом намеренно: служба задаёт сборку один раз
	// при открытии журнала, и менять её на ходу нечему.
	if drugaya := Sborka("9.9.9"); drugaya != s {
		t.Fatalf("второй вызов дал %q вместо %q", drugaya, s)
	}
}

func TestVyklyuchennyyZhurnalMolchitNaOperatsii(t *testing.T) {
	// Нулевой указатель это выключенный журнал: звать его будут с каждой
	// операции, и падать на этом нельзя.
	var z *Zhurnal
	z.ZadatSborku("1.2.0")
	if err := z.SobytieOperatsii(Operatsiya{Vid: "podklyuchenie", Itog: "ok"}); err != nil {
		t.Fatalf("выключенный журнал ответил ошибкой: %v", err)
	}
}
