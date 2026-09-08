package petlya_test

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/test/petlya"
)

// Самый простой случай: ядро поднято, трафик через него доходит до мишени.
//
// Проверяется каркас, а не транспорт: вход mixed, выход direct, между ними
// ничего. Если этот тест красный, все транспортные тесты ниже врут одинаково.
func TestYadroNesyotPryamo(t *testing.T) {
	petlya.Storozhit(t)
	m := petlya.NovayaMishen(t, "pryamo")

	y := petlya.PodnyatYadro(t, map[string]any{
		"log":       map[string]any{"level": "error"},
		"inbounds":  []any{map[string]any{"type": "mixed", "tag": "vhod", "listen": "127.0.0.1", "listen_port": petlya.SvobodnyyPort(t)}},
		"outbounds": []any{map[string]any{"type": "direct"}},
	})

	if imya := y.SprositCherezProksi(t, m.Adres); imya != "pryamo" {
		t.Fatalf("через ядро пришло %q, а мишень зовут %q", imya, "pryamo")
	}
}

// Ядро, не поднявшееся на битом конфиге, обязано сказать ПОЧЕМУ.
//
// Иначе первая же опечатка в конфиге выглядит как «сервер не отвечает», и час
// уходит на поиск сервера, который ни при чём. 07.09.2026 ровно это стоило
// целого опыта: конфиг был записан с сигнатурой UTF-8, ядро ответило
// `invalid character`, а выглядело оно как молчащий сервер.
func TestBityyKonfigZhaluetsyaVnyatno(t *testing.T) {
	oshibka := petlya.PodnyatYadroSOshibkoy(t, map[string]any{
		"inbounds": []any{map[string]any{"type": "vydumannyy-vhod"}},
	})
	if oshibka == "" {
		t.Fatal("ядро проглотило выдуманный тип входящего: контроль ничего не меряет")
	}
	t.Logf("ядро отказалось внятно: %s", oshibka)
}
