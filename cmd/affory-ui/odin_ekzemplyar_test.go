package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// О1 аудита 1.6.1. Второй запуск показывает живое окно, а второй --trey (вход
// в сеанс при уже открытом окне) не делает ничего.
func TestVtoroyZapuskPokazyvaetOkno(t *testing.T) {
	pokazano := 0
	o := odinEkzemplyar(func() { pokazano++ })
	if o == nil || o.UniqueID == "" || o.OnSecondInstanceLaunch == nil {
		t.Fatalf("одиночный запуск не задан: %+v", o)
	}
	o.OnSecondInstanceLaunch(application.SecondInstanceData{Args: []string{`C:\Affory\affory-ui.exe`}})
	if pokazano != 1 {
		t.Fatalf("второй запуск показал окно %d раз", pokazano)
	}
	o.OnSecondInstanceLaunch(application.SecondInstanceData{Args: []string{`C:\Affory\affory-ui.exe`, flagTrey}})
	if pokazano != 1 {
		t.Fatal("второй --trey вытащил окно из трея")
	}
}

// Имя мьютекса повторяет то, что даёт ему Wails: разойдись они, преемник не
// ждал бы прежнего окна вовсе.
func TestImyaMyuteksaKakUWails(t *testing.T) {
	if imyaMyuteksa() != "wails-app-"+idOkna+"-sim" {
		t.Fatal(imyaMyuteksa())
	}
}

// Преемник (окно с правами, окно после обновления) ждёт, пока прежнее окно
// отпустит мьютекс. Иначе он увидел бы живой экземпляр, передал бы ему
// сигнал и вышел, а прежнее через миг закрылось бы само, и окна не осталось.
func TestPreemnikZhdyotUkhodaPrezhnego(t *testing.T) {
	imya := fmt.Sprintf("affory-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	u, err := windows.UTF16PtrFromString(imya)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateMutex(nil, false, u)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		windows.CloseHandle(h)
	}()
	nachalo := time.Now()
	if !dozhdatsyaMyuteksa(imya, 5*time.Second) {
		t.Fatal("ожидание кончилось сроком, хотя мьютекс отпустили")
	}
	if proshlo := time.Since(nachalo); proshlo < 250*time.Millisecond || proshlo > 2*time.Second {
		t.Fatalf("ждал %v", proshlo)
	}
	// Мьютекса нет вовсе: ждать нечего.
	nachalo = time.Now()
	if !dozhdatsyaMyuteksa(imya, 5*time.Second) || time.Since(nachalo) > 200*time.Millisecond {
		t.Fatal("ждал несуществующий мьютекс")
	}
}
