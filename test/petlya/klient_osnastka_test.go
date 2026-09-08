package petlya

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Клиентская сторона петли.
//
// Исходящие берутся У ПРОДУКТА: ссылки разбирает наш ssylki, конфиг собирает
// наш genkonfig, вместе с селектором, группой авто и управляющим портом.
// Тестового здесь ровно три вещи, и каждая из них - замена того, что на рабочей
// машине трогать нельзя: вместо TUN вход mixed, вместо маршрутов пустой набор
// правил, вместо чужого сайта в urltest своя мишень.
type Klient struct {
	*Yadro
	AdresKlash  string
	SekretKlash string
}

// Vybor это то, из чего собирается клиент: список узлов, выбранный из них и
// режим.
type Vybor struct {
	Uzly []Uzel
	// Vybran это индекс в Uzly. Ноль означает первый, и это же умолчание.
	Vybran int
	// Rezhim пустой означает ручной, как и в самом генераторе.
	Rezhim protokol.Rezhim
	// Kesh это experimental.cache_file. Пустой означает «без кэша», и тогда
	// ядро забывает выбор между запусками.
	Kesh string
	// ProbaURL это адрес, который дёргает urltest. Пустой означает мишень,
	// поднятую тестом: продуктовый адрес ведёт в интернет, а тест, зависящий
	// от чужой сети, обвиняет продукт за чужие сбои.
	ProbaURL string
}

// PodnyatKlienta поднимает клиента к одному узлу.
func PodnyatKlienta(t *testing.T, u Uzel) *Yadro {
	t.Helper()
	return PodnyatVybor(t, Vybor{Uzly: []Uzel{u}}).Yadro
}

// PodnyatVybor поднимает клиента ко всему набору узлов сразу и повторяет
// продуктовый порядок подъёма.
//
// Порядок важнее, чем кажется. Ядро НЕ обязано слушаться поля default: при
// включённом cache_file оно восстанавливает прошлый выбор, и служба поэтому
// навязывает тег через управляющий порт сразу после старта (navyazatVybor в
// komandy_serverov.go). Петля, поднимавшая одно голое ядро, показывала на этом
// месте красное и обвиняла продукт в дефекте, которого у него нет.
func PodnyatVybor(t *testing.T, v Vybor) *Klient {
	t.Helper()
	konfig, klash := KonfigKlienta(t, v)
	k := &Klient{
		Yadro:       PodnyatYadro(t, konfig),
		AdresKlash:  klash.Adres,
		SekretKlash: klash.Sekret,
	}
	k.NavyazatVybor(t, tegVybrannogo(t, v))
	return k
}

// NavyazatVybor делает то же, что служба после старта ядра, и тем же
// продуктовым кодом.
func (k *Klient) NavyazatVybor(t *testing.T, teg string) {
	t.Helper()
	ctx, otmena := context.WithTimeout(context.Background(), 15*time.Second)
	defer otmena()
	if err := yadra.ZhdatKlash(ctx, k.AdresKlash, k.SekretKlash); err != nil {
		t.Fatalf("управляющий порт не поднялся: %v (журнал: %s)", err, k.Zhurnal())
	}
	if err := yadra.PostavitVybor(ctx, k.AdresKlash, k.SekretKlash, genkonfig.TegSelector, teg); err != nil {
		t.Fatalf("выбор не навязался: %v", err)
	}
}

// tegVybrannogo это тег того выхода, на который ставит службa: группа авто в
// автоматическом режиме, иначе выбранный сервер. Так же считает tegRezhima.
func tegVybrannogo(t *testing.T, v Vybor) string {
	t.Helper()
	if v.Rezhim == protokol.RezhimAvto {
		return genkonfig.TegAvto
	}
	return v.Uzly[v.Vybran].Teg(t)
}

// KlashPetli это координаты управляющего порта поднятого клиента.
type KlashPetli struct {
	Adres  string
	Sekret string
}

// KonfigKlienta нужен отдельно там, где конфиг перед подъёмом ПОРТЯТ.
func KonfigKlienta(t *testing.T, v Vybor) (map[string]any, KlashPetli) {
	t.Helper()
	if len(v.Uzly) == 0 {
		t.Fatal("клиенту не дали ни одного узла")
	}
	if v.Vybran < 0 || v.Vybran >= len(v.Uzly) {
		t.Fatalf("выбран узел %d, а их %d", v.Vybran, len(v.Uzly))
	}

	servery := make([]protokol.Server, 0, len(v.Uzly))
	for _, u := range v.Uzly {
		ssylka := u.Ssylka()
		s, err := ssylki.Razobrat(ssylka)
		if err != nil {
			t.Fatalf("наш разбор не понял свою же ссылку %q: %v", ssylka, err)
		}
		if u.Transport != "" && s.Transport != u.Transport {
			t.Fatalf("ссылка разобралась в транспорт %q, а узел поднят как %q",
				s.Transport, u.Transport)
		}
		servery = append(servery, s)
	}

	portKlash := SvobodnyyPort(t)
	klash := KlashPetli{
		Adres:  fmt.Sprintf("127.0.0.1:%d", portKlash),
		Sekret: "sekret-petli",
	}

	telo, err := genkonfig.SingBox(genkonfig.Vhod{
		Server:         servery[v.Vybran],
		Servery:        servery,
		Rezhim:         v.Rezhim,
		Kandidaty:      []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Resolver:       netip.MustParseAddr("127.0.0.1"),
		PutiProtsessov: []string{`C:\affory\affory-svc.exe`, `C:\affory\sing-box.exe`},
		FaylKesha:      v.Kesh,
		ClashApi:       genkonfig.ClashApi{Adres: "127.0.0.1", Port: portKlash, Sekret: klash.Sekret},
	})
	if err != nil {
		t.Fatalf("генератор не собрал конфиг: %v", err)
	}

	var sobrannoe struct {
		Ishodyashchie []map[string]any `json:"outbounds"`
		Opyt          map[string]any   `json:"experimental"`
	}
	if err := json.Unmarshal(telo, &sobrannoe); err != nil {
		t.Fatalf("конфиг продукта не разобрался: %v", err)
	}

	proba := v.ProbaURL
	if proba == "" {
		proba = NovayaMishen(t, "proba-urltest").Adres
	}
	for i, o := range sobrannoe.Ishodyashchie {
		dobavitKoren(t, o, korenUzla(v.Uzly, o["tag"]))
		podmenitProbu(o, proba)
		sobrannoe.Ishodyashchie[i] = o
	}

	vhodPort := SvobodnyyPort(t)
	return map[string]any{
		"inbounds": []any{map[string]any{
			"type": "mixed", "tag": genkonfig.TegProksiVhod,
			"listen": "127.0.0.1", "listen_port": vhodPort,
		}},
		"outbounds": kakLyuboy(sobrannoe.Ishodyashchie),
		// Правил нет намеренно: весь трафик в селектор. Мишени стоят на петле, и
		// правило ip_is_private из продуктового конфига увело бы их мимо
		// туннеля, то есть тест зеленел бы при полностью мёртвом транспорте.
		"route": map[string]any{"final": genkonfig.TegSelector},
		// Имена узла и мишени резолвятся здесь, а не в файле hosts машины:
		// править hosts рабочей машины ради теста недопустимо, и переживший
		// прогон мусор там ломал бы не тест, а человека.
		"dns": map[string]any{
			// Тег местного резолвера продуктовый: прямой исходящий, собранный
			// генератором, ссылается на него по имени, и ядро отвергает конфиг
			// целиком, если такого сервера нет («domain resolver not found»).
			// Резолвит его здесь тот же hosts: в сеть за именами петли ходить
			// незачем, а «local» повёл бы запрос к домашнему резолверу.
			"servers": []any{map[string]any{
				"type": "hosts", "tag": genkonfig.TegMestnyy,
				"predefined": map[string]any{imyaUzla: []string{"127.0.0.1"}},
			}},
		},
		"experimental": sobrannoe.Opyt,
	}, klash
}

// korenUzla ищет корень того узла, чей это исходящий. Чужой корень к делу не
// подходит: сертификаты у узлов разные, и один на всех означал бы, что
// проверка подлинности сошлась случайно.
func korenUzla(uzly []Uzel, teg any) string {
	stroka, _ := teg.(string)
	for _, u := range uzly {
		if u.CaPEM == "" {
			continue
		}
		if stroka == genkonfig.TegKandidata(idSsylki(u)) {
			return u.CaPEM
		}
	}
	return ""
}

func idSsylki(u Uzel) string {
	s, err := ssylki.Razobrat(u.Ssylka())
	if err != nil {
		return ""
	}
	return s.Id
}

// podmenitProbu уводит urltest на мишень петли.
//
// Продуктовый адрес это www.gstatic.com, то есть чужая сеть. Тест, который
// зависит от неё, обвиняет продукт за то, чего продукт не делал, и этот класс
// ошибок стоил разбора целого дня 06.09.2026.
func podmenitProbu(o map[string]any, proba string) {
	if o["type"] != "urltest" {
		return
	}
	o["url"] = proba
	// Интервал продуктовый (три минуты) означал бы, что первая же проба и
	// последняя: за прогон переизбрания не случится ни разу.
	o["interval"] = "10s"
}

// dobavitKoren кладёт корень петли в tls.certificate там, где его некому
// заменить.
//
// Это единственная правка выдачи генератора, и она добавляет доверие, а не
// снимает проверку: server_name и алгоритмы остаются те же, что собрал продукт.
// Свой корень в хранилище рабочей машины не заводится намеренно.
//
// Исходящий С ПИНОМ корня не получает вовсе, и не по нашему выбору: ядро
// отвечает «certificate_public_key_sha256 is conflict with certificate or
// certificate_path» и не поднимается (проверено 07.09.2026). Отсюда же следует
// факт про сам продукт: пин это САМОСТОЯТЕЛЬНАЯ проверка подлинности, а не
// добавка поверх цепочки, то есть сервер с самоподписанным сертификатом и
// объявленным пином рабочий.
func dobavitKoren(t *testing.T, o map[string]any, caPEM string) {
	t.Helper()
	if caPEM == "" {
		return
	}
	tls, est := o["tls"].(map[string]any)
	if !est {
		return
	}
	if _, spinom := tls["certificate_public_key_sha256"]; spinom {
		return
	}
	tls["certificate"] = []string{caPEM}
}

func kakLyuboy(spisok []map[string]any) []any {
	out := make([]any, 0, len(spisok))
	for _, o := range spisok {
		out = append(out, o)
	}
	return out
}

func urlEscape(s string) string { return url.QueryEscape(s) }
