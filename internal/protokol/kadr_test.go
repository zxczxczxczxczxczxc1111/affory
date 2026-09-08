package protokol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKadrKruglyyReys(t *testing.T) {
	// A frame must survive a round trip byte for byte. If a field name drifts,
	// the two halves will still compile and still fail to understand each other,
	// which is the worst kind of broken.
	ishod := Kadr{Tip: "cmd", Id: 7, Imya: "status"}
	bayty, err := json.Marshal(ishod)
	if err != nil {
		t.Fatalf("не сериализуется: %v", err)
	}
	if string(bayty) != `{"tip":"cmd","id":7,"imya":"status"}` {
		t.Fatalf("форма кадра поехала: %s", bayty)
	}
	var nazad Kadr
	if err := json.Unmarshal(bayty, &nazad); err != nil {
		t.Fatalf("не разбирается: %v", err)
	}
	// Not `nazad != ishod`: RawMessage is a slice, and Go refuses to compare
	// those. Compare the fields that matter instead of fighting the language.
	if nazad.Tip != ishod.Tip || nazad.Id != ishod.Id || nazad.Imya != ishod.Imya {
		t.Fatalf("кадр не тот: %+v", nazad)
	}
}

func TestStatusRazvoditVybrannogoINesushchego(t *testing.T) {
	// server_id отвечал сразу на два вопроса: что человек выбрал и что несёт
	// трафик. В ручном режиме это одно и то же, в автоматическом расходится, и
	// одно поле начинает врать на оба вопроса сразу.
	o := StatusOtvet{Sostoyanie: SostPodnyat, NesushchiyId: "bbb"}
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, est := m["server_id"]; est {
		t.Error("server_id остался на проводе: он отвечал сразу на два вопроса, ради чего всё и разводится")
	}
	if _, est := m["vybran_id"]; est {
		t.Error("выбора нет, поле обязано отсутствовать, а не приезжать пустым")
	}
	if m["nesushchiy_id"] != "bbb" {
		t.Errorf("несущий %v, а служба знает bbb", m["nesushchiy_id"])
	}
}

func TestAvtozapuskIPodklyuchenieRazvedeny(t *testing.T) {
	// Один флаг на два решения показал бы «подключаться при старте» включённым
	// сразу после установки, потому что автозапуск по §9.2 включён.
	var st StatusOtvet
	st.Avtozapusk = true
	if st.PodklyuchatPriStarte {
		t.Fatal("подключение при старте включилось вместе с автозапуском: поля не разведены")
	}
	// The wire name is part of the contract: protokol.ts mirrors it by hand.
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"podklyuchat_pri_starte":false`) {
		t.Fatalf("на проводе нет поля podklyuchat_pri_starte: %s", b)
	}
}
