package main

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Ф1 от 05.09.2026. Окно закрывается крестиком в трей, значит «Выход» это уже
// не «свернуть», а «я закончил». Пока режим «весь трафик» включён, закончить
// по-настоящему нельзя: замок остаётся на машине, и человек узнаёт об этом
// только тогда, когда служба почему-либо не вернётся.
//
// Дверь обязана НАЗЫВАТЬ то, что сделает. Строка «Выход», снимающая защиту
// молча, хуже отсутствия двери: человек нажал одно, получил другое.
func TestVyhodIzTreyaNazyvaetSnyatieZashchity(t *testing.T) {
	// Туннель опущен у обоих вызовов намеренно: здесь проверяется РОВНО
	// измерение замка, а опускание туннеля у него своя проверка рядом.
	podpisBez, snyatBez, _ := punktVyhoda(protokol.SostVyklyuchen, false)
	if snyatBez {
		t.Error("режим выключен, а выход собрался его снимать: снимать нечего")
	}
	if podpisBez != "Выход" {
		t.Errorf("подпись без режима %q, ожидалось «Выход»", podpisBez)
	}

	podpisS, snyatS, _ := punktVyhoda(protokol.SostVyklyuchen, true)
	if !snyatS {
		t.Error("режим включён, а выход его не снимает: человек уходит, оставляя машину запертой")
	}
	if podpisS == podpisBez {
		t.Errorf("подпись одна и та же (%q) при разном поведении: нажатие обещает не то,"+
			" что делает", podpisS)
	}
	if !strings.Contains(strings.ToLower(podpisS), "защит") {
		t.Errorf("подпись %q не называет снятие защиты: человек не узнает, что именно"+
			" отключил", podpisS)
	}
	// Правило интерфейса: точек в конце нет.
	if strings.HasSuffix(podpisS, ".") || strings.HasSuffix(podpisBez, ".") {
		t.Error("точка в конце пункта меню")
	}
}
