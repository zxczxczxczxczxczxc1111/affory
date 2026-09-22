package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Проверка программ в карточке сервиса (D2, 22.09.2026).
//
// Карточка несёт пути клиента, найденные окном среди запущенных программ.
// Служба принимает их на веру ровно настолько, насколько может проверить: имя
// файла обязано быть в каталоге у ЭТОГО сервиса.

func trafikSoSteam(puti ...string) protokol.PravilaTrafika {
	return protokol.PravilaTrafika{PoUmolchaniyu: protokol.TrafikVPN,
		Servisy: []protokol.PraviloServisa{{Id: "steam", Marshrut: protokol.TrafikVPN, Programmy: puti}}}
}

func TestProgrammaServisaPrinimaetsyaINormalizuetsya(t *testing.T) {
	put := faylProby(t)
	r, err := proveritTrafik(trafikSoSteam(strings.ToUpper(put)), protokol.PravilaTrafika{})
	if err != nil {
		t.Fatalf("путь настоящей программы не принят: %v", err)
	}
	if len(r.Servisy) != 1 || len(r.Servisy[0].Programmy) != 1 {
		t.Fatalf("программы сервиса: %+v", r.Servisy)
	}
	// Нормализованный путь, а не то, что прислало окно: ядро сравнивает строку.
	if r.Servisy[0].Programmy[0] == strings.ToUpper(put) {
		t.Error("путь не нормализован: ядро сравнивает строку и не найдёт процесс")
	}
}

func TestChuzhoyExeVKartochkeServisaOtvergnut(t *testing.T) {
	// Иначе карточка «Steam» вела бы маршрут для любого файла, который окно в
	// неё положило: это правило приложения под чужой подписью.
	chuzhoy := filepath.Join(t.TempDir(), "chuzhoy.exe")
	_, err := proveritTrafik(trafikSoSteam(chuzhoy), protokol.PravilaTrafika{})
	if !errors.Is(err, errPraviloNegodno) {
		t.Fatalf("ошибка %v, ждали отказ", err)
	}
	if !strings.Contains(err.Error(), "chuzhoy.exe") {
		t.Errorf("отказ не называет файл: %v", err)
	}
}

func TestProgrammaServisaTrebuetPolnogoPutiKExe(t *testing.T) {
	for _, put := range []string{`steam.exe`, `C:\Steam\steam.dll`} {
		if _, err := proveritTrafik(trafikSoSteam(put), protokol.PravilaTrafika{}); !errors.Is(err, errPraviloNegodno) {
			t.Errorf("%q принят: %v", put, err)
		}
	}
}

func TestSnesennayaProgrammaServisaNeLomaetSpisok(t *testing.T) {
	// Тот же тупик, что уже стоил продукту отказа на ВЕСЬ список: человек снёс
	// игру, и править правила стало нельзя вовсе, включая осиротевшее.
	propal := filepath.Join(t.TempDir(), "Steam", "steam.exe")
	bylo := trafikSoSteam(propal)
	r, err := proveritTrafik(trafikSoSteam(propal), bylo)
	if err != nil {
		t.Fatalf("принятый ранее путь отвергнут после пропажи файла: %v", err)
	}
	if len(r.Servisy[0].Programmy) != 1 || r.Servisy[0].Programmy[0] != propal {
		t.Fatalf("путь потерян: %+v", r.Servisy)
	}
	// А НОВЫЙ путь, которого на диске нет, по-прежнему не принимается.
	novyy := filepath.Join(t.TempDir(), "Steam2", "steam.exe")
	if _, err := proveritTrafik(trafikSoSteam(novyy), bylo); !errors.Is(err, errPraviloNegodno) {
		t.Errorf("выдуманный путь принят: %v", err)
	}
}

func TestProgrammyServisaDedupBezUchetaRegistra(t *testing.T) {
	put := faylProby(t)
	r, err := proveritTrafik(trafikSoSteam(put, strings.ToUpper(put)), protokol.PravilaTrafika{})
	if err != nil {
		t.Fatalf("отказ: %v", err)
	}
	if len(r.Servisy[0].Programmy) != 1 {
		t.Fatalf("одна программа записана дважды: %v", r.Servisy[0].Programmy)
	}
}

func TestSlishkomMnogoProgrammVKartochke(t *testing.T) {
	puti := make([]string, programmNaServis+1)
	for i := range puti {
		puti[i] = faylProby(t)
	}
	if _, err := proveritTrafik(trafikSoSteam(puti...), protokol.PravilaTrafika{}); !errors.Is(err, errPraviloNegodno) {
		t.Fatalf("список без границы принят: %v", err)
	}
}
