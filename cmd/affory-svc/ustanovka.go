package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

const imyaSluzhby = "AfforySvc"

const argumentUstanovki = "install-idle"

func razreshenAvtopodyom(args []string) bool {
	for _, arg := range args {
		if arg == argumentUstanovki {
			return false
		}
	}
	return true
}

func podgotovitUstanovku() error {
	if err := ostanovitSluzhbu(); err != nil {
		return err
	}
	return errors.Join(set.VyklyuchitVesTrafik(), set.VernutIPv6())
}

// How long we wait for the SCM to actually do what it was asked. Stopping and
// deleting are both asynchronous: the call returns, the service keeps living,
// and the next command fails for reasons that look like our bug.
const zhdatSCM = 30 * time.Second

var errNetSluzhby = errors.New("служба не установлена")

// Швы: снятие спрашивает брандмауэр и правит его, а тест обязан гоняться без
// прав администратора и без живого netsh.
var (
	zapertaLiMashina = set.VesTrafikVklyuchyon
	raspechatatVes   = set.VyklyuchitVesTrafik
)

// raspechatatPeredSnyatiem возвращает машину в сеть ДО удаления службы.
//
// Иначе снятие программы при включённом режиме оставляет политику Block, наши
// разрешающие правила на процессы, которых больше нет, и человека без единой
// команды, которой это чинить: CLI уходит вместе со службой.
//
// Отказ здесь ОСТАНАВЛИВАЕТ снятие. Это выглядит грубо, но восстановимо:
// программа ещё на месте, попробовать можно снова. Обратный выбор невосстановим.
func raspechatatPeredSnyatiem() error {
	zaperta, err := zapertaLiMashina()
	if err != nil {
		return fmt.Errorf("не удалось выяснить, заперта ли машина: %w", err)
	}
	if !zaperta {
		// Человек мог поставить себе политику Block задолго до нас. Снятие
		// нашей программы не повод её распечатывать.
		return nil
	}
	if err := raspechatatVes(); err != nil {
		return fmt.Errorf("машина заперта режимом, и снять его не вышло: %w."+
			" Служба НЕ удалена: без неё чинить будет нечем", err)
	}
	return nil
}

// vosstanovlenieSluzhby описывает, что SCM делает со ВНЕЗАПНО умершей службой.
//
// До 05.09.2026 восстановления не было вовсе, с обоснованием «свой сторож сам
// перезапускает». Сторож (yadra.Storozhit) живёт ВНУТРИ службы и поднимает
// ЯДРО; убитую службу он не воскрешает, потому что умирает вместе с ней. Цена
// замерена в госте: после «Снять задачу» при включённом режиме «весь трафик»
// машина осталась запертой и без сети на +2, +15, +45 и +105 секундах, и дальше
// тоже, потому что возвращать службу было НЕКОМУ.
//
// Второй страх того же обоснования (AmneziaVPN, приходящая с того света после
// каждой остановки) проверен и не сбылся: SCM отличает внезапную смерть от
// штатной. Замер с контролем, тот же гость: после TerminateProcess служба
// вернулась сама, после `sc stop` осталась лежать и через 25 секунд. Удаление и
// обновление ходят через Control(svc.Stop), то есть их это не касается.
// Флаг SetRecoveryActionsOnNonCrashFailures намеренно НЕ трогаем: с ним
// восстановление ловило бы и штатный выход с ненулевым кодом, то есть ровно
// то воскрешение, которого боялись.
func vosstanovlenieSluzhby() ([]mgr.RecoveryAction, uint32) {
	// Отступ растёт: служба, падающая прямо на старте, не должна крутить машину
	// в цикле перезапусков. Первая задержка короткая, потому что человек в этот
	// момент сидит без интернета.
	return []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 600
}

// Install is deliberately boring: remove whatever is there, create, configure
// recovery, start, wait.
func ustanovit(putBinarya string) error {
	if err := sostoyanie.ZavestiKatalogDannyh(); err != nil {
		return err
	}
	// Аварийный лист кладётся рядом с программой. Прежде его клал только скрипт
	// стенда, то есть на реальной машине его не было ровно тогда, когда он
	// нужен: интернета нет, интерфейса нет, читать нечего.
	//
	// Отказ здесь установку НЕ рушит: программа без листа работает, а вот
	// молчать об этом нельзя.
	if err := polozhitAvariynyy(filepath.Dir(putBinarya)); err != nil {
		log.Printf("аварийный лист не положен рядом с программой: %v", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("нет доступа к диспетчеру служб: %w", err)
	}
	defer m.Disconnect()

	// Reinstall over an existing service, not "already installed, go away". The
	// refusal version turns every update into a manual two-step where the human
	// has to know the uninstall command exists.
	// raspechatat=false: это ОБНОВЛЕНИЕ, а не снятие. Снять здесь режим значило
	// бы распечатать машину человеку, который просил её запереть и всего лишь
	// поставил новую версию.
	if err := snyatCherez(m, false); err != nil && !errors.Is(err, errNetSluzhby) {
		return err
	}
	if err := errors.Join(set.VyklyuchitVesTrafik(), set.VernutIPv6()); err != nil {
		return fmt.Errorf("сеть после прежней установки не восстановлена: %w", err)
	}

	s, err := m.CreateService(imyaSluzhby, putBinarya, mgr.Config{
		DisplayName: "Affory",
		Description: "Туннель Affory",
		StartType:   mgr.StartAutomatic,
		// LocalSystem and nothing else: machine DPAPI requires it, and so does
		// writing into Program Files during an update.
		ServiceStartName: "LocalSystem",
	})
	if err != nil {
		return fmt.Errorf("не удалось создать службу: %w", err)
	}
	defer s.Close()

	// Восстановление ставится СРАЗУ после создания и до старта: между этими
	// строками служба ещё не могла умереть, а после старта могла бы.
	//
	// Отказ здесь РУШИТ установку, в отличие от автозапуска ниже. Без
	// восстановления режим «весь трафик» перестаёт быть обратимым: убитая служба
	// не возвращается, машина остаётся запертой, и починить это изнутри нечем.
	// Молча поставить программу без этого свойства значит пообещать защиту,
	// которая один раз запрёт человека насмерть.
	deystviya, sbros := vosstanovlenieSluzhby()
	if err := s.SetRecoveryActions(deystviya, sbros); err != nil {
		return fmt.Errorf("восстановление службы не настроено, а без него запертая машина"+
			" не возвращается в сеть: %w", err)
	}

	// Between create and start, and not anywhere else: the files are in place by
	// now, and the service has not had a chance to touch them yet. Redaction 2
	// promised this in prose and never wrote the call, which is how a security
	// mechanism becomes a paragraph.
	if err := hranenie.SnyatOtpechatki(sostoyanie.KatalogProgrammy()); err != nil {
		return fmt.Errorf("отпечатки не сняты: %w", err)
	}

	// §9.2 default: the interface starts at logon. Registered here, in the
	// install sequence the spec spells out, and not fatal: a machine whose Run
	// key refuses a write still gets a working tunnel, and the setting stays
	// reachable from the Settings tab.
	if err := vklyuchitAvtozapusk(putInterfeysa()); err != nil {
		log.Printf("автозапуск интерфейса не зарегистрирован: %v", err)
	}

	if err := s.Start(argumentUstanovki); err != nil {
		return fmt.Errorf("служба создана, но не стартовала: %w", err)
	}
	return zhdatSostoyaniya(s, svc.Running)
}

func snyat() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("нет доступа к диспетчеру служб: %w", err)
	}
	defer m.Disconnect()
	return snyatCherez(m, true)
}

func snyatCherez(m *mgr.Mgr, raspechatat bool) error {
	s, err := m.OpenService(imyaSluzhby)
	if err != nil {
		return fmt.Errorf("%w: %v", errNetSluzhby, err)
	}
	defer s.Close()

	st, err := s.Query()
	if err != nil {
		return fmt.Errorf("состояние службы не читается: %w", err)
	}
	if st.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err != nil {
			// Already stopping is not a failure. Refusing to uninstall because
			// somebody else pressed stop a second earlier would be ceremony.
			var errno windows.Errno
			if !errors.As(err, &errno) || errno != windows.ERROR_SERVICE_NOT_ACTIVE {
				return fmt.Errorf("остановка не принята: %w", err)
			}
		}
		// This wait is the whole point of the rewrite. Control() returns at once,
		// and Delete() on a live process only MARKS the service for deletion: it
		// disappears when the last handle closes, which is whenever. The check
		// right after would then flap between "gone" and "still here" depending
		// on how fast the machine is that morning.
		if err := zhdatSostoyaniya(s, svc.Stopped); err != nil {
			return err
		}
	}

	// Порядок: сначала остановить, потом распечатать, потом удалить. Наоборот
	// живая служба переучредила бы режим прямо у нас за спиной, а удалять до
	// распечатывания значит остаться без инструмента на середине дела.
	if raspechatat {
		if err := raspechatatPeredSnyatiem(); err != nil {
			return err
		}
		// Real uninstall, not an update: the interface must not come back at
		// next logon pointing at a binary that is about to be deleted.
		if err := vyklyuchitAvtozapusk(); err != nil {
			log.Printf("автозапуск интерфейса не снят: %v", err)
		}
	}
	return s.Delete()
}

func zhdatSostoyaniya(s *mgr.Service, hotim svc.State) error {
	do := time.Now().Add(zhdatSCM)
	for time.Now().Before(do) {
		st, err := s.Query()
		if err != nil {
			return fmt.Errorf("состояние службы не читается: %w", err)
		}
		if st.State == hotim {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("служба не пришла в состояние %d за %s", hotim, zhdatSCM)
}
