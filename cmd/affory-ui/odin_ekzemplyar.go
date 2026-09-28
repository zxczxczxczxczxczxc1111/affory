package main

import (
	"errors"
	"log"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// Одно окно на сеанс (О1 аудита 1.6.1). Два окна это два трея, два
// наблюдателя и две подписки на события службы, а человек, щёлкнувший ярлык
// при окне в трее, получал второе окно вместо своего.

// idOkna это UniqueID одиночного запуска Wails.
const idOkna = "affory-okno"

// flagSmena передаётся окну-преемнику: окну с правами (PerezapustitSPravami) и
// окну после обновления (PerezapustitOkno). Прежнее окно при этом ещё живо и
// уходит само чуть позже. Без флага преемник увидел бы живой экземпляр,
// передал бы ему сигнал и вышел, а прежнее закрылось бы следом, и окна не
// осталось бы вовсе.
const flagSmena = "--smena"

// Сколько преемник ждёт прежнее окно. Оно уходит через 0,7 с после запуска
// преемника, запас на медленную машину.
const srokSmeny = 10 * time.Second

// odinEkzemplyar это опции одиночного запуска. Второй запуск показывает живое
// окно, а второй --trey (вход в сеанс при уже открытом окне) не делает ничего.
func odinEkzemplyar(pokazat func()) *application.SingleInstanceOptions {
	return &application.SingleInstanceOptions{
		UniqueID: idOkna,
		OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
			if slices.Contains(d.Args, flagTrey) {
				return
			}
			pokazat()
		},
	}
}

// imyaMyuteksa повторяет имя, которое Wails (v3.0.0-beta.26,
// single_instance_windows.go) даёт мьютексу одиночного запуска. Сверяется
// тестом, а при обновлении Wails (Б9) перепроверяется по исходникам.
func imyaMyuteksa() string { return "wails-app-" + idOkna + "-sim" }

// dozhdatsyaMyuteksa ждёт, пока именованный мьютекс исчезнет, то есть пока
// прежний процесс не выйдет. false значит, что срок вышел.
func dozhdatsyaMyuteksa(imya string, srok time.Duration) bool {
	u, err := windows.UTF16PtrFromString(imya)
	if err != nil {
		log.Printf("имя мьютекса окна негодно: %v", err)
		return false
	}
	konec := time.Now().Add(srok)
	for {
		h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, u)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return true
		}
		// Мьютекс есть. Отказ в доступе (прежнее окно с правами, это без них)
		// тоже значит, что он есть: ждём так же.
		if err == nil {
			windows.CloseHandle(h)
		}
		if !time.Now().Before(konec) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
