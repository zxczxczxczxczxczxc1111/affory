package set

import (
	"fmt"
	"net/netip"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Маршруты по умолчанию спрашиваются у таблицы маршрутизации, а не выводятся из
// наличия шлюза у адаптера.
//
// Разница не теоретическая и стоила продукту жалобы 21.09.2026. Radmin VPN,
// Hamachi, VirtualBox Host-Only и любой чужой туннель поднимают адаптер, дают
// ему адрес и шлюз своей частной сети и при этом НЕ объявляют маршрут
// 0.0.0.0/0: наружу трафик через них не идёт. Для GetAdaptersAddresses такой
// адаптер неотличим от физического канала, а метрика у него бывает лучше, и
// выбор по шлюзу отдавал ему первое место. Дальше служба спрашивала у него
// резолвер, не получала ни одного и роняла подъём туннеля на каждой попытке,
// называя человеку чужой адаптер.
//
// Таблица отвечает на тот вопрос, который задан: кто несёт умолчание и с какой
// метрикой маршрута. Метрика маршрута складывается с метрикой интерфейса ровно
// так же, как их складывает сама Windows, разводя два умолчания.

// marshrutyUmolchaniya это шов. Тесты подменяют его и проверяют выбор, не имея
// ни таблицы маршрутов, ни сети.
var marshrutyUmolchaniya = marshrutyUmolchaniyaSistemnye

// prefiksyMarshrutov это второй шов, для выбора свободной подсети туннеля.
var prefiksyMarshrutov = prefiksyMarshrutovSistemnye

// prefiksyMarshrutovSistemnye отдаёт префиксы назначения из таблицы, кроме
// умолчания.
//
// Нужны, чтобы узнать, занята ли подсеть, которую мы собираемся отдать своему
// TUN. Адресов адаптеров для этого мало: Docker Desktop держит пул
// 172.17.0.0/16 … 172.31.0.0/16 и создаёт сети по мере надобности, а маршрут на
// такую сеть в таблице есть всегда, даже когда её шлюз не поднят.
//
// 0.0.0.0/0 пропускается намеренно: он содержит вообще всё и объявил бы занятым
// любой адрес.
func prefiksyMarshrutovSistemnye() ([]netip.Prefix, error) {
	var tabl *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &tabl); err != nil {
		return nil, fmt.Errorf("GetIpForwardTable2: %w", err)
	}
	defer windows.FreeMibTable(unsafe.Pointer(tabl))

	if tabl.NumEntries == 0 {
		return nil, nil
	}
	var itog []netip.Prefix
	stroki := unsafe.Slice(&tabl.Table[0], tabl.NumEntries)
	for i := range stroki {
		s := &stroki[i]
		if s.DestinationPrefix.PrefixLength == 0 ||
			s.DestinationPrefix.Prefix.Family != windows.AF_INET {
			continue
		}
		adres := (*windows.RawSockaddrInet4)(unsafe.Pointer(&s.DestinationPrefix.Prefix))
		p, err := netip.AddrFrom4(adres.Addr).Prefix(int(s.DestinationPrefix.PrefixLength))
		if err != nil {
			continue
		}
		itog = append(itog, p)
	}
	return itog, nil
}

// marshrutyUmolchaniyaSistemnye отдаёт лучшую метрику маршрута 0.0.0.0/0 по
// индексу интерфейса. Индекса в карте нет значит умолчания у адаптера нет.
func marshrutyUmolchaniyaSistemnye() (map[uint32]uint32, error) {
	var tabl *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &tabl); err != nil {
		return nil, fmt.Errorf("GetIpForwardTable2: %w", err)
	}
	defer windows.FreeMibTable(unsafe.Pointer(tabl))

	itog := map[uint32]uint32{}
	if tabl.NumEntries == 0 {
		return itog, nil
	}
	stroki := unsafe.Slice(&tabl.Table[0], tabl.NumEntries)
	for i := range stroki {
		s := &stroki[i]
		if s.DestinationPrefix.PrefixLength != 0 ||
			s.DestinationPrefix.Prefix.Family != windows.AF_INET {
			continue
		}
		// Префикс нулевой длины с ненулевым адресом система не выдаёт, но
		// проверка стоит одной строки, а без неё в карту попал бы чужой
		// маршрут, если Windows когда-нибудь начнёт их так писать.
		adres := (*windows.RawSockaddrInet4)(unsafe.Pointer(&s.DestinationPrefix.Prefix))
		if adres.Addr != [4]byte{} {
			continue
		}
		if bylo, est := itog[s.InterfaceIndex]; est && bylo <= s.Metric {
			continue
		}
		itog[s.InterfaceIndex] = s.Metric
	}
	return itog, nil
}
