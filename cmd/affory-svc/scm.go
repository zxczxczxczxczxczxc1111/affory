package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/fon"
)

// Доклад диспетчеру служб о долгом шаге (Г2 и Г3 аудита 1.8.0).
//
// Старт службы это уборка брандмауэра, трекер процессов и чтение хранилища,
// остановка это отмена подъёма, опускание туннеля и снятие защиты. Прежде
// служба докладывала «запускается» или «останавливается» один раз, без
// отметки хода, и установщик ждал её ровно 30 секунд: медленная машина
// выглядела зависшей службой, и обновление откатывалось.
const (
	shagDokladaSCM = 2 * time.Second
	podskazkaSCM   = 10 * time.Second
)

// dokladyvatSCM шлёт SCM растущую отметку хода, пока не позовут hvatit.
// hvatit ждёт, пока доклад замолчит: следующий статус (Running или выход из
// Execute) обязан прийти после последней отметки, а не наперегонки с ней.
func dokladyvatSCM(st chan<- svc.Status, sost svc.State) (hvatit func()) {
	gotovo := make(chan struct{})
	zamolchal := make(chan struct{})
	fon.Zapustit("докладе диспетчеру служб", func() {
		defer close(zamolchal)
		otmetka := uint32(1)
		st <- svc.Status{State: sost, CheckPoint: otmetka, WaitHint: uint32(podskazkaSCM.Milliseconds())}
		t := time.NewTicker(shagDokladaSCM)
		defer t.Stop()
		for {
			select {
			case <-gotovo:
				return
			case <-t.C:
				otmetka++
				st <- svc.Status{State: sost, CheckPoint: otmetka, WaitHint: uint32(podskazkaSCM.Milliseconds())}
			}
		}
	})
	return func() {
		close(gotovo)
		<-zamolchal
	}
}

// zhdatSCMMaks это потолок ожидания службы, которая докладывает ход. Отметка
// растёт по таймеру, а не по делу, и без потолка зависший шаг ждали бы вечно.
const zhdatSCMMaks = 3 * time.Minute

func zhdatSostoyaniya(s *mgr.Service, hotim svc.State) error {
	return zhdatSostoyaniyaPo(s.Query, hotim, zhdatSCM, zhdatSCMMaks, 300*time.Millisecond)
}

// zhdatSostoyaniyaPo ждёт состояния так, как ждёт сам SCM: срок продлевается
// на подсказку службы всякий раз, когда растёт её отметка хода, но не дальше
// потолка. Служба, упавшая по дороге к Running, это отказ сразу, а не через
// полминуты.
func zhdatSostoyaniyaPo(sprosit func() (svc.Status, error), hotim svc.State, srok, potolok, shag time.Duration) error {
	nach := time.Now()
	do, predel := nach.Add(srok), nach.Add(potolok)
	var otmetka uint32
	for {
		st, err := sprosit()
		if err != nil {
			return fmt.Errorf("состояние службы не читается: %w", err)
		}
		if st.State == hotim {
			return nil
		}
		if hotim == svc.Running && st.State == svc.Stopped {
			return fmt.Errorf("служба остановилась, не дойдя до работы (код выхода %d)", st.Win32ExitCode)
		}
		if st.CheckPoint > otmetka {
			otmetka = st.CheckPoint
			podskazka := time.Duration(st.WaitHint) * time.Millisecond
			if podskazka <= 0 {
				podskazka = srok
			}
			if prodlit := time.Now().Add(podskazka); prodlit.After(do) {
				do = prodlit
			}
			if do.After(predel) {
				do = predel
			}
		}
		if !time.Now().Before(do) {
			return fmt.Errorf("служба не пришла в состояние %d за %s", hotim, time.Since(nach).Round(time.Second))
		}
		time.Sleep(shag)
	}
}

// Срок предвыключения задаётся явно (Г3). Умолчание Windows от версии к
// версии разное, а снятие защиты с записью файла состояния на медленном
// диске занимает секунды.
const srokPredvyklyucheniya = 30 * time.Second

// SERVICE_PRESHUTDOWN_INFO: в x/sys есть номер уровня, а структуры нет.
type predvyklyuchenieInfo struct{ timeout uint32 }

func zadatPredvyklyuchenie(s *mgr.Service, srok time.Duration) error {
	info := predvyklyuchenieInfo{timeout: uint32(srok.Milliseconds())}
	if err := windows.ChangeServiceConfig2(s.Handle, windows.SERVICE_CONFIG_PRESHUTDOWN_INFO, (*byte)(unsafe.Pointer(&info))); err != nil {
		return fmt.Errorf("срок предвыключения службы не задан: %w", err)
	}
	return nil
}

// avariynyyVyhod завершает процесс без доклада SERVICE_STOPPED (Г5 аудита
// 1.8.0). Для SCM это падение, и он перезапускает службу по правилам
// восстановления. Штатный выход с кодом 1 он падением не считает, и глухая
// служба без канала оставалась лежать остановленной. Флаг восстановления на
// штатных отказах не включаем: с ним воскресала бы и остановленная человеком.
var avariynyyVyhod = func(prichina error) {
	log.Printf("канал упал: %v. Завершаю процесс аварийно, диспетчер служб перезапустит службу", prichina)
	os.Exit(3)
}
