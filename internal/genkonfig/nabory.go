package genkonfig

// Наборы правил (rule_set). В sing-box 1.14 geosite и geoip удалены и валят
// запуск, доменные списки живут только в наборах формата .srs.
//
// Three facts from the 1.14 sources, each one a field below:
//   - download_detour is deprecated and leaves in 1.16; the replacement is
//     http_client with detour inside its dial fields;
//   - initial_path is the file the set boots from until the first successful
//     download, which is the whole answer to "cold start with no network";
//   - a remote set needs experimental.cache_file, or it forgets everything it
//     downloaded the moment the core exits.

// NaborPravil это один удалённый набор. Генератор адресов не выдумывает: URL и
// файл приезжают от службы, которая и знает, где лежат данные.
type NaborPravil struct {
	// Teg это то, на что ссылается правило маршрута.
	Teg string
	// URL набора. Хост этого адреса ОБЯЗАН входить в Kandidaty: загрузка идёт
	// мимо туннеля (решение владельца 31.08.2026), а мимо туннеля ходит только
	// то, что стоит в правиле петли.
	URL string
	// Fayl это initial_path: с него набор поднимается, пока загрузка не удалась.
	Fayl string
}

// Наборы обновляются раз в сутки: это умолчание ядра, но записанное явно, чтобы
// смена умолчания в следующей версии не поменяла поведение молча.
const intervalObnovleniyaNaborov = "24h"

// razdelNaborov собирает route.rule_set. Пустой вход даёт nil, а не пустой
// список: пустой раздел в конфиге это вопрос «а что здесь было?» через год.
func razdelNaborov(v Vhod) []any {
	if len(v.Nabory) == 0 {
		return nil
	}
	r := make([]any, 0, len(v.Nabory))
	for _, n := range v.Nabory {
		r = append(r, map[string]any{
			"type": "remote", "format": "binary", "tag": n.Teg,
			"url":          n.URL,
			"initial_path": n.Fayl,
			// detour is a dial field of the client, so "direct" here means the
			// download never enters the tunnel. Without an explicit client
			// the download would follow route.final, and route.final is the
			// tunnel: the set would then need the tunnel it exists to steer.
			"http_client":     map[string]any{"detour": TegPryamo},
			"update_interval": intervalObnovleniyaNaborov,
		})
	}
	return r
}

// praviloNaborov даёт одно правило на все наборы: домен из любого набора идёт
// мимо туннеля. Это удобное исключение, а не петлевое, поэтому в режиме «весь
// трафик» его нет (инвариант 6) и стоит оно НИЖЕ hijack-dns.
func praviloNaborov(v Vhod) (map[string]any, bool) {
	if len(v.Nabory) == 0 || v.VesTrafik {
		return nil, false
	}
	tegi := make([]string, 0, len(v.Nabory))
	for _, n := range v.Nabory {
		tegi = append(tegi, n.Teg)
	}
	return map[string]any{"rule_set": tegi, "outbound": TegPryamo}, true
}

// keshFayl это experimental.cache_file. store_rdrc не пишется намеренно: поле
// устарело в 1.14 и уходит в 1.16.
func keshFayl(v Vhod) (map[string]any, bool) {
	if v.FaylKesha == "" {
		return nil, false
	}
	return map[string]any{"enabled": true, "path": v.FaylKesha}, true
}
