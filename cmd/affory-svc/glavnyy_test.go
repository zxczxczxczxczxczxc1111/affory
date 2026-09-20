package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// Каталог данных уводится в сторону на ВЕСЬ прогон пакета, до первого теста.
//
// Конструктор службы подключает настоящее хранилище секретов
// (`hranenie.Novyy()` по `sostoyanie.KatalogDannyh()`), и фикстура, забывшая
// подменить шов записи, пишет в блоб ЖИВОЙ машины. 20.09.2026 так и случилось:
// `TestZhurnalKomandNePishetTel` слал `setSubscription`, адрес
// `panel.example/sub/tokenchik-sekretnyy` ложился в набор владельца второй
// записью и активной, и после каждой сборки выпуска он видел у себя тестовую
// подписку, взявшуюся ниоткуда. Разбор упирался в то, что команды в журнале
// службы нет: тест зовёт `Obrabotat` напрямую, минуя канал.
//
// Подмена здесь, а не в каждой фикстуре: фикстур много, забыть можно в любой,
// и цена забывчивости — правка чужого рабочего хранилища.
func TestMain(m *testing.M) {
	vrem, err := os.MkdirTemp("", "affory-testy-")
	if err != nil {
		panic("каталог для тестов не заведён: " + err.Error())
	}
	sostoyanie.PodmenitKatalogDannyh(vrem)
	kod := m.Run()
	_ = os.RemoveAll(vrem)
	os.Exit(kod)
}

// Заслон: прогон обязан идти МИМО настоящего каталога данных.
//
// Судья смотрит не на переменную, а на путь, по которому хранилище секретов
// действительно пишет: подмена, снятая по невнимательности, вернёт продукту
// путь живой машины, и следующая фикстура без шва снова перепишет чужой набор.
func TestProgonNeTrogaetZhivoyKatalogDannyh(t *testing.T) {
	nash := sostoyanie.KatalogDannyh()
	if nash == filepath.Join(`C:\ProgramData`, "Affory") {
		t.Fatal("тесты идут по НАСТОЯЩЕМУ каталогу данных: любая фикстура без " +
			"подменённого шва перепишет блоб живой машины")
	}
	if _, err := os.Stat(nash); err != nil {
		t.Fatalf("подменённый каталог данных недоступен: %v", err)
	}
	// Хранилище секретов обязано смотреть туда же: путь оно берёт в момент
	// создания, и подмена, случившаяся позже конструктора, ничего не спасёт.
	hr := hranenie.Novyy()
	if err := hr.Sohranit([]byte(`{"servery":[]}`)); err != nil {
		t.Fatalf("запись в подменённое хранилище не прошла: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nash, hranenie.ImyaSekretov)); err != nil {
		t.Fatalf("хранилище пишет НЕ в подменённый каталог: %v", err)
	}
}
