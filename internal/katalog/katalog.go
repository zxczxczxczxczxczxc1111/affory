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
	Id     string   `json:"id"`
	Imya   string   `json:"imya"`
	// Без omitempty: каталог уезжает в окно как есть, а пропавшее поле там
	// читается как сбой формата, не как «доменов нет».
	Domeny []string `json:"domeny"`
	// Programmy это ИМЕНА файлов клиента сервиса (D2, 22.09.2026): Discord.exe,
	// steam.exe. Не пути: у Discord в пути номер сборки, у лаунчеров диск
	// установки, и прибитый путь дал бы правило на несуществующий файл. Путь
	// берётся с машины среди запущенных, как в C6.
	//
	// Сервис без доменов это законная запись: у Steam и Epic Games набора
	// доменов в iplist нет вовсе, а маршрут человеку нужен именно по программе.
	Programmy []string `json:"programmy"`
	Istochnik string   `json:"istochnik,omitempty"`
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
		if s.Id == "" || s.Imya == "" {
			return k, fmt.Errorf("неполный набор сервиса %q", s.Id)
		}
		// Пустой и там, и там это запись ни о чём: правило по ней не накроет
		// ничего, а на экране выглядело бы работающим.
		if len(s.Domeny) == 0 && len(s.Programmy) == 0 {
			return k, fmt.Errorf("сервис %q не несёт ни доменов, ни программ", s.Id)
		}
		// Источник спрашивается с ДОМЕНОВ: они чужие, приехали из iplist, и
		// сверять их есть с чем. Имена наших программ ниоткуда не приезжают.
		if len(s.Domeny) > 0 && !strings.HasPrefix(s.Istochnik, "https://github.com/rekryt/iplist/") {
			return k, fmt.Errorf("у доменов сервиса %q нет источника", s.Id)
		}
	}
	// Пустой список, а не nil: каталог уезжает в окно, а `null` на месте
	// массива там означает «не мерили» по контракту fallback и рисуется
	// прочерком вместо нуля.
	for i := range k.Servisy {
		if k.Servisy[i].Domeny == nil {
			k.Servisy[i].Domeny = []string{}
		}
		if k.Servisy[i].Programmy == nil {
			k.Servisy[i].Programmy = []string{}
		}
	}
	return k, nil
}

// Est отвечает, знает ли ТЕКУЩИЙ каталог такой сервис.
//
// Вопрос не праздный: каталог едет внутри программы и меняется с её выпусками.
// WhatsApp и Netflix ушли из него 16.09.2026, и правило, записанное прежней
// версией, осталось лежать в наборе, указывая в пустоту.
func Est(id string) bool {
	_, err := Domeny(id)
	return err == nil
}

// Programmy отдаёт имена файлов клиента сервиса. Пустой список это «клиента у
// сервиса нет», а не ошибка: у YouTube его и правда нет.
func Programmy(id string) ([]string, error) {
	k, err := Chitat()
	if err != nil {
		return nil, err
	}
	for _, s := range k.Servisy {
		if s.Id == id {
			return slices.Clone(s.Programmy), nil
		}
	}
	return nil, fmt.Errorf("неизвестный сервис %q", id)
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
