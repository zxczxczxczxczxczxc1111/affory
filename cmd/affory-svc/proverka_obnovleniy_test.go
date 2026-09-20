package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Проверка обновлений (план «шесть удобств» §5): последний выпуск на GitHub,
// сравнение по числам, загрузка архива с проверкой хеша из .sha256 рядом и
// установка тем же подменщиком.

const adresVypuska = "https://obn.example/latest"

// opisanieVypuska это ответ releases/latest с двумя нужными файлами и одним
// посторонним (установщиком), как в настоящем выпуске.
func opisanieVypuska(versiya string) []byte {
	return []byte(`{"tag_name":"v` + versiya + `","assets":[
		{"name":"Affory-` + versiya + `-setup.exe","size":9,"browser_download_url":"https://obn.example/d/setup.exe"},
		{"name":"affory-` + versiya + `.zip","size":123,"browser_download_url":"https://obn.example/d/affory-` + versiya + `.zip"},
		{"name":"affory-` + versiya + `.zip.sha256","size":80,"browser_download_url":"https://obn.example/d/affory-` + versiya + `.zip.sha256"}]}`)
}

// vypuskVSeti отвечает за три адреса выпуска; всё прочее это чужой адрес.
func vypuskVSeti(versiya, hesh string, arhiv []byte, sprosili *[]string) func(context.Context, string, int64, func(int64, int64)) ([]byte, error) {
	return func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
		if sprosili != nil {
			*sprosili = append(*sprosili, adres)
		}
		switch adres {
		case adresVypuska:
			return opisanieVypuska(versiya), nil
		case "https://obn.example/d/affory-" + versiya + ".zip.sha256":
			return []byte(hesh + "  affory-" + versiya + ".zip\n"), nil
		case "https://obn.example/d/affory-" + versiya + ".zip":
			// Настоящая загрузка докладывает долю по ходу чтения тела; без этого
			// подставная сеть тихо отменяла бы полосу в окне.
			if hod != nil {
				hod(int64(len(arhiv))/2, int64(len(arhiv)))
				hod(int64(len(arhiv)), int64(len(arhiv)))
			}
			return arhiv, nil
		}
		return nil, errors.New("чужой адрес " + adres)
	}
}

func TestNoveeSravnivaetPoChislam(t *testing.T) {
	sluchai := []struct {
		kandidat, tekushchaya string
		novee                 bool
	}{
		{"0.6.3", "0.6.2", true}, {"0.7.0", "0.6.9", true}, {"1.0.0", "0.9.9", true},
		{"0.6.2", "0.6.2", false}, {"0.6.1", "0.6.2", false}, {"0.10.0", "0.9.0", true},
		{"v0.6.3", "0.6.2", true}, {"dev", "0.6.2", false}, {"0.6.3", "dev", false}, {"0.6", "0.6.2", false},
	}
	for _, c := range sluchai {
		if got := novee(c.kandidat, c.tekushchaya); got != c.novee {
			t.Errorf("novee(%q, %q) = %v, ждали %v", c.kandidat, c.tekushchaya, got, c.novee)
		}
	}
}

func sVersiey(t *testing.T, versiya string) *Sluzhba {
	t.Helper()
	byla := versiyaProgrammy
	versiyaProgrammy = versiya
	t.Cleanup(func() { versiyaProgrammy = byla })
	s := podstavnaya(t, nil)
	s.adresObnovleniy = adresVypuska
	// Отступы между попытками сняты: проверяем ЧИСЛО заходов, а не секунды сна.
	// С настоящими паузами каждый тест отказа сети стоил бы три секунды петли.
	s.zhdat = func(context.Context, time.Duration) bool { return true }
	return s
}

// Повторы проверки обновления, 20.09.2026.
//
// Живая машина: `checkUpdate -> отказ update-check-failed`, следом через
// пятнадцать секунд `-> ok`, и между ними не менялось ничего. Запрос за
// описанием выпуска идёт мимо туннеля (конфиг ядра уводит affory-svc.exe в
// direct), то есть по каналу провайдера, и одиночная попытка на нём это
// лотерея. Повтором до этой правки служил человек, нажимавший кнопку ещё раз.

func TestProverkaPovtoryaetOtkazSeti(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	set := vypuskVSeti("0.6.3", hex.EncodeToString(bytes.Repeat([]byte{1}, 32)), nil, nil)
	zahodov := 0
	s.skachatFayl = func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
		if adres == adresVypuska {
			zahodov++
			if zahodov == 1 {
				return nil, errors.New("dial tcp 140.82.121.5:443: i/o timeout")
			}
		}
		return set(ctx, adres, predel, hod)
	}
	n, err := s.proveritObnovlenie(context.Background())
	if err != nil || n == nil || n.Versiya != "0.6.3" {
		t.Fatalf("одна осечка сети обязана лечиться повтором, а не человеком: %+v %v", n, err)
	}
	if zahodov != 2 {
		t.Fatalf("заходов за описанием %d, ждали 2", zahodov)
	}
}

func TestProverkaSdayotsyaPosleVsehPopytok(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	zahodov := 0
	s.skachatFayl = func(context.Context, string, int64, func(int64, int64)) ([]byte, error) {
		zahodov++
		return nil, errors.New("сети нет")
	}
	if _, err := s.proveritObnovlenie(context.Background()); err == nil {
		t.Fatal("мёртвая сеть прошла как успех")
	}
	if zahodov != popytokProverki {
		t.Fatalf("заходов %d, ждали %d: повторы либо не идут, либо не кончаются", zahodov, popytokProverki)
	}
}

// Негодный выпуск это НЕ отказ сети. Повторять его значит трижды получить один
// и тот же ответ и втрое дольше держать человека у окна.
func TestNegodnyyVypuskNePovtoryaetsya(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	zahodov := 0
	s.skachatFayl = func(context.Context, string, int64, func(int64, int64)) ([]byte, error) {
		zahodov++
		return []byte(`{"tag_name":"poslednyaya","assets":[]}`), nil
	}
	if _, err := s.proveritObnovlenie(context.Background()); err == nil {
		t.Fatal("тег не вида X.Y.Z прошёл как успех")
	}
	if zahodov != 1 {
		t.Fatalf("описание спрошено %d раза, ждали один: повтор тут только тянет время", zahodov)
	}
}

// Отмена сверху обрывает повторы: ждать уже некому.
func TestOtmenaObryvaetPovtory(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	zahodov := 0
	s.skachatFayl = func(context.Context, string, int64, func(int64, int64)) ([]byte, error) {
		zahodov++
		return nil, errors.New("сети нет")
	}
	s.zhdat = func(context.Context, time.Duration) bool { return false }
	if _, err := s.proveritObnovlenie(context.Background()); err == nil {
		t.Fatal("мёртвая сеть прошла как успех")
	}
	if zahodov != 1 {
		t.Fatalf("заходов %d, ждали один: отмена не оборвала повторы", zahodov)
	}
}

// Причина отказа обязана уехать в журнал службы.
//
// Журнал команд пишет один код, `update-check-failed`, и по нему видно, что
// человек жал кнопку трижды, но не видно, обо что он бился. Разбор жалобы
// 20.09.2026 уткнулся ровно в это.
func TestPrichinaOtkazaProverkiUezzhaetVZhurnal(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	zhurnal := zhurnalProgona(t)
	s.skachatFayl = func(context.Context, string, int64, func(int64, int64)) ([]byte, error) {
		return nil, errors.New("dial tcp 140.82.121.5:443: i/o timeout")
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodObnovlenieNeProvereno {
		t.Fatalf("checkUpdate без сети: %+v", o)
	}
	zapis := zhurnal.String()
	if !strings.Contains(zapis, "i/o timeout") {
		t.Fatalf("причина отказа не попала в журнал: %q", zapis)
	}
	if !strings.Contains(zapis, "попытка 1 из") || !strings.Contains(zapis, "попытка 3 из") {
		t.Fatalf("в журнале не видно хода попыток: %q", zapis)
	}
	// Итог нажатия отдельной строкой: по ней в журнале видно, что причина
	// пришла человеку, а не осталась внутренним сбоем расписания.
	if !strings.Contains(zapis, "по команде") {
		t.Fatalf("итог нажатия не отличим от захода расписания: %q", zapis)
	}
}

func TestProverkaNahoditNovuyuVersiyu(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	var sprosili []string
	s.skachatFayl = vypuskVSeti("0.6.3", hex.EncodeToString(bytes.Repeat([]byte{1}, 32)), nil, &sprosili)
	_, sob := s.Podpisatsya()
	n, err := s.proveritObnovlenie(context.Background())
	if err != nil || n == nil || n.Versiya != "0.6.3" || n.Razmer != 123 {
		t.Fatalf("находка %+v, %v", n, err)
	}
	// Описание и хеш, но НЕ архив: проверка не качает 20 МБ ради строки в статусе.
	if len(sprosili) != 2 || sprosili[0] != adresVypuska || !strings.HasSuffix(sprosili[1], ".zip.sha256") {
		t.Fatalf("спросили %v", sprosili)
	}
	st := s.Status()
	if st.Obnovlenie == nil || st.Obnovlenie.Versiya != "0.6.3" || st.ObnovlenieProvereno == nil || st.VersiyaProgrammy != "0.6.2" {
		t.Fatalf("статус без находки: %+v", st)
	}
	select {
	case k := <-sob:
		if k.Imya != "state" {
			t.Fatalf("событие %s, ждали state", k.Imya)
		}
	case <-time.After(time.Second):
		t.Fatal("новая версия не породила события состояния")
	}
	// Та же версия во второй раз: отметка обновляется, события нет.
	if _, err := s.proveritObnovlenie(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-sob:
		t.Fatalf("повтор той же версии породил событие %s", k.Imya)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestProverkaBezNovoyVersiiStavitOtmetku(t *testing.T) {
	s := sVersiey(t, "0.6.3")
	s.skachatFayl = vypuskVSeti("0.6.3", hex.EncodeToString(bytes.Repeat([]byte{1}, 32)), nil, nil)
	n, err := s.proveritObnovlenie(context.Background())
	if err != nil || n != nil {
		t.Fatalf("та же версия: %+v %v", n, err)
	}
	if st := s.Status(); st.Obnovlenie != nil || st.ObnovlenieProvereno == nil {
		t.Fatalf("статус %+v", st)
	}
}

func TestProverkaSetOtkazNeTrogaetNahodku(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	s.skachatFayl = func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
		return nil, errors.New("сети нет")
	}
	if _, err := s.proveritObnovlenie(context.Background()); err == nil {
		t.Fatal("отказ сети прошёл как успех")
	}
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "checkUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodObnovlenieNeProvereno {
		t.Fatalf("checkUpdate без сети: %+v", o)
	}
}

func TestDevNeProveryaet(t *testing.T) {
	s := sVersiey(t, "dev")
	s.skachatFayl = func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
		t.Fatal("сборка dev полезла в сеть")
		return nil, nil
	}
	if _, err := s.proveritObnovlenie(context.Background()); !errors.Is(err, errNetVersii) {
		t.Fatalf("dev: %v", err)
	}
}

func arhivVPamyati(t *testing.T) ([]byte, string) {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	z, _ := w.Create("affory-svc.exe")
	z.Write([]byte("n"))
	w.Close()
	h := sha256.Sum256(b.Bytes())
	return b.Bytes(), hex.EncodeToString(h[:])
}

func TestDownloadUpdateKachaetSveryaetIStavit(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	arhiv, hesh := arhivVPamyati(t)
	s.skachatFayl = vypuskVSeti("0.6.3", hesh, arhiv, nil)
	zapuskov := 0
	s.zapustitPodmenshchika = func(prog, novaya string) error { zapuskov++; return nil }
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "downloadUpdate"})
	if o.Oshib != nil {
		t.Fatalf("отказ: %+v", o.Oshib)
	}
	if zapuskov != 1 {
		t.Fatalf("подменщик запущен %d раз", zapuskov)
	}
	put := filepath.Join(s.dirDannyh, katalogObnovleniy, "affory-0.6.3.zip")
	if _, err := os.Stat(put); err != nil {
		t.Fatalf("архив не лёг в каталог данных: %v", err)
	}
	if _, err := os.Stat(put + ".sha256"); err != nil {
		t.Fatalf("рядом нет .sha256: %v", err)
	}
	var otv map[string]any
	if err := json.Unmarshal(o.Telo, &otv); err != nil || otv["zapushchena"] != true {
		t.Fatalf("ответ %s", o.Telo)
	}
}

func TestDownloadUpdateOtvergaetChuzhoyHesh(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	arhiv, _ := arhivVPamyati(t)
	s.skachatFayl = vypuskVSeti("0.6.3", hex.EncodeToString(bytes.Repeat([]byte{2}, 32)), arhiv, nil)
	zapuskov := 0
	s.zapustitPodmenshchika = func(prog, novaya string) error { zapuskov++; return nil }
	o := s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "downloadUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodObnovlenieNeSkachano || zapuskov != 0 {
		t.Fatalf("архив с чужим хешем: %+v, запусков %d", o, zapuskov)
	}
}

// Выпуск без архива (или с архивом по http) это отказ проверки, не «нет
// обновлений»: иначе описание, подменённое по дороге, увело бы загрузку на
// чужой узел.
func TestVypuskBezArhivaPoHttpsOtvergaetsya(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	s.skachatFayl = func(ctx context.Context, adres string, predel int64, hod func(bylo, vsego int64)) ([]byte, error) {
		return []byte(`{"tag_name":"v0.6.3","assets":[{"name":"affory-0.6.3.zip","size":1,"browser_download_url":"http://obn.example/d/affory-0.6.3.zip"},{"name":"affory-0.6.3.zip.sha256","size":1,"browser_download_url":"https://obn.example/d/affory-0.6.3.zip.sha256"}]}`), nil
	}
	if _, err := s.proveritObnovlenie(context.Background()); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("архив по http прошёл: %v", err)
	}
	if st := s.Status(); st.ObnovlenieProvereno != nil {
		t.Fatal("негодный выпуск поставил отметку проверки")
	}
}

func TestDownloadUpdateTrebuetAdmina(t *testing.T) {
	s := sVersiey(t, "0.6.2")
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "downloadUpdate"})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("без прав: %+v", o)
	}
}

// Ответ 404 значит не «сломалось», а «списка выпусков не видно»: репозиторий
// закрыт или выпусков нет вовсе. Голый «код ответа 404» человек читает как
// поломку программы и идёт чинить не то. Проверяется настоящая загрузка, а не
// шов: формулировка живёт именно в ней.
func TestZakrytyyRepozitoriyObyasnyaetsya(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := skachatPoSeti(context.Background(), srv.URL, 1<<20, nil)
	if err == nil {
		t.Fatal("404 прошёл как успех")
	}
	if !strings.Contains(err.Error(), "закрыт") {
		t.Fatalf("по тексту не понять, что смотреть некуда: %v", err)
	}
}
