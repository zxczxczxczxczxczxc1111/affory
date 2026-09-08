package genkonfig

import (
	"errors"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Транспорт, которого ядро больше не несёт, не имеет права уносить с собой
// ВЕСЬ конфиг.
//
// Отказ от xhttp снимает с клиента чужой форк ядра (решение владельца
// 06.09.2026), но записи с этим транспортом у людей уже сохранены. Сборка
// конфига идёт одним куском: `SingBox` возвращает на ПЕРВОЙ негодной записи,
// и одна такая запись означала бы клиент, который не поднимается никогда и
// объясняет это словами про исходящий номер ноль.
//
// Ровно этот класс уже покупали 02.09.2026, когда один сервер чужой подписки
// из тридцати девяти уносил все остальные, а наружу это приезжало как
// «TUN-адаптер не появился». Второй раз платить незачем.
func TestNeizvestnyyTransportKandidataPropuskaetsya(t *testing.T) {
	v := obraztsovyyVhod()
	staryy := v.Server
	staryy.Id = "staraya-zapis-xhttp"
	staryy.Transport = "xhttp"
	// Выбран ЖИВОЙ сервер, негодный лежит рядом. Это обычный случай после
	// обновления: человек ничего не менял, запись просто осталась.
	v.Servery = []protokol.Server{v.Server, staryy}

	b, err := SingBox(v)
	if err != nil {
		t.Fatalf("конфиг не собрался из-за соседней негодной записи: %v", err)
	}
	tekst := string(b)
	if strings.Contains(tekst, staryy.Id) {
		t.Errorf("негодная запись %s попала в конфиг", staryy.Id)
	}
	if !strings.Contains(tekst, TegKandidata(v.Server.Id)) {
		t.Errorf("живой кандидат из конфига пропал вместе с негодным")
	}
}

// Пропуск не превращается в молчание, когда пропускать больше нечего.
//
// Пустой список кандидатов дал бы селектор без единого исходящего: ядро такое
// принимает, туннель не несёт, а человеку сказать нечего. Отказ обязан
// назвать причину здесь, а не там.
func TestVseKandidatyNeizvestny(t *testing.T) {
	v := obraztsovyyVhod()
	v.Server.Transport = "xhttp"
	v.Servery = []protokol.Server{v.Server}

	if _, err := SingBox(v); !errors.Is(err, ErrTransport) {
		t.Fatalf("ожидался ErrTransport, получено: %v", err)
	}
}

// Выбранный человеком сервер молча подменять нельзя.
//
// Если несущим станет соседняя запись, экран покажет одно, а трафик пойдёт
// другим путём. Это хуже отказа: отказ видно.
func TestVybrannyyNeizvestnyyEtoOtkaz(t *testing.T) {
	v := obraztsovyyVhod()
	zhivoy := v.Server
	zhivoy.Id = "zhivoy"
	v.Server.Transport = "xhttp"
	v.Servery = []protokol.Server{v.Server, zhivoy}

	if _, err := SingBox(v); !errors.Is(err, ErrTransport) {
		t.Fatalf("ожидался ErrTransport на выбранном сервере, получено: %v", err)
	}
}

// Реестр это единственный судья, и xhttp в нём больше нет.
func TestXhttpUshyolIzReestra(t *testing.T) {
	if Izvestnyy("xhttp") {
		t.Error("xhttp всё ещё числится известным транспортом")
	}
	for _, tr := range VseTransporty() {
		if tr == "xhttp" {
			t.Error("xhttp всё ещё в списке VseTransporty")
		}
	}
}
