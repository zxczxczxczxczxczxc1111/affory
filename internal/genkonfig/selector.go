package genkonfig

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Теги групп. Ссылка на несуществующий тег это ровно та ошибка, которую
// sing-box check не находит вовсе, поэтому строки живут в одном месте.
const (
	TegSelector = "vybor"
	TegAvto     = "avto"
)

// Параметры urltest выписаны ЧИСЛАМИ и в одном месте.
//
// «Берутся из спеки» без самих чисел это формулировка, по которой нельзя
// отличить сделанное по спеке от поставленного по вкусу: эталонная фикстура
// закрепила бы любое значение, каким бы оно ни было. Расхождение со спекой
// ловится грепом по этим именам.
const (
	UrltestURL = "https://www.gstatic.com/generate_204" // §6 спеки
	// 10 минут вместо трёх, поднято 21.09.2026.
	//
	// urltest пробует ВСЕХ кандидатов разом, а все наши ключи живут на одном
	// адресе. Каждый цикл это залп из пяти TLS-рукопожатий на пять разных
	// нестандартных портов одного IP плюс три QUIC-сессии туда же, в одно окно
	// в несколько сотен миллисекунд. Так не делает ни один браузер, и разборы
	// поведенческого слоя ТСПУ называют такой залп прямым поводом заморозить
	// адрес на пару минут — снаружи это выглядит как «интернет падает сам».
	//
	// 10 минут это значение Hiddify по умолчанию (URLTestInterval 600 в
	// hiddify_option.go). Цена: сменившийся лучший сервер в режиме «авто»
	// узнаётся позже, а idle_timeout ниже и так гасит пробы простаивающей
	// группы целиком.
	UrltestInterval    = "10m"
	UrltestTolerance   = 50 // мс
	UrltestIdleTimeout = "30m"
)

// TegKandidata строит тег по идентификатору сервера.
//
// Один тег на всех означал бы, что два кандидата схлопнутся
// в селекторе В ОДИН, причём молча: наша sveritTegi складывает объявленные теги
// в map, и дубликат исчезает там без единого слова.
func TegKandidata(id string) string { return prefiksKandidata + id }

const prefiksKandidata = "srv-"

// IdIzTega разбирает тег обратно. Нужно ровно в одном месте: ядро называет
// негодный исходящий НОМЕРОМ, а исключать надо сервер, и мостом между ними
// служит тег. Обратное преобразование живёт рядом с прямым намеренно: разъедься
// они, номер сводился бы не к тому серверу, и молча.
func IdIzTega(teg string) (string, bool) {
	if !strings.HasPrefix(teg, prefiksKandidata) {
		return "", false
	}
	return strings.TrimPrefix(teg, prefiksKandidata), true
}

// kandidaty возвращает список серверов, из которых строится селектор.
//
// Пустой список это НЕ ошибка входа: обычный однокандидатный конфиг это тот же
// селектор из одного элемента. Две ветки кода вместо одной разошлись бы ровно
// там, где их перестают одинаково проверять.
func (v Vhod) kandidaty() []protokol.Server {
	if len(v.Servery) > 0 {
		return v.Servery
	}
	return []protokol.Server{v.Server}
}

// tegiAvto это кандидаты ГРУППЫ АВТО: все теги минус убранные человеком (A5).
//
// Пустой ответ невозможен: убрать последнего не даёт служба, но страхуем и
// здесь. Набор мог прийти из чужого профиля или пережить обновление подписки,
// после которого в области не осталось никого, а urltest без единого
// исходящего это конфиг, который ядро отвергает целиком. Честнее вернуть
// автомату всех, чем оставить человека без туннеля из-за настройки.
func tegiAvto(v Vhod, tegi []string) []string {
	if len(v.VneAvto) == 0 {
		return tegi
	}
	vne := make(map[string]bool, len(v.VneAvto))
	for _, id := range v.VneAvto {
		vne[TegKandidata(id)] = true
	}
	ostavshiesya := make([]string, 0, len(tegi))
	for _, t := range tegi {
		if !vne[t] {
			ostavshiesya = append(ostavshiesya, t)
		}
	}
	if len(ostavshiesya) == 0 {
		return tegi
	}
	return ostavshiesya
}

// gruppy строит urltest и selector.
func gruppy(v Vhod, tegi []string, vybrannyy string) []any {
	// urltest первым в списке селектора: «авто» это то, что человек включает,
	// когда ему всё равно, а таких большинство.
	vSelektore := append([]string{TegAvto}, tegi...)
	poumolchaniyu := vybrannyy
	// В авто умолчание это ГРУППА, а не сервер: иначе «авто» остаётся чучелом,
	// группа объявлена и встать по умолчанию не может никогда.
	if poumolchaniyu == "" || v.Rezhim == protokol.RezhimAvto {
		poumolchaniyu = TegAvto
	}
	return []any{
		map[string]any{
			// Кандидаты АВТО, а не все теги: в селекторе убранный сервер
			// остаётся и выбирается руками, в автомате его нет.
			"type": "urltest", "tag": TegAvto, "outbounds": tegiAvto(v, tegi),
			"url": UrltestURL, "interval": UrltestInterval,
			"tolerance": UrltestTolerance, "idle_timeout": UrltestIdleTimeout,
			// Существующие соединения НЕ рвём. Порог «TCP не рвутся дольше
			// секунды» снят из спеки именно потому, что поведение задаётся этим
			// полем, а не удачей.
			"interrupt_exist_connections": false,
		},
		map[string]any{
			"type": "selector", "tag": TegSelector, "outbounds": vSelektore,
			"default":                     poumolchaniyu,
			"interrupt_exist_connections": false,
		},
	}
}

// sveritKandidatov ловит то, что не поймает ни check, ни sveritTegi.
func sveritKandidatov(v Vhod) error {
	vidno := map[string]bool{}
	for _, a := range v.Kandidaty {
		vidno[a.String()] = true
	}
	vstrechalis := map[string]bool{}
	for _, s := range v.kandidaty() {
		if s.Id == "" {
			return fmt.Errorf("%w: у кандидата %q нет идентификатора", ErrNetKandidatov, s.Imya)
		}
		if vstrechalis[s.Id] {
			return fmt.Errorf("%w: идентификатор %s встречается дважды", ErrNetKandidatov, s.Id)
		}
		vstrechalis[s.Id] = true

		// Адрес кандидата ОБЯЗАН быть в правиле петли, включая тех, куда
		// селектор сейчас не смотрит: urltest пробит их всё равно, и проба
		// уходит до того, как выбран выход. Не попал адрес в ip_cidr, значит
		// петля на старте.
		//
		// Проверяется только литерал: имя резолвит вызывающий, и здесь мы про
		// его адреса ничего не знаем.
		if a, err := netip.ParseAddr(s.Host); err == nil && !vidno[a.String()] {
			return fmt.Errorf("%w: адреса кандидата %s нет в правиле петли", ErrNetKandidatov, s.Id)
		}
	}
	return nil
}
