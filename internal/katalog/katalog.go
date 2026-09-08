package katalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

//go:embed servisy.json
var dannye []byte

type Servis struct {
	Id        string   `json:"id"`
	Imya      string   `json:"imya"`
	Domeny    []string `json:"domeny"`
	Istochnik string   `json:"istochnik"`
}

type Katalog struct {
	Versiya    string   `json:"versiya"`
	Reviziya   string   `json:"reviziya"`
	Istochnik  string   `json:"istochnik"`
	Litsenziya string   `json:"litsenziya"`
	Servisy    []Servis `json:"servisy"`
}

func Chitat() (Katalog, error) {
	var k Katalog
	if err := json.Unmarshal(dannye, &k); err != nil {
		return k, fmt.Errorf("каталог сервисов повреждён: %w", err)
	}
	if len(k.Servisy) == 0 || k.Versiya == "" {
		return k, fmt.Errorf("каталог сервисов пуст")
	}
	for _, s := range k.Servisy {
		if s.Id == "" || len(s.Domeny) == 0 || !strings.HasPrefix(s.Istochnik, "https://github.com/rekryt/iplist/") {
			return k, fmt.Errorf("неполный набор сервиса %q", s.Id)
		}
	}
	return k, nil
}

func Domeny(id string) ([]string, error) {
	k, err := Chitat()
	if err != nil {
		return nil, err
	}
	for _, s := range k.Servisy {
		if s.Id == id {
			return slices.Clone(s.Domeny), nil
		}
	}
	return nil, fmt.Errorf("неизвестный сервис %q", id)
}
