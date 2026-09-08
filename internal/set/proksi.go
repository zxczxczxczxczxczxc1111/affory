package set

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Блок системного прокси в реестре.
//
// **Affory под TUN реестр НЕ пишет вовсе.** Это остаток от режима прокси,
// который в спеке отвергнут: туннель заворачивает трафик на уровне маршрутов, и
// приложениям знать про прокси незачем. Поэтому логика тут простая до
// неприличия: любой ВКЛЮЧЁННЫЙ системный прокси это чужой, и мы про него
// говорим вслух, а не молча снимаем.
//
// Молча снимать нельзя по той же причине, по которой в этой спеке казнён Sota:
// это чужая настройка. Человек мог поставить прокси сам, и корпоративная
// политика тоже ставит его сама. Наше дело назвать, а решать ему.
const putProksi = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type Proksi struct {
	Vklyuchen bool
	Adres     string
}

// SistemnyyProksi читает состояние прокси текущего пользователя.
//
// HKCU, а не HKLM: браузеры и большинство приложений смотрят именно сюда.
// Служба работает под SYSTEM, и её собственный HKCU это не то, что видит
// человек, поэтому вызывающему придётся читать куст пользователя отдельно.
// Волна 2 читает свой, и это честное ограничение, а не недосмотр.
func SistemnyyProksi() (Proksi, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, putProksi, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return Proksi{}, nil // ключа нет: прокси не настраивали никогда
		}
		return Proksi{}, fmt.Errorf("ветка прокси не открылась: %w", err)
	}
	defer k.Close()

	vkl, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return Proksi{}, fmt.Errorf("ProxyEnable не прочитан: %w", err)
	}
	adres, _, err := k.GetStringValue("ProxyServer")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return Proksi{}, fmt.Errorf("ProxyServer не прочитан: %w", err)
	}
	return Proksi{Vklyuchen: vkl == 1, Adres: adres}, nil
}

// Chuzhoy отвечает, есть ли ЧУЖОЙ перехват. nashPort это порт нашего входа
// mixed, ноль означает, что прокси мы не поднимали.
//
// Прежняя редакция считала чужим любой включённый прокси и объясняла это тем,
// что «своего у нас не бывает по построению». Это перестало быть правдой в тот
// день, когда рядом с TUN появился вход mixed: он заведён ровно затем, чтобы
// человек прописал его системным прокси, и старое правило начало обвинять в
// перехвате нас самих.
//
// Правило строгое: наш только тот прокси, у которого КАЖДАЯ точка смотрит на
// нашу петлю и наш порт. Windows умеет писать значение по схемам
// («http=…;https=…»), и если https уходит в сторону, трафик перехватывают, как
// бы честно ни выглядел http.
func (p Proksi) Chuzhoy(nashPort int) bool {
	if !p.Vklyuchen || p.Adres == "" {
		return false
	}
	if nashPort == 0 {
		return true
	}
	tochki := strings.Split(p.Adres, ";")
	for _, t := range tochki {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// «http=127.0.0.1:10809» это одна точка со схемой слева.
		if i := strings.IndexByte(t, '='); i >= 0 {
			t = t[i+1:]
		}
		if !nashaTochka(t, nashPort) {
			return true
		}
	}
	return false
}

func nashaTochka(adres string, nashPort int) bool {
	host, port, err := net.SplitHostPort(adres)
	if err != nil {
		return false
	}
	if port != strconv.Itoa(nashPort) {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.IsLoopback()
}
