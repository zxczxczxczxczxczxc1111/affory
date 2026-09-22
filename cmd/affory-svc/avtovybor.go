package main

import (
	"errors"
	"slices"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Область автовыбора (A5, 22.09.2026).
//
// «Авто» перебирало ВСЕ серверы набора, и область эта нигде не называлась. На
// экране человек видел список серверов, а из чего именно выбирает автомат, не
// говорил ни один экран: политика была, объяснения не было.
//
// Область такая: серверы АКТИВНОЙ подписки и добавленные вручную. Серверы
// запасных подписок в неё не входят и войти не могут - их ключи вообще не
// доезжают до конфига ядра, потому что активная подписка ровно одна (см. шапку
// podpiski.go). Это не недоработка, а решение: сложить ключи двух панелей в
// один список значит показать человеку два одноимённых узла с разными ключами.
//
// Внутри области человек теперь распоряжается сам: сервер можно убрать из
// автовыбора и вернуть. Выбрать его руками по-прежнему можно - убранный уходит
// из группы urltest, а не из списка.

var errPoslednyyVAvto = errors.New("это последний сервер автовыбора: без него автомату не из чего выбирать")
var errServerNeNayden = errors.New("сервер не найден")

// UchastvuetVAvto отвечает, входит ли сервер в область автовыбора.
func (n Nabor) UchastvuetVAvto(id string) bool {
	return !slices.Contains(n.VneAvto, id)
}

// AvtoKandidaty это серверы, из которых выбирает автомат.
//
// Пустой ответ невозможен по построению: см. ZadatUchastieVAvto, а на случай
// набора, испорченного мимо него, страхует сам генератор (gruppy).
func (n Nabor) AvtoKandidaty() []protokol.Server {
	out := make([]protokol.Server, 0, len(n.Servery))
	for _, s := range n.Servery {
		if n.UchastvuetVAvto(s.Id) {
			out = append(out, s)
		}
	}
	return out
}

// ZadatUchastieVAvto включает сервер в область или убирает из неё.
//
// Последнего убрать нельзя, и отказ здесь честнее тихого «ладно»: пустая группа
// urltest это конфиг, который ядро отвергнет целиком, то есть человек нажал бы
// на переключатель и получил неработающий VPN.
func (n *Nabor) ZadatUchastieVAvto(id string, uchastvuet bool) error {
	if !slices.ContainsFunc(n.Servery, func(s protokol.Server) bool { return s.Id == id }) {
		return errServerNeNayden
	}
	if uchastvuet {
		n.VneAvto = slices.DeleteFunc(n.VneAvto, func(v string) bool { return v == id })
		return nil
	}
	if !n.UchastvuetVAvto(id) {
		return nil
	}
	if len(n.AvtoKandidaty()) <= 1 {
		return errPoslednyyVAvto
	}
	n.VneAvto = append(n.VneAvto, id)
	return nil
}

// PrivestiAvtovybor выбрасывает из списка исключений то, чего в наборе уже нет.
//
// Зовётся на каждом чтении набора рядом с PrivestiPodpiski и обязана быть
// идемпотентной по той же причине. Без неё список исключений рос бы вечно:
// подписка меняет состав серверов раз в 12 часов, и снятая запись оставляла бы
// за собой строку, а через полгода таких строк сотни - и одна из них однажды
// совпала бы с идентификатором нового сервера.
func (n *Nabor) PrivestiAvtovybor() {
	if len(n.VneAvto) == 0 {
		return
	}
	n.VneAvto = slices.DeleteFunc(n.VneAvto, func(v string) bool {
		return !slices.ContainsFunc(n.Servery, func(s protokol.Server) bool { return s.Id == v })
	})
	if len(n.VneAvto) == 0 {
		n.VneAvto = nil
	}
}

// setAutoMember это команда: убрать сервер из автовыбора или вернуть.
//
// Новый состав едет в ядро СЛЕДУЮЩИМ подъёмом, а не сейчас. Состав группы
// urltest задаётся конфигом, и менять его на живом ядре нечем: clash_api умеет
// выбрать другого кандидата, но не переписать список. Рвать рабочее соединение
// ради настройки, которая нужна автомату завтра, значит повторить B1.
func (s *Sluzhba) setAutoMember(id string, uchastvuet bool) error {
	return s.pravitNabor(func(n *Nabor) error {
		return n.ZadatUchastieVAvto(id, uchastvuet)
	})
}
