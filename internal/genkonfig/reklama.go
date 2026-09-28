package genkonfig

import (
	"errors"
	"slices"
	"strings"
)

// Блокировка рекламы и трекеров внутри ядра (28.09.2026).
//
// Почему в ядре, а не сменой резолвера: рекламные домены Яндекса и Mail.ru
// (an.yandex.ru, mc.yandex.ru, ad.mail.ru) входят в набор ru, их DNS уходит
// местному резолверу, а в режиме «Только выбранное» местному уходит почти всё.
//
// Почему НЕ через Nabory: у Nabory смысл «мимо туннеля» зашит в praviloNaborov
// (route direct) и dnsPravila (server mestnyy). Набор блокировки там увёл бы
// рекламу напрямую и открытым текстом, а в режиме «весь трафик» исчез бы.

// Reklama это блокировка рекламы. nil в Vhod означает «выключена».
type Reklama struct {
	// Fayl это готовый набор .srs. Файл ОБЯЗАН быть годным: local-набор без
	// него роняет весь конфиг (FATAL "initialize router: parse rule-set").
	// Гарантию даёт служба сверкой sha256 с метой перед каждой сборкой.
	Fayl string
	// Razresheno это исключения человека, вместе с поддоменами (domain_suffix).
	Razresheno []string
	// Svoi это ТОЧНЫЕ имена (domain), к которым ходит сама служба. Не
	// суффиксы: yandex.ru суффиксом вывел бы из-под блокировки an.yandex.ru.
	Svoi []string
}

var errReklamaNepolnaya = errors.New("блокировка рекламы задана неполно")

const tegReklamy = "reklama"

// kanareykaDoH: NXDOMAIN на это имя выключает автоматический DoH Firefox.
const kanareykaDoH = "use-application-dns.net"

// domenyNeBlokiruyutsya: имена, которых нет константами ни в одном пакете
// Affory: проверка сети Windows (NCSI) и хосты, куда GitHub перенаправляет
// загрузку выпуска. Остальные свои имена служба присылает в Svoi.
var domenyNeBlokiruyutsya = []string{
	"www.msftconnecttest.com", "ipv6.msftconnecttest.com", "dns.msftncsi.com",
	"github.com", "codeload.github.com", "objects.githubusercontent.com",
	"release-assets.githubusercontent.com",
}

// Отрицательный ответ несёт SOA с TTL и минимумом 60 с (RFC 2308). Владелец
// ".", а не "*.": "*." ядро переписало бы в имя вопроса (rewriteRecords).
const soaBloka = ". 60 IN SOA blok.affory.invalid. nobody.affory.invalid. 1 3600 600 86400 60"

func naborReklamy(v Vhod) (map[string]any, bool) {
	if v.Reklama == nil {
		return nil, false
	}
	return map[string]any{"type": "local", "format": "binary", "tag": tegReklamy, "path": v.Reklama.Fayl}, true
}

// svoiImena: постоянные плюс присланные службой, строчные, без повторов, по
// порядку. Не пуст никогда.
func svoiImena(v Vhod) []string {
	vse := make([]string, 0, len(domenyNeBlokiruyutsya)+len(v.Reklama.Svoi))
	for _, d := range slices.Concat(domenyNeBlokiruyutsya, v.Reklama.Svoi) {
		vse = append(vse, strings.ToLower(d))
	}
	slices.Sort(vse)
	return slices.Compact(vse)
}

// isklyucheniyaReklamy это «кроме»: свои имена точно, исключения человека с
// поддоменами. Внутри одного правила domain и domain_suffix складываются через
// «или», invert переворачивает всё правило. domain_suffix пишется только
// непустым: пустой совпал бы со всем.
func isklyucheniyaReklamy(v Vhod) map[string]any {
	m := map[string]any{"domain": svoiImena(v), "invert": true}
	if len(v.Reklama.Razresheno) > 0 {
		m["domain_suffix"] = v.Reklama.Razresheno
	}
	return m
}

// dnsPravilaReklamy: канарейка Firefox и сам блок. predefined NXDOMAIN, а не
// reject: reject отвечает REFUSED, то есть «спроси другой сервер».
func dnsPravilaReklamy(v Vhod) []any {
	if v.Reklama == nil {
		return nil
	}
	return []any{
		map[string]any{"domain": []string{kanareykaDoH},
			"action": "predefined", "rcode": "NXDOMAIN", "ns": []string{soaBloka}},
		map[string]any{
			"type": "logical", "mode": "and",
			"rules":  []any{map[string]any{"rule_set": []string{tegReklamy}}, isklyucheniyaReklamy(v)},
			"action": "predefined", "rcode": "NXDOMAIN", "ns": []string{soaBloka},
		},
	}
}

// praviloReklamy режет соединение, имя которого узнал sniff. no_drop ОБЯЗАТЕЛЕН:
// без него после 50 срабатываний за 30 с ядро переходит на drop, и TCP через TUN
// висит до таймаута (10 из 60 в замере 28.09.2026). check не ловит ни его
// пропуск, ни опечатку в имени.
func praviloReklamy(v Vhod) (map[string]any, bool) {
	if v.Reklama == nil {
		return nil, false
	}
	return map[string]any{
		"type": "logical", "mode": "and",
		"rules": []any{
			map[string]any{"rule_set": []string{tegReklamy}},
			isklyucheniyaReklamy(v),
			// Наши процессы через локальный прокси (подписка, обновление, адрес
			// выхода, замер). На TUN они уже ушли правилом процессов выше.
			map[string]any{"process_path": v.PutiProtsessov, "invert": true},
		},
		"action": "reject", "no_drop": true,
	}, true
}

func proveritReklamu(v Vhod) error {
	r := v.Reklama
	if r == nil {
		return nil
	}
	if r.Fayl == "" {
		return errors.Join(errReklamaNepolnaya, errors.New("нет файла набора"))
	}
	for _, d := range slices.Concat(r.Razresheno, r.Svoi) {
		if d == "" {
			return errors.Join(errReklamaNepolnaya, errors.New("пустое имя в исключениях"))
		}
	}
	return nil
}
