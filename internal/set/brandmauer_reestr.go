package set

import (
	"errors"
	"fmt"
	"strings"

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

// Шов. Тесты описывают машину выводом netsh и читают состояние из него
// (brandmauer_test.go, bezReestra): живой реестр прошёл бы мимо фикстуры.
var chitatIzReestra = sostoyanieIzReestra

// Умолчания Windows для профиля, которые действуют, пока значения в реестре
// нет: брандмауэр включён, входящие запрещены, исходящие разрешены.
var umolchaniyaProfilya = map[string]uint64{
	"EnableFirewall":        1,
	"DefaultInboundAction":  1,
	"DefaultOutboundAction": 0,
}

// sostoyanieIzReestra отдаёт состояние профиля из реестра.
//
// Недостающее значение это умолчание Windows (П2 аудита 1.8.0). Прежде
// неполная ветка уводила в разбор вывода netsh, а он на русской Windows
// печатает «ВКЛ» вместо ON: режим «весь трафик» не включался, осиротевший
// замок не снимался, удаление программы останавливалось. Windows при
// недостающем значении действует умолчанием, так что оно и есть правда о
// профиле, а не догадка.
//
// Отказ только там, где реестр не читается вовсе, кроме отсутствия ветки.
func sostoyanieIzReestra(profil string) (ProfilDo, error) {
	vetka, est := vetkiProfiley[profil]
	if !est {
		return ProfilDo{}, fmt.Errorf("профиль %q неизвестен", profil)
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, korenBrandmauera+`\`+vetka, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return sostoyanieIzZnacheniy(profil, func(string) (uint64, error) { return 0, registry.ErrNotExist })
	}
	if err != nil {
		return ProfilDo{}, fmt.Errorf("ветка профиля %s не открылась: %w", profil, err)
	}
	defer k.Close()
	return sostoyanieIzZnacheniy(profil, func(imya string) (uint64, error) {
		v, _, err := k.GetIntegerValue(imya)
		return v, err
	})
}

// sostoyanieIzZnacheniy собирает профиль из трёх значений, подставляя
// умолчание вместо отсутствующего.
func sostoyanieIzZnacheniy(profil string, chitat func(string) (uint64, error)) (ProfilDo, error) {
	znach := map[string]uint64{}
	for imya, umolch := range umolchaniyaProfilya {
		v, err := chitat(imya)
		switch {
		case errors.Is(err, registry.ErrNotExist):
			v = umolch
		case err != nil:
			return ProfilDo{}, fmt.Errorf("значение %s профиля %s не прочитано: %w", imya, profil, err)
		}
		znach[imya] = v
	}
	return ProfilDo{
		Imya:      profil,
		Vklyuchen: znach["EnableFirewall"] != 0,
		Politika:  politikaIzChisel(znach["DefaultInboundAction"], znach["DefaultOutboundAction"]),
	}, nil
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

// Правила, заведённые netsh, лежат в постоянном хранилище брандмауэра одной
// строкой на правило: `v2.33|Action=Allow|Dir=Out|...|Name=Affory-Allow-Tun|`.
// Шов: тест подменяет реестр списком имён.
var imenaPravilVReestre = chitatImenaPravil

// chitatImenaPravil отдаёт имена всех правил постоянного хранилища (Г1 аудита
// 1.8.0). Уборка на старте спрашивала netsh о каждом из трёх десятков возможных
// имён и снимала каждое вслепую: до минуты на медленной машине, и всё это время
// служба не отвечала SCM. Реестр отвечает за миллисекунды и без языка системы.
func chitatImenaPravil() (map[string]bool, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, korenBrandmauera+`\FirewallRules`, registry.QUERY_VALUE)
	if err != nil {
		return nil, fmt.Errorf("ветка правил брандмауэра не открылась: %w", err)
	}
	defer k.Close()
	znacheniya, err := k.ReadValueNames(0)
	if err != nil {
		return nil, fmt.Errorf("список правил брандмауэра не прочитан: %w", err)
	}
	est := make(map[string]bool, len(znacheniya))
	for _, z := range znacheniya {
		s, _, err := k.GetStringValue(z)
		if err != nil {
			// Непрочитанное правило может оказаться нашим: неполный список
			// оставил бы его висеть. Вызывающий тогда снимает по полному.
			return nil, fmt.Errorf("правило %s не прочитано: %w", z, err)
		}
		if imya := imyaIzZapisi(s); imya != "" {
			est[imya] = true
		}
	}
	return est, nil
}

func imyaIzZapisi(zapis string) string {
	for _, pole := range strings.Split(zapis, "|") {
		if imya, est := strings.CutPrefix(pole, "Name="); est {
			return imya
		}
	}
	return ""
}
