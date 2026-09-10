package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// SetKillSwitch saves a preference. Firewall ownership only exists during a connection.
func (s *Sluzhba) SetKillSwitch(vkl bool) error {
	if !vkl {
		s.mu.Lock()
		changed := s.killSwitch
		s.mu.Unlock()
		if err := s.osvoboditSet(); err != nil {
			return err
		}
		if err := s.zapomnitZashchitu(false); err != nil {
			return err
		}
		// Restore ordinary routing as well as the firewall; the core is not telepathic.
		if changed {
			return s.perepodklyuchit(context.Background())
		}
		return nil
	}
	n, err := s.nabor()
	if err != nil {
		return err
	}
	if n.Pravila.Trafik != nil && estPryamoyTrafik(*n.Pravila.Trafik) {
		return fmt.Errorf("для блокировки сети вне VPN уберите прямые маршруты приложений и сайтов")
	}

	// Критерий «ядро живо» это ПУСТОЙ адрес clash_api, а не состояние. Тот же
	// критерий записан в komandy_serverov.go и profil.go, и разойтись им нельзя:
	// opustit обнуляет порт, а otkaz и ne-neset держатся, пока крутится
	// восстановление, то есть по состоянию человеку отвечали бы про ядро,
	// которого уже нет, и наоборот.
	if adres, _ := s.dostupKKlash(); adres == "" {
		return s.zapomnitZashchitu(true)
	}

	// Порядок шагов и есть решение от 02.09.2026.
	//
	// Режим меняет НЕ ТОЛЬКО брандмауэр: в конфиге ядра отменяется правило
	// ip_is_private, иначе частные сети продолжают идти мимо туннеля. Конфиг
	// ядра на лету не меняется, значит нужен перезапуск.
	//
	// Перезапуск идёт ДО запирания намеренно. Обратный порядок при неудачном
	// перезапуске оставляет машину запертой БЕЗ СЕТИ, и вернуть её можно только
	// аварийным файлом. При этом порядке неудача оставляет машину открытой,
	// то есть худший случай это отсутствие защиты, а не отсутствие связи.
	if err := s.perepodnyatPodRezhim(); err != nil {
		return err
	}

	s.mu.Lock()
	tun := s.tun
	s.mu.Unlock()
	if len(tun.Adresa) == 0 {
		s.zabytRezhim()
		return set.ErrNetTunnelya
	}

	r, err := s.spisokRazreshyonnogo(tun)
	if err != nil {
		s.zabytRezhim()
		return err
	}
	// namerenno = true: команду дал человек. Признак уезжает в файл отката и
	// защищает запертую машину от того, чтобы следующий старт службы её
	// распечатал сам (см. set.SnyatOsirotevshee).
	if err := s.aktivirovatZaslon(r); err != nil {
		s.zabytRezhim()
		// Состояние спрашивается ЗАНОВО: между входом и этой строкой стоит
		// переподъём, и снимок с порога успел устареть.
		s.postavit(s.Status().Sostoyanie, &protokol.Oshibka{
			Kod: kodRezhima(err), Tekst: err.Error()})
		return err
	}
	return s.zapomnitZashchitu(true)
}

func (s *Sluzhba) zapomnitZashchitu(vkl bool) error {
	if err := s.pravitSost(func(f *sostoyanie.SostoyanieFayla) { f.KillSwitch = vkl }); err != nil {
		return err
	}
	s.mu.Lock()
	s.killSwitch = vkl
	s.mu.Unlock()
	return nil
}

func (s *Sluzhba) aktivirovatZaslon(r set.Razreshyonnoe) error {
	s.muZaslon.Lock()
	defer s.muZaslon.Unlock()
	if adres, _ := s.dostupKKlash(); adres == "" {
		return errPodyomOtmenyon
	}
	s.mu.Lock()
	s.zaslonAktiven = true
	s.mu.Unlock()
	return s.vklyuchitVes(r, true)
}

func (s *Sluzhba) osvoboditSet() error {
	s.muZaslon.Lock()
	defer s.muZaslon.Unlock()
	s.mu.Lock()
	aktiven := s.zaslonAktiven
	s.mu.Unlock()
	if !aktiven {
		return nil
	}
	if err := s.vyklyuchitVes(); err != nil {
		s.postavit(s.Status().Sostoyanie, &protokol.Oshibka{Kod: kodRezhima(err), Tekst: "Не удалось восстановить сеть: " + err.Error()})
		return err
	}
	s.mu.Lock()
	s.zaslonAktiven = false
	s.mu.Unlock()
	return nil
}

// perepodnyatPodRezhim поднимает туннель заново с конфигом ядра под режим.
//
// Признак поднимается ДО перезапуска: его читает sobratTun, и только через него
// в конфиг попадает отмена ip_is_private. При любой неудаче признак снимается
// обратно, иначе служба считала бы режим включённым, не заперев машину.
func (s *Sluzhba) perepodnyatPodRezhim() error {
	s.mu.Lock()
	s.killSwitch, s.podRezhim = true, true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.podRezhim = false
		s.mu.Unlock()
	}()

	if err := s.perepodklyuchit(context.Background()); err != nil {
		s.zabytRezhim()
		return fmt.Errorf("туннель не переподнялся под режим, машина осталась открытой: %w", err)
	}
	return nil
}

func (s *Sluzhba) zabytRezhim() {
	s.mu.Lock()
	s.killSwitch = false
	s.mu.Unlock()
}

// spisokRazreshyonnogo собирает шесть пунктов из системы, а не из констант.
//
// Шлюз и резолвер спрашиваются ИСКЛЮЧАЯ туннель: под туннелем нужен физический
// канал, а без исключения система честно ответила бы про сам туннель, потому
// что трафик сейчас идёт именно туда.
func (s *Sluzhba) spisokRazreshyonnogo(tun set.Adapter) (set.Razreshyonnoe, error) {
	kandidaty, err := s.kandidatySIsklyucheniem()
	if err != nil {
		return set.Razreshyonnoe{}, err
	}
	puti, err := s.putiProtsessov()
	if err != nil {
		return set.Razreshyonnoe{}, err
	}

	// Именно IPv4, а не первый попавшийся. Windows вешает на интерфейс
	// link-local fe80:: наравне с 172.19.0.1, порядок выдачи
	// GetAdaptersAddresses не гарантирован, а правило по link-local запрещает
	// весь IPv4 через туннель молча: брандмауэр не ошибается, человек видит
	// машину без сети.
	adres, est := pervyyIPv4(tun.Adresa)
	if !est {
		return set.Razreshyonnoe{}, fmt.Errorf("%w: у адаптера %s нет адреса IPv4",
			set.ErrNetTunnelya, tun.Imya)
	}

	r := set.Razreshyonnoe{
		AdresTun:  adres,
		Kandidaty: kandidaty,
		Protsessy: puti,
	}
	// Шлюз и резолвер это удобство, а не обязательность: без них умрут принтер
	// и локальная сеть, но туннель будет жить. Поэтому их отсутствие не рушит
	// режим, а лишь сужает список.
	if g, err := set.ShlyuzKrome(tun.Indeks); err == nil {
		r.Shlyuz = g
	}
	if d, err := set.LokalnyyResolverKrome(tun.Indeks); err == nil {
		r.Resolver = d
	}
	return r, nil
}

func pervyyIPv4(a []netip.Addr) (netip.Addr, bool) {
	for _, k := range a {
		if k.Is4() {
			return k, true
		}
	}
	return netip.Addr{}, false
}

// PomnitZapertuyu поднимает признак режима, когда машину заперли ДО этого запуска.
//
// Вызывается при старте, если брандмауэр подтвердил блокировку, а файл отката
// говорит, что её ставил человек. Без этого статус противоречил бы брандмауэру:
// интернета нет, а служба отвечает, что режим выключен, и выключить его через
// интерфейс станет нечем.
func (s *Sluzhba) PomnitZapertuyu(vkl bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.killSwitch = vkl
	s.zaslonAktiven = vkl
}

// putiProtsessov это ЕДИНСТВЕННЫЙ источник списка наших исполняемых файлов.
//
// Потребителей два: правило process_path в конфиге туннеля и разрешающие правила
// режима «весь трафик». Раньше каждый собирал список сам, и они разошлись:
// конфиг туннеля не знал про sing-box. Пока ядер было два, дыра открывалась
// ровно на тех транспортах, где наружу ходит именно пропущенный бинарь, и была
// незаметна на остальных. С одним ядром (задача П5) путей два: ядро и служба.
func (s *Sluzhba) putiProtsessov() ([]string, error) {
	sluzhba, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("свой путь неизвестен: %w", err)
	}
	return []string{putYadraTun(), sluzhba}, nil
}

// PeresobratRazresheniya обновляет разрешающие правила под ТЕКУЩИЙ список.
//
// Зовётся после любого изменения списка серверов и подписки. Причина узкая и
// конкретная: разрешающее правило заводится по адресам ВСЕХ серверов сразу, но
// заводится ОДИН РАЗ, в момент включения режима. Сервер, добавленный позже,
// в правила не попадает вовсе, и переключение на него даёт отсутствие сети без
// единой ошибки на экране: брандмауэр молчит, ядро молчит, человек видит
// «подключено» и мёртвый браузер.
//
// При ВЫКЛЮЧЕННОМ режиме не делает ничего. Заводить правила здесь значило бы
// включать человеку запор, которого он не просил.
//
// Обёртка над peresobratRazresheniya для вызывающих, которым «не потребовалось»
// и «удалось» одинаково хороши: подъём туннеля при выключенном режиме это
// именно такой случай.
func (s *Sluzhba) PeresobratRazresheniya() error {
	_, err := s.peresobratRazresheniya()
	return err
}

// peresobratRazresheniya отвечает ДВУМЯ значениями: были ли правила
// пересобраны и как это прошло.
//
// Пересборка, которой не было, обязана отличаться от удавшейся. Прежде обе
// возвращали nil, то есть УСПЕХ, и вызывающий не мог узнать, что список ушёл на
// диск, а брандмауэр остался под прежние адреса.
//
// Критерий «ядро живо» тот же, что у заслона набора: ПУСТОЙ адрес clash_api, а
// не состояние. Два разных критерия в одном пути записи означали бы, что
// заслон пропускает правку, а пересборка её не догоняет, и наоборот.
func (s *Sluzhba) peresobratRazresheniya() (bool, error) {
	s.mu.Lock()
	vkl, tun, aktiven := s.killSwitch, s.tun, s.zaslonAktiven
	s.mu.Unlock()
	if !vkl {
		// Режим выключен: пересобирать нечего и не для чего. Честное
		// «не потребовалось», а не успех.
		return false, nil
	}
	adres, _ := s.dostupKKlash()
	if adres == "" {
		if !aktiven {
			return false, nil
		}
		// Ядра нет, а машина ЗАПЕРТА: режим переживает падение туннеля, ради
		// этого его и включают. Набор правил целиком тут не собрать, он
		// привязан к адресу TUN-адаптера, которого без туннеля не бывает.
		//
		// Прежде здесь стояло молчаливое «не потребовалось», и удалённый сервер
		// оставался разрешённым наружу до следующего подъёма (ПРОВАЛ 3.7
		// живого прогона 03.09.2026). Список серверов от туннеля не зависит,
		// и переписать ОДНО правило можно всегда.
		return s.suzitKandidatov()
	}
	if len(tun.Adresa) == 0 {
		// Режим включён, ядро живо, а адаптера с адресом нет. Правило режима
		// привязывается к адресу туннеля, значит пересобрать нечем, и молчать
		// об этом нельзя: правила остались под прежний список.
		return false, fmt.Errorf("%w: правила не пересобраны, у адаптера туннеля нет адреса",
			set.ErrNetTunnelya)
	}
	r, err := s.spisokRazreshyonnogo(tun)
	if err != nil {
		return false, err
	}
	// Порядок: сначала ЗАВЕСТИ новые правила, потом снять лишние. Обратный
	// порядок означает окно, в котором машина либо заперта насмерть, либо
	// распечатана, и длится оно ровно столько, сколько работает netsh.
	//
	// namerenno = true: режим уже включён человеком, и пересборка его не
	// переучреждает. Передать false значило бы разрешить следующему старту
	// службы распечатать запертую машину.
	if err := s.aktivirovatZaslon(r); err != nil {
		return false, err
	}
	return true, nil
}

// suzitKandidatov переписывает ОДНО правило, список адресов серверов, при
// мёртвом ядре.
//
// Отказ сбора адресов сюда доезжает как отказ пересборки, а не как тишина:
// молчащий резолвер при запертой машине означает, что правило осталось под
// прежний список, и человеку это надо сказать.
//
// Пустой набор (удалён ПОСЛЕДНИЙ сервер) это тоже отказ, а не повод снять
// правило. Снятое правило отрезало бы заодно адрес подписки, который входит в
// тот же список, и человек остался бы с запертой машиной, без серверов и без
// способа их получить. Живой путь на пустом наборе отвечает ровно так же
// (spisokRazreshyonnogo отдаёт тот же отказ), и разойтись им нельзя: один и тот
// же поступок человека давал бы разный итог в зависимости от того, поднят ли
// туннель.
func (s *Sluzhba) suzitKandidatov() (bool, error) {
	kandidaty, err := s.kandidatySIsklyucheniem()
	if err != nil {
		return false, err
	}
	if err := s.suzitServery(kandidaty); err != nil {
		return false, err
	}
	return true, nil
}

// kodRezhima выбирает код отказа режима «весь трафик» по ПРИЧИНЕ, а не по
// месту, где отказ случился.
//
// Прежде и диспетчер, и оба состояния ставили firewall-failed на любой отказ.
// Плата видна на двух причинах сразу: молчащий резолвер и выключенный
// брандмауэр оба выглядели как «не отработал netsh», то есть человек шёл
// чинить исправное. Экраны для обеих причин были написаны и не показывались
// ни разу.
//
// Одна функция на все три точки намеренно: три копии разошлись бы молча, и
// одна из точек перестала бы называть причину.
func kodRezhima(err error) string {
	switch {
	case errors.Is(err, ErrRezolverMolchit):
		return protokol.KodDnsResolveFailed
	case errors.Is(err, set.ErrBrandmauerVyklyuchen):
		return protokol.KodFirewallDisabled
	default:
		return protokol.KodFirewallFailed
	}
}
