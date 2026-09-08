package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Наблюдатель обязан ЗАКОНЧИТЬСЯ к моменту возврата из Disconnect, а не когда-то
// после.
//
// Найдено детектором гонок, который на этом проекте не запускался ни разу.
// Горутина наблюдателя переживала свой тест и читала periodNablyudeniya, пока
// следующий тест его писал. Гонка в тестах была симптомом, болезнь в продукте:
// Disconnect отменял контекст и уходил, не дожидаясь никого.
//
// Проверка написана ровно тем же приёмом, каким дефект нашёлся: после возврата
// из Disconnect тест ПИШЕТ ту переменную, которую читает наблюдатель. Живая
// горутина превращает это в гонку, и прогон с -race краснеет. Без -race тест
// зелёный всегда, поэтому детектор гонок обязателен в воротах: без него эта
// проверка не проверяет ничего.
func TestDisconnectDozhidaetsyaNablyudatelya(t *testing.T) {
	s := podstavnaya(t, nil)
	rabotal := make(chan struct{})
	var odin atomic.Bool
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if !odin.CompareAndSwap(false, true) {
			select {
			case <-rabotal:
			default:
				close(rabotal)
			}
		}
		return time.Millisecond, nil
	}
	s.period = time.Millisecond

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	<-rabotal // наблюдатель точно живой, иначе проверять нечего

	s.Disconnect()

	// Запись в переменную, которую читает наблюдатель. Если он ещё жив, -race
	// назовёт это гонкой и тест провалится.
	s.period = 2 * time.Millisecond
}

// Ручное отключение НЕ ДОЛЖНО отменяться наблюдателем.
//
// Сценарий: человек жмёт «отключить», наблюдатель в этот момент сидит ВНУТРИ
// замера. Замер падает, потому что туннель как раз опускают. Наблюдатель
// считает это аварией, ставит ne-neset поверх выключенного и запускает
// восстановление, которое видит ne-neset и честно подключается обратно. Кнопка
// «отключить» перестаёт работать, а комментарий рядом с кодом обещает ровно
// обратное: «осознанное отключение человеком не переподключает никогда».
//
// Момент ловится точно, а не выжиданием: замер сообщает, что вошёл, и ждёт
// разрешения выйти. Disconnect зовётся между этими двумя точками.
func TestRuchnoeOtklyuchenieNeOtmenyaetsyaNablyudatelem(t *testing.T) {
	s := podstavnaya(t, nil)
	var podnyatiy atomic.Int32
	voshyol := make(chan struct{})
	otpustit := make(chan struct{})
	var pervyy, soobshchil atomic.Bool

	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if pervyy.CompareAndSwap(false, true) {
			podnyatiy.Add(1)
			return time.Millisecond, nil
		}
		if soobshchil.CompareAndSwap(false, true) {
			close(voshyol)
			<-otpustit
		}
		return 0, errors.New("исходящий не отвечает (код 504)")
	}
	s.period = time.Millisecond
	s.provalov = 1

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}

	<-voshyol // наблюдатель внутри замера
	go func() {
		// Отпускаем замер уже ПОСЛЕ того, как Disconnect начал отменять
		// контекст: это и есть спорный момент, ради которого написан тест.
		time.Sleep(20 * time.Millisecond)
		close(otpustit)
	}()
	s.Disconnect()

	do := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(do) {
		if sost := s.Status().Sostoyanie; sost != protokol.SostVyklyuchen {
			t.Fatalf("ручное отключение отменено: состояние стало %s", sost)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := podnyatiy.Load(); n != 1 {
		t.Fatalf("туннель поднимался %d раза вместо одного", n)
	}
}

// Два подъёма одновременно обязаны дать РОВНО ОДИН туннель.
//
// Состояние проверялось под мьютексом, мьютекс отпускался, и только потом
// начиналась работа. Человек жмёт «подключить» ровно тогда, когда сработало
// автовосстановление: обе горутины видят vyklyuchen, обе идут поднимать. Два
// ядра, два TUN-адаптера, два набора правил брандмауэра, и кто из них потом
// опустится по Disconnect, не определено.
//
// До автовосстановления Connect звался только из канала, то есть
// последовательно. Второй вызывающий появился позже, и однопоточное допущение
// молча перестало выполняться, хотя сам код никто не трогал.
func TestDvaOdnovremennyhPodyomaDayutOdinTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	var podyomov atomic.Int32
	prezhniy := s.podnyatTunnel
	s.podnyatTunnel = func(ctx context.Context) (set.Adapter, error) {
		podyomov.Add(1)
		// Задержка изображает настоящий подъём: ядро стартует не мгновенно, и
		// именно в этом окне второй вызывающий успевает пройти проверку.
		time.Sleep(30 * time.Millisecond)
		return prezhniy(ctx)
	}

	start := make(chan struct{})
	gotovy := make(chan struct{})
	var udach atomic.Int32
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			if err := s.Connect(context.Background()); err == nil {
				udach.Add(1)
			}
			gotovy <- struct{}{}
		}()
	}
	close(start)
	// Сторожевой таймер обязателен: при затирании s.otmena первый наблюдатель
	// не отменяется никогда, Disconnect виснет на ожидании, и тест не падает, а
	// висит. Повисший тест это худшая форма красного: в воротах он выглядит как
	// сломанный прогон, а не как найденный дефект.
	for i := 0; i < 2; i++ {
		select {
		case <-gotovy:
		case <-time.After(10 * time.Second):
			t.Fatal("подъём не вернулся за 10 с: похоже, отмена первого наблюдателя потеряна")
		}
	}

	if n := podyomov.Load(); n != 1 {
		t.Fatalf("туннель поднимался %d раз: два ядра и два набора правил брандмауэра", n)
	}
	// Оба вызывающих отвечают успехом, а туннель поднялся ОДИН. С Б1 второй не
	// отказывает: он попросил подключения и получил его, поднимал сам или нет.
	// Отказом наружу это выходило как баннер поверх работающего туннеля, причём
	// баннер выдуманный (all-servers-down), потому что кода у такой ошибки нет.
	if n := udach.Load(); n != 2 {
		t.Fatalf("успешных ответов %d, ожидали два при одном подъёме", n)
	}
}

// Файл состояния после удачного подъёма обязан нести ВСЁ, что понадобится
// восстановлению после нештатной смерти службы.
//
// Три записи шли независимо и полными снимками, поэтому каждая следующая
// затирала предыдущую. Последняя из них сидит на УСПЕШНОМ пути и стирала порт
// clash_api, секрет и адаптер, записанные подъёмом туннеля. Живой файл это
// подтверждал: port_clash и adapter_tun отсутствовали, зато лежало мёртвое
// port_socks. Восстановление после такого не знает ни у кого спрашивать, ни
// какой адаптер за собой убирать.
func TestFaylSostoyaniyaNeZatiraetSamSebya(t *testing.T) {
	s := podstavnaya(t, nil)
	var posledniy sostoyanie.SostoyanieFayla
	var mu sync.Mutex
	s.zapisat = func(f sostoyanie.SostoyanieFayla) error {
		mu.Lock()
		defer mu.Unlock()
		posledniy = f
		return nil
	}

	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	mu.Lock()
	f := posledniy
	mu.Unlock()

	if f.Sostoyanie != protokol.SostPodnyat {
		t.Errorf("в файле состояние %s, а туннель поднят", f.Sostoyanie)
	}
	if f.PortClash == 0 {
		t.Error("порт clash_api затёрт: спрашивать живое ядро будет нечем")
	}
	if f.SekretClash == "" {
		t.Error("секрет clash_api затёрт")
	}
	if f.AdapterTun == "" || f.IndeksTun == 0 {
		t.Error("адаптер затёрт: убрать за собой после смерти службы будет нечего")
	}
	if f.ConnectNach == nil || f.ProbaPervaya == nil {
		t.Error("отметки замеров затёрты: пороги приёмки считать не от чего")
	}
}
