package sostoyanie

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// RasshirenieZapasa это суффикс прежней версии файла рядом с ним.
const RasshirenieZapasa = ".bak"

// ZapisatNadyozhno кладёт telo в put так, чтобы ни оборванная запись, ни
// пропавшее питание не оставили на месте файла мусор (Н3 аудита 1.6.1).
//
// Прежде три файла писались через общий `.tmp` и os.Rename. Переименование на
// NTFS атомарно, но без Sync и без WRITE_THROUGH оно может дойти до диска
// раньше самих байтов: после пропавшего питания на месте файла оказывается
// нулевой хвост. Для файла отката это машина, которая не знает, что заперта;
// для секретов это все серверы человека.
//
// Порядок:
//  1. тело пишется во временный файл с уникальным именем в том же каталоге и
//     сбрасывается на диск. Имя уникальное: два писателя с общим `.tmp`
//     затирали бы друг другу запись посреди неё;
//  2. прежняя версия, если godno её принимает, уезжает в put+".bak" тем же
//     порядком. Битый файл не должен затереть годный запас; nil у godno
//     значит «годна любая»;
//  3. временный файл встаёт на место через MoveFileEx с WRITE_THROUGH: вызов
//     возвращается, только когда перенос дошёл до диска.
func ZapisatNadyozhno(put string, telo []byte, godno func([]byte) bool) error {
	if staroe, err := os.ReadFile(put); err == nil && (godno == nil || godno(staroe)) {
		if err := polozhit(put+RasshirenieZapasa, staroe); err != nil {
			return fmt.Errorf("запасная копия не записана: %w", err)
		}
	}
	return polozhit(put, telo)
}

// ProchitatSZapasom читает put, а если он не читается или razobrat его не
// принимает, берёт запасную копию. izZapasa говорит, что ответ из запаса.
//
// Отсутствие put это отсутствие, и запас тогда не читается: удалённый файл не
// должен воскресать из копии. Ошибка при двух негодных копиях та, что у
// основной: она и объясняет, что случилось.
func ProchitatSZapasom(put string, razobrat func([]byte) error) (telo []byte, izZapasa bool, err error) {
	telo, err = os.ReadFile(put)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	if err == nil {
		if err = razobrat(telo); err == nil {
			return telo, false, nil
		}
	}
	zapas, errZ := os.ReadFile(put + RasshirenieZapasa)
	if errZ == nil && razobrat(zapas) == nil {
		return zapas, true, nil
	}
	return nil, false, err
}

// UdalitSZapasom удаляет файл вместе с запасной копией. Отсутствие любого из
// двух не ошибка.
func UdalitSZapasom(put string) error {
	var oshibki []error
	for _, p := range []string{put, put + RasshirenieZapasa} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			oshibki = append(oshibki, err)
		}
	}
	return errors.Join(oshibki...)
}

func polozhit(put string, telo []byte) error {
	f, err := os.CreateTemp(filepath.Dir(put), filepath.Base(put)+".*.tmp")
	if err != nil {
		return fmt.Errorf("временный файл не создан: %w", err)
	}
	vremen := f.Name()
	_, err = f.Write(telo)
	if err == nil {
		err = f.Sync()
	}
	if errZ := f.Close(); err == nil {
		err = errZ
	}
	if err == nil {
		err = perenesti(vremen, put)
	}
	if err != nil {
		// Временный файл наш и уже мусор: оставленный, он копился бы с
		// каждым отказом. Отказ убрать его приклеивается к основной причине.
		if errU := os.Remove(vremen); errU != nil && !errors.Is(errU, os.ErrNotExist) {
			err = errors.Join(err, errU)
		}
		return err
	}
	return nil
}

// PopytokPerenosa ограничивает ожидание занятой цели: пауза вдвое от 50 мс,
// всего около трёх секунд. Дольше сканер килобайтный файл не держит, а человек
// за это время ещё не начал гадать, что случилось.
const PopytokPerenosa = 7

// spat это шов: тест отказа переноса не должен спать по-настоящему.
var spat = time.Sleep

// perenesti ставит ot на место k и переживает занятую цель.
//
// Файл, открытый кем угодно (Defender проверяет свежую запись, наш же
// читатель мгновением раньше), не заменяется: ERROR_SHARING_VIOLATION или
// ERROR_ACCESS_DENIED. Прогон в госте 02.09.2026 потерял на этом обновление
// подписки. Остальные отказы настоящие и возвращаются сразу.
func perenesti(ot, k string) error {
	otU, err := windows.UTF16PtrFromString(ot)
	if err != nil {
		return err
	}
	kU, err := windows.UTF16PtrFromString(k)
	if err != nil {
		return err
	}
	pauza := 50 * time.Millisecond
	for i := 0; ; i++ {
		err = windows.MoveFileEx(otU, kU, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if !zanyat(err) || i == PopytokPerenosa-1 {
			return fmt.Errorf("%s не встал на место %s: %w", filepath.Base(ot), filepath.Base(k), err)
		}
		spat(pauza)
		pauza *= 2
	}
}

func zanyat(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
