package set

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// ErrImyaNeRazreshilos значит дыру в правиле петли, а не мелкую неудачу.
var ErrImyaNeRazreshilos = errors.New("имя не разрешилось")

// Сколько ждать системный резолвер. Без предела подъём туннеля висел бы на
// молчащем DNS, а «висит» это единственный исход, под который в §9.1 нет экрана.
var TaymautRezolva = 5 * time.Second

// SobratAdresa это ЕДИНСТВЕННЫЙ источник адресов для двух списков сразу:
// правила петли в конфиге sing-box и разрешающих правил брандмауэра.
//
// Два независимых сборщика неизбежно разошлись бы, и разошлись бы тихо. Адрес,
// попавший в петлю, но не в брандмауэр, даёт неработающий туннель в запертом
// режиме. Адрес, попавший в брандмауэр, но не в петлю, даёт петлю: трафик к
// серверу уходит В туннель, который сам держится на этом трафике.
//
// Имена резолвятся ЗДЕСЬ и СИСТЕМНЫМ резолвером, то есть до подъёма TUN. Позже
// было бы поздно: резолв пошёл бы через туннель, которого ещё нет.
//
// zagruzki это адреса прочих загрузок мимо туннеля (наборы rule_set, §«Загрузки
// идут мимо туннеля»): их хосты входят в список брандмауэра наравне с подпиской.
func SobratAdresa(servery []protokol.Server, podpiska string, zagruzki ...string) (Adresa, error) {
	ctx, otmena := context.WithTimeout(context.Background(), TaymautRezolva)
	defer otmena()
	return sobratAdresaS(ctx, net.DefaultResolver.LookupNetIP, servery, podpiska, zagruzki...)
}

// Adresa это итог ОДНОГО прохода резолвера, общий для обоих списков.
//
// Списки с С2 аудита 1.6.1 разные. Правило петли в ядре берёт только серверы и
// только их порты: прежде оно пускало мимо туннеля всё, что шло на адрес
// подписки и raw.githubusercontent.com, а это общие адреса хостинга и GitHub
// Pages. Службе и загрузкам ядра правило петли не нужно: службу ведёт мимо
// туннеля правило процессов, загрузку наборов свой клиент с detour direct.
// Брандмауэр по-прежнему берёт всё, но только на этих портах.
type Adresa struct {
	// Vse это адреса серверов, подписок и загрузок: правило брандмауэра.
	Vse []netip.Addr
	// Porty это порты того же списка в записи netsh: «443», «20000-30000».
	Porty []string
	// Servery это адреса, которые дал хост каждого сервера, литерал тоже.
	// Неразрешившегося имени здесь нет.
	Servery map[string][]netip.Addr
}

type rezolver func(ctx context.Context, set, host string) ([]netip.Addr, error)

func sobratAdresaS(ctx context.Context, r rezolver, servery []protokol.Server, podpiska string, zagruzki ...string) (Adresa, error) {
	hosty := make([]string, 0, len(servery)+1+len(zagruzki))
	hostServera := map[string]bool{}
	porty := map[string]bool{}
	for _, s := range servery {
		if s.Host != "" {
			hosty = append(hosty, s.Host)
			hostServera[s.Host] = true
		}
		for _, p := range portyServera(s) {
			porty[p] = true
		}
	}
	// Адрес подписки в списке ОБЯЗАТЕЛЬНО. Без него запертый режим отрезает
	// обновление подписки ровно тогда, когда список серверов протух и обновить
	// его нужнее всего.
	for _, u := range append([]string{podpiska}, zagruzki...) {
		if u == "" {
			continue
		}
		h, p, err := hostIPortIzURL(u)
		if err != nil {
			return Adresa{}, err
		}
		if h != "" {
			hosty = append(hosty, h)
			porty[p] = true
		}
	}

	a := Adresa{Servery: map[string][]netip.Addr{}}
	vidno := map[netip.Addr]bool{}
	sprosheno := map[string]bool{}
	var nerazreshilis []string

	for _, h := range hosty {
		if sprosheno[h] {
			continue
		}
		sprosheno[h] = true
		adresa, err := razreshit(ctx, r, h)
		if err != nil {
			// Имя, которое не разрешилось, НЕ проглатывается. Пропустить его
			// молча значит оставить дыру в правиле петли: сервер потом
			// зарезолвится по TTL уже внутри туннеля, и трафик к нему пойдёт в
			// туннель, который на нём же и держится.
			nerazreshilis = append(nerazreshilis, h)
			continue
		}
		if hostServera[h] {
			a.Servery[h] = adresa
		}
		for _, ad := range adresa {
			if !vidno[ad] {
				vidno[ad] = true
				a.Vse = append(a.Vse, ad)
			}
		}
	}

	// Порядок устойчивый. Резолвер возвращает адреса в переменном порядке, и без
	// сортировки конфиг переписывался бы на каждом подъёме, а ядро
	// перезапускалось бы там, где ничего не изменилось.
	sort.Slice(a.Vse, func(i, j int) bool { return a.Vse[i].Less(a.Vse[j]) })
	a.Porty = slices.Sorted(maps.Keys(porty))

	if len(nerazreshilis) > 0 {
		// Список отдаётся ВМЕСТЕ с ошибкой: то, что разрешилось, вызывающему
		// пригодится, а решать, поднимать ли туннель с дырой, не нам.
		//
		// Отдельным типом, а не текстом: вызывающему нужны ИМЕНА, чтобы назвать
		// человеку исключённые серверы. Выдирать их обратно из строки значило бы
		// разбирать собственное сообщение об ошибке.
		return a, &OshibkaRazresheniya{Imena: nerazreshilis}
	}
	if len(a.Vse) == 0 {
		return Adresa{}, fmt.Errorf("%w: собирать нечего", ErrImyaNeRazreshilos)
	}
	return a, nil
}

// razreshit отдаёт адреса хоста без дубликатов и в устойчивом порядке.
//
// Литерал не резолвим: резолвер на IP отвечает по-разному в зависимости от
// настроек системы, а нам тут гадать не о чем.
func razreshit(ctx context.Context, r rezolver, h string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(h); err == nil {
		return []netip.Addr{a.Unmap()}, nil
	}
	adresa, err := r(ctx, "ip", h)
	if err != nil {
		return nil, err
	}
	if len(adresa) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrImyaNeRazreshilos, h)
	}
	for i := range adresa {
		adresa[i] = adresa[i].Unmap()
	}
	slices.SortFunc(adresa, netip.Addr.Compare)
	return slices.Compact(adresa), nil
}

// hostIPortIzURL достаёт имя и порт из адреса подписки или загрузки.
func hostIPortIzURL(s string) (string, string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", "", fmt.Errorf("адрес подписки не разобран: %w", err)
	}
	// Имя, а не Host: у второго может быть порт, и резолвер на «example.org:443»
	// ответит отказом, который выглядел бы как недоступная подписка.
	port := u.Port()
	if port == "" {
		port = "443"
		if strings.EqualFold(u.Scheme, "http") {
			port = "80"
		}
	}
	if _, godno := portNetsh(port); !godno {
		return "", "", fmt.Errorf("в адресе %s негодный порт %q", u.Hostname(), port)
	}
	return u.Hostname(), port, nil
}

// portyServera даёт порты сервера в записи netsh. Порты hy2 приезжают из ссылки
// («20000-30000,443»), и негодная часть отбрасывается: netsh отверг бы правило
// целиком, и с ним весь режим «весь трафик».
func portyServera(s protokol.Server) []string {
	if s.Porty == "" {
		if p, godno := portNetsh(strconv.Itoa(s.Port)); godno {
			return []string{p}
		}
		return nil
	}
	var itog []string
	for _, ch := range strings.Split(s.Porty, ",") {
		if p, godno := portNetsh(strings.TrimSpace(ch)); godno {
			itog = append(itog, p)
		}
	}
	return itog
}

// portNetsh проверяет порт или диапазон «от-до» в пределах 1-65535.
func portNetsh(s string) (string, bool) {
	ot, do, diapazon := strings.Cut(s, "-")
	a, err := strconv.Atoi(ot)
	if err != nil || a < 1 || a > 65535 {
		return "", false
	}
	if !diapazon {
		return strconv.Itoa(a), true
	}
	b, err := strconv.Atoi(do)
	if err != nil || b < a || b > 65535 {
		return "", false
	}
	return fmt.Sprintf("%d-%d", a, b), true
}

// KomandyRazresheniya открывает список команд для проверки состава.
//
// Экспортируется ради теста, и это осознанно: состав двух списков обязан
// сверяться со ВХОДОМ, а не один список с другим. Сравнение двух обёрток над
// одним сборщиком истинно всегда, включая случаи «список пуст» и «кандидат
// потерян по дороге».
func KomandyRazresheniya(r Razreshyonnoe) [][]string { return pravilaRazresheniya(r) }

// OshibkaRazresheniya несёт ИМЕНА, которые не разрешились.
//
// Заведена после живого прогона на стенде 01.09.2026: в подписке из одиннадцати
// узлов один перестал резолвиться, и включение режима «весь трафик» отказало
// целиком. Человек видел отказ и не понимал, при чём тут сервер, которым он не
// пользуется. Решено: исключать с уведомлением, а для уведомления
// нужны имена.
type OshibkaRazresheniya struct {
	Imena []string
}

func (o *OshibkaRazresheniya) Error() string {
	return ErrImyaNeRazreshilos.Error() + ": " + strings.Join(o.Imena, ", ")
}

// Is держит errors.Is(err, ErrImyaNeRazreshilos) рабочим: вызывающие, которым
// имена не нужны, не должны знать про новый тип.
func (o *OshibkaRazresheniya) Is(cel error) bool { return cel == ErrImyaNeRazreshilos }
