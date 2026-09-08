package petlya_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/test/petlya"
)

// Мишень отвечает СВОИМ именем, и это весь её смысл.
//
// Сегодняшняя приёмка доказывает «трафик прошёл» запросом в api.ipify.org, то
// есть ставит вердикт о продукте в зависимость от чужого сайта и домашней сети.
// Хуже того, смену несущего она доказывает сменой ВНЕШНЕГО адреса, а у всех
// наших входов адрес один, и судья переключения из-за этого печатает НЕГОДЕН с
// 0.7.0. Мишень с именем снимает оба вопроса: две мишени за двумя серверами
// отвечают по-разному, и переключение видно без всякого интернета.
func TestMishenNazyvaetSebya(t *testing.T) {
	m := petlya.NovayaMishen(t, "servak-A")

	otvet, err := http.Get(m.Adres)
	if err != nil {
		t.Fatalf("мишень не ответила: %v", err)
	}
	defer otvet.Body.Close()
	telo, err := io.ReadAll(otvet.Body)
	if err != nil {
		t.Fatalf("тело не прочиталось: %v", err)
	}
	if string(telo) != "servak-A" {
		t.Fatalf("мишень назвалась %q, ждали %q", telo, "servak-A")
	}
}

// Две мишени обязаны различаться: на этом стоит вся проверка переключения.
func TestDveMisheniRazlichayutsya(t *testing.T) {
	a := petlya.NovayaMishen(t, "servak-A")
	b := petlya.NovayaMishen(t, "servak-B")
	if a.Adres == b.Adres {
		t.Fatalf("обе мишени сели на один адрес %s: различить сервера будет нечем", a.Adres)
	}
	if petlya.SprositImya(t, a.Adres) == petlya.SprositImya(t, b.Adres) {
		t.Fatal("мишени назвались одинаково: переключение доказывать нечем")
	}
}
