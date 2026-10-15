package main

import (
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Срок ответа на подключение против худшего подъёма (жалоба 15.10.2026).
//
// Окно ждёт ответа на connect столько, сколько велит protokol.SrokOtveta, и до
// 16.10.2026 это была минута. В госте при молчащих серверах подъём шёл 34 с по
// TCP и 43 с по hy2, а на машине медленнее стенда переходил за минуту: окно
// рисовало «не получилось выполнить действие», пока служба ещё поднимала.
//
// Замер здесь не годится: на здоровой машине подъём укладывается в секунды и
// не говорит ничего. Дорог ХУДШИЙ случай, а он складывается из ожиданий, у
// каждого из которых свой потолок.
const (
	// Из internal/yadra/klash.go, где они не экспортируются: ядро обязано
	// ответить по управляющему порту за srokGotov после старта, а любой запрос
	// к нему, от навязывания выбора до пробы, ждёт ответа до srokZaprosa.
	srokGotovYadra   = 5 * time.Second
	srokZaprosaYadra = 10 * time.Second
	// Чего в сумме нет вовсе: пачки netsh, проверки конфига ядром, чтения
	// набора. Сроков у них нет, и на слабой машине они идут секундами.
	zapasPodyoma = 30 * time.Second
)

// hudshayaPopytka это одна попытка connect: уход прежнего адаптера, новый
// адаптер, ответ ядра, навязанный выбор и окно проб, в конце которого
// последняя проба начинается за миг до срока и ждёт свой запрос целиком.
func hudshayaPopytka() time.Duration {
	return zhdatUhodaTun + zhdatAdaptera + srokGotovYadra + srokZaprosaYadra + zhdatPodyoma + srokZaprosaYadra
}

func TestSrokOtvetaPerekryvaetHudshiyPodyom(t *testing.T) {
	hudshiy := time.Duration(popytokPodyoma)*hudshayaPopytka() + srokUtochneniya
	if srok := protokol.SrokOtveta("connect"); srok < hudshiy+zapasPodyoma {
		t.Errorf("connect: окно ждёт %v, а худший подъём %v и запас %v", srok, hudshiy, zapasPodyoma)
	}
}

// Смена сервера на живом ядре: замок выбора, который подъём держит на
// навязывании, прежний выбор, новый, проба, откат, вопрос «кто несёт», каждый
// поход до srokZaprosa, и доспрос причины отказа.
func TestSrokOtvetaPerekryvaetHudshuyuSmenuServera(t *testing.T) {
	hudshaya := 6*srokZaprosaYadra + srokUtochneniya
	if srok := protokol.SrokOtveta("setServer"); srok < hudshaya+zapasPodyoma {
		t.Errorf("setServer: окно ждёт %v, а худшая смена %v и запас %v", srok, hudshaya, zapasPodyoma)
	}
}
