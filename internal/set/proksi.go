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

// Чтения «своего» прокси здесь НЕТ, и это не упущение. Единственный, кто им
// пользовался, была служба под LocalSystem, то есть читала куст S-1-5-18 и не
// видела ничего. Функция, которую после починки не зовёт никто, это выключенный
// механизм, и сторож мёртвого кода ловит такие первым же прогоном.

// ProksiCheloveka это настройка прокси одного вошедшего человека.
type ProksiCheloveka struct {
	Sid string
	Proksi
}

// ProksiLyudey читает прокси у ВОШЕДШИХ ЛЮДЕЙ, а не у самой службы.
//
// Заведено 21.09.2026, потому что сторож перехвата не работал никогда. Служба
// живёт под LocalSystem, и HKEY_CURRENT_USER для неё это куст S-1-5-18: там
// прокси не бывает по построению, а значит `foreign-proxy-hijack` не срабатывал
// ни разу, хотя защиту от v2rayN, nekoray и корпоративного PAC продукт обещал.
//
// Кусты берутся из HKEY_USERS: там загружены профили всех, кто сейчас в
// системе. Отбираются только настоящие люди (S-1-5-21-… и S-1-12-1-… у учётных
// записей Entra ID); служебные кусты и ветки *_Classes пропускаются. Человек,
// который не вошёл, прокси и не поднимет, поэтому отсутствие куста это не
// потеря.
func ProksiLyudey() ([]ProksiCheloveka, error) {
	kusty, err := registry.OpenKey(registry.USERS, "", registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("HKEY_USERS не открылся: %w", err)
	}
	defer kusty.Close()
	imena, err := kusty.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("кусты пользователей не перечислены: %w", err)
	}

	var itog []ProksiCheloveka
	var otkazy []error
	for _, sid := range imena {
		if !sidCheloveka(sid) {
			continue
		}
		p, err := proksiIzKusta(registry.USERS, sid+`\`+putProksi)
		if err != nil {
			// Куст может уйти между перечислением и чтением: человек вышел из
			// системы. Это не отказ всей проверки, поэтому копим и идём дальше.
			otkazy = append(otkazy, fmt.Errorf("%s: %w", sid, err))
			continue
		}
		itog = append(itog, ProksiCheloveka{Sid: sid, Proksi: p})
	}
	if len(itog) == 0 && len(otkazy) > 0 {
		return nil, errors.Join(otkazy...)
	}
	return itog, nil
}

// sidCheloveka отсеивает служебные кусты (S-1-5-18, -19, -20) и ветки классов.
func sidCheloveka(sid string) bool {
	if strings.HasSuffix(sid, "_Classes") {
		return false
	}
	return strings.HasPrefix(sid, "S-1-5-21-") || strings.HasPrefix(sid, "S-1-12-1-")
}

func proksiIzKusta(kust registry.Key, put string) (Proksi, error) {
	k, err := registry.OpenKey(kust, put, registry.QUERY_VALUE)
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
