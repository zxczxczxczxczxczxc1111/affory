package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Запасной сервер в ручном режиме (жалоба 17.10.2026).
//
// У сотрудника в ручном режиме перестал отвечать выбранный сервер: путь от
// него до сервера пропал на четыре минуты, сам сервер жил и вёз других.
// Наблюдатель опустил туннель, восстановление восемь раз подряд стучалось в
// тот же сервер, а живые серверы рядом не пробовались ни разу. Человек сидел
// без VPN, а окно писало «ни один сервер не отвечает», хотя спрошен был один.
//
// Теперь молчащий выбранный сервер временно подменяет группа авто, и подменяет
// ВСЛУХ: окно пишет, что выбранный не отвечает и VPN идёт через другой. Молча
// отдать человеку другую страну нельзя (см. navyazatVybor), вслух можно, это
// решение владельца 17.10.2026. Как только выбранный снова отвечает, служба
// возвращается на него сама.

// uspehovDoVozvrata: столько удачных замеров выбранного сервера подряд, прежде
// чем вернуться на него. Один удачный после пропажи пути ещё не значит, что
// путь вернулся: у сотрудника перед полной пропажей сервер час отвечал через
// раз.
const uspehovDoVozvrata = 2

// rezhimPodyoma это режим, по которому собирается конфиг и навязывается выбор
// ядру. Совпадает с режимом набора, кроме одного случая: ручной режим, а
// выбранный сервер не отвечает и временно подменён группой авто.
//
// Режим набора при этом не меняется: человек выбирал ручной, и окно
// показывает ручной.
func (s *Sluzhba) rezhimPodyoma(n Nabor) protokol.Rezhim {
	r := rezhimNabora(n)
	if r != protokol.RezhimRuchnoy {
		return r
	}
	id := s.zapasnoyDlya()
	if id == "" {
		return r
	}
	// Подмена относится к КОНКРЕТНОМУ выбору. Человек выбрал другой сервер или
	// подписка его унесла: подменять больше нечего.
	srv, err := n.VybrannyyServer()
	if err != nil || srv.Id != id {
		return r
	}
	return protokol.RezhimAvto
}

// zapasnoyDlya отвечает, какой выбранный сервер сейчас подменён. Пусто:
// никакой.
func (s *Sluzhba) zapasnoyDlya() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.zapasnoyVmesto
}

func (s *Sluzhba) postavitZapasnoy(srv protokol.Server) {
	s.mu.Lock()
	s.zapasnoyVmesto, s.zapasnoyVmestoImya, s.zapasnoyUspehov = srv.Id, srv.Imya, 0
	s.mu.Unlock()
}

// snyatZapasnoy забывает подмену и отвечает, была ли она.
func (s *Sluzhba) snyatZapasnoy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	bylo := s.zapasnoyVmesto != ""
	s.zapasnoyVmesto, s.zapasnoyVmestoImya, s.zapasnoyUspehov = "", "", 0
	return bylo
}

// estKemPodmenit отвечает, есть ли в группе авто хоть один сервер, кроме
// выбранного. Нет: подменять некем, и пробовать незачем.
//
// Убранный человеком из авто сервер подменой не считается: человек сам сказал,
// что автомату его брать нельзя.
func estKemPodmenit(n Nabor, vybrannyy string) bool {
	vne := make(map[string]bool, len(n.VneAvto))
	for _, id := range n.VneAvto {
		vne[id] = true
	}
	for _, srv := range n.Servery {
		if srv.Id != vybrannyy && !vne[srv.Id] {
			return true
		}
	}
	return false
}

// kogoPodmenit отвечает, какой выбранный сервер подменять, или false, если
// подменять нечего: режим не ручной, подмена уже идёт или подменить некем.
func (s *Sluzhba) kogoPodmenit() (protokol.Server, bool) {
	n, err := s.nabor()
	if err != nil {
		log.Printf("запасной сервер не пробую: набор не прочитан: %v", err)
		return protokol.Server{}, false
	}
	if rezhimNabora(n) != protokol.RezhimRuchnoy || s.zapasnoyDlya() != "" {
		return protokol.Server{}, false
	}
	srv, err := n.VybrannyyServer()
	if err != nil || !estKemPodmenit(n, srv.Id) {
		return protokol.Server{}, false
	}
	return srv, true
}

// pereytiNaZapasnoy подменяет в ЖИВОМ туннеле молчащий выбранный сервер
// группой авто и отвечает, понёс ли туннель трафик после подмены.
//
// Не понёс: выбор возвращается на выбранный сервер, и наблюдатель опускает
// туннель как прежде. Отвечать «понёс» на непроверенное нельзя: наблюдатель
// тогда сбросил бы счёт провалов и оставил человека на мёртвом туннеле.
func (s *Sluzhba) pereytiNaZapasnoy(ctx context.Context, adres, sekret, teg string) bool {
	// Тот же замок, что у setServer и setRouteMode: человек, выбравший сервер
	// ровно в эту секунду, не должен получить подмену поверх своего выбора.
	s.muVybor.Lock()
	defer s.muVybor.Unlock()
	srv, ok := s.kogoPodmenit()
	if !ok {
		return false
	}
	nach := time.Now()
	if err := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); err != nil {
		log.Printf("запасной сервер: ядро не переключилось на группу авто: %v", err)
		return false
	}
	s.otmetitSmenuVybora(nach)
	// Группа авто держит свою прошлую пробу, а в ней молчащий сервер мог быть
	// лучшим. Перемер выбирает заново сразу.
	if _, err := s.peremeritAvto(ctx, adres, sekret); err != nil {
		log.Printf("запасной сервер: группа авто не перемерена: %v", err)
	}
	if _, err := s.zameritYadro(ctx, adres, sekret, teg, nach); err != nil {
		log.Printf("и через группу авто исходящий %s не несёт: %v", teg, err)
		vozvrat := time.Now()
		if e := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegKandidata(srv.Id)); e != nil {
			log.Printf("выбор не вернулся на %s после неудачной подмены: %v", srv.Id, e)
		}
		s.otmetitSmenuVybora(vozvrat)
		return false
	}
	s.postavitZapasnoy(srv)
	// Сторож поднимает упавшее ядро по файлу конфига: без этой записи он вернул
	// бы ядро на молчащий сервер.
	if err := s.perepisatVyborVKonfige(genkonfig.TegAvto); err != nil {
		log.Printf("подмена не записана в конфиг, перезапуск ядра вернёт выбранный сервер: %v", err)
	}
	log.Printf("выбранный сервер %s (%s) не отвечает, VPN временно идёт через группу авто", srv.Id, srv.Imya)
	s.uznatNesushchego(ctx, adres, sekret)
	s.izvestit("state", s.Status())
	return true
}

// uznatNesushchego спрашивает ядро, кто несёт трафик, сразу после смены выбора,
// а не следующим тиком наблюдателя: окно говорит про подмену, только когда
// несущий уже не выбранный, и узнавать это через тик значит промолчать тик.
func (s *Sluzhba) uznatNesushchego(ctx context.Context, adres, sekret string) {
	t, err := s.nesyot(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto)
	if err != nil {
		log.Printf("несущий после смены выбора не спрошен: %v", err)
		return
	}
	if id, ok := genkonfig.IdIzTega(t); ok {
		s.zapomnitNesushchego(id)
	}
}

// proveritVozvrat меряет подменённый выбранный сервер напрямую, через его
// собственный исходящий, и возвращается на него, когда тот ответил
// uspehovDoVozvrata раз подряд.
//
// Мерится живым ядром: выбранный сервер остаётся в нём исходящим и при
// подмене, поднимать для этого второе ядро незачем.
func (s *Sluzhba) proveritVozvrat(ctx context.Context, adres, sekret, teg string) {
	id := s.zapasnoyDlya()
	if id == "" {
		return
	}
	_, err := s.zamerit(ctx, adres, sekret, genkonfig.TegKandidata(id))
	s.mu.Lock()
	if s.zapasnoyVmesto != id {
		s.mu.Unlock()
		return
	}
	if err != nil {
		s.zapasnoyUspehov = 0
		s.mu.Unlock()
		return
	}
	s.zapasnoyUspehov++
	gotov := s.zapasnoyUspehov >= uspehovDoVozvrata
	s.mu.Unlock()
	if gotov {
		s.vernutsyaNaVybrannyy(ctx, adres, sekret, teg, id)
	}
}

// vernutsyaNaVybrannyy снимает подмену в живом туннеле. Трафик через
// выбранный сервер проверяется пробой, как при переключении человеком: замер
// задержки сервера ещё не значит, что через него пойдут данные.
func (s *Sluzhba) vernutsyaNaVybrannyy(ctx context.Context, adres, sekret, teg, id string) {
	s.muVybor.Lock()
	defer s.muVybor.Unlock()
	// Человек мог выбрать сервер сам, пока шёл замер: его выбор главнее.
	if s.zapasnoyDlya() != id {
		return
	}
	sbrosit := func() {
		s.mu.Lock()
		s.zapasnoyUspehov = 0
		s.mu.Unlock()
	}
	smena := time.Now()
	if err := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegKandidata(id)); err != nil {
		log.Printf("возврат на выбранный сервер %s не прошёл: %v", id, err)
		sbrosit()
		return
	}
	s.otmetitSmenuVybora(smena)
	if _, err := s.zameritYadro(ctx, adres, sekret, teg, smena); err != nil {
		log.Printf("выбранный сервер %s отвечал на замер, но трафик через него не пошёл, остаюсь на запасном: %v", id, err)
		vozvrat := time.Now()
		if e := s.postavitVybor(ctx, adres, sekret, genkonfig.TegSelector, genkonfig.TegAvto); e != nil {
			// Ядро осталось на выбранном. Наблюдатель увидит это следующей
			// пробой и поступит как с любым молчащим сервером.
			log.Printf("выбор не вернулся на группу авто: %v", e)
		}
		s.otmetitSmenuVybora(vozvrat)
		sbrosit()
		return
	}
	s.snyatZapasnoy()
	if err := s.perepisatVyborVKonfige(genkonfig.TegKandidata(id)); err != nil {
		log.Printf("возврат на выбранный сервер не записан в конфиг: %v", err)
	}
	log.Printf("выбранный сервер %s снова отвечает, VPN вернулся на него", id)
	s.uznatNesushchego(ctx, adres, sekret)
	s.izvestit("state", s.Status())
}

// zapasnoyDoTun пробует группу авто, когда в ручном режиме выбранный сервер не
// ответил на проверку до TUN. Это путь восстановления: туннеля нет, и
// подменять в живом ядре нечего.
//
// Отвечает, была ли вторая проверка, и её итог. Не было: подменять нечего, и
// у вызывающего остаётся прежний отказ.
func (s *Sluzhba) zapasnoyDoTun(moyo int, prichina error) (bool, error) {
	srv, ok := s.kogoPodmenit()
	if !ok {
		return false, nil
	}
	// Подмена ставится ДО проверки: конфиг проверки собирается по
	// rezhimPodyoma, и без неё вторая проверка спросила бы тот же сервер.
	s.postavitZapasnoy(srv)
	s.mu.Lock()
	if s.pokolenieP != moyo {
		s.mu.Unlock()
		s.snyatZapasnoy()
		return true, errPodyomOtmenyon
	}
	pctx, otm := context.WithCancel(context.Background())
	s.otmena = otm
	s.mu.Unlock()
	err := s.proveritServer(pctx)
	otm()
	if s.podyomOtmenyon(moyo) {
		s.snyatZapasnoy()
		return true, errPodyomOtmenyon
	}
	if err == nil || errors.Is(err, errProverkaNeSostoyalas) {
		log.Printf("выбранный сервер %s (%s) не отвечает (%v), поднимаю VPN через группу авто", srv.Id, srv.Imya, prichina)
		return true, nil
	}
	s.snyatZapasnoy()
	return true, err
}
