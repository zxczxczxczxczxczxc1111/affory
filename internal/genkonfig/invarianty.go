package genkonfig

import (
	"errors"
	"fmt"
	"sort"
)

// ErrVisyachiyTeg это то, чего sing-box check не находит НИКОГДА.
//
// Measured over five runs on sing-box 1.14.0: a dangling route.final, a dangling
// outbound in a rule, a dangling detour on a DNS server, a dangling server in a
// DNS rule and a dangling rule_set all exit 0 without a word. The judge we were
// counting on for invariants 1, 2 and 5 does not judge this at all, so the
// generator carries its own check and runs it before returning anything.
var ErrVisyachiyTeg = errors.New("ссылка на необъявленный тег")

// sveritTegi собирает множество объявленных тегов и сверяет с упомянутыми.
func sveritTegi(k map[string]any) error {
	obyavleny := map[string]bool{}
	dobavit := func(v any) {
		if m, ok := v.(map[string]any); ok {
			if t, ok := m["tag"].(string); ok && t != "" {
				obyavleny[t] = true
			}
		}
	}
	for _, r := range []string{"inbounds", "outbounds"} {
		for _, v := range spisok(k[r]) {
			dobavit(v)
		}
	}
	if d, ok := k["dns"].(map[string]any); ok {
		for _, v := range spisok(d["servers"]) {
			dobavit(v)
		}
	}
	// Теги наборов живут в своём пространстве имён у ядра, но проверка у нас
	// одна: имя набора, совпавшее с именем исходящего, было бы отдельной
	// бедой, а не поводом заводить вторую проверку.
	if r, ok := k["route"].(map[string]any); ok {
		for _, v := range spisok(r["rule_set"]) {
			dobavit(v)
		}
	}

	var upomyanuty []string
	pomyanut := func(v any) {
		if s, ok := v.(string); ok && s != "" {
			upomyanuty = append(upomyanuty, s)
		}
	}
	if d, ok := k["dns"].(map[string]any); ok {
		pomyanut(d["final"])
		for _, v := range spisok(d["servers"]) {
			if m, ok := v.(map[string]any); ok {
				pomyanut(m["detour"])
			}
		}
		for _, v := range spisok(d["rules"]) {
			pomyanutVPravile(v, pomyanut)
		}
	}
	// Источник адреса сервера у каждого исходящего: висячий тег здесь ядро
	// приняло бы молча, а сервер потом не нашёл бы адреса вовсе.
	for _, v := range spisok(k["outbounds"]) {
		if m, ok := v.(map[string]any); ok {
			pomyanut(m["domain_resolver"])
		}
	}
	if r, ok := k["route"].(map[string]any); ok {
		pomyanut(r["final"])
		if dr, ok := r["default_domain_resolver"].(map[string]any); ok {
			pomyanut(dr["server"])
		}
		for _, v := range spisok(r["rules"]) {
			pomyanutVPravile(v, pomyanut)
		}
	}

	var bez []string
	for _, t := range upomyanuty {
		if !obyavleny[t] {
			bez = append(bez, t)
		}
	}
	if len(bez) == 0 {
		return nil
	}
	sort.Strings(bez)
	return fmt.Errorf("%w: %v", ErrVisyachiyTeg, bez)
}

// Логические правила вкладываются друг в друга, поэтому обход рекурсивный: тег,
// спрятанный на два уровня вглубь, ничем не лучше тега на верхнем.
func pomyanutVPravile(v any, pomyanut func(any)) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	pomyanut(m["outbound"])
	// Правила маршрута и DNS обходятся одной функцией: у DNS-правила тег
	// сервера, у правила маршрута тег исходящего.
	pomyanut(m["server"])
	// rule_set is a list of tags, and a dangling one passes check silently
	// (measured on 1.14.0, see ErrVisyachiyTeg).
	for _, t := range spisok(m["rule_set"]) {
		pomyanut(t)
	}
	// Вложенные логические правила обходятся рекурсивно: тег, спрятанный
	// вглубь, ничем не лучше тега наверху.
	for _, vl := range spisok(m["rules"]) {
		pomyanutVPravile(vl, pomyanut)
	}
}

var errPustoeUsloviye = errors.New("пустой список в условии правила")

// sveritUsloviya ищет пустые списки в правилах маршрута и DNS. Пустой список
// на верхнем уровне check пропускает с кодом 0, а ядро считает такое условие
// выполненным ВСЕГДА: domain_suffix [] с reject резал весь интернет, rule_set []
// с predefined отвечал NXDOMAIN на всё (замер 28.09.2026, sing-box 1.14.2).
func sveritUsloviya(k map[string]any) error {
	var obhod func(put string, v any) error
	obhod = func(put string, v any) error {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		for klyuch, zn := range m {
			switch z := zn.(type) {
			case []string:
				if len(z) == 0 {
					return fmt.Errorf("%w: %s.%s", errPustoeUsloviye, put, klyuch)
				}
			case []any:
				if len(z) == 0 {
					return fmt.Errorf("%w: %s.%s", errPustoeUsloviye, put, klyuch)
				}
				if klyuch == "rules" {
					for i, vl := range z {
						if err := obhod(fmt.Sprintf("%s.rules[%d]", put, i), vl); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	for _, razdel := range []string{"route", "dns"} {
		r, _ := k[razdel].(map[string]any)
		for i, v := range spisok(r["rules"]) {
			if err := obhod(fmt.Sprintf("%s.rules[%d]", razdel, i), v); err != nil {
				return err
			}
		}
	}
	return nil
}

func spisok(v any) []any {
	switch s := v.(type) {
	case []any:
		return s
	case []string:
		// Генератор кладёт списки тегов как []string (praviloNaborov,
		// dnsPravila), и до 28.09.2026 проверка их не видела вовсе: висячий
		// rule_set в настоящем конфиге проходил молча, а check 1.14.2 в
		// правилах маршрута его тоже пропускает (exit 0).
		r := make([]any, len(s))
		for i, x := range s {
			r[i] = x
		}
		return r
	}
	return nil
}
