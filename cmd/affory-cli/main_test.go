package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func TestFlagiPoslePodkomandyDohodyat(t *testing.T) {
	// Разбор стандартным flag.Parse останавливается на первом позиционном
	// аргументе, поэтому «profile export --out файл» отдал бы --out в остаток.
	// Экспорт ушёл бы писать в пустое имя, а проверка напечатала бы зелёный,
	// ничего не выгрузив.
	slova, hvost := razdelit([]string{"profile", "export", "--out", "C:\\tmp\\p.affory"})
	if len(slova) != 2 || slova[0] != "profile" || slova[1] != "export" {
		t.Fatalf("слова команды разобраны как %v", slova)
	}
	if len(hvost) != 2 || hvost[0] != "--out" {
		t.Fatalf("флаги разобраны как %v", hvost)
	}
}

func TestSsylkaNeSchitaetsyaFlagom(t *testing.T) {
	// vless:// с дефиса не начинается, и граница обязана пройти после ссылки, а
	// не перед ней. Ссылка взята НАСТОЯЩЕЙ формы: в uuid и в параметрах дефисы
	// есть, и проверка на «содержит дефис» вместо «начинается с дефиса» съела бы
	// ровно её. Первая редакция теста брала ссылку без единого дефиса и такую
	// подмену пропускала.
	slova, hvost := razdelit([]string{"servers", "add",
		"vless://11111111-2222-3333-4444-555555555555@host:443?security=reality&fp=chrome&type=tcp#NL-1"})
	if len(slova) != 3 {
		t.Fatalf("ссылка не попала в слова команды: %v", slova)
	}
	if len(hvost) != 0 {
		t.Fatalf("хвост не пуст: %v", hvost)
	}
}

func TestTeloProfilyaNePechataetsya(t *testing.T) {
	// Первый же отладочный прогон экспорта положил бы весь профиль в консоль и
	// в лог сеанса. Профиль это все ключи машины разом.
	telo, _ := json.Marshal(map[string]string{"profil": "QUZGT1JZLVBSRkwx-секрет"})
	vyvod := dlyaPechati(protokol.Kadr{Imya: "exportProfile", Telo: telo})
	if strings.Contains(vyvod, "секрет") {
		t.Fatalf("тело профиля напечатано: %s", vyvod)
	}
	if vyvod != "<скрыто>" {
		t.Fatalf("вывод %q, ожидалось <скрыто>", vyvod)
	}
}

func TestObychnoeTeloPechataetsya(t *testing.T) {
	// Зеркальный случай. Без него «скрывать всё подряд» прошло бы за защиту, а
	// живые проверки волны 3 читают именно списки серверов.
	telo, _ := json.Marshal(map[string]any{"vybran": "nl"})
	vyvod := dlyaPechati(protokol.Kadr{Imya: "listServers", Telo: telo})
	if !strings.Contains(vyvod, "nl") {
		t.Fatalf("обычное тело скрыто: %s", vyvod)
	}
}

func TestAdresPodpiskiNePechataetsyaEhom(t *testing.T) {
	// setSubscription отвечает признаком, но команда в списке секретных, и
	// проверяется именно это: если завтра ответ начнёт возвращать сам адрес,
	// печать его не покажет.
	telo, _ := json.Marshal(map[string]string{"adres": "https://panel.example/sub/tokenchik"})
	if strings.Contains(dlyaPechati(protokol.Kadr{Imya: "setSubscription", Telo: telo}), "tokenchik") {
		t.Fatal("адрес подписки напечатан")
	}
}

func TestPravilaIzFaylaChitayutsyaKakEst(t *testing.T) {
	// The rules go through a FILE, not argv: a path is not a secret, and a
	// list of processes on the command line would be re-typed by nobody.
	f := filepath.Join(t.TempDir(), "pravila.json")
	// Two backslashes in JSON are one in the path; the shell that wrote the
	// first version of this line ate one of them, and the test failed on the
	// fixture, not on the code.
	if err := os.WriteFile(f, []byte(`{"protsessy":["C:\\a\\b.exe"],"domeny":["example.org"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := pravilaIzFayla(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Protsessy) != 1 || p.Protsessy[0] != `C:\a\b.exe` || len(p.Domeny) != 1 {
		t.Fatalf("прочитано %+v", p)
	}
	if _, err := pravilaIzFayla(filepath.Join(t.TempDir(), "net.json")); err == nil {
		t.Fatal("несуществующий файл прочитан")
	}
}

func TestPravilaSBOMChitayutsya(t *testing.T) {
	// PowerShell's Set-Content -Encoding UTF8 writes a BOM, and so does
	// Notepad. Found on the stand 03.09.2026: the file was written the most
	// ordinary Windows way and the CLI answered "invalid character 'ï'",
	// which reads as a broken product, not as a broken file.
	f := filepath.Join(t.TempDir(), "pravila-bom.json")
	telo := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"protsessy":["C:\\a\\b.exe"],"domeny":[]}`)...)
	if err := os.WriteFile(f, telo, 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := pravilaIzFayla(f)
	if err != nil {
		t.Fatalf("файл с BOM не прочитан: %v", err)
	}
	if len(p.Protsessy) != 1 {
		t.Fatalf("прочитано %+v", p)
	}
}

// Отказ службы называется ОДНИМ способом, откуда бы он ни пришёл.
//
// kanal.Zvat отдаёт отказ И кадром, И ошибкой вида «код: текст». Первый барьер
// печатал ошибку и выходил, не дав второму дойти до «отказ [код]: текст»: вид
// отказа зависел от того, какой барьер сработал раньше, а приёмка читает вид.
// Ветка с квадратными скобками при этом была недостижима и потому не
// существовала, хотя её объяснял целый абзац.
func TestOtkazSluzhbyNazyvaetsyaOdnimSposobom(t *testing.T) {
	o := protokol.Kadr{Oshib: &protokol.Oshibka{
		Kod: "selected-server-gone", Tekst: "серверов нет"}}
	// Ровно как в жизни: тот же отказ приезжает вторым возвращаемым значением.
	tekst, ploho := tekstOtkaza(o, errors.New("selected-server-gone: серверов нет"))
	if !ploho {
		t.Fatal("отказ назван успехом")
	}
	if tekst != "отказ [selected-server-gone]: серверов нет" {
		t.Errorf("отказ напечатан как %q, а назвать код экрану нечем", tekst)
	}
}

// Ошибка СВЯЗИ это не отказ службы: кода у неё нет, и выдумывать скобки не из
// чего. Печатается как есть.
func TestOshibkaSvyaziPechataetsyaKakEst(t *testing.T) {
	tekst, ploho := tekstOtkaza(protokol.Kadr{}, errors.New("команда connect не ушла: канал закрыт"))
	if !ploho {
		t.Fatal("ошибка связи названа успехом")
	}
	if tekst != "команда connect не ушла: канал закрыт" {
		t.Errorf("ошибка связи переписана в %q", tekst)
	}
}

// Успех не имеет права стать отказом: иначе выход 1 приезжает на исправной
// команде, и вся приёмка краснеет разом.
func TestUspehNeNazyvaetsyaOtkazom(t *testing.T) {
	if _, ploho := tekstOtkaza(protokol.Kadr{Imya: "status"}, nil); ploho {
		t.Error("успешный ответ назван отказом")
	}
}

func TestPravilaIzFaylaNesutVyklyuchatelRuSpiska(t *testing.T) {
	// Выключатель российского списка едет тем же файлом. Отсутствие поля это
	// «не трогали»: файл со списками, написанный до появления выключателя, не
	// должен возвращать ru-набор на место молча.
	kat := t.TempDir()
	bez := filepath.Join(kat, "bez-polya.json")
	if err := os.WriteFile(bez, []byte(`{"protsessy":[],"domeny":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := pravilaIzFayla(bez)
	if err != nil {
		t.Fatal(err)
	}
	if p.BezRuSpiska != nil {
		t.Fatalf("поля в файле нет, а CLI шлёт службе %v", *p.BezRuSpiska)
	}

	s := filepath.Join(kat, "s-polem.json")
	if err := os.WriteFile(s, []byte(`{"protsessy":[],"domeny":[],"bez_ru_spiska":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := pravilaIzFayla(s)
	if err != nil {
		t.Fatal(err)
	}
	if v.BezRuSpiska == nil || !*v.BezRuSpiska {
		t.Fatalf("выключатель из файла не прочитан: %+v", v)
	}
}
