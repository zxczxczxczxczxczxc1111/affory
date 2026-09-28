// Package reklama это список блокировки рекламы HaGeZi Multi в синтаксисе
// AdBlock (28.09.2026). Разбор СТРОГИЙ: всё, кроме заголовка, комментариев и
// «||домен^», это отказ всего списка. Урезанный или подменённый список,
// принятый молча, ослабил бы защиту без единого слова, а отказ оставляет
// прежний список работать и называет причину в окне.
package reklama

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type Uroven string

const (
	Bazovyy      Uroven = "light"
	rasshirennyy Uroven = "multi" // снаружи уровень приходит строкой
)

type opisanie struct {
	adres, metka string // metka ищется в строке "! Title:"
	minimum      int    // ~45% числа правил на 28.09.2026: 44 757 и 200 054
}

var urovni = map[Uroven]opisanie{
	Bazovyy:      {"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/light.txt", "Multi LIGHT", 20000},
	rasshirennyy: {"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/multi.txt", "Multi NORMAL", 100000},
}

var errNeizvestnyyUroven = errors.New("уровень списка неизвестен")

// Privesti: "" это Bazovyy и true; незнакомое это Bazovyy и false, решает
// вызывающий. Регистр не прощается: уровень пишет только сама программа.
func Privesti(s string) (Uroven, bool) {
	if s == "" {
		return Bazovyy, true
	}
	if _, est := urovni[Uroven(s)]; est {
		return Uroven(s), true
	}
	return Bazovyy, false
}

func (u Uroven) Adres() string { return urovni[u].adres }

// VseAdresa: адреса всех уровней, для своих хостов службы.
func VseAdresa() []string {
	a := make([]string, 0, len(urovni))
	for _, u := range []Uroven{Bazovyy, rasshirennyy} {
		a = append(a, urovni[u].adres)
	}
	return a
}

type Spisok struct {
	Tekst   []byte // только строки "||домен^\n"; вход для convert
	Pravil  int
	Versiya string     // "! Version:"
	Sobran  *time.Time // "! Last modified:"
}

// Жизненно важные домены. Правило, равное такому домену или его родителю
// (кроме зоны верхнего уровня), это отказ всего списка: такой список отрезал
// бы человеку поиск, почту, банк или обновления Windows. На 28.09.2026 ни
// одного совпадения в light, multi и pro.
var zhiznennoVazhnye = []string{
	"google.com", "www.google.com", "youtube.com", "www.youtube.com", "googlevideo.com", "i.ytimg.com",
	"yandex.ru", "ya.ru", "dzen.ru", "vk.com", "mail.ru", "ok.ru", "rutube.ru",
	"microsoft.com", "windowsupdate.com", "live.com", "www.msftconnecttest.com",
	"apple.com", "icloud.com", "github.com", "api.github.com", "githubusercontent.com", "raw.githubusercontent.com",
	"cloudflare.com", "cp.cloudflare.com", "www.gstatic.com", "api.ipify.org", "wikipedia.org",
	"telegram.org", "web.telegram.org", "whatsapp.com", "discord.com",
	"gosuslugi.ru", "sberbank.ru", "online.sberbank.ru", "tbank.ru", "ozon.ru", "wildberries.ru",
	"openai.com", "chatgpt.com", "anthropic.com", "claude.ai", "amazon.com", "spotify.com",
	"steamcommunity.com", "store.steampowered.com", "twitch.tv",
}

// zapretnye: домен правила -> жизненно важный домен, который он бы отрезал.
// Строится один раз, проверка строки O(1).
var zapretnye = func() map[string]string {
	m := map[string]string{}
	for _, d := range zhiznennoVazhnye {
		for r := d; strings.Contains(r, "."); r = r[strings.IndexByte(r, '.')+1:] {
			m[r] = d
		}
	}
	return m
}()

// Обязательные домены: российская и мировая реклама. Список, в котором их
// нет, неполный, как бы он ни выглядел.
var obyazatelnye = []string{
	"an.yandex.ru", "mc.yandex.ru", "ad.mail.ru", "top-fwz1.mail.ru",
	"googleads.g.doubleclick.net", "pagead2.googlesyndication.com",
}

func Razobrat(telo []byte, u Uroven) (Spisok, error) {
	o, est := urovni[u]
	if !est {
		return Spisok{}, fmt.Errorf("%w: %q", errNeizvestnyyUroven, u)
	}
	var (
		sp        Spisok
		tekst     bytes.Buffer
		zayavleno = -1
		titul     string
		zagolovok bool
		pravila   = map[string]bool{}
	)
	tekst.Grow(len(telo))
	for nomer, ostatok := 0, telo; len(ostatok) > 0; {
		nomer++
		var s []byte
		if i := bytes.IndexByte(ostatok, '\n'); i >= 0 {
			s, ostatok = ostatok[:i], ostatok[i+1:]
		} else {
			s, ostatok = ostatok, nil
		}
		stroka := strings.TrimSuffix(string(s), "\r")
		if stroka == "" {
			continue
		}
		if !zagolovok {
			if stroka != "[Adblock Plus]" {
				return Spisok{}, fmt.Errorf("строка %d: это не список AdBlock: %s", nomer, obrezat(stroka))
			}
			zagolovok = true
			continue
		}
		if stroka[0] == '!' {
			pole, znachenie, _ := strings.Cut(strings.TrimSpace(stroka[1:]), ":")
			znachenie = strings.TrimSpace(znachenie)
			switch pole {
			case "Title":
				titul = znachenie
			case "Version":
				sp.Versiya = znachenie
			case "Last modified":
				if t, err := time.Parse("2 Jan 2006 15:04 MST", znachenie); err == nil {
					t = t.UTC()
					sp.Sobran = &t
				}
			case "Number of entries":
				n, err := strconv.Atoi(znachenie)
				if err != nil || n < 0 {
					return Spisok{}, fmt.Errorf("строка %d: число правил не число: %s", nomer, obrezat(stroka))
				}
				zayavleno = n
			}
			continue
		}
		domen, ok := domenPravila(stroka)
		if !ok {
			return Spisok{}, fmt.Errorf("строка %d: такое правило список не допускает: %s", nomer, obrezat(stroka))
		}
		if vazhnyy, est := zapretnye[domen]; est {
			return Spisok{}, fmt.Errorf("строка %d: правило отрезало бы %s: %s", nomer, vazhnyy, obrezat(stroka))
		}
		pravila[domen] = true
		sp.Pravil++
		tekst.WriteString(stroka)
		tekst.WriteByte('\n')
	}
	if !zagolovok {
		return Spisok{}, errors.New("ответ пуст, списка в нём нет")
	}
	if !strings.Contains(titul, "HaGeZi") || !strings.Contains(titul, o.metka) {
		return Spisok{}, fmt.Errorf("это не список %s, заголовок %q", o.metka, obrezat(titul))
	}
	if zayavleno < 0 {
		return Spisok{}, errors.New("в списке нет числа правил, целостность не проверить")
	}
	if zayavleno != sp.Pravil {
		return Spisok{}, fmt.Errorf("в списке %d правил, а заявлено %d: ответ обрезан или подменён", sp.Pravil, zayavleno)
	}
	for _, d := range obyazatelnye {
		if !pokryt(pravila, d) {
			return Spisok{}, fmt.Errorf("в списке нет %s, список неполный", d)
		}
	}
	if sp.Pravil < o.minimum {
		return Spisok{}, fmt.Errorf("в списке всего %d правил, у %s их меньше %d не бывает", sp.Pravil, o.metka, o.minimum)
	}
	sp.Tekst = tekst.Bytes()
	return sp, nil
}

// domenPravila принимает только «||домен^». Разбор ручной, без regexp: 229
// тысяч строк за 25-160 мс.
func domenPravila(s string) (string, bool) {
	if !strings.HasPrefix(s, "||") || !strings.HasSuffix(s, "^") {
		return "", false
	}
	d := s[2 : len(s)-1]
	if len(d) == 0 || len(d) > 253 || !strings.Contains(d, ".") || net.ParseIP(d) != nil {
		return "", false
	}
	for _, metka := range strings.Split(d, ".") {
		if len(metka) == 0 || len(metka) > 63 {
			return "", false
		}
		for i := 0; i < len(metka); i++ {
			c := metka[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return "", false
			}
		}
	}
	return d, true
}

// pokryt: домен режется правилом на себя или на родителя.
func pokryt(pravila map[string]bool, d string) bool {
	for r := d; strings.Contains(r, "."); r = r[strings.IndexByte(r, '.')+1:] {
		if pravila[r] {
			return true
		}
	}
	return false
}

func obrezat(s string) string {
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

// Meta лежит рядом с набором в reklama.json и описывает ЛЕЖАЩИЙ файл.
type Meta struct {
	Uroven     Uroven     `json:"uroven"`
	Pravil     int        `json:"pravil"`
	Versiya    string     `json:"versiya,omitempty"`
	Sobran     *time.Time `json:"sobran,omitempty"`
	Sha256     string     `json:"sha256"`             // sha256 reklama.srs
	Affory     string     `json:"affory,omitempty"`   // версия программы, чьё ядро собрало файл
	Proveren   *time.Time `json:"proveren,omitempty"` // последний УДАЧНЫЙ заход в сеть
	Vstroennyy bool       `json:"vstroennyy,omitempty"`
	Padenie    *Padenie   `json:"padenie,omitempty"`
}

// Padenie: скачанный список того же уровня короче действующего больше чем
// на треть.
type Padenie struct {
	Pravil  int       `json:"pravil"`
	Vpervye time.Time `json:"vpervye"`
}

const Period = 24 * time.Hour

// Сколько ждать, прежде чем поверить, что список и правда стал короче.
const podtverzhdeniePadeniya = 20 * time.Hour

// SleduyushchiyZahod: нет меты, другой уровень, встроенный список, ни одного
// удачного захода или отметка из будущего - идти сейчас; иначе через сутки
// от удачи.
func SleduyushchiyZahod(m *Meta, u Uroven, seychas time.Time) time.Time {
	if m == nil || m.Uroven != u || m.Vstroennyy || m.Proveren == nil || m.Proveren.After(seychas) {
		return seychas
	}
	return m.Proveren.Add(Period)
}

// SverkaSPrezhnim решает, принять ли список, который короче действующего того
// же уровня больше чем на 30%. Выход из отказа без ручной кнопки: такое же
// падение (±5%), подтверждённое через 20 ч и больше, принимается. Возвращает,
// принять ли, что записать в мету и не раньше какого срока повторять.
//
// povtorPosle нужен расписанию: без него оно ходило бы за списком каждый час
// и скачало бы multi (4,6 МБ) около 20 раз до решения.
func SverkaSPrezhnim(novoe int, m *Meta, u Uroven, seychas time.Time) (prinyat bool, zapomnit *Padenie, povtorPosle time.Time) {
	if m == nil || m.Uroven != u || m.Vstroennyy || m.Pravil == 0 {
		return true, nil, time.Time{}
	}
	if novoe*10 >= m.Pravil*7 {
		return true, nil, time.Time{}
	}
	if p := m.Padenie; p != nil {
		raznitsa := novoe - p.Pravil
		if raznitsa < 0 {
			raznitsa = -raznitsa
		}
		if raznitsa*20 <= p.Pravil {
			if !seychas.Before(p.Vpervye.Add(podtverzhdeniePadeniya)) {
				return true, nil, time.Time{}
			}
			return false, p, p.Vpervye.Add(podtverzhdeniePadeniya)
		}
	}
	return false, &Padenie{Pravil: novoe, Vpervye: seychas}, seychas.Add(podtverzhdeniePadeniya)
}
