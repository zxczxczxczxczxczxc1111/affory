package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// sPamyatyu подставляет хранилище секретов в память, чтобы тест не писал в
// живой ProgramData машины разработчика.
func sPamyatyu(t *testing.T, nachalnoe []byte) *Sluzhba {
	t.Helper()
	s := podstavnaya(t, nil)
	telo := nachalnoe
	s.sekretyChitat = func() ([]byte, error) { return telo, nil }
	s.sekretyPisat = func(b []byte) error { telo = b; return nil }
	return s
}

func adminom(ctx context.Context) context.Context {
	return kanal.SDopuskom(ctx, kanal.Dopusk{Admin: true})
}

func neAdminom(ctx context.Context) context.Context {
	return kanal.SDopuskom(ctx, kanal.Dopusk{Admin: false})
}

func TestEksportTolkoAdminu(t *testing.T) {
	// The pipe admits INTERACTIVE by design. These two commands hand over every
	// key and the subscription URL, so they check separately.
	s := sPamyatyu(t, []byte(`{"servery":[]}`))
	o := s.Obrabotat(neAdminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "exportProfile", Telo: []byte(`{"parol":"x"}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("не-админ получил профиль: %v", o.Oshib)
	}
}

func TestImportTolkoAdminu(t *testing.T) {
	s := sPamyatyu(t, nil)
	o := s.Obrabotat(neAdminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "importProfile", Telo: []byte(`{"parol":"x","profil":"AAA"}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("не-админ увёл трафик машины: %v", o.Oshib)
	}
}

func TestBezDopuskaEtoNeAdmin(t *testing.T) {
	// A context that never went through the check must not read as «admin».
	// Trusting an absent value would hand the keys to anything that manages to
	// call Obrabotat without a connection, and that is exactly the shape of a
	// future refactor bug.
	s := sPamyatyu(t, []byte(`{"servery":[]}`))
	// Голый Background НАРОЧНО: этот тест про то, что отсутствие сведений о
	// допуске трактуется как «не админ». Подставить сюда ctxAdmina значило бы
	// проверять обратное утверждение.
	o := s.Obrabotat(context.Background(), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "exportProfile", Telo: []byte(`{"parol":"x"}`),
	})
	if o.Oshib == nil || o.Oshib.Kod != protokol.KodTrebuetsyaAdmin {
		t.Fatalf("контекст без допуска сошёл за админский: %v", o.Oshib)
	}
}

func TestKrugovoyReysCherezKanal(t *testing.T) {
	const nachalnoe = `{"servery":[{"id":"aaa"}]}`
	s := sPamyatyu(t, []byte(nachalnoe))
	o := s.Obrabotat(adminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "exportProfile", Telo: []byte(`{"parol":"dlinnyy-parol"}`),
	})
	if o.Oshib != nil {
		t.Fatalf("экспорт отказал: %v", o.Oshib)
	}
	var vyvoz struct {
		Profil string `json:"profil"`
	}
	if err := json.Unmarshal(o.Telo, &vyvoz); err != nil {
		t.Fatal(err)
	}
	if vyvoz.Profil == "" {
		t.Fatal("экспорт вернул пустой профиль")
	}
	if strings.Contains(vyvoz.Profil, "aaa") {
		t.Fatal("идентификатор сервера виден в выгрузке")
	}

	// Импорт на ЧИСТУЮ службу: профиль существует ради машины, где блоба не было.
	chistaya := sPamyatyu(t, nil)
	telo, _ := json.Marshal(map[string]string{"parol": "dlinnyy-parol", "profil": vyvoz.Profil})
	o = chistaya.Obrabotat(adminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 2, Imya: "importProfile", Telo: telo,
	})
	if o.Oshib != nil {
		t.Fatalf("импорт отказал: %v", o.Oshib)
	}
	nazad, err := chistaya.sekretyChitat()
	if err != nil {
		t.Fatal(err)
	}
	if string(nazad) != nachalnoe {
		t.Fatalf("после импорта в хранилище %q", nazad)
	}
}

func TestImportSNevernymParolemNeTrogaetSushchestvuyushchee(t *testing.T) {
	// A failed import must not be a way to wipe the machine's own servers. The
	// человек tried a password and got it wrong; that is not consent to lose
	// everything.
	const bylo = `{"servery":[{"id":"svoy"}]}`
	s := sPamyatyu(t, []byte(bylo))
	chuzhoy, err := hranenie.Eksport("drugoy-parol", []byte(`{"servery":[{"id":"chuzhoy"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	telo, _ := json.Marshal(map[string]string{
		"parol": "ne-tot", "profil": vBase64(chuzhoy),
	})
	o := s.Obrabotat(adminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "importProfile", Telo: telo,
	})
	if o.Oshib == nil {
		t.Fatal("импорт с неверным паролем прошёл")
	}
	nazad, _ := s.sekretyChitat()
	if string(nazad) != bylo {
		t.Fatalf("неудавшийся импорт стёр свои серверы: %q", nazad)
	}
}

func TestPustoyParolOtvergaetsyaKanalom(t *testing.T) {
	s := sPamyatyu(t, []byte(`{"servery":[]}`))
	o := s.Obrabotat(adminom(context.Background()), protokol.Kadr{
		Tip: "cmd", Id: 1, Imya: "exportProfile", Telo: []byte(`{"parol":""}`),
	})
	if o.Oshib == nil {
		t.Fatal("экспорт с пустым паролем прошёл")
	}
}

func TestParolNikogdaNePopadaetVOtvet(t *testing.T) {
	// The password travels in the clear over a local pipe, which is acceptable.
	// What is not acceptable is echoing it back into a frame that anything else
	// might log.
	const parol = "OCHEN-ZAMETNYY-PAROL"
	s := sPamyatyu(t, []byte(`{"servery":[]}`))
	for _, imya := range []string{"exportProfile", "importProfile"} {
		telo, _ := json.Marshal(map[string]string{"parol": parol, "profil": "мусор"})
		o := s.Obrabotat(adminom(context.Background()), protokol.Kadr{
			Tip: "cmd", Id: 1, Imya: imya, Telo: telo,
		})
		ves := string(o.Telo)
		if o.Oshib != nil {
			ves += o.Oshib.Tekst
		}
		if strings.Contains(ves, parol) {
			t.Fatalf("%s вернул пароль в ответе: %s", imya, ves)
		}
	}
}

func TestTeloSekretnyhKadrovNeLogiruetsya(t *testing.T) {
	// The ban lives next to the logger, not in somebody's head.
	const parol = "OCHEN-ZAMETNYY-PAROL"
	telo := []byte(`{"parol":"` + parol + `"}`)
	for _, imya := range []string{"exportProfile", "importProfile", "setSubscription"} {
		if protokol.TeloMozhnoLogirovat(imya) {
			t.Fatalf("%s разрешён к печати целиком", imya)
		}
		if strings.Contains(protokol.TeloDlyaZhurnala(imya, telo), parol) {
			t.Fatalf("%s: пароль уехал бы в журнал", imya)
		}
	}
	// Зеркальный случай: запрет не должен превратиться в «не печатаем ничего».
	if !protokol.TeloMozhnoLogirovat("connect") {
		t.Fatal("запрет расползся на обычные команды")
	}
	if protokol.TeloDlyaZhurnala("connect", []byte("vidno")) != "vidno" {
		t.Fatal("обычная команда перестала печататься")
	}
}

// kadrImporta собирает кадр импорта с настоящим зашифрованным профилем.
func kadrImporta(t *testing.T, nabor string) protokol.Kadr {
	t.Helper()
	const parol = "parol-dlinnyy-dostatochno"
	blob, err := hranenie.Eksport(parol, []byte(nabor))
	if err != nil {
		t.Fatal(err)
	}
	telo, err := json.Marshal(map[string]string{"parol": parol, "profil": vBase64(blob)})
	if err != nil {
		t.Fatal(err)
	}
	return protokol.Kadr{Tip: "cmd", Id: 1, Imya: "importProfile", Telo: telo}
}

// Импорт заменил список ЦЕЛИКОМ, значит ядро несёт трафик через сервер,
// которого в наборе больше нет. Отказ пересборки правил не имеет права
// оборвать команду на полпути: секреты уже переписаны, и остановившись здесь,
// служба оставляет запертую машину с разрешающими правилами под старые адреса
// и поднятым туннелем на чужом сервере.
func TestImportPriOtkazePeresborkiNeOstavlyaetTunnelNaChuzhomServere(t *testing.T) {
	s := podstavnaya(t, nil)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Режим «весь трафик» включён, а пересборка под новый список проваливается.
	s.vklyuchitVes = func(set.Razreshyonnoe, bool) error { return errors.New("netsh не отработал") }
	s.mu.Lock()
	s.killSwitch = true
	s.mu.Unlock()

	o := s.Obrabotat(adminom(context.Background()),
		kadrImporta(t, `{"servery":[{"id":"chuzhoy","host":"203.0.113.77","port":443,"transport":"reality-tcp"}]}`))
	if o.Oshib == nil {
		t.Fatal("отказ пересборки правил назван успехом")
	}
	if o.Oshib.Kod != protokol.KodFirewallFailed {
		t.Fatalf("код %q, ожидался firewall-failed", o.Oshib.Kod)
	}
	if st := s.Status().Sostoyanie; st == protokol.SostPodnyat {
		t.Error("после отказа импорта туннель остался поднятым на сервере, которого в наборе больше нет")
	}
	if adres, _ := s.dostupKKlash(); adres != "" {
		t.Error("ядро живо после отказа импорта: оно продолжает нести трафик по старой картине")
	}
}
