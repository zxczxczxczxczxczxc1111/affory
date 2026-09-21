package set

import "net/netip"

// Сети чужих туннелей, которые режим «весь трафик» убивал молча.
//
// Разрешённые частные диапазоны это RFC 1918 плюс link-local, и этого мало.
// Radmin VPN, Hamachi и Tailscale сидят НЕ в частных сетях: первые два заняли
// чужие публичные /8 (26.0.0.0/8 у Radmin, 25.0.0.0/8 у Hamachi), третий живёт
// в CGNAT 100.64.0.0/10. При включённой блокировке сети вне VPN политика Block
// убивает их целиком, и человек видит не «Affory запретил», а «Radmin перестал
// работать».
//
// Разрешаются НЕ всегда, а только когда такой адаптер в системе действительно
// есть. Разница принципиальная: 25.0.0.0/8 и 26.0.0.0/8 это настоящие публичные
// адреса, которые эти программы самовольно заняли, и постоянная дыра в них на
// машине без Radmin была бы дырой мимо туннеля в чистом поле.
var chuzhieSeti = []netip.Prefix{
	netip.MustParsePrefix("26.0.0.0/8"),    // Radmin VPN
	netip.MustParsePrefix("25.0.0.0/8"),    // LogMeIn Hamachi
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT: Tailscale, Nebula, провайдерский NAT
}

// SetiChuzhihTunneley отдаёт те из этих сетей, в которых ПРЯМО СЕЙЧАС есть
// поднятый адаптер. krome это индексы, которые не учитываются (наш туннель).
//
// Ошибка перечисления это пустой список, а не отказ: без списка режим всего
// лишь останется прежним, а отказ сорвал бы подъём туннеля целиком.
func SetiChuzhihTunneley(krome ...uint32) []string {
	spisok, err := perechislit()
	if err != nil {
		return nil
	}
	propustit := make(map[uint32]bool, len(krome))
	for _, i := range krome {
		propustit[i] = true
	}
	var itog []string
	for _, podset := range chuzhieSeti {
		for _, a := range spisok {
			if propustit[a.Indeks] || a.Sostoyanie != sostoyanieVverh || a.Tip == tipPetli {
				continue
			}
			if soderzhit(podset, a.Adresa) {
				itog = append(itog, podset.String())
				break
			}
		}
	}
	return itog
}

func soderzhit(podset netip.Prefix, adresa []netip.Addr) bool {
	for _, adr := range adresa {
		if podset.Contains(adr) {
			return true
		}
	}
	return false
}
