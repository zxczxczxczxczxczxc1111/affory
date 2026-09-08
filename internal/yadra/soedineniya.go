package yadra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Soedinenie это одно живое соединение по версии clash_api.
//
// Vyhod это ПЕРВОЕ звено цепочки: sing-box пишет её от исходящего к группе
// («srv-nl», «vybor»), и последнее звено это имя селектора, а не сервер.
type Soedinenie struct {
	Id       string
	Host     string
	Adres    string
	Port     int
	Protsess string
	Vyhod    string
	Pravilo  string
	Nachalo  time.Time
}

// Soedineniya читает /connections живого ядра. Уровень журнала ядра не
// трогается: решения маршрутизации ядро пишет на уровне info, а у нас warn,
// и собирать журнал из лога значило бы залить диск всеми доменами подряд.
func Soedineniya(ctx context.Context, adres, sekret string) ([]Soedinenie, error) {
	telo, kod, err := zaprosS(ctx, http.MethodGet, fmt.Sprintf("http://%s/connections", adres), sekret, nil, 4<<20)
	if err != nil {
		return nil, err
	}
	switch {
	case kod == http.StatusUnauthorized || kod == http.StatusForbidden:
		return nil, sekretNePrinyat(adres, kod)
	case kod != http.StatusOK:
		return nil, fmt.Errorf("соединения ядра не отданы (код %d)", kod)
	}
	var o struct {
		Soedineniya []struct {
			Id       string `json:"id"`
			Metadata struct {
				Host        string `json:"host"`
				Adres       string `json:"destinationIP"`
				Port        string `json:"destinationPort"`
				ProcessPath string `json:"processPath"`
			} `json:"metadata"`
			Nachalo time.Time `json:"start"`
			Tsep    []string  `json:"chains"`
			Pravilo string    `json:"rule"`
		} `json:"connections"`
	}
	if err := json.Unmarshal(telo, &o); err != nil {
		return nil, fmt.Errorf("список соединений не разбирается: %w", err)
	}
	itog := make([]Soedinenie, 0, len(o.Soedineniya))
	for _, c := range o.Soedineniya {
		port, _ := strconv.Atoi(c.Metadata.Port)
		s := Soedinenie{Id: c.Id, Host: c.Metadata.Host, Adres: c.Metadata.Adres, Port: port,
			Protsess: c.Metadata.ProcessPath, Pravilo: c.Pravilo, Nachalo: c.Nachalo}
		if len(c.Tsep) > 0 {
			s.Vyhod = c.Tsep[0]
		}
		itog = append(itog, s)
	}
	return itog, nil
}
