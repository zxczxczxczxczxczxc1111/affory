package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/puti"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// Выгрузка диагностики (О6 аудита 1.6.1).
//
// Каталог данных открыт только SYSTEM и администраторам, и обычный
// пользователь журналов не получал вовсе: Проводник отвечал отказом. Служба
// читает их сама, вычищает тем же, чем отказы подписки, и отдаёт окну
// порциями. Файл пишет окно от имени человека, служба по чужому пути не пишет.
// Разбор: vpn/docs/issledovaniya/2026-09-28-vygruzka-diagnostiki.md.

const (
	// Хвост одного журнала вместе с его файлом .1.
	hvostZhurnala = 1 << 20
	// Порция в base64 выходит на треть больше, и кадр с ней укладывается в
	// предел канала 1 МиБ (protokol/ramka.go).
	porciyaVygruzki = 512 << 10
	// Снимок, который никто не забирает, не держит память службы вечно.
	srokSnimkaVygruzki = 5 * time.Minute
)

type snimokVygruzki struct {
	mu     sync.Mutex
	id     string
	dannye []byte
	kogda  time.Time
}

func (s *Sluzhba) exportDiagnostics(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Id          string `json:"id"`
		Smeshchenie int    `json:"smeshchenie"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
		}
	}
	v := &s.vygruzkaDiag
	v.mu.Lock()
	defer v.mu.Unlock()
	if telo.Id == "" {
		id, err := nomerVygruzki()
		if err != nil {
			return otkazIz(k, protokol.KodVnutrennyayaOshibka, err)
		}
		v.id, v.dannye, v.kogda = id, s.sobratDiagnostiku(), time.Now()
	} else if telo.Id != v.id || time.Since(v.kogda) > srokSnimkaVygruzki {
		return otkaz(k.Id, k.Imya, protokol.KodDiagnostikaUstarela,
			"выгрузка диагностики прервалась, сохрани её заново")
	}
	if telo.Smeshchenie < 0 || telo.Smeshchenie > len(v.dannye) {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch,
			fmt.Sprintf("смещение %d вне выгрузки в %d байт", telo.Smeshchenie, len(v.dannye)))
	}
	konec := min(telo.Smeshchenie+porciyaVygruzki, len(v.dannye))
	otv := otvet(k.Id, k.Imya, map[string]any{
		"id":          v.id,
		"vsego":       len(v.dannye),
		"smeshchenie": telo.Smeshchenie,
		"kusok":       base64.StdEncoding.EncodeToString(v.dannye[telo.Smeshchenie:konec]),
	})
	// Последняя порция отдана: несколько мегабайт памяти службе больше не нужны.
	if konec == len(v.dannye) {
		v.id, v.dannye = "", nil
	}
	return otv
}

func nomerVygruzki() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("номер выгрузки не выдан: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// sobratDiagnostiku собирает шапку и хвосты журналов и вычищает всё разом.
// Отказ одного источника не срывает выгрузку: он записывается в файл строкой,
// и это тоже сведения.
func (s *Sluzhba) sobratDiagnostiku() []byte {
	var b bytes.Buffer
	b.WriteString("Affory, выгрузка диагностики\n")
	fmt.Fprintf(&b, "служба: %s\n", diagnostika.Sborka(versiyaProgrammy))
	s.shapkaDiagnostiki(&b)
	if status, err := json.MarshalIndent(s.Status(), "", "  "); err == nil {
		fmt.Fprintf(&b, "статус:\n%s\n", status)
	} else {
		fmt.Fprintf(&b, "статус не собран: %v\n", err)
	}

	var servery []protokol.Server
	var adresa []string
	n, err := s.nabor()
	if err == nil {
		servery = n.Servery
		for _, p := range n.Podpiski {
			adresa = append(adresa, p.Adres)
		}
		if n.Podpiska != "" {
			adresa = append(adresa, n.Podpiska)
		}
	} else {
		// Без набора известные секреты неизвестны, но вычистка по виду ссылок
		// работает и так. Человеку это надо знать до отправки файла.
		fmt.Fprintf(&b, "набор серверов не прочитан (%v): ключи вычищены только по виду ссылок\n", err)
	}

	katalog := filepath.Join(s.dirDannyh, puti.PodkatalogZhurnalov)
	for _, imya := range puti.ZhurnalySluzhby {
		zhurnaly.DopisatRazdel(&b, imya, filepath.Join(katalog, imya), hvostZhurnala)
	}
	return []byte(ssylki.Vychistit(b.String(), servery, adresa))
}

