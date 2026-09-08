package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// idProby это идентификатор, который addServer выведет из ssylkaProby.
// Считается тем же способом, что и в продукте: гвоздь на двенадцать знаков
// хеша сломался бы на первой же смене правила.
var idProby = ssylki.Id("203.0.113.9", 8443, "reality-tcp")

func soderzhitServer(n Nabor, id string) bool {
	for _, srv := range n.Servery {
		if srv.Id == id {
			return true
		}
	}
	return false
}

// hranilishcheProby подставляет хранилище, устроенное как настоящее: один блоб,
// сериализация на запись и разбор на чтение.
//
// Фикстура podstavnaya держит НАБОР структурой и отдаёт его копией, а копия
// структуры делит с оригиналом массив под срезом серверов. Две команды в двух
// горутинах ловили бы на этом гонку самой фикстуры, а не продукта. Настоящее
// хранилище (internal/hranenie) отдаёт байты, и naborIzHranilishcha разбирает
// их в свежий набор на каждом чтении.
func hranilishcheProby(t *testing.T, s *Sluzhba, n Nabor) {
	t.Helper()
	blob, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	s.sekretyChitat = func() ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		return append([]byte(nil), blob...), nil
	}
	s.sekretyPisat = func(b []byte) error {
		mu.Lock()
		defer mu.Unlock()
		blob = append([]byte(nil), b...)
		return nil
	}
	s.nabor = s.naborIzHranilishcha
}

// konfigVyboraDlyaTesta уводит переписку выбора во ВРЕМЕННЫЙ файл.
//
// Без этого каждый зелёный прогон setServer правил конфиг УСТАНОВЛЕННОГО на
// машине клиента: putKonfigaTun это C:\ProgramData\Affory\sing-box.json, и
// поле default уезжало на тег тестового сервера. Обязателен в любом тесте,
// который доводит setServer до конца.
func konfigVyboraDlyaTesta(t *testing.T, soderzhimoe string) string {
	t.Helper()
	put := filepath.Join(t.TempDir(), "sing-box.json")
	if err := os.WriteFile(put, []byte(soderzhimoe), 0o600); err != nil {
		t.Fatal(err)
	}
	bylo := putKonfigaVybora
	putKonfigaVybora = func() string { return put }
	t.Cleanup(func() { putKonfigaVybora = bylo })
	return put
}

// konfigProby это минимальный конфиг с селектором: ровно то, что правит
// perepisatVyborVKonfige, и ничего сверх.
const konfigProby = `{"outbounds":[{"tag":"vybor","type":"selector","default":"avto"}]}`

// vypolnitTiho это vypolnit без *testing.T.
//
// Из горутины t.Fatal звать нельзя: он валит ГОРУТИНУ, а тест продолжается и
// оканчивается либо зелёным, либо зависанием на wg.Wait.
func vypolnitTiho(s *Sluzhba, imya string, telo any) protokol.Kadr {
	b, err := json.Marshal(telo)
	if err != nil {
		return otkaz(1, imya, protokol.KodProtocolMismatch, err.Error())
	}
	return s.Obrabotat(ctxAdmina(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: imya, Telo: b})
}

// Two commands, two goroutines, one blob on disk. Before the lock the loser
// silently overwrote the winner: a freshly added server just vanished.
func TestDobavlenieIUdaleniyeNeTeryayutDrugDruga(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		vypolnitTiho(s, "addServer", map[string]string{"ssylka": ssylkaProby})
	}()
	go func() {
		defer wg.Done()
		vypolnitTiho(s, "removeServer", map[string]string{"id": "de"})
	}()
	wg.Wait()

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitServer(n, idProby) {
		t.Error("добавленный сервер потерян: удаление затёрло чужую правку")
	}
	if soderzhitServer(n, "de") {
		t.Error("удалённый сервер воскрес: добавление затёрло чужую правку")
	}
}

// Второй участник это РАСПИСАНИЕ, а не человек: obnovitPodpisku идёт без
// команды, без прав и без единого клика, поэтому пара «добавил сервер, а в это
// время тикнуло расписание» случается сама.
func TestDobavlenieINeTeryaetsyaPodRaspisaniemPodpiski(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{
		Servery:  []protokol.Server{serverProby()},
		Podpiska: "https://panel.example/zhivaya",
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		vypolnitTiho(s, "addServer", map[string]string{"ssylka": ssylkaProby})
	}()
	go func() {
		defer wg.Done()
		_, _, _ = s.obnovitPodpisku(context.Background())
	}()
	wg.Wait()

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if !soderzhitServer(n, idProby) {
		t.Error("добавленный сервер потерян: обновление подписки затёрло правку человека")
	}
	// Зеркало: подписка привозит vtoroyServer, и её правка теряться тоже не
	// имеет права. Без этой проверки тест зелен на реализации, которая просто
	// не пускает расписание к набору вовсе.
	if !soderzhitServer(n, "de") {
		t.Error("сервер из подписки потерян: добавление затёрло правку расписания")
	}
}

// konfigVyboraPropal уводит переписку выбора на путь, которого НЕТ.
func konfigVyboraPropal(t *testing.T) {
	t.Helper()
	put := filepath.Join(t.TempDir(), "propal", "sing-box.json")
	bylo := putKonfigaVybora
	putKonfigaVybora = func() string { return put }
	t.Cleanup(func() { putKonfigaVybora = bylo })
}

// sZhivymTunnelem поднимает туннель на наборе из двух серверов.
func sZhivymTunnelem(t *testing.T) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"})
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

// Проба нового кандидата не прошла, и возврат на прежний тоже. Ядро осталось
// на кандидате, который не несёт, а switch-target-not-carrying по договору
// значит «выбор возвращён, подключение цело».
func TestNeudachnyyOtkatNeNazyvayetsyaTselymPodklyucheniem(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)

	var postavleno []string
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		postavleno = append(postavleno, teg)
		if len(postavleno) > 1 {
			return errors.New("ядро не ответило на возврат")
		}
		return nil
	}
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("503")
	}

	o := vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	if o.Oshib == nil {
		t.Fatal("мёртвый кандидат с провалившимся возвратом назван успехом")
	}
	if o.Oshib.Kod == protokol.KodNovyyNeNesyot {
		t.Error("откат не удался, ядро осталось на неработающем кандидате: это не «новый не несёт»")
	}
	if s.Status().Sostoyanie == protokol.SostPodnyat {
		t.Errorf("состояние %q обещает работающий туннель, которого нет", s.Status().Sostoyanie)
	}
}

// Отказ переписи выбора в конфиге перестаёт быть только строкой журнала.
//
// Выбор живёт в ПАМЯТИ ядра: горячей перезагрузки конфига нет, и перезапуск
// ядра сторожем вернул бы трафик на сервер из файла, оставив экран на новом.
func TestOtkazPerepisiVyboraDoezzhaetDoEkrana(t *testing.T) {
	konfigVyboraPropal(t)
	s := sZhivymTunnelem(t)
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return genkonfig.TegKandidata("de"), nil
	}

	o := vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	// Трафик уже переключён, поэтому команда НЕ отказ: отказ увёл бы экран
	// обратно на сервер, которого в ядре больше нет.
	if o.Oshib != nil {
		t.Fatalf("переключение отказано целиком, хотя трафик уже идёт на новый сервер: %s", o.Oshib.Kod)
	}
	var st protokol.StatusOtvet
	if err := json.Unmarshal(o.Telo, &st); err != nil {
		t.Fatal(err)
	}
	if st.Oshib == nil {
		t.Fatal("отказ переписи остался в журнале: перезапуск ядра молча вернёт прежний сервер")
	}
	if st.Oshib.Kod != protokol.KodPereklyuchenieNeDoehalo {
		t.Fatalf("код %q, ожидался switch-failed", st.Oshib.Kod)
	}
}

// Зеркало: удавшаяся перепись НЕ обязана вешать человеку баннер. Без него
// проверка выше зелена на реализации, которая жалуется всегда.
func TestUdavshayasyaPerepisVyboraNichegoNeSoobshchaet(t *testing.T) {
	put := konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		return genkonfig.TegKandidata("de"), nil
	}

	o := vypolnit(t, s, "setServer", map[string]string{"id": "de"})
	if o.Oshib != nil {
		t.Fatalf("переключение не прошло: %s", o.Oshib.Kod)
	}
	var st protokol.StatusOtvet
	if err := json.Unmarshal(o.Telo, &st); err != nil {
		t.Fatal(err)
	}
	if st.Oshib != nil {
		t.Fatalf("баннер на ровном месте: %v", st.Oshib)
	}
	// И сама перепись обязана СЛУЧИТЬСЯ: без этой проверки тест зелен на
	// реализации, которая конфиг вовсе не трогает.
	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(telo), genkonfig.TegKandidata("de")) {
		t.Fatalf("выбор не записан в конфиг: %s", telo)
	}
}

// Задача И1. Режим маршрута на ЖИВОМ ядре переключается В ЯДРЕ, а не обещанием
// переподъёма.
//
// Прежняя редакция писала только набор и отвечала «применится со следующего
// подъёма»: человек, нажавший «авто» при поднятом туннеле, обязан был порвать
// себе все соединения, чтобы применить переключение, которое ядро делает на
// лету тем же PUT /proxies, каким его делает setServer.
func TestSetRouteModeVAvtoStavitGruppuVZhivomYadre(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)

	var gruppy, tegi []string
	s.postavitVybor = func(_ context.Context, _, _, gruppa, teg string) error {
		gruppy, tegi = append(gruppy, gruppa), append(tegi, teg)
		return nil
	}

	o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"})
	if o.Oshib != nil {
		t.Fatalf("переход в авто на живом ядре отказал: %s", o.Oshib.Kod)
	}
	if len(tegi) != 1 || tegi[0] != genkonfig.TegAvto {
		t.Fatalf("в ядро поставлено %v, ожидался ровно один %q", tegi, genkonfig.TegAvto)
	}
	// Группа проверяется отдельно: PUT в чужую группу проходит с тем же 204 и
	// не меняет ничего, то есть выглядит успехом.
	if gruppy[0] != genkonfig.TegSelector {
		t.Errorf("выбор поставлен в группу %q, а живёт он в %q", gruppy[0], genkonfig.TegSelector)
	}
	if trebuet(t, o) {
		t.Error("режим переключён в ядре, а ответ всё равно требует переподъёма")
	}
}

// Ручной режим возвращает трафик на ВЫБРАННЫЙ сервер, а не на группу и не на
// первый попавшийся. Тег считается тем же способом, что и поле default при
// сборке конфига (genkonfig.gruppy): разъедься они, ядро и файл говорили бы
// разное, и заметить это было бы нечем.
func TestSetRouteModeVRuchnoyStavitVybrannyySerever(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t) // выбран nl

	var tegi []string
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		tegi = append(tegi, teg)
		return nil
	}

	if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "ruchnoy"}); o.Oshib != nil {
		t.Fatalf("переход в ручной отказал: %s", o.Oshib.Kod)
	}
	if len(tegi) != 1 || tegi[0] != genkonfig.TegKandidata("nl") {
		t.Fatalf("в ядро поставлено %v, ожидался %q", tegi, genkonfig.TegKandidata("nl"))
	}
}

// Выбор живёт в ПАМЯТИ ядра: горячей перезагрузки конфига нет. Сторож,
// перезапустив ядро, поднял бы его по файлу и молча вернул прежний режим, а
// экран остался бы на новом. Тот же замер 02.09.2026, из-за которого перепись
// стоит в setServer.
func TestSetRouteModePerepisyvaetRezhimVKonfige(t *testing.T) {
	put := konfigVyboraDlyaTesta(t, `{"outbounds":[{"tag":"vybor","type":"selector","default":"srv-nl"}]}`)
	s := sZhivymTunnelem(t)

	if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"}); o.Oshib != nil {
		t.Fatalf("переход в авто отказал: %s", o.Oshib.Kod)
	}
	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Ishodyashchie []struct {
			Teg        string `json:"tag"`
			Umolchanie string `json:"default"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	if k.Ishodyashchie[0].Umolchanie != genkonfig.TegAvto {
		t.Errorf("в конфиге default %q, а режим переключён в авто: перезапуск ядра вернёт прежний маршрут",
			k.Ishodyashchie[0].Umolchanie)
	}
}

// Ядро отвергло PUT, значит набор НЕ меняется и наружу уходит отказ. Записать
// режим, которого ядро не знает, значит показать человеку авто и вести его
// трафик по ручному выбору, и никакого способа заметить это у него нет.
func TestSetRouteModeNeZapisyvaetRezhimKogdaYadroOtvergloVybor(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		return errors.New("ядро не приняло выбор")
	}

	o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "ruchnoy"})
	if o.Oshib == nil {
		t.Fatal("ядро отвергло переключение, а команда ответила успехом")
	}
	// Код называется ТОЧНО. Проверка «лишь бы не secrets-unreadable» зелена и
	// на чужой таблице: вызов ушёл в kodRezhima из killswitch.go, та отдала
	// firewall-failed, и человека послали чинить брандмауэр вместо связи.
	if o.Oshib.Kod != protokol.KodPereklyuchenieNeDoehalo {
		t.Errorf("отказ ядра назван %q, а это отказ переключения", o.Oshib.Kod)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Rezhim == protokol.RezhimRuchnoy {
		t.Error("режим записан в набор, хотя ядро его не приняло")
	}
}

// Замок muVybor держится на ВЕСЬ цикл смены режима, а не только на запись.
//
// Между чтением набора (откуда берётся тег) и PUT в ядро стоит сетевой вызов.
// Без ОБЩЕГО с setServer замка две команды переплетаются так: setRouteMode
// прочитала «выбран nl», setServer успела целиком перевести и ядро, и набор на
// de, и только после этого setRouteMode ставит в ядро srv-nl. Экран говорит
// «Германия», трафик идёт через Нидерланды, и заметить это человеку нечем.
//
// Судит ПОРЯДОК, в котором ядро увидело выборы, а не факт вызова: тег
// записывается в конце заглушки, то есть тогда же, когда его увидело бы ядро.
func TestSmenaRezhimaNeRazezhaetsyaSPereklyucheniemServera(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t) // выбран nl, в наборе nl и de

	pustilVtorogo, konecVtorogo := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var postavleno []string
	nachalos := false
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		mu.Lock()
		pervyy := !nachalos
		nachalos = true
		mu.Unlock()
		if pervyy {
			// Набор уже прочитан, тег уже посчитан: отсюда и начинается окно.
			close(pustilVtorogo)
			// Соседа ждём ОГРАНИЧЕННО. Под замком он не дойдёт сюда никогда, и
			// это правильный исход, а не зависание теста.
			select {
			case <-konecVtorogo:
			case <-time.After(300 * time.Millisecond):
			}
		}
		mu.Lock()
		postavleno = append(postavleno, teg)
		mu.Unlock()
		return nil
	}

	gotovo := make(chan struct{})
	go func() {
		defer close(gotovo)
		vypolnitTiho(s, "setRouteMode", map[string]string{"rezhim": "ruchnoy"})
	}()
	<-pustilVtorogo
	vypolnitTiho(s, "setServer", map[string]string{"id": "de"})
	close(konecVtorogo)
	<-gotovo

	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := n.VybrannyyServer()
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	posledniy := postavleno[len(postavleno)-1]
	mu.Unlock()
	if posledniy != genkonfig.TegKandidata(srv.Id) {
		t.Errorf("в наборе выбран %q, а ядро последним увидело %q: экран и трафик разъехались",
			srv.Id, posledniy)
	}
}

// connect --server X при ЖИВОМ туннеле это переключение, а не подъём.
//
// Идемпотентный Connect (полоса Б) на поднятом туннеле возвращает nil, ничего
// не подняв, а выбор записывался ДО него. Итог: набор говорит «Германия»,
// трафик идёт через Нидерланды, человеку сказано «готово», и заметить это
// нечем. Замерено строкой 2 судьи proverit-otvety-sostoyaniy.ps1.
func TestConnectSoServeromNaZhivomTunnelePereklyuchaetYadro(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t) // выбран nl

	var tegi []string
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		tegi = append(tegi, teg)
		return nil
	}

	if o := vypolnit(t, s, "connect", map[string]string{"server": "de"}); o.Oshib != nil {
		t.Fatalf("connect --server на живом туннеле отказал: %s", o.Oshib.Kod)
	}
	if len(tegi) != 1 || tegi[0] != genkonfig.TegKandidata("de") {
		t.Fatalf("в ядро поставлено %v, а человек просил de: набор сменился, трафик нет", tegi)
	}
}

// Та же команда на МЁРТВОМ кандидате. Успех здесь хуже отказа: человек уверен,
// что сидит в Германии, а сидит там же, где сидел.
func TestConnectSoServeromNaZhivomTunneleNeNazyvaetMyortvyyUspehom(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)
	s.zamerit = func(context.Context, string, string, string) (time.Duration, error) {
		return 0, errors.New("503")
	}

	o := vypolnit(t, s, "connect", map[string]string{"server": "de"})
	if o.Oshib == nil {
		t.Fatal("кандидат не несёт, а connect --server ответил успехом")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Vybran == "de" {
		t.Error("выбор записан на сервер, который не понёс трафик")
	}
}

// Узкое окно: подъём уже идёт, ядра ещё нет. Идущий подъём УЖЕ прочитал набор,
// значит запись выбора к нему не применится, а Connect ответит успехом по
// идемпотентности. Отвечать успехом на «подключись к de» и понести nl это тот
// же обман, только отложенный на четыре секунды.
func TestConnectSoServeromVoVremyaPodyomaNeObeshchaetChuzhoyVybor(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{
		Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"})
	s.postavit(protokol.SostPodnimaetsya, nil)

	o := vypolnit(t, s, "connect", map[string]string{"server": "de"})
	if o.Oshib == nil {
		t.Fatal("подъём уже шёл, выбор к нему не применится, а команда ответила успехом")
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Vybran == "de" {
		t.Error("выбор записан, хотя применить его было некуда")
	}
}
