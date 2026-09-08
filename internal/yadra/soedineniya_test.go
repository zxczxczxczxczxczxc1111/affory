package yadra

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// Задача 6.2. Журнал берётся из clash_api, а не из текстового журнала ядра:
// решения маршрутизации ядро пишет на уровне info, а у нас warn.

const obrazecSoedineniy = `{"downloadTotal":1,"uploadTotal":2,"connections":[
 {"id":"a1","metadata":{"network":"tcp","type":"","sourceIP":"172.19.0.1","destinationIP":"203.0.113.21","sourcePort":"51000","destinationPort":"443","host":"discord.com","processPath":"C:\\Discord\\Discord.exe"},
  "upload":10,"download":20,"start":"2026-09-03T11:00:00Z","chains":["srv-nl","vybor"],"rule":"final","rulePayload":""},
 {"id":"b2","metadata":{"network":"udp","destinationIP":"1.1.1.1","destinationPort":"53","host":"","processPath":""},
  "upload":0,"download":0,"start":"2026-09-03T11:00:01Z","chains":["direct"],"rule":"process_path","rulePayload":"C:\\steam.exe"}
]}`

func TestSoedineniyaRazbirayutsya(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" {
			http.Error(w, r.URL.Path, 404)
			return
		}
		fmt.Fprint(w, obrazecSoedineniy)
	})
	sp, err := Soedineniya(context.Background(), adres, "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(sp) != 2 {
		t.Fatalf("соединений %d", len(sp))
	}
	a := sp[0]
	if a.Id != "a1" || a.Host != "discord.com" || a.Adres != "203.0.113.21" || a.Port != 443 ||
		a.Protsess != `C:\Discord\Discord.exe` || a.Vyhod != "srv-nl" || a.Pravilo != "final" {
		t.Fatalf("первое соединение разобрано как %+v", a)
	}
	if a.Nachalo.IsZero() {
		t.Fatal("время начала не разобрано")
	}
	// Выход это ПЕРВОЕ звено цепочки: sing-box пишет её от исходящего к
	// группе, и последнее звено это имя селектора, а не сервер.
	if sp[1].Vyhod != "direct" || sp[1].Port != 53 {
		t.Fatalf("второе соединение разобрано как %+v", sp[1])
	}
}

func TestSoedineniya401EtoOshibka(t *testing.T) {
	adres := podstavnoyKlash(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	if _, err := Soedineniya(context.Background(), adres, "s"); err == nil {
		t.Fatal("401 прошёл за успех")
	}
}
