package set

import "net/netip"

// Подсеть своего туннеля выбирается, а не вбита константой.
//
// 172.19.0.1/30 стоял в генераторе жёстко, и это адрес прямо посреди пула
// Docker Desktop: он раздаёт сети из 172.17.0.0/16 … 172.31.0.0/16, и третья-
// четвёртая созданная сеть получает ровно 172.19.0.0/16, причём её шлюзом
// становится 172.19.0.1 - тот самый адрес. WSL2 и Hyper-V раздают из
// 172.16.0.0/12 динамически при каждом старте хоста. При совпадении Windows
// получает два маршрута на пересекающиеся префиксы, и туннель либо не несёт,
// либо уводит в никуда чужие контейнеры. Ни проверки, ни диагностики на это не
// было вовсе.
//
// Первым кандидатом остаётся прежний адрес: на чистой машине поведение не
// меняется ни на байт, и это важнее красоты списка.
var PodsetiTun = []netip.Prefix{
	netip.MustParsePrefix("172.19.0.1/30"),
	netip.MustParsePrefix("172.23.0.1/30"),
	netip.MustParsePrefix("172.29.0.1/30"),
	netip.MustParsePrefix("10.172.19.1/30"),
	netip.MustParsePrefix("10.254.19.1/30"),
}

// SvobodnayaPodsetTun отдаёт первую подсеть, которую никто не занял, и признак
// «пришлось уйти с обычного адреса».
//
// Занятой считается подсеть, если её адрес лежит внутри чужого адаптера или
// внутри чужого маршрута. Маршруты важнее адресов: сеть Docker существует и
// маршрут на неё есть даже тогда, когда её шлюз не поднят адаптером.
//
// krome это индексы адаптеров, которые не в счёт (наш собственный туннель от
// прошлого подъёма). Если заняты все кандидаты, отдаётся первый: подняться с
// риском пересечения лучше, чем не подняться вовсе, и об этом говорит второй
// возвращаемый признак.
func SvobodnayaPodsetTun(krome ...uint32) (netip.Prefix, bool) {
	zanyato := zanyatye(krome...)
	for _, p := range PodsetiTun {
		if !zanyato(p.Addr()) {
			return p, p != PodsetiTun[0]
		}
	}
	return PodsetiTun[0], true
}

// zanyatye строит проверку «этот адрес уже чей-то».
func zanyatye(krome ...uint32) func(netip.Addr) bool {
	propustit := make(map[uint32]bool, len(krome))
	for _, i := range krome {
		propustit[i] = true
	}
	var chuzhie []netip.Prefix
	if spisok, err := perechislit(); err == nil {
		for _, a := range spisok {
			if propustit[a.Indeks] || a.Sostoyanie != sostoyanieVverh || a.Tip == tipPetli {
				continue
			}
			for _, adr := range a.Adresa {
				if adr.Is4() {
					chuzhie = append(chuzhie, netip.PrefixFrom(adr, 32))
				}
			}
		}
	}
	if marshruty, err := prefiksyMarshrutov(); err == nil {
		chuzhie = append(chuzhie, marshruty...)
	}
	return func(adr netip.Addr) bool {
		for _, p := range chuzhie {
			if p.Contains(adr) {
				return true
			}
		}
		return false
	}
}
