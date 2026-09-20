package main

import (
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// След состава подписок в журнале.
//
// Судьи здесь стерегут не поведение продукта, а его НАБЛЮДАЕМОСТЬ. Разбор
// жалобы 20.09.2026 («подписка возвращается после каждого обновления») трижды
// кончился ничем именно потому, что все три починки набора молчаливые: состав
// менялся без команды и без строки. Диагностика, которую можно молча выключить
// следующей правкой, не диагностика, поэтому она под тестом наравне с продуктом.

// Перенос старого одиночного поля добавляет запись в список. Это единственный
// путь, которым запись появляется БЕЗ команды человека, и он обязан называть
// себя: иначе «подписка вернулась сама» неотличимо от «её кто-то добавил».
func TestPerenosStarogoPolyaNazyvaetSebya(t *testing.T) {
	n := Nabor{Podpiska: "https://panel.example.net/sub/tok"}

	pochinki := n.PrivestiPodpiski()

	if len(pochinki) == 0 {
		t.Fatal("перенос старого поля прошёл молча")
	}
	if !strings.Contains(pochinki[0], "panel.example.net") {
		t.Errorf("починка не назвала узел: %q", pochinki[0])
	}
	if strings.Contains(strings.Join(pochinki, " "), "/sub/tok") {
		t.Errorf("в журнал уехал ПУТЬ подписки, а это пропуск к ключам: %q", pochinki)
	}
}

// Набор, который уже в порядке, не должен давать ни одной строки: иначе сито в
// skazatRedko станет единственным, что отделяет журнал от мусора, а чтение
// набора идёт на каждую команду интерфейса.
func TestIspravnyyNaborMolchit(t *testing.T) {
	n := Nabor{
		Podpiski:  []ZapisPodpiski{{Id: "aaa", Adres: "https://panel.example.net/sub"}},
		Aktivnaya: "aaa",
	}

	if pochinki := n.PrivestiPodpiski(); len(pochinki) != 0 {
		t.Errorf("исправный набор дал строки в журнал: %q", pochinki)
	}
}

// Активная, указывающая в пустоту, чинится взятием первой записи. При этом
// ключи НЕ перекладываются, то есть рабочий список остаётся от прежней
// активной, и на экране чужая подписка выглядит выбранной человеком.
//
// Это ровно та картина, которую владелец видел после обновления, и она обязана
// быть в журнале целиком: и сам факт починки, и предупреждение про ключи.
func TestSlomannayaAktivnayaNazyvaetSebyaIGovoritProKlyuchi(t *testing.T) {
	n := Nabor{
		Servery: []protokol.Server{{Id: "s1", IzPodpiski: true}},
		Podpiski: []ZapisPodpiski{
			{Id: "pervaya", Adres: "https://pervaya.example.net/sub"},
			{Id: "vtoraya", Adres: "https://vtoraya.example.net/sub"},
		},
		Aktivnaya: "takoy-zapisi-net",
	}

	pochinki := n.PrivestiPodpiski()

	if len(pochinki) == 0 {
		t.Fatal("подмена активной прошла молча")
	}
	stroka := strings.Join(pochinki, " ")
	if !strings.Contains(stroka, "pervaya.example.net") {
		t.Errorf("починка не назвала новую активную: %q", stroka)
	}
	if !strings.Contains(stroka, "ключи НЕ переложены") {
		t.Errorf("починка молчит про неперекладку ключей: %q", stroka)
	}
	if n.Aktivnaya != "pervaya" {
		t.Errorf("активной стала %q, а ждали первую", n.Aktivnaya)
	}
	if len(n.Servery) != 1 {
		t.Errorf("рабочий список тронут: %d записей вместо 1", len(n.Servery))
	}
}

// Описание состава идёт УЗЛАМИ. Адрес подписки это секрет класса ключа, а
// журнал службы читают глазами и прикладывают к отчётам.
func TestOpisanieSostavaNeNesyotAdresa(t *testing.T) {
	n := Nabor{
		Podpiski: []ZapisPodpiski{
			{Id: "a", Adres: "https://panel.example.net/sub/SEKRETNYY-TOKEN", Servery: nil},
			{Id: "b", Adres: "https://zapas.example.net/sub/VTOROY-TOKEN",
				Servery: []protokol.Server{{Id: "s1"}, {Id: "s2"}}},
		},
		Aktivnaya: "a",
	}

	opisanie := opisatPodpiski(n)

	if strings.Contains(opisanie, "TOKEN") {
		t.Fatalf("в описание уехал путь подписки: %q", opisanie)
	}
	if !strings.Contains(opisanie, "panel.example.net(активная") {
		t.Errorf("активная не помечена: %q", opisanie)
	}
	if !strings.Contains(opisanie, "zapas.example.net(запас, ключей 2)") {
		t.Errorf("запасная описана неверно: %q", opisanie)
	}
}

// Сравнение составов ловит и появление записи, и исчезновение, и подмену одной
// на другую при том же их числе. Последнее важнее всего: список из двух записей
// легко перепутать с тем же списком, в котором одна запись другая.
func TestSravnenieSostavovLovitPodmenu(t *testing.T) {
	do := Nabor{Podpiski: []ZapisPodpiski{{Id: "a"}, {Id: "b"}}}
	posle := Nabor{Podpiski: []ZapisPodpiski{{Id: "a"}, {Id: "c"}}}

	if sostavySovpadayut(sostavPodpisok(do), sostavPodpisok(posle)) {
		t.Error("подмена одной записи на другую сочтена совпадением")
	}
	if !sostavySovpadayut(sostavPodpisok(do), sostavPodpisok(do)) {
		t.Error("один и тот же состав сочтён разным")
	}
	// Порядок записей в наборе не меняет состава: список сортируется. Иначе
	// строка в журнале появлялась бы от любой перестановки.
	pereshtasovan := Nabor{Podpiski: []ZapisPodpiski{{Id: "b"}, {Id: "a"}}}
	if !sostavySovpadayut(sostavPodpisok(do), sostavPodpisok(pereshtasovan)) {
		t.Error("перестановка записей сочтена изменением состава")
	}
}
