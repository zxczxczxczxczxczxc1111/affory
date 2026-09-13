package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Ошибка ровно того вида, в каком она приходит от клиента clash_api, когда
// процесса ядра уже нет: система отвергает соединение, не дойдя до сокета.
func otvergnutoeSoedinenie() error {
	return fmt.Errorf("clash_api не отвечает: %w", &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: os.NewSyscallError("connectex", syscall.Errno(10061)),
	})
}

// Режим МЕНЯЕТСЯ, когда ядра нет на связи, и это не то же самое, что отказ
// живого ядра (см. TestSetRouteModeNeZapisyvaetRezhimKogdaYadroOtvergloVybor).
//
// Поймано живым прогоном в госте 13.09.2026. Ядро легло, наблюдатель опустил
// его и запустил восстановление, человек в этот момент переключает режим в
// авто. PUT отвергается на уровне соединения, команда отвечает switch-failed,
// набор остаётся ручным, и восстановление пять минут подряд поднимает ровно
// тот сервер, который лёг. Нажать ещё раз не помогает: попадаешь в то же окно.
func TestSetRouteModeZapisyvaetRezhimKogdaYadraNetNaSvyazi(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)
	s.postavitVybor = func(context.Context, string, string, string, string) error {
		return otvergnutoeSoedinenie()
	}

	o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"})
	if o.Oshib != nil {
		t.Fatalf("ядра нет на связи, а команда ответила отказом %q: %s", o.Oshib.Kod, o.Oshib.Tekst)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Rezhim != protokol.RezhimAvto {
		t.Errorf("режим в наборе %q, а человек переключил в авто: следующий подъём снова возьмёт прежний выбор", n.Rezhim)
	}
}

// Сервер ВЫБИРАЕТСЯ, когда ядра нет на связи, по той же причине и тем же
// правилом, что и режим выше.
//
// «Сервер лёг, выберу другой» это самое первое, что делает человек, и именно
// в этот момент ядра уже нет: авария опустила его секундой раньше. До
// починки команда отвечала «текущий выбор ядра не прочитан» и не запоминала
// НИЧЕГО, то есть отказывала ровно там, где нужнее всего.
func TestSetServerZapominaetVyborKogdaYadraNetNaSvyazi(t *testing.T) {
	konfigVyboraDlyaTesta(t, konfigProby)
	s := sZhivymTunnelem(t)
	s.vyborGruppy = func(context.Context, string, string, string) (string, error) {
		return "", otvergnutoeSoedinenie()
	}

	if err := s.setServer(context.Background(), "de"); err != nil {
		t.Fatalf("ядра нет на связи, а выбор сервера отказал: %v", err)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if n.Vybran != "de" {
		t.Errorf("в наборе выбран %q, а человек выбрал de: подъём вернёт его на лёгший сервер", n.Vybran)
	}
}
