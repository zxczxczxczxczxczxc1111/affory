package genkonfig

import (
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Молчаливая родня находки 43.
//
// Ветки ws, grpc и xhttp включали TLS БЕЗУСЛОВНО. Для сервера, у которого в
// ссылке `security=none`, это значит открытый транспорт, завёрнутый в TLS: ядро
// такой конфиг принимает, `check` его не отвергает, а соединения не будет
// никогда. Поймать это может ТОЛЬКО проверка содержимого: живой check тут
// бессилен по построению, и потому отдельный тест, а не профиль.
func TestBezTlsVKonfigeNetBlokaTls(t *testing.T) {
	sluchai := []struct {
		imya      string
		transport string
	}{
		{"ws без tls", "ws"},
		{"grpc без tls", "grpc"},
		{"httpupgrade без tls", "httpupgrade"},
	}
	for _, sl := range sluchai {
		t.Run(sl.imya, func(t *testing.T) {
			v := obraztsovyyVhod()
			s := v.Server
			s.Transport = sl.transport
			s.BezTLS = true
			s.PublicKey = ""
			s.ShortId = ""
			s.Put = "/put"
			v.Server = s
			v.Servery = []protokol.Server{s}

			telo, err := SingBox(v)
			if err != nil {
				t.Fatalf("генератор отказал: %v", err)
			}
			ish := ishodyashchiyPoTegu(t, telo, TegKandidata(s.Id))
			if _, est := ish["tls"]; est {
				t.Errorf("у сервера без TLS в конфиге есть блок tls: открытый транспорт завёрнут в TLS, "+
					"ядро это примет и соединения не будет: %v", ish["tls"])
			}
		})
	}
}

// КОНТРОЛЬ: с TLS блок обязан БЫТЬ. Без этой половины правка проходит и на
// генераторе, который выкинул TLS у всех, а это сломанный продукт целиком.
func TestSTlsBlokTlsOstayotsya(t *testing.T) {
	v := obraztsovyyVhod()
	s := v.Server
	s.Transport = "ws"
	s.BezTLS = false
	s.Put = "/put"
	v.Server = s
	v.Servery = []protokol.Server{s}

	telo, err := SingBox(v)
	if err != nil {
		t.Fatalf("генератор отказал: %v", err)
	}
	ish := ishodyashchiyPoTegu(t, telo, TegKandidata(s.Id))
	tls, est := ish["tls"].(map[string]any)
	if !est {
		t.Fatal("у сервера с TLS блока tls нет: соединение не состоится вовсе")
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled = %v", tls["enabled"])
	}
}

func ishodyashchiyPoTegu(t *testing.T, telo []byte, teg string) map[string]any {
	t.Helper()
	var k struct {
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	for _, o := range k.Outbounds {
		if o["tag"] == teg {
			return o
		}
	}
	t.Fatalf("исходящего %s в конфиге нет", teg)
	return nil
}
