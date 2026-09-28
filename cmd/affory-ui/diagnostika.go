package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// Выгрузка диагностики (О6 аудита 1.6.1). Каталог журналов открыт только
// администраторам, поэтому журналы собирает служба и отдаёт порциями, а файл
// пишет окно от имени человека по пути из диалога. Содержимое через страницу
// не проходит: она узнаёт только итог.

// Шов для тестов: диалог сохранения.
var vybratKudaDiagnostiku = (*most).dialogDiagnostiki

// Предел числа порций: выгрузка шести журналов по 1 МиБ это полтора десятка
// порций, и служба, которая отдаёт их без конца, это дефект, а не журнал.
const maksPorciy = 64

func (m *most) dialogDiagnostiki(imya string) (string, error) {
	d := m.app.Dialog.SaveFile().
		SetMessage("Куда сохранить диагностику Affory").
		SetFilename(imya).
		AddFilter("Текст (*.txt)", "*.txt")
	if kat := katalogDiagnostiki(); kat != "" {
		d = d.SetDirectory(kat)
	}
	return otvetDialoga(d.PromptForSingleSelection())
}

// katalogDiagnostiki это «Документы» человека.
//
// Без каталога диалог открывается в рабочем каталоге процесса, а ярлык
// запускает окно из Program Files: человеку без прав туда не записать, и на
// «Сохранить» он первым делом получал отказ Windows. Выгрузка сделана как раз
// для него (приёмка 1.7.0 в госте, 28.09.2026).
//
// Отказ оболочки не повод не показывать диалог: пустой каталог значит, что
// папку выберет сам диалог, как до этой правки.
func katalogDiagnostiki() string {
	put, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		log.Printf("папка «Документы» не найдена, диалог диагностики откроется где решит сам: %v", err)
		return ""
	}
	return put
}

// SohranitDiagnostiku спрашивает, куда сохранить, забирает выгрузку у службы и
// пишет файл. Пустая строка без ошибки значит, что человек передумал.
func (m *most) SohranitDiagnostiku() (string, error) {
	imya := "affory-diagnostika-" + time.Now().Format("2006-01-02-150405") + ".txt"
	put, err := vybratKudaDiagnostiku(m, imya)
	if err != nil || put == "" {
		return "", err
	}
	dannye, err := m.zabratDiagnostiku()
	if err != nil {
		return "", err
	}
	if err := zapisatTselikom(put, dannye); err != nil {
		return "", err
	}
	return fmt.Sprintf("сохранено: %s, %s", filepath.Base(put), razmerFayla(len(dannye))), nil
}

func (m *most) zabratDiagnostiku() ([]byte, error) {
	var id string
	var dannye []byte
	for range maksPorciy {
		telo, err := json.Marshal(map[string]any{"id": id, "smeshchenie": len(dannye)})
		if err != nil {
			return nil, err
		}
		k, err := m.komandaSluzhbe("exportDiagnostics", telo)
		if err != nil {
			return nil, err
		}
		var p struct {
			Id          string `json:"id"`
			Vsego       int    `json:"vsego"`
			Smeshchenie int    `json:"smeshchenie"`
			Kusok       string `json:"kusok"`
		}
		if err := json.Unmarshal(k.Telo, &p); err != nil {
			return nil, fmt.Errorf("порция диагностики не разобрана: %w", err)
		}
		if p.Smeshchenie != len(dannye) {
			return nil, fmt.Errorf("служба отдала порцию со смещения %d, а ждали %d", p.Smeshchenie, len(dannye))
		}
		kusok, err := base64.StdEncoding.DecodeString(p.Kusok)
		if err != nil {
			return nil, fmt.Errorf("порция диагностики не декодируется: %w", err)
		}
		id = p.Id
		dannye = append(dannye, kusok...)
		if len(dannye) >= p.Vsego {
			return dannye, nil
		}
		if len(kusok) == 0 {
			return nil, errors.New("служба отдала пустую порцию посреди выгрузки")
		}
	}
	return nil, fmt.Errorf("выгрузка не закончилась за %d порций", maksPorciy)
}

// zapisatTselikom пишет во временный файл рядом и подменяет им целевой: при
// отказе на полпути прежний файл остаётся как был, а полфайла не остаётся.
func zapisatTselikom(put string, dannye []byte) error {
	vremennyy, err := os.CreateTemp(filepath.Dir(put), ".affory-diagnostika-*")
	if err != nil {
		return fmt.Errorf("файл не создан: %w", err)
	}
	imya := vremennyy.Name()
	_, err = vremennyy.Write(dannye)
	if errZakr := vremennyy.Close(); err == nil {
		err = errZakr
	}
	if err == nil {
		err = os.Rename(imya, put)
	}
	if err != nil {
		if errUd := os.Remove(imya); errUd != nil && !errors.Is(errUd, os.ErrNotExist) {
			return fmt.Errorf("файл не записан: %w (временный %s остался: %v)", err, imya, errUd)
		}
		return fmt.Errorf("файл не записан: %w", err)
	}
	return nil
}

func razmerFayla(n int) string {
	switch {
	case n >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f МБ", float64(n)/(1<<20)), ".", ",", 1)
	case n >= 1<<10:
		return fmt.Sprintf("%d КБ", n>>10)
	default:
		return fmt.Sprintf("%d Б", n)
	}
}
