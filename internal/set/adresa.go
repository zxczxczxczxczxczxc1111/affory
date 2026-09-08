package set

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
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
// идут мимо туннеля»): их хосты входят в оба списка наравне с подпиской.
func SobratAdresa(servery []protokol.Server, podpiska string, zagruzki ...string) ([]netip.Addr, error) {
	ctx, otmena := context.WithTimeout(context.Background(), TaymautRezolva)
	defer otmena()
	return sobratAdresaS(ctx, net.DefaultResolver.LookupNetIP, servery, podpiska, zagruzki...)
}

type rezolver func(ctx context.Context, set, host string) ([]netip.Addr, error)

func sobratAdresaS(ctx context.Context, r rezolver, servery []protokol.Server, podpiska string, zagruzki ...string) ([]netip.Addr, error) {
	hosty := make([]string, 0, len(servery)+1+len(zagruzki))
	for _, s := range servery {
		if s.Host != "" {
			hosty = append(hosty, s.Host)
		}
	}
	// Адрес подписки в списке ОБЯЗАТЕЛЬНО. Без него запертый режим отрезает
	// обновление подписки ровно тогда, когда список серверов протух и обновить
	// его нужнее всего.
	if podpiska != "" {
		h, err := hostIzURL(podpiska)
		if err != nil {
			return nil, err
		}
		if h != "" {
			hosty = append(hosty, h)
		}
	}
	for _, z := range zagruzki {
		h, err := hostIzURL(z)
		if err != nil {
			return nil, err
		}
		if h != "" {
			hosty = append(hosty, h)
		}
	}

	vidno := map[netip.Addr]bool{}
	var itog []netip.Addr
	var nerazreshilis []string

	for _, h := range hosty {
		// Литерал не резолвим: резолвер на IP отвечает по-разному в зависимости
		// от настроек системы, а нам тут гадать не о чем.
		if a, err := netip.ParseAddr(h); err == nil {
			if !vidno[a.Unmap()] {
				vidno[a.Unmap()] = true
				itog = append(itog, a.Unmap())
			}
			continue
		}
		adresa, err := r(ctx, "ip", h)
		if err != nil || len(adresa) == 0 {
			// Имя, которое не разрешилось, НЕ проглатывается. Пропустить его
			// молча значит оставить дыру в правиле петли: сервер потом
			// зарезолвится по TTL уже внутри туннеля, и трафик к нему пойдёт в
			// туннель, который на нём же и держится.
			nerazreshilis = append(nerazreshilis, h)
			continue
		}
		for _, a := range adresa {
			a = a.Unmap()
			if !vidno[a] {
				vidno[a] = true
				itog = append(itog, a)
			}
		}
	}

	// Порядок устойчивый. Резолвер возвращает адреса в переменном порядке, и без
	// сортировки конфиг переписывался бы на каждом подъёме, а ядро
	// перезапускалось бы там, где ничего не изменилось.
	sort.Slice(itog, func(i, j int) bool { return itog[i].Less(itog[j]) })

	if len(nerazreshilis) > 0 {
		// Список отдаётся ВМЕСТЕ с ошибкой: то, что разрешилось, вызывающему
		// пригодится, а решать, поднимать ли туннель с дырой, не нам.
		//
		// Отдельным типом, а не текстом: вызывающему нужны ИМЕНА, чтобы назвать
		// человеку исключённые серверы. Выдирать их обратно из строки значило бы
		// разбирать собственное сообщение об ошибке.
		return itog, &OshibkaRazresheniya{Imena: nerazreshilis}
	}
	if len(itog) == 0 {
		return nil, fmt.Errorf("%w: собирать нечего", ErrImyaNeRazreshilos)
	}
	return itog, nil
}

// hostIzURL достаёт имя из адреса подписки.
func hostIzURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("адрес подписки не разобран: %w", err)
	}
	// Имя, а не Host: у второго может быть порт, и резолвер на «example.org:443»
	// ответит отказом, который выглядел бы как недоступная подписка.
	return u.Hostname(), nil
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
// пользуется. Решение владельца: исключать с уведомлением, а для уведомления
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
