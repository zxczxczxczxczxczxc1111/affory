package petlya

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Пара «сервер плюс своя мишень».
//
// Отличить сервер A от сервера B по трафику снаружи нельзя, если внешний адрес
// у них общий: ровно поэтому судья переключения на стенде печатает НЕГОДЕН с
// 0.7.0. Здесь каждый сервер уводит ОДНО И ТО ЖЕ имя в свою мишень, и несущий
// читается прямо из ответа.
type UzelSMishenyu struct {
	Uzel
	// Imya это то, что отвечает мишень.
	Imya string
	// Adres одинаков у обоих узлов пары: имя и порт общие, различаются только
	// адреса, в которые это имя резолвит сервер.
	Adres string
}

// imyaMisheni одно на всю пару: разные имена сделали бы запрос различимым уже у
// клиента, и переключение проверялось бы клиентской стороной вместо серверной.
const imyaMisheni = "mishen.example"

// ParaUzlov поднимает два сервера, каждый со своей мишенью.
func ParaUzlov(t *testing.T) (UzelSMishenyu, UzelSMishenyu) {
	t.Helper()
	a, b := ParaMishenei(t, "mishen-A", "mishen-B")
	return UzelKMisheni(t, a), UzelKMisheni(t, b)
}

// UzelKMisheni поднимает trojan-сервер, который ведёт имя мишени в ЭТУ мишень.
//
// Транспорт здесь не проверяется, он взят самым простым из рабочих: волна
// транспортов уже доказала, что trojan несёт, и повторять это в проверке выбора
// значит смешивать два вопроса в одном красном.
func UzelKMisheni(t *testing.T, m *Mishen) UzelSMishenyu {
	t.Helper()
	u := trojanKAdresu(t, m.Host)
	return UzelSMishenyu{Uzel: u, Imya: m.Imya, Adres: m.PoImeni}
}

// MyortvyyUzel это ссылка на сервер, которого нет.
//
// Нужен авто-режиму: живой из живого и живого выбирается всегда, а вопрос стоит
// про живого из живого и МЁРТВОГО.
func MyortvyyUzel(t *testing.T) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	// Порт занимаем и тут же отпускаем: слушать на нём никто не будет.
	port := SvobodnyyPort(t)
	return Uzel{
		Transport: "trojan", CaPEM: sert.CaPEM,
		sekret: "parol-myortvogo", pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf("trojan://%s@%s:%d?sni=%s&pinPubKeySHA256=%s#petlya-myortvyy",
				sekret, imyaUzla, port, imyaUzla, urlEscape(pin))
		},
	}
}

// Teg отдаёт тег этого узла в конфиге продукта. Через НАШ же разбор и НАШ же
// генератор: тег, посчитанный тестом по своей формуле, разъехался бы с
// продуктовым молча, и переключение проверялось бы на несуществующий выход.
func (u Uzel) Teg(t *testing.T) string {
	t.Helper()
	s, err := ssylki.Razobrat(u.Ssylka())
	if err != nil {
		t.Fatalf("своя же ссылка не разобралась: %v", err)
	}
	return genkonfig.TegKandidata(s.Id)
}

func trojanKAdresu(t *testing.T, kuda string) Uzel {
	t.Helper()
	sert := NovyySertifikat(t, imyaUzla)
	port := SvobodnyyPort(t)
	parol := "parol-petli-" + kuda

	konfig := srvKonfig(map[string]any{
		"type": "trojan", "tag": "vhod",
		"listen": "127.0.0.1", "listen_port": port,
		"users": []any{map[string]any{"password": parol}},
		"tls":   tlsServera(sert),
	})
	// Разводка живёт на СЕРВЕРЕ: клиент шлёт одно и то же имя, а куда оно
	// приедет, решает тот, кто нёс трафик.
	konfig["dns"] = map[string]any{
		"servers": []any{map[string]any{
			"type": "hosts", "tag": "hosts",
			"predefined": map[string]any{imyaMisheni: []string{kuda}},
		}},
	}
	server := PodnyatServer(t, konfig)

	return Uzel{
		Transport: "trojan", CaPEM: sert.CaPEM, Server: server,
		sekret: parol, pin: sert.Pin,
		postroit: func(sekret, pin string) string {
			return fmt.Sprintf("trojan://%s@%s:%d?sni=%s&pinPubKeySHA256=%s#petlya-%s",
				sekret, imyaUzla, port, imyaUzla, urlEscape(pin), kuda)
		},
	}
}

// ParaMishenei поднимает две мишени на РАЗНЫХ петлевых адресах и ОДНОМ порту.
//
// Порт общий намеренно: он попадает в запрашиваемый адрес, и разные порты
// сделали бы запросы к A и к B различимыми ещё до туннеля.
func ParaMishenei(t *testing.T, imyaA, imyaB string) (*Mishen, *Mishen) {
	t.Helper()
	a, b := dvaSlushatelya(t, "127.0.0.2", "127.0.0.3")
	return mishenNaSlushatele(t, a, imyaA), mishenNaSlushatele(t, b, imyaB)
}

// NovayaMishenNaAdrese поднимает одиночную мишень на заданном петлевом адресе.
func NovayaMishenNaAdrese(t *testing.T, adres, imya string) *Mishen {
	t.Helper()
	l, err := net.Listen("tcp", adres+":0")
	if err != nil {
		t.Fatalf("мишень не встала на %s: %v", adres, err)
	}
	return mishenNaSlushatele(t, l, imya)
}

// dvaSlushatelya ищет порт, свободный на ОБОИХ адресах сразу.
//
// Занять, отпустить и занять снова нельзя: между отпусканием и повторным
// захватом порт уводит кто угодно, и тест падал бы раз в сколько-то прогонов
// по причине, к продукту отношения не имеющей.
func dvaSlushatelya(t *testing.T, adresA, adresB string) (net.Listener, net.Listener) {
	t.Helper()
	for popytka := 0; popytka < 20; popytka++ {
		a, err := net.Listen("tcp", adresA+":0")
		if err != nil {
			t.Fatalf("мишень не встала на %s: %v", adresA, err)
		}
		port := a.Addr().(*net.TCPAddr).Port
		b, err := net.Listen("tcp", fmt.Sprintf("%s:%d", adresB, port))
		if err == nil {
			return a, b
		}
		a.Close()
	}
	t.Fatalf("порт, свободный на %s и на %s сразу, не нашёлся за 20 попыток", adresA, adresB)
	return nil, nil
}

func mishenNaSlushatele(t *testing.T, l net.Listener, imya string) *Mishen {
	t.Helper()
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, imya)
	}))
	s.Listener.Close()
	s.Listener = l
	s.Start()
	t.Cleanup(s.Close)

	adres := l.Addr().(*net.TCPAddr)
	return &Mishen{
		Adres:   s.URL,
		Imya:    imya,
		Host:    adres.IP.String(),
		PoImeni: fmt.Sprintf("http://%s:%d", imyaMisheni, adres.Port),
	}
}
