package set

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/wfp"
)

// Запрет режима «весь трафик» стоит собственными фильтрами WFP, а не политикой
// брандмауэра Windows. Решено 18.10.2026 после опыта в госте: при выключенной
// службе брандмауэра (MpsSvc) политика Block не действует вовсе, машина,
// запертая по записи, на деле открыта, а выключить режим нельзя, потому что
// netsh отказывает на каждой команде. WFP от MpsSvc не зависит.
//
// Все объекты постоянные. Упавшая служба Affory запрет не снимает, а после
// перезагрузки служба базовой фильтрации (BFE) поднимает его сама, ещё до
// старта нашей службы. Снимает его только выключение режима, остановка службы
// или установщик.
//
// Слой один: исходящее соединение и первая отправка UDP, отдельно для IPv4 и
// IPv6. В своём подслое запрет без условий стоит с наименьшим весом, а
// разрешения выше него. Запрет любого подслоя бьёт разрешения остальных,
// поэтому чужие разрешения, включая встроенные правила Windows, его не снимут.
// Наши разрешения при этом мягкие: пропущенное нами дальше решают остальные
// подслои, и запреты самого брандмауэра продолжают работать.

var osnovaAffory = wfp.Osnova{
	Provayder: windows.GUID{Data1: 0x5a4effcc, Data2: 0xebe0, Data3: 0x4afc,
		Data4: [8]byte{0x92, 0x94, 0x72, 0x55, 0x97, 0x6e, 0x71, 0x2c}},
	Podsloy: windows.GUID{Data1: 0xccc1feb7, Data2: 0x1135, Data3: 0x4d25,
		Data4: [8]byte{0xab, 0x35, 0xfb, 0x10, 0x5c, 0xf4, 0x63, 0xb3}},
	Imya: "Affory: весь трафик через VPN",
	// Наибольший: подслои с большим весом решают раньше, и чужое жёсткое
	// разрешение выше нас обошло бы запрет.
	VesPodsloya: 0xFFFF,
}

// Ключи фильтров выводятся из одного: два последних байта это вид фильтра и
// семейство. Постоянный ключ позволяет заменить ровно свой фильтр, а снятие
// и проверка идут перебором по провайдеру и от этого списка не зависят.
var bazaKlyuchey = windows.GUID{Data1: 0xbc2fbec7, Data2: 0xbcf3, Data3: 0x4774,
	Data4: [8]byte{0xa2, 0x1c, 0x01, 0xd9, 0x09, 0xf3, 0x00, 0x00}}

type vidFiltra uint8

const (
	vidZapret vidFiltra = iota + 1
	vidPetlya
	vidTun
	vidServery
	vidMestnye
	vidDhcp
	vidDns
	vidProtsessy
)

var imenaVidov = map[vidFiltra]string{
	vidZapret:    "запрет всего остального",
	vidPetlya:    "внутри машины",
	vidTun:       "через туннель",
	vidServery:   "к серверам VPN",
	vidMestnye:   "локальная сеть",
	vidDhcp:      "получение адреса (DHCP)",
	vidDns:       "DNS к резолверу системы",
	vidProtsessy: "ядро и служба Affory",
}

func klyuchFiltra(vid vidFiltra, v6 bool) windows.GUID {
	k := bazaKlyuchey
	k.Data4[6] = byte(vid)
	k.Data4[7] = 4
	if v6 {
		k.Data4[7] = 6
	}
	return k
}

func klyuchiVida(vid vidFiltra) []windows.GUID {
	return []windows.GUID{klyuchFiltra(vid, false), klyuchFiltra(vid, true)}
}

// Вес внутри подслоя: разрешения выше запрета.
const (
	vesZapreta      = 0
	vesRazresheniya = 10
)

const (
	protokolTcp = 6
	protokolUdp = 17
)

// Частные сети. К диапазонам прежнего правила Affory-Allow-Local добавлены
// групповые адреса и широковещание: прежде их пускали встроенные правила
// Windows, а запрет WFP бьёт и их. Ни то, ни другое за пределы сети не уходит.
var mestnyeV4 = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
}

// Для IPv6 то же: адреса канала, частные и групповые.
var mestnyeV6 = []netip.Prefix{
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("ff00::/8"),
}

func filtr(vid vidFiltra, v6 bool, usloviya ...wfp.Uslovie) wfp.Filtr {
	semeystvo := "IPv4"
	if v6 {
		semeystvo = "IPv6"
	}
	f := wfp.Filtr{
		Klyuch:    klyuchFiltra(vid, v6),
		Imya:      fmt.Sprintf("Affory: %s (%s)", imenaVidov[vid], semeystvo),
		V6:        v6,
		Razreshit: vid != vidZapret,
		Ves:       vesRazresheniya,
		Usloviya:  usloviya,
	}
	if vid == vidZapret {
		f.Ves = vesZapreta
	}
	return f
}

func oboihSemeystv(f func(v6 bool) (wfp.Filtr, bool)) []wfp.Filtr {
	var itog []wfp.Filtr
	for _, v6 := range []bool{false, true} {
		if k, est := f(v6); est {
			itog = append(itog, k)
		}
	}
	return itog
}

// naborFiltrov это весь запрет целиком: пункты прежних правил netsh плюс то,
// что прежде пускали встроенные правила Windows (внутри машины, DHCP).
func naborFiltrov(r Razreshyonnoe) ([]wfp.Filtr, error) {
	var itog []wfp.Filtr
	for _, v6 := range []bool{false, true} {
		itog = append(itog, filtr(vidZapret, v6), filtr(vidPetlya, v6, wfp.Petlya()))
	}

	// Исходящий с адреса TUN-адаптера. Только тот, что есть: адрес у туннеля
	// IPv4, и IPv6 через него не идёт.
	if r.AdresTun.IsValid() {
		a := r.AdresTun.Unmap()
		itog = append(itog, filtr(vidTun, a.Is6(), wfp.LokalnyyAdres(a)))
	}

	servery, err := filtryServerov(r.Kandidaty, r.Porty)
	if err != nil {
		return nil, err
	}
	itog = append(itog, servery...)

	mestnye, err := filtryMestnyh(r.Shlyuz, r.ChuzhieSeti)
	if err != nil {
		return nil, err
	}
	itog = append(itog, mestnye...)

	// DHCP: без него через сутки-другие истекает аренда адреса и пропадает
	// вся сеть, включая туннель. Адрес назначения любой: сервер DHCP у
	// провайдера бывает и за пределами частных сетей.
	itog = append(itog,
		filtr(vidDhcp, false, wfp.Protokol(protokolUdp), wfp.LokalnyyPort(68), wfp.UdalyonnyePorty(67, 67)),
		filtr(vidDhcp, true, wfp.Protokol(protokolUdp), wfp.LokalnyyPort(546), wfp.UdalyonnyePorty(547, 547)))

	itog = append(itog, filtryDns(r.Resolver)...)

	// Ядро и служба: им наружу можно всё. Путь, которого нет на диске, WFP
	// опознать не может, и фильтр с ним не встал бы вовсе.
	var programmy []wfp.Uslovie
	for _, p := range r.Protsessy {
		if _, err := os.Stat(p); err != nil {
			log.Printf("программа %s не разрешена в запрете: %v", p, err)
			continue
		}
		programmy = append(programmy, wfp.Prilozhenie(p))
	}
	if len(programmy) > 0 {
		itog = append(itog, oboihSemeystv(func(v6 bool) (wfp.Filtr, bool) {
			return filtr(vidProtsessy, v6, programmy...), true
		})...)
	}
	return itog, nil
}

// filtryServerov: адреса серверов и всех кандидатов, включая узел подписки, на
// их портах, TCP и UDP. Пусто без адресов или портов: фильтр без них пускал бы
// на любой адрес или любой порт.
func filtryServerov(kandidaty []netip.Addr, porty []string) ([]wfp.Filtr, error) {
	if len(porty) == 0 {
		return nil, nil
	}
	var usloviyaPortov []wfp.Uslovie
	for _, p := range porty {
		ot, do, err := razobratPorty(p)
		if err != nil {
			return nil, err
		}
		usloviyaPortov = append(usloviyaPortov, wfp.UdalyonnyePorty(ot, do))
	}
	return oboihSemeystv(func(v6 bool) (wfp.Filtr, bool) {
		var u []wfp.Uslovie
		for _, a := range kandidaty {
			a = a.Unmap()
			if !a.IsValid() || a.Is6() != v6 {
				continue
			}
			u = append(u, wfp.UdalyonnyySet(netip.PrefixFrom(a, a.BitLen())))
		}
		if len(u) == 0 {
			return wfp.Filtr{}, false
		}
		u = append(u, usloviyaPortov...)
		u = append(u, wfp.Protokol(protokolTcp), wfp.Protokol(protokolUdp))
		return filtr(vidServery, v6, u...), true
	}), nil
}

// razobratPorty читает порт в записи списка серверов: «443» или «20000-30000».
func razobratPorty(s string) (uint16, uint16, error) {
	otS, doS, diapazon := strings.Cut(s, "-")
	ot, err := strconv.ParseUint(otS, 10, 16)
	if err != nil || ot == 0 {
		return 0, 0, fmt.Errorf("порт %q не разобран", s)
	}
	if !diapazon {
		return uint16(ot), uint16(ot), nil
	}
	do, err := strconv.ParseUint(doS, 10, 16)
	if err != nil || do < ot {
		return 0, 0, fmt.Errorf("порт %q не разобран", s)
	}
	return uint16(ot), uint16(do), nil
}

// filtryMestnyh: частные сети, шлюз и сети чужих туннелей этой машины.
func filtryMestnyh(shlyuz netip.Addr, chuzhie []string) ([]wfp.Filtr, error) {
	seti := slices.Concat(mestnyeV4, mestnyeV6)
	if shlyuz.IsValid() {
		a := shlyuz.Unmap()
		seti = append(seti, netip.PrefixFrom(a, a.BitLen()))
	}
	for _, s := range chuzhie {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("сеть чужого туннеля %q не разобрана: %w", s, err)
		}
		seti = append(seti, p)
	}
	return oboihSemeystv(func(v6 bool) (wfp.Filtr, bool) {
		var u []wfp.Uslovie
		for _, p := range seti {
			if p.Addr().Is6() == v6 {
				u = append(u, wfp.UdalyonnyySet(p))
			}
		}
		return filtr(vidMestnye, v6, u...), len(u) > 0
	}), nil
}

// filtryDns: порт 53 к резолверу системы, TCP и UDP, только хосту системных
// служб, где живёт служба DNS. Прежнее правило netsh сужалось до самой службы
// dnscache; у WFP такое сужение требует дескриптора безопасности, и граница
// здесь проходит по svchost.exe: в нём живут только системные службы. Пусто
// без резолвера.
func filtryDns(resolver netip.Addr) []wfp.Filtr {
	if !resolver.IsValid() {
		return nil
	}
	svchost, err := putSvchost()
	if err != nil {
		log.Printf("DNS к резолверу не разрешён в запрете: %v", err)
		return nil
	}
	a := resolver.Unmap()
	return []wfp.Filtr{filtr(vidDns, a.Is6(),
		wfp.Prilozhenie(svchost),
		wfp.UdalyonnyySet(netip.PrefixFrom(a, a.BitLen())),
		wfp.UdalyonnyePorty(53, 53),
		wfp.Protokol(protokolTcp), wfp.Protokol(protokolUdp))}
}

// ServeryVZaprete отдаёт адреса, к которым запрет пускает как к серверам.
//
// Экспортируется ради теста другого пакета, и это осознанно: состав обязан
// сверяться со ВХОДОМ, а не один список с другим.
func ServeryVZaprete(r Razreshyonnoe) ([]string, error) {
	nabor, err := naborFiltrov(r)
	if err != nil {
		return nil, err
	}
	var itog []string
	for _, f := range nabor {
		if !slices.Contains(klyuchiVida(vidServery), f.Klyuch) {
			continue
		}
		for _, u := range f.Usloviya {
			if p, est := u.UdalyonnayaSet(); est {
				itog = append(itog, p.Addr().String())
			}
		}
	}
	slices.Sort(itog)
	return itog, nil
}

// zamokWfp это шов: настоящий говорит с BFE, тестовый держит фильтры в
// памяти. Живой BFE в тесте недопустим: постоянный запрет на машине
// разработки переживает и тест, и перезагрузку.
type zamokWfp interface {
	// Postavit заменяет весь набор одной транзакцией: машина видит либо
	// прежний запрет, либо новый, без окна между ними.
	Postavit(nabor []wfp.Filtr) error
	// Zamenit меняет фильтры с этими ключами одной транзакцией, если запрет
	// стоит. Отвечает, стоял ли он.
	Zamenit(klyuchi []windows.GUID, nabor []wfp.Filtr) (bool, error)
	// Snyat снимает всё своё и отвечает, было ли что снимать.
	Snyat() (bool, error)
	// Stoit отвечает, стоит ли запрет.
	Stoit() (bool, error)
}

var zamok zamokWfp = zamokBfe{}

type zamokBfe struct{}

func vSeanse(f func(s *wfp.Seans) error) error {
	s, err := wfp.Otkryt()
	if err != nil {
		return err
	}
	return errors.Join(f(s), s.Zakryt())
}

func zapretSredi(klyuchi []windows.GUID) bool {
	for _, k := range klyuchiVida(vidZapret) {
		if slices.Contains(klyuchi, k) {
			return true
		}
	}
	return false
}

func (zamokBfe) Postavit(nabor []wfp.Filtr) error {
	return vSeanse(func(s *wfp.Seans) error {
		prezhnie, err := s.KlyuchiFiltrov(osnovaAffory.Provayder)
		if err != nil {
			return err
		}
		return s.Tranzakciya(func() error {
			if err := s.ZavestiOsnovu(osnovaAffory); err != nil {
				return err
			}
			for _, k := range prezhnie {
				if err := s.UdalitFiltr(k); err != nil {
					return err
				}
			}
			for _, f := range nabor {
				if err := s.DobavitFiltr(osnovaAffory, f); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (zamokBfe) Zamenit(klyuchi []windows.GUID, nabor []wfp.Filtr) (stoit bool, err error) {
	err = vSeanse(func(s *wfp.Seans) error {
		est, err := s.KlyuchiFiltrov(osnovaAffory.Provayder)
		if err != nil {
			return err
		}
		if stoit = zapretSredi(est); !stoit {
			return nil
		}
		return s.Tranzakciya(func() error {
			for _, k := range klyuchi {
				if err := s.UdalitFiltr(k); err != nil {
					return err
				}
			}
			for _, f := range nabor {
				if err := s.DobavitFiltr(osnovaAffory, f); err != nil {
					return err
				}
			}
			return nil
		})
	})
	return stoit, err
}

func (zamokBfe) Snyat() (bylo bool, err error) {
	err = vSeanse(func(s *wfp.Seans) error {
		est, err := s.KlyuchiFiltrov(osnovaAffory.Provayder)
		if err != nil {
			return err
		}
		bylo = len(est) > 0
		return s.Tranzakciya(func() error {
			for _, k := range est {
				if err := s.UdalitFiltr(k); err != nil {
					return err
				}
			}
			return s.UdalitOsnovu(osnovaAffory)
		})
	})
	return bylo, err
}

func (zamokBfe) Stoit() (stoit bool, err error) {
	err = vSeanse(func(s *wfp.Seans) error {
		est, err := s.KlyuchiFiltrov(osnovaAffory.Provayder)
		stoit = zapretSredi(est)
		return err
	})
	return stoit, err
}

// sBfe переводит недоступность BFE в отказ, который служба показывает как
// «режим недоступен», а не как «повтори».
func sBfe(err error) error {
	if err != nil && errors.Is(err, wfp.ErrNedostupen) && !errors.Is(err, ErrBrandmauerVyklyuchen) {
		return fmt.Errorf("%w: %w", ErrBrandmauerVyklyuchen, err)
	}
	return err
}
