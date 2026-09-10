package main

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// serveryDlyaPodyoma отдаёт список серверов, известных службе.
func (s *Sluzhba) serveryDlyaPodyoma() ([]protokol.Server, error) {
	n, err := s.nabor()
	if err != nil {
		return nil, err
	}
	if len(n.Servery) == 0 {
		return nil, ErrNetServerov
	}
	return n.Servery, nil
}

// adresaKandidatov это ЕДИНСТВЕННЫЙ источник адресов для обоих списков.
//
// Имена резолвятся здесь, то есть ДО подъёма TUN и системным резолвером. Позже
// было бы поздно: резолв пошёл бы через туннель, которого ещё нет.
//
// Адрес подписки входит в список ОБЯЗАТЕЛЬНО: без него запертый режим отрезает
// обновление подписки ровно тогда, когда список серверов протух и обновить его
// нужнее всего.
func (s *Sluzhba) adresaKandidatov() ([]netip.Addr, error) {
	n, err := s.nabor()
	if err != nil {
		return nil, err
	}
	if len(n.Servery) == 0 {
		return nil, ErrNetServerov
	}
	// Хосты наборов входят наравне с подпиской: загрузка идёт мимо туннеля, а
	// мимо туннеля ходит только то, что стоит в обоих списках.
	return s.sobratAdresaSet(n.Servery, n.Podpiska, s.adresaNaborov()...)
}

// kandidatySIsklyucheniem собирает адреса и НЕ падает из-за одного мёртвого
// хоста.
//
// Найдено живым прогоном 01.09.2026: в подписке из одиннадцати узлов один
// перестал резолвиться, и включение режима «весь трафик» отказало целиком.
// Решено: исключать с уведомлением.
//
// Дыры в правиле петли это не делает. Дыра появилась бы, если бы ядро потом
// зарезолвило имя ВНУТРИ туннеля и пошло к серверу через него же. Но к серверу,
// которого нет в DNS, ядро не пойдёт вовсе: имя не разрешится и у него.
//
// Пустой список кандидатов остаётся отказом: правило петли без единого адреса
// это запертая машина без выхода к собственному серверу.
func (s *Sluzhba) kandidatySIsklyucheniem() ([]netip.Addr, error) {
	kandidaty, err := s.sobratAdresa()
	if err == nil {
		return kandidaty, nil
	}

	var oshib *set.OshibkaRazresheniya
	if !errors.As(err, &oshib) {
		return nil, fmt.Errorf("адреса кандидатов не собраны: %w", err)
	}
	if len(kandidaty) == 0 {
		// Не разрешилось НИ ОДНО имя. Прежде это уезжало голой ошибкой и
		// доезжало до firewall-failed, то есть человек шёл чинить netsh при
		// исправном netsh. Причина другая и действие другое: молчит резолвер.
		//
		// Оба %w намеренно: вызывающему нужен новый признак, а прежний судья
		// исключения одного хоста по-прежнему спрашивает ErrImyaNeRazreshilos.
		return nil, fmt.Errorf("%w: %w", ErrRezolverMolchit, err)
	}

	// Молча выкинуть сервер значит оставить человека с подпиской, которая тихо
	// стала короче.
	log.Printf("исключены неразрешившиеся адреса: %s (%s)",
		strings.Join(oshib.Imena, ", "), protokol.KodNameResolveFailed)
	s.izvestit("serversUnreachable", map[string]any{
		"kod":   protokol.KodNameResolveFailed,
		"imena": oshib.Imena,
		"tekst": "эти адреса не разрешились и исключены из правил: " + strings.Join(oshib.Imena, ", "),
	})
	return kandidaty, nil
}

// ErrRezolverMolchit значит «резолвер не ответил ни по одному имени», и это НЕ
// то же самое, что ErrImyaNeRazreshilos с непустым остатком.
//
// Различие ведёт человека в разные стороны: одно имя из одиннадцати это повод
// смотреть на сервер (name-resolve-failed), ни одного это повод смотреть на
// сеть и DNS (dns-resolve-failed). Сегодня второе выглядело как поломка
// брандмауэра, потому что кода на проводе не производил никто.
var ErrRezolverMolchit = errors.New("резолвер не ответил ни по одному имени")
