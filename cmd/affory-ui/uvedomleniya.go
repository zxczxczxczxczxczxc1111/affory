package main

import (
	"log"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Всплывающие уведомления из трея (план «шесть удобств» §4). Смысл ровно в
// том, что происходит БЕЗ команды человека: ядро в авто-режиме сменило
// несущего, туннель перестал нести, туннель вернулся. Подъём по кнопке не
// уведомление: человек смотрит на окно. Отключение человеком тоже.

type Uvedomlenie struct {
	Zagolovok, Tekst string
}

// chtoSoobshchit решает по паре состояний. Чистая функция, чтобы правила
// проверялись таблицей, а не запуском Windows.
func chtoSoobshchit(pred, nov protokol.StatusOtvet) (Uvedomlenie, bool) {
	prichina := ""
	if nov.Oshib != nil {
		prichina = nov.Oshib.Tekst
	}
	// Обновление показывается один раз на версию, в любом состоянии туннеля.
	if nov.Obnovlenie != nil && (pred.Obnovlenie == nil || pred.Obnovlenie.Versiya != nov.Obnovlenie.Versiya) {
		return Uvedomlenie{"есть обновление " + nov.Obnovlenie.Versiya, "установить можно в настройках"}, true
	}
	bylPodnyat := pred.Sostoyanie == protokol.SostPodnyat
	bylaAvariya := pred.Sostoyanie == protokol.SostNeNeset || pred.Sostoyanie == protokol.SostVosstanavl || pred.Sostoyanie == protokol.SostOtkaz
	switch nov.Sostoyanie {
	case protokol.SostPodnyat:
		if bylaAvariya {
			return Uvedomlenie{"туннель восстановлен", "несёт " + nov.NesushchiyImya}, true
		}
		if bylPodnyat && pred.NesushchiyId != "" && nov.NesushchiyId != "" && pred.NesushchiyId != nov.NesushchiyId {
			return Uvedomlenie{"несёт " + nov.NesushchiyImya, "ядро переключило сервер"}, true
		}
	case protokol.SostNeNeset:
		if pred.Sostoyanie != protokol.SostNeNeset {
			return Uvedomlenie{"туннель не несёт", prichina}, true
		}
	case protokol.SostOtkaz:
		if pred.Sostoyanie != protokol.SostOtkaz {
			return Uvedomlenie{"подключиться не удалось", prichina}, true
		}
	}
	return Uvedomlenie{}, false
}

// Uvedomlyatel помнит прошлое состояние и показывает toast через службу
// уведомлений Wails. Отказ показа это строка в журнале: уведомление
// вторично, состояние на экране первично.
type Uvedomlyatel struct {
	sluzhba *notifications.NotificationService
	mu      sync.Mutex
	pred    protokol.StatusOtvet
	nomer   int
}

func novyyUvedomlyatel(sluzhba *notifications.NotificationService) *Uvedomlyatel {
	return &Uvedomlyatel{sluzhba: sluzhba, pred: protokol.StatusOtvet{Sostoyanie: protokol.SostSluzhbaMolchit}}
}

// Prinyat получает каждое состояние, как трей.
func (u *Uvedomlyatel) Prinyat(st protokol.StatusOtvet) {
	u.mu.Lock()
	pred := u.pred
	u.pred = st
	u.nomer++
	nomer := u.nomer
	u.mu.Unlock()
	uv, est := chtoSoobshchit(pred, st)
	if !est || u.sluzhba == nil {
		return
	}
	go func() {
		if err := u.sluzhba.SendNotification(notifications.NotificationOptions{
			ID: "affory-" + itoa(nomer), Title: uv.Zagolovok, Body: uv.Tekst, ThreadID: "affory",
		}); err != nil {
			log.Printf("уведомление не показано: %v", err)
		}
	}()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
