package set

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// Состояние брандмауэра читается из реестра, а не из вывода netsh.
//
// Разбор вывода был написан с оговоркой «значения netsh НЕ локализуются, в
// отличие от ключей», и для политики это правда: `BlockInbound,AllowOutbound`
// одинаково на всех языках. А вот состояние профиля печатается словом, и слово
// это переводится: на русской Windows там «ВКЛ», а не `ON`. Регулярка искала
// именно ON или OFF, не находила ничего и возвращала жёсткий отказ — то есть
// на нерусской... вернее, на любой НЕанглийской машине режим «весь трафик»
// нельзя было ни включить, ни снять, и осиротевшая защита не опознавалась.
//
// Проверить это на английской машине разработки нельзя в принципе, а ставить
// работу защиты в зависимость от языка системы незачем: те же три числа лежат
// в реестре, где у них нет языка.
const korenBrandmauera = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy`

// Профиль private в реестре зовётся Standard: имя досталось от Windows XP и
// с тех пор не менялось.
var vetkiProfiley = map[string]string{
	"domain":  "DomainProfile",
	"private": "StandardProfile",
	"public":  "PublicProfile",
}

// Шов. Тесты кормят разбор фикстурой вместо netsh, и живой реестр машины
// прошёл бы мимо неё: фикстура утверждала бы одно, а проверяемый код читал
// бы другое.
var chitatIzReestra = sostoyanieIzReestra

// sostoyanieIzReestra отдаёт состояние профиля и признак того, что ответ полон.
//
// Неполный ответ (нет хотя бы одного из трёх значений) это не ошибка: Windows
// в таком случае действует умолчанием, а угадывать умолчание значит однажды
// вернуть человеку чужую политику. Тогда решает запасной путь через netsh.
func sostoyanieIzReestra(profil string) (ProfilDo, bool) {
	vetka, est := vetkiProfiley[profil]
	if !est {
		return ProfilDo{}, false
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, korenBrandmauera+`\`+vetka, registry.QUERY_VALUE)
	if err != nil {
		return ProfilDo{}, false
	}
	defer k.Close()

	vklyuchen, _, err := k.GetIntegerValue("EnableFirewall")
	if err != nil {
		return ProfilDo{}, false
	}
	vhod, _, err := k.GetIntegerValue("DefaultInboundAction")
	if err != nil {
		return ProfilDo{}, false
	}
	vyhod, _, err := k.GetIntegerValue("DefaultOutboundAction")
	if err != nil {
		return ProfilDo{}, false
	}
	return ProfilDo{
		Imya:      profil,
		Vklyuchen: vklyuchen != 0,
		Politika:  politikaIzChisel(vhod, vyhod),
	}, true
}

// politikaIzChisel собирает строку ровно в том виде, в каком её принимает
// обратно `netsh advfirewall set <профиль>profile firewallpolicy`.
//
// 0 это Allow, 1 это Block; других значений у этих полей нет.
func politikaIzChisel(vhod, vyhod uint64) string {
	return fmt.Sprintf("%sInbound,%sOutbound", deystvie(vhod), deystvie(vyhod))
}

func deystvie(v uint64) string {
	if v == 1 {
		return "Block"
	}
	return "Allow"
}
