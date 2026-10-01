package genkonfig

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func vhodSZamerom() Vhod {
	v := obraztsovyyVhod()
	vtoroy := v.Server
	// Двоеточие и дефис в идентификаторе НАРОЧНО: такие бывают у записей,
	// переименованных при совпадении, и в логин HTTP прокси они не годятся.
	vtoroy.Id, vtoroy.Host = "nl:2-x", "203.0.113.7"
	v.Servery = []protokol.Server{v.Server, vtoroy}
	v.Kandidaty = append(v.Kandidaty, netip.MustParseAddr("203.0.113.7"))
	v.Zamer = &VhodZamera{Port: 10810, Parol: "p"}
	return v
}

// Логин входа замеров ведёт ровно в исходящий своего сервера, и правила стоят
// выше всех: иначе обход или правило процессов увели бы замер не туда.
func TestVhodZameraVedyotKazhdyyLoginVSvoySerever(t *testing.T) {
	v := vhodSZamerom()
	k := sobrat(t, v)

	var vhod map[string]any
	for _, x := range spisok(k["inbounds"]) {
		if m, ok := x.(map[string]any); ok && m["tag"] == TegZamerVhod {
			vhod = m
		}
	}
	if vhod == nil {
		t.Fatal("входа замеров нет")
	}
	if vhod["listen"] != "127.0.0.1" {
		t.Fatalf("вход замеров слушает %v, а не петлю", vhod["listen"])
	}
	if n := len(spisok(vhod["users"])); n != 2 {
		t.Fatalf("логинов %d, серверов два", n)
	}
	if first, _ := spisok(k["inbounds"])[0].(map[string]any); first["tag"] != "tun-in" {
		t.Fatalf("первым входом стал %v, а не туннель", first["tag"])
	}

	pravila := pravilaIz(t, k)
	for i, s := range v.Servery {
		m, _ := pravila[i].(map[string]any)
		login := PolzovatelZamera(s.Id)
		if strings.ContainsAny(login, ":-") {
			t.Fatalf("логин %q с недопустимым знаком", login)
		}
		users, _ := m["auth_user"].([]any)
		if len(users) != 1 || users[0] != login || m["outbound"] != TegKandidata(s.Id) {
			t.Fatalf("правило %d ведёт не туда: %v", i, m)
		}
	}
	posledneye, _ := pravila[len(v.Servery)].(map[string]any)
	if posledneye["action"] != "reject" {
		t.Fatalf("неизвестный логин не отбивается: %v", posledneye)
	}
}

// Без настройки входа нет: прежние конфиги не меняются ни на байт.
func TestBezZameraVhodaNet(t *testing.T) {
	v := vhodSZamerom()
	v.Zamer = nil
	k := sobrat(t, v)
	for _, x := range spisok(k["inbounds"]) {
		if m, ok := x.(map[string]any); ok && m["tag"] == TegZamerVhod {
			t.Fatal("вход замеров появился без настройки")
		}
	}
	if i := indeksPravila(t, k, func(m map[string]any) bool { return m["auth_user"] != nil }); i >= 0 {
		t.Fatalf("правило замера %d без входа", i)
	}
}

// Без TUN вход замеров остаётся единственным: временное ядро при выключенном
// VPN меряет пинг именно через него.
func TestZamerBezTun(t *testing.T) {
	v := vhodSZamerom()
	v.BezTun = true
	vhody := spisok(sobrat(t, v)["inbounds"])
	if len(vhody) != 1 {
		t.Fatalf("входов без TUN %d, ждали один вход замеров", len(vhody))
	}
	if m, _ := vhody[0].(map[string]any); m["tag"] != TegZamerVhod {
		t.Fatalf("без TUN поднят вход %v", m["tag"])
	}
}
