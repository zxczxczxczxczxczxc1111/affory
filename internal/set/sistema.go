// Пакет set спрашивает у Windows то, что нельзя знать заранее: какой адаптер
// сейчас несёт трафик, какой у него шлюз и какой резолвер.
//
// Everything here is asked of the OS at tunnel-raise time and nothing is
// remembered between runs. A laptop moves between networks, a docked machine
// changes adapters mid-session, DHCP renews at 3am. Каждое из этих событий
// делает вчерашний ответ ложью, а ложь про адрес резолвера уезжает сразу в три
// места: в конфиг ядра, в правило петли и в список разрешённого.
package set

import (
	"errors"
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sostoyanieVverh = windows.IfOperStatusUp
	tipPetli        = windows.IF_TYPE_SOFTWARE_LOOPBACK
)

// ErrNetAdaptera отдаётся, когда ни один адаптер не годится в кандидаты.
var ErrNetAdaptera = errors.New("нет адаптера с маршрутом по умолчанию")

// ErrNetResolvera отдаётся отдельно от ErrNetAdaptera: адаптер может нести
// маршрут по умолчанию и при этом не объявлять ни одного DNS-сервера. Молча
// вернуть нулевой адрес значит положить "invalid Addr" в конфиг ядра, а ядро
// откажется стартовать с сообщением совсем про другое.
var ErrNetResolvera = errors.New("у активного адаптера нет резолвера")

// Adapter это ровно то, что нужно волне 2, и ничего сверх. Полный
// IpAdapterAddresses сюда не тащится намеренно: он неудобен в тестах и половина
// его полей никогда не будет прочитана.
type Adapter struct {
	Indeks     uint32
	Imya       string
	Opisanie   string
	Tip        uint32
	Sostoyanie uint32
	Metrika    uint32
	// Adresa это СВОИ адреса адаптера. Нужны правилу брандмауэра: у netsh нет
	// привязки к адаптеру по имени вовсе, и разрешить исходящий через туннель
	// можно только через localip с адресом этого туннеля.
	Adresa    []netip.Addr
	Shlyuzy   []netip.Addr
	Resolvery []netip.Addr
}

// Шов. Тесты подменяют перечисление и проверяют выбор, не имея сети вовсе.
var perechislit = perechislitSistemnye

// Adaptery отдаёт снимок адаптеров системы.
func Adaptery() ([]Adapter, error) { return perechislit() }

// vybrat выбирает адаптер, который сейчас несёт трафик наружу.
//
// Правило простое и намеренно тупое: адаптер должен быть поднят, не быть петлёй
// и объявлять хотя бы один шлюз, а среди таких берётся наименьшая метрика
// интерфейса. Это та же величина, что печатает Get-NetIPInterface, и та же, по
// которой Windows разводит два маршрута по умолчанию.
//
// Функция чистая: весь разговор с системой остался в perechislit. Поэтому её
// поведение проверяется списком структур, а не сетью, и правило можно менять,
// не поднимая ничего.
func vybrat(spisok []Adapter, krome ...uint32) (Adapter, error) {
	propustit := make(map[uint32]bool, len(krome))
	for _, i := range krome {
		propustit[i] = true
	}

	var luchshiy Adapter
	nashli := false
	for _, a := range spisok {
		switch {
		case propustit[a.Indeks], a.Sostoyanie != sostoyanieVverh,
			a.Tip == tipPetli, len(a.Shlyuzy) == 0:
			continue
		}
		// Равные метрики разводятся индексом, иначе порядок перечисления
		// системой становится частью поведения, а он не обещан никем.
		if !nashli || a.Metrika < luchshiy.Metrika ||
			(a.Metrika == luchshiy.Metrika && a.Indeks < luchshiy.Indeks) {
			luchshiy, nashli = a, true
		}
	}
	if !nashli {
		return Adapter{}, ErrNetAdaptera
	}
	return luchshiy, nil
}

// AktivnyyAdapter отдаёт имя адаптера, несущего трафик наружу ПРЯМО СЕЙЧАС.
//
// Оговорка, которую надо знать: при поднятом туннеле это будет туннель, и это
// правильный ответ на заданный вопрос. Когда нужен физический канал под
// туннелем, зовётся вариант Krome с индексом TUN-адаптера, который волна 2.3
// узнаёт при его подъёме.
func AktivnyyAdapter() (string, error) { return AktivnyyAdapterKrome() }

func AktivnyyAdapterKrome(krome ...uint32) (string, error) {
	spisok, err := perechislit()
	if err != nil {
		return "", err
	}
	a, err := vybrat(spisok, krome...)
	if err != nil {
		return "", err
	}
	return a.Imya, nil
}

func Shlyuz() (netip.Addr, error) { return ShlyuzKrome() }

func ShlyuzKrome(krome ...uint32) (netip.Addr, error) {
	a, err := aktivnyy(krome...)
	if err != nil {
		return netip.Addr{}, err
	}
	return a.Shlyuzy[0], nil
}

func LokalnyyResolver() (netip.Addr, error) { return LokalnyyResolverKrome() }

func LokalnyyResolverKrome(krome ...uint32) (netip.Addr, error) {
	a, err := aktivnyy(krome...)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(a.Resolvery) == 0 {
		return netip.Addr{}, fmt.Errorf("%w: %s", ErrNetResolvera, a.Imya)
	}
	return a.Resolvery[0], nil
}

func aktivnyy(krome ...uint32) (Adapter, error) {
	spisok, err := perechislit()
	if err != nil {
		return Adapter{}, err
	}
	return vybrat(spisok, krome...)
}

// perechislitSistemnye спрашивает GetAdaptersAddresses, а не реестр: в реестре
// лежит то, что настроено, а в API то, что действует. Разница видна ровно
// тогда, когда она дорога: DHCP выдал другой резолвер, а запись осталась.
func perechislitSistemnye() ([]Adapter, error) {
	// Дружелюбное имя НЕ пропускается: именно оно потом уезжает в netsh и в
	// сообщения человеку, а Description отличается и годится только для опознания.
	const flagi = windows.GAA_FLAG_SKIP_ANYCAST |
		windows.GAA_FLAG_SKIP_MULTICAST |
		windows.GAA_FLAG_INCLUDE_GATEWAYS

	// The size is asked for, not guessed, and the loop exists because adapters
	// can appear between the two calls. Three tries is not superstition: it is
	// one for the answer, one for the race, one for the second race.
	razmer := uint32(15 * 1024)
	var syroy []byte
	for popytka := 0; popytka < 3; popytka++ {
		syroy = make([]byte, razmer)
		err := windows.GetAdaptersAddresses(windows.AF_INET,
			flagi, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&syroy[0])), &razmer)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil, fmt.Errorf("GetAdaptersAddresses: %w", err)
		}
		if popytka == 2 {
			return nil, fmt.Errorf("GetAdaptersAddresses: буфер растёт быстрее, чем мы его просим")
		}
	}

	var itog []Adapter
	for p := (*windows.IpAdapterAddresses)(unsafe.Pointer(&syroy[0])); p != nil; p = p.Next {
		a := Adapter{
			Indeks:     p.IfIndex,
			Imya:       windows.UTF16PtrToString(p.FriendlyName),
			Opisanie:   windows.UTF16PtrToString(p.Description),
			Tip:        p.IfType,
			Sostoyanie: p.OperStatus,
			Metrika:    p.Ipv4Metric,
		}
		for u := p.FirstUnicastAddress; u != nil; u = u.Next {
			if adr, ok := netip.AddrFromSlice(u.Address.IP()); ok {
				a.Adresa = append(a.Adresa, adr.Unmap())
			}
		}
		for g := p.FirstGatewayAddress; g != nil; g = g.Next {
			if adr, ok := netip.AddrFromSlice(g.Address.IP()); ok {
				a.Shlyuzy = append(a.Shlyuzy, adr.Unmap())
			}
		}
		for d := p.FirstDnsServerAddress; d != nil; d = d.Next {
			adr, ok := netip.AddrFromSlice(d.Address.IP())
			if !ok {
				continue
			}
			a.Resolvery = append(a.Resolvery, adr.Unmap())
		}
		itog = append(itog, a)
	}
	return itog, nil
}
