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
			if m, ok := v.(map[string]any); ok {
				pomyanut(m["server"])
			}
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
	// rule_set is a list of tags, and a dangling one passes check silently
	// (measured on 1.14.0, see ErrVisyachiyTeg).
	for _, t := range spisok(m["rule_set"]) {
		pomyanut(t)
	}
	// rule_set is a list of tags, and a dangling one passes check silently
	// (measured on 1.14.0, see ErrVisyachiyTeg).
	for _, vl := range spisok(m["rules"]) {
		pomyanutVPravile(vl, pomyanut)
	}
}

func spisok(v any) []any {
	s, _ := v.([]any)
	return s
}
