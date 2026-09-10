package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// resheniyeOPravah is the DECISION, not a mirror of the code: the test compares
// it against komandyDlyaAdmina, so a silent change on either side fails here.
// Owner's call 03.09.2026: admin only where the blast radius is the whole
// machine or the secrets leave it.
//
// Имена НЕ переписываются руками: их даёт vetkiDispetchera(t) разбором
// диспетчера. Прежний granicaDopuska был вторым экземпляром imenaKomand(), и
// сходились они ровно потому, что врали одинаково: setAutostart и
// setConnectOnStart отсутствовали в обоих.
var resheniyeOPravah = map[string]bool{
	"hello": false, "status": false, "subscribeStats": false,
	"setJournal": false, "setDiagnostics": false, "clearJournal": false,
	"checkUpdate": false, "getServerHealth": false,
	"checkExitIp": false, "checkLeaks": false,
	// measureDelays спрашивает у ядра и у сети то, что и так видно на экране:
	// жив ли узел и сколько до него. Ни машины, ни секретов он не трогает, а
	// админом здесь становится то, чей радиус поражения вся машина.
	"measureDelays": false,
	// measureBandwidth качает через НАШ же прокси на мишень, которую назвал сам
	// человек. Ни машины, ни секретов не трогает; трафик тратит его собственный
	// и по его же команде.
	"measureBandwidth": false,
	"startSpeedTest":   false, "speedTestStatus": false, "cancelSpeedTest": false,
	"connect": false, "disconnect": false, "setServer": false,
	"setRouteMode": false, "listServers": false, "listRules": false,
	"setAutostart": false, "setConnectOnStart": false,
	// Полоса канала это число про домашний интернет, а не про права на машине:
	// UAC на её правку означал бы запрос прав ради поля ввода. Худшее, что даёт
	// чужая программа с этим доступом, это испорченный замер собственной
	// скорости, тогда как setKillSwitch выше меняет маршрут всего трафика.
	"setBandwidth": false,
	"addServer":    false, "removeServer": false,
	"setSubscription": false, "refreshSubscription": false, "setRules": false,
	"setKillSwitch": true, "exportProfile": true, "importProfile": true,
	"downloadUpdate": true, "installUpdate": true,
}

// Команда без прав администратора обязана получить отказ, а не выполниться.
//
// Угроза не «другой человек за машиной», а «другая программа от тебя». Windows
// даёт админу раздельный токен, поэтому браузер, игра и скрипт из npm
// postinstall сидят в INTERACTIVE, куда канал пускает намеренно. До этой
// проверки такой процесс мог без единого запроса прав добавить свой сервер и
// увести весь трафик машины.
func TestKomandyDlyaAdminaOtvergayutsyaBezPrav(t *testing.T) {
	// Контекст БЕЗ допуска: отсутствие сведений трактуется как «не админ».
	ctx := context.Background()
	// Имена из разбора, флаг из решения: список admin-команд нигде не написан
	// руками третий раз.
	for _, imya := range vetkiDispetchera(t) {
		if !resheniyeOPravah[imya] {
			continue
		}
		t.Run(imya, func(t *testing.T) {
			s := podstavnaya(t, nil)
			o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: imya, Telo: json.RawMessage(`{}`)})
			if o.Oshib == nil {
				t.Fatalf("команда %s выполнена без прав администратора", imya)
			}
			if o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
				t.Fatalf("команда %s отказала кодом %s, ожидался %s",
					imya, o.Oshib.Kod, protokol.KodTrebuetsyaAdmin)
			}
		})
	}
}

// Обратная сторона: ежедневные команды НЕ должны требовать прав. Иначе UAC
// вылезает на каждое подключение, и программа превращается в то, что отключают.
func TestEzhednevnyeKomandyRabotayutBezPrav(t *testing.T) {
	for _, imya := range []string{"hello", "status", "listServers"} {
		t.Run(imya, func(t *testing.T) {
			s := podstavnaya(t, nil)
			o := s.Obrabotat(context.Background(), protokol.Kadr{Id: 1, Imya: imya})
			if o.Oshib != nil && o.Oshib.Kod == protokol.KodTrebuetsyaAdmin {
				t.Fatalf("команда %s потребовала админа: UAC на каждый запуск", imya)
			}
		})
	}
}

// Ворота против забытой команды: новая ветка диспетчера обязана попасть в
// решение о правах, а не унаследовать его по умолчанию.
func TestKazhdayaKomandaImeetResheniyeOPravah(t *testing.T) {
	for _, v := range vetkiDispetchera(t) {
		nuzhen, prinyato := resheniyeOPravah[v]
		if !prinyato {
			t.Errorf("для ветки %q решение о правах не принято", v)
			continue
		}
		if komandyDlyaAdmina[v] != nuzhen {
			t.Errorf("ветка %q: решено админ=%v, в коде админ=%v", v, nuzhen, komandyDlyaAdmina[v])
		}
	}
	for imya := range komandyDlyaAdmina {
		if _, prinyato := resheniyeOPravah[imya]; !prinyato {
			t.Errorf("komandyDlyaAdmina держит %q, а решения о нём нет", imya)
		}
	}
}

// Допуск администратора проходит насквозь: иначе предыдущие тесты были бы
// зелёными и на службе, которая отказывает всем подряд.
func TestSAdminskimDopuskomKomandaProhodit(t *testing.T) {
	s := podstavnaya(t, nil)
	ctx := kanal.SDopuskom(context.Background(), kanal.Dopusk{Admin: true})
	o := s.Obrabotat(ctx, protokol.Kadr{Id: 1, Imya: "setKillSwitch", Telo: json.RawMessage(`{"vkl":false}`)})
	if o.Oshib != nil && o.Oshib.Kod == protokol.KodTrebuetsyaAdmin {
		t.Fatal("админу отказано: проверка прав отвергает всех и ничего не доказывает")
	}
}

// Сверка версий обязана быть ДВУСТОРОННЕЙ.
//
// Клиент присылает свою версию в теле hello и проверяет ответ службы. Служба
// свою версию объявляет, а чужую не читала вовсе. Старый интерфейс против новой
// службы получал зелёное приветствие и падал позже, на поле, которого не знает,
// то есть ровно в том месте, где сверка версий должна была его остановить.
func TestHelloOtvergaetChuzhuyuVersiyu(t *testing.T) {
	s := podstavnaya(t, nil)

	o := s.Obrabotat(context.Background(), protokol.Kadr{
		Id: 1, Imya: "hello", Telo: json.RawMessage(`{"protocol":99}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodProtocolMismatch {
		t.Fatalf("чужая версия принята: %+v", o.Oshib)
	}

	// КОНТРОЛЬ: своя версия обязана проходить, иначе проверка отвергает всех и
	// канал не открывается вовсе.
	o = s.Obrabotat(context.Background(), protokol.Kadr{
		Id: 2, Imya: "hello",
		Telo: json.RawMessage(fmt.Sprintf(`{"protocol":%d}`, protokol.Versiya)),
	})
	if o.Oshib != nil {
		t.Fatalf("своя версия отвергнута: %+v", o.Oshib)
	}
}

// Импорт профиля обязан пересобрать правила брандмауэра.
//
// sohranitIPeresobrat объявлена в коде ЕДИНСТВЕННЫМ путём изменения списка
// серверов, и четыре команды идут через неё. Импорт профиля писал секреты
// напрямую, а он меняет список целиком. При включённом режиме «весь трафик»
// импортированные серверы оказывались заблокированы собственным kill-switch:
// человек видит их на экране и не может подключиться ни к одному.
// Импорт заменяет список ЦЕЛИКОМ и пишет блоб мимо sohranitIPeresobrat, то есть
// мимо заслона. Заслон здесь и не годится: он означал бы «нельзя импортировать
// профиль, пока подключён». Импорт меняет всю картину сразу, и нести трафик по
// старой картине не значит ничего, поэтому туннель опускается.
//
// Утверждения ДВА, и первое унаследовано от прежнего теста: правила обязаны
// пересобраться ДО опускания, иначе машина остаётся запертой под старые адреса
// и подключиться после импорта нельзя вовсе.
func TestImportProfilyaPeresobiraetPravilaIOpuskaetTunnel(t *testing.T) {
	s := podstavnaya(t, nil)
	peresobrali := false
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { peresobrali = true; return nil }
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()
	// Нужен НАСТОЯЩИЙ подъём, а не выставленное руками состояние: опускание
	// смотрит на адрес clash_api, который выставляет только Connect.
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}

	telo := []byte(`{"servery":[{"id":"a","host":"203.0.113.4","port":443,"transport":"reality-tcp"}]}`)
	blob, err := hranenie.Eksport("parol-dlinnyy-dostatochno", telo)
	if err != nil {
		t.Fatal(err)
	}
	o := vypolnit(t, s, "importProfile", map[string]string{
		"parol": "parol-dlinnyy-dostatochno", "profil": vBase64(blob)})
	if o.Oshib != nil {
		t.Fatalf("импорт отказал: %+v", o.Oshib)
	}
	if !peresobrali {
		t.Fatal("правила не пересобраны ДО опускания: машина заперта под старые адреса, подключиться нельзя")
	}
	if st := s.Status(); st.Sostoyanie != protokol.SostVyklyuchen {
		t.Fatalf("состояние %q: ядро продолжает нести трафик через сервер, которого в наборе больше нет", st.Sostoyanie)
	}
}
