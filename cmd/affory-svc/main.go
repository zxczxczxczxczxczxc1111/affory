// affory-svc is the half that has rights. It runs as LocalSystem, owns the
// cores, the routes and the firewall, and talks to the interface through one
// named pipe. Everything it can refuse to do, it refuses.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"golang.org/x/sys/windows/svc"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/zhurnaly"
)

// versiyaProgrammy подставляет сборка выпуска (ustanovka\sobrat-reliz.ps1) через
// -ldflags -X; в дереве разработки остаётся dev.
var versiyaProgrammy = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			put, err := os.Executable()
			if err != nil {
				log.Fatalf("не удалось узнать свой путь: %v", err)
			}
			if err := ustanovit(put); err != nil {
				log.Fatalf("установка не удалась: %v", err)
			}
			fmt.Println("служба установлена")
			return
		case "prepare-install":
			if err := podgotovitUstanovku(); err != nil {
				log.Fatalf("подготовка установки не удалась: %v", err)
			}
			return
		case rezhimPodmeny:
			// Подменщик: копия новой службы во временном каталоге (задача 6.5).
			if len(os.Args) < 4 {
				log.Fatalf("swap: нужны каталог программы и каталог новой сборки")
			}
			podmenit(os.Args[2], os.Args[3])
			return
		case "uninstall":
			if err := snyat(); err != nil {
				log.Fatalf("снятие не удалось: %v", err)
			}
			fmt.Println("служба снята")
			// Keys go only on the explicit flag the interface sets after the
			// human answered. Program directory removal is scheduled from
			// outside: this binary is inside it.
			if err := snyatDannye(sostoyanie.KatalogDannyh(), steretKlyuchiIz(os.Args)); err != nil {
				log.Fatalf("данные не удалены: %v", err)
			}
			if steretKlyuchiIz(os.Args) {
				fmt.Println("данные и ключи стёрты")
			} else {
				fmt.Println("данные и ключи оставлены")
			}
			if err := udalitKatalogProgrammy(); err != nil {
				log.Fatalf("%v", err)
			}
			// Про окно сказано вслух: снятие ЗАКРЫВАЕТ его, и человек, у которого
			// оно было открыто, узнаёт причину до того, как удивится.
			fmt.Println("каталог программы удаляется, окно закрывается вместе с ним")
			return
		}
	}
	// No arguments means SCM started us. A human running this by hand gets a
	// confusing hang instead of an error, which is exactly what every Windows
	// service does, so at least we are in good company.
	if err := svc.Run(imyaSluzhby, &sluzhba{}); err != nil {
		log.Fatalf("служба упала: %v", err)
	}
}

type sluzhba struct{}

func (s *sluzhba) Execute(args []string, r <-chan svc.ChangeRequest, st chan<- svc.Status) (bool, uint32) {
	st <- svc.Status{State: svc.StartPending}

	// A service has no console, so log.Printf writes into the void and every
	// failure looks like silence. Found the hard way: the pipe refused a client
	// and the reason was printed to nobody. The journal proper arrives in wave 6;
	// this is the minimum that makes the service debuggable at all.
	// Три файла на три компонента в `log\`, с ротацией по 10 МБ и тремя
	// поколениями каждый (долги 0 и 4 волны 6): жизнь службы, жалобы ядра и
	// журнал команд читаются по отдельности. Отказ открыть файл службу не
	// останавливает: без журнала хуже, чем без сети, но не наоборот.
	var zhurnalKomand *log.Logger
	if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "sluzhba.log"); err == nil {
		log.SetOutput(zh)
		defer zh.Close()
	}
	if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "yadro.log"); err == nil {
		yadra.Zhurnal = log.New(zh, "", log.LstdFlags)
		defer zh.Close()
	}
	if zh, err := zhurnaly.Otkryt(sostoyanie.KatalogZhurnalov(), "komandy.log"); err == nil {
		zhurnalKomand = log.New(zh, "", log.LstdFlags)
		defer zh.Close()
	}
	log.Printf("служба запускается, программа %s, версия протокола %d", versiyaProgrammy, protokol.Versiya)

	// Осиротевшая защита снимается ДО того, как служба начнёт принимать команды.
	// Машина, оставшаяся запертой после нештатной смерти службы, не должна ждать,
	// пока кто-то догадается подключиться к ней по каналу, который сам по этой
	// же причине может быть недоступен.
	zapertaNamerenno := false
	if snyato, zaperta, err := set.SnyatOsirotevshee(); err != nil {
		log.Printf("осиротевшую защиту снять не удалось: %v", err)
	} else if snyato {
		log.Printf("снята осиротевшая защита (%s)", protokol.KodKillswitchOrphan)
	} else if zaperta {
		zapertaNamerenno = true
		// Не ошибка и не повод вмешиваться: человек запер машину намеренно, и
		// распечатать её сам себе значит отменить единственное, ради чего режим
		// включают. Но сказать вслух обязаны.
		log.Printf("машина осталась запертой НАМЕРЕННО, туннеля нет (%s)", protokol.KodKillswitchOrphan)
	}

	// Права каталога данных выставляются не только при установке, но и на
	// КАЖДОМ старте. Каталог можно удалить руками, и созданный заново он
	// унаследует права родителя, то есть станет читаемым всем. Вызов
	// идемпотентен: /inheritance:r и /grant:r заменяют, а не копят.
	//
	// Отказ здесь службу НЕ останавливает: секреты и так лежат под DPAPI, а
	// остановка службы при запертой машине оставила бы человека без сети и без
	// команды, которой это чинить.
	if err := sostoyanie.ZavestiKatalogDannyh(); err != nil {
		log.Printf("права каталога данных не выставлены при старте: %v", err)
	}

	ctx, otmena := context.WithCancel(context.Background())
	defer otmena()
	yadro := NovayaSluzhba()
	defer yadro.Zavershit()
	yadro.zhurnalKomand = zhurnalKomand
	// Признак поднимается ПОСЛЕ создания службы, но вычислен до неё: иначе статус
	// противоречил бы брандмауэру, отвечая "режим выключен" запертой машине.
	if zapertaNamerenno {
		yadro.PomnitZapertuyu(true)
	}
	// Расписание подписки. Пять повторов внутри загрузки существуют ровно
	// потому, что служба стартует Automatic, то есть раньше, чем поднимается
	// сеть, и первый заход почти всегда приходится на этот момент. До находки 15
	// комментарий про это был, а самой загрузки не было.
	yadro.ZapustitRaspisanie()
	// Второе решение §9.2: туннель при старте поднимается только по флагу, и
	// поднимает его служба, а не окно: окна при входе может не быть вовсе.
	ubratHvostyPodmeny()
	yadro.pokazatItogObnovleniya()
	if razreshenAvtopodyom(args) {
		go yadro.PodklyuchitPriStarte(ctx)
	}
	go yadro.vestiZhurnal(ctx)
	oshibki := make(chan error, 1)
	go func() { oshibki <- yadro.Obsluzhivat(ctx) }()

	st <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-oshibki:
			// The pipe is the only way anybody talks to us. Losing it and staying
			// alive would mean a service that runs and answers nothing, which
			// looks exactly like a hung machine.
			if err != nil {
				log.Printf("канал упал: %v", err)
			}
			st <- svc.Status{State: svc.StopPending}
			return false, 1
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				st <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				st <- svc.Status{State: svc.StopPending}
				// Zavershit, а не Disconnect: у службы две фоновые горутины,
				// наблюдатель и восстановление, и вторая переживала остановку.
				// Служба «остановлена», а её горутина через отступ поднимает
				// туннель обратно.
				return false, 0
			}
		}
	}
}
