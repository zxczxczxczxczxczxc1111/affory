package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Запасной сервер в ручном режиме (жалоба 17.10.2026): выбранный сервер
// молчит, живые рядом есть, а человек сидел без VPN.

// yadroSVyborom это подставное ядро, которое помнит, что ему выбрали: замер
// группы и несущий отвечают по этому выбору, как настоящее ядро.
type yadroSVyborom struct {
	mu         sync.Mutex
	vybor      string
	postavleno []string
}

func (y *yadroSVyborom) postavit(teg string) {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.vybor = teg
	y.postavleno = append(y.postavleno, teg)
}

func (y *yadroSVyborom) tekushchiy() string {
	y.mu.Lock()
	defer y.mu.Unlock()
	return y.vybor
}

func (y *yadroSVyborom) istoriya() []string {
	y.mu.Lock()
	defer y.mu.Unlock()
	return append([]string(nil), y.postavleno...)
}

// konfigSVyboromNl это конфиг с ручным выбором nl: подмена обязана переписать
// default на группу, а возврат вернуть его обратно.
const konfigSVyboromNl = `{"outbounds":[{"tag":"vybor","type":"selector","default":"srv-nl"}]}`

func vyborVKonfige(t *testing.T, put string) string {
	t.Helper()
	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	var k struct {
		Outbounds []struct {
			Tag     string `json:"tag"`
			Default string `json:"default"`
		} `json:"outbounds"`
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		t.Fatal(err)
	}
	for _, o := range k.Outbounds {
		if o.Tag == genkonfig.TegSelector {
			return o.Default
		}
	}
	t.Fatalf("селектора нет в конфиге: %s", telo)
	return ""
}

// ruchnoyNaDvuh это служба в ручном режиме на nl, рядом живой de, и ядро,
// которое отвечает по своему выбору. nlZhiv решает, несёт ли nl.
func ruchnoyNaDvuh(t *testing.T, n Nabor) (*Sluzhba, *yadroSVyborom, *atomic.Bool) {
	t.Helper()
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, n)
	// Служба читает режим из хранилища при запуске, а хранилище подставлено
	// уже после него.
	s.mu.Lock()
	s.rezhim = rezhimNabora(n)
	s.mu.Unlock()
	y := &yadroSVyborom{}
	nlZhiv := &atomic.Bool{}
	nlZhiv.Store(true)
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		y.postavit(teg)
		return nil
	}
	s.nesyot = func(context.Context, string, string, string, string) (string, error) {
		if v := y.tekushchiy(); v != genkonfig.TegAvto {
			return v, nil
		}
		return genkonfig.TegKandidata("de"), nil
	}
	s.zamerit = func(_ context.Context, _, _, teg string) (time.Duration, error) {
		if teg == genkonfig.TegKandidata("nl") {
			if nlZhiv.Load() {
				return time.Millisecond, nil
			}
			return 0, errors.New("исходящий srv-nl не отвечает (код 504)")
		}
		if y.tekushchiy() == genkonfig.TegAvto || nlZhiv.Load() {
			return time.Millisecond, nil
		}
		return 0, errors.New("исходящий vybor не отвечает (код 504)")
	}
	s.period = 10 * time.Millisecond
	s.periodNesushchego = 10 * time.Millisecond
	return s, y, nlZhiv
}

func dvaServeraNl() Nabor {
	return Nabor{Servery: []protokol.Server{serverProby(), vtoroyServer()}, Vybran: "nl"}
}

func TestRuchnoyRezhimPodmenyaetMolchashchiyServerIVozvrashchaetsya(t *testing.T) {
	put := konfigVyboraDlyaTesta(t, konfigSVyboromNl)
	s, y, nlZhiv := ruchnoyNaDvuh(t, dvaServeraNl())
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	nlZhiv.Store(false)

	dozhdatsya(t, "подмена молчащего выбранного сервера", func() bool {
		return s.Status().VremennoVmesto == "Нидерланды"
	})
	st := s.Status()
	if st.Sostoyanie != protokol.SostPodnyat {
		t.Fatalf("состояние %s, а живой сервер рядом есть", st.Sostoyanie)
	}
	if st.NesushchiyId != "de" {
		t.Fatalf("несущий %q, ждали de", st.NesushchiyId)
	}
	// Подмена это не смена режима: человек выбирал ручной и nl.
	if st.RezhimMarshruta != protokol.RezhimRuchnoy || st.VybranId != "nl" {
		t.Fatalf("режим %q, выбран %q: подмена переписала выбор человека", st.RezhimMarshruta, st.VybranId)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	if rezhimNabora(n) != protokol.RezhimRuchnoy || n.Vybran != "nl" {
		t.Fatalf("в наборе режим %q и выбор %q", rezhimNabora(n), n.Vybran)
	}
	// Сторож поднимает упавшее ядро по файлу: там обязана стоять группа.
	if got := vyborVKonfige(t, put); got != genkonfig.TegAvto {
		t.Fatalf("в конфиге выбор %q, перезапуск ядра вернул бы молчащий сервер", got)
	}

	nlZhiv.Store(true)
	dozhdatsya(t, "возврат на ожившый выбранный сервер", func() bool {
		return s.zapasnoyDlya() == ""
	})
	st = s.Status()
	if st.VremennoVmesto != "" || st.NesushchiyId != "nl" {
		t.Fatalf("после возврата временно вместо %q, несущий %q", st.VremennoVmesto, st.NesushchiyId)
	}
	if got := y.tekushchiy(); got != genkonfig.TegKandidata("nl") {
		t.Fatalf("в ядре выбор %q после возврата", got)
	}
	if got := vyborVKonfige(t, put); got != genkonfig.TegKandidata("nl") {
		t.Fatalf("в конфиге выбор %q после возврата", got)
	}
}

func TestVozvratTolkoPosleDvuhUspehovPodryad(t *testing.T) {
	// Сервер, который отвечает через раз, ещё не вернулся: у сотрудника перед
	// полной пропажей пути было ровно так.
	konfigVyboraDlyaTesta(t, konfigProby)
	s := podstavnaya(t, nil)
	var postavleno []string
	s.postavitVybor = func(_ context.Context, _, _, _, teg string) error {
		postavleno = append(postavleno, teg)
		return nil
	}
	otvety := []error{nil, errors.New("504"), nil, nil}
	var zamerov int
	s.zamerit = func(_ context.Context, _, _, teg string) (time.Duration, error) {
		if teg != genkonfig.TegKandidata("nl") {
			return time.Millisecond, nil
		}
		err := otvety[zamerov]
		zamerov++
		return time.Millisecond, err
	}
	s.postavitZapasnoy(serverProby())

	for i := 0; i < 3; i++ {
		s.proveritVozvrat(context.Background(), "127.0.0.1:52715", "sekret", genkonfig.TegSelector)
	}
	if len(postavleno) != 0 || s.zapasnoyDlya() != "nl" {
		t.Fatalf("вернулись после ответов «да, нет, да»: выбор %v", postavleno)
	}
	s.proveritVozvrat(context.Background(), "127.0.0.1:52715", "sekret", genkonfig.TegSelector)
	if s.zapasnoyDlya() != "" {
		t.Fatal("после двух ответов подряд подмена осталась")
	}
	if len(postavleno) != 1 || postavleno[0] != genkonfig.TegKandidata("nl") {
		t.Fatalf("выбор в ядре %v, ждали возврат на srv-nl", postavleno)
	}
}

func TestBezKandidatovVAvtoPodmenyatNekem(t *testing.T) {
	// de убран человеком из авто: автомату его брать нельзя, и туннель
	// рвётся, как и прежде.
	konfigVyboraDlyaTesta(t, konfigSVyboromNl)
	n := dvaServeraNl()
	n.VneAvto = []string{"de"}
	s, y, nlZhiv := ruchnoyNaDvuh(t, n)
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	nlZhiv.Store(false)
	dozhdatsya(t, "разрыв без подмены", func() bool {
		return s.Status().Sostoyanie != protokol.SostPodnyat
	})
	for _, teg := range y.istoriya() {
		if teg == genkonfig.TegAvto {
			t.Fatal("подменили сервером, который человек убрал из авто")
		}
	}
}

func TestPodmenaNeNesyotVyborVozvrashchaetsya(t *testing.T) {
	// Группа авто тоже молчит: выбор возвращается на выбранный, и туннель
	// рвётся. Ответить «понёс» на непроверенное значило бы оставить человека
	// на мёртвом туннеле.
	konfigVyboraDlyaTesta(t, konfigSVyboromNl)
	s, y, nlZhiv := ruchnoyNaDvuh(t, dvaServeraNl())
	var vsyoMolchit atomic.Bool
	staryy := s.zamerit
	s.zamerit = func(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
		if vsyoMolchit.Load() {
			return 0, errors.New("исходящий не отвечает (код 504)")
		}
		return staryy(ctx, adres, sekret, teg)
	}
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("подъём не прошёл: %v", err)
	}
	nlZhiv.Store(false)
	vsyoMolchit.Store(true)
	dozhdatsya(t, "разрыв после бесполезной подмены", func() bool {
		return s.Status().Sostoyanie != protokol.SostPodnyat
	})
	ist := y.istoriya()
	if len(ist) < 3 || ist[1] != genkonfig.TegAvto || ist[2] != genkonfig.TegKandidata("nl") {
		t.Fatalf("выбор в ядре %v, ждали подъём на nl, подмену и возврат на nl", ist)
	}
	if s.zapasnoyDlya() != "" {
		t.Fatal("неудачная подмена осталась числиться")
	}
}

func TestVosstanovleniePrRuchnomRezhimeProbuetGruppu(t *testing.T) {
	// Путь восстановления: туннеля нет, выбранный не ответил на проверку до
	// TUN. Прежде здесь стоял отказ «ни один сервер не отвечает», хотя
	// спрошен был один.
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, dvaServeraNl())
	var rezhimy []protokol.Rezhim
	s.proveritServer = func(context.Context) error {
		n, err := s.nabor()
		if err != nil {
			return err
		}
		rezhimy = append(rezhimy, s.rezhimPodyoma(n))
		if len(rezhimy) == 1 {
			return errors.New("исходящий srv-nl не отвечает")
		}
		return nil
	}
	s.mu.Lock()
	moyo := s.pokolenieP
	s.mu.Unlock()
	if err := s.proveritServerDoTun(context.Background(), true, moyo); err != nil {
		t.Fatalf("подъём через группу отказан: %v", err)
	}
	if len(rezhimy) != 2 || rezhimy[0] != protokol.RezhimRuchnoy || rezhimy[1] != protokol.RezhimAvto {
		t.Fatalf("проверки шли в режимах %v, ждали ручной и авто", rezhimy)
	}
	if s.zapasnoyDlya() != "nl" {
		t.Fatalf("подмена числится за %q", s.zapasnoyDlya())
	}
}

func TestVosstanovlenieGruppaTozheMolchit(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, dvaServeraNl())
	var proverok int
	s.proveritServer = func(context.Context) error {
		proverok++
		return errors.New("исходящий не отвечает")
	}
	s.mu.Lock()
	moyo := s.pokolenieP
	s.mu.Unlock()
	err := s.proveritServerDoTun(context.Background(), true, moyo)
	if err == nil || errors.Is(err, errPodyomOtmenyon) {
		t.Fatalf("ответ %v, ждали отказ", err)
	}
	if proverok != 2 {
		t.Fatalf("проверок %d, ждали две: выбранный и группа", proverok)
	}
	// Теперь «ни один сервер» правда: спрошены все.
	if o := s.Status().Oshib; o == nil || o.Kod != protokol.KodAllServersDown {
		t.Fatalf("ошибка %v, ждали all-servers-down", o)
	}
	if s.zapasnoyDlya() != "" {
		t.Fatal("неудачная подмена осталась числиться")
	}
}

func TestVosstanovlenieOdnogoServeraSprashivaetOdinRaz(t *testing.T) {
	s := podstavnaya(t, nil)
	hranilishcheProby(t, s, Nabor{Servery: []protokol.Server{serverProby()}, Vybran: "nl"})
	var proverok int
	s.proveritServer = func(context.Context) error {
		proverok++
		return errors.New("исходящий не отвечает")
	}
	s.mu.Lock()
	moyo := s.pokolenieP
	s.mu.Unlock()
	if err := s.proveritServerDoTun(context.Background(), true, moyo); err == nil {
		t.Fatal("молчащий единственный сервер пропущен")
	}
	if proverok != 1 {
		t.Fatalf("проверок %d, а подменять некем", proverok)
	}
}

func TestKonfigPodyomaIdyotPoPodmene(t *testing.T) {
	// Подъём восстановления собирает конфиг с группой в default, иначе ядро
	// поднялось бы на молчащем сервере, ради которого подмена и заведена.
	s := podstavnayaSPolnymNaborom(t)
	defoltSelektora := func() string {
		t.Helper()
		telo, _, _, err := s.sobratTun(nil, false)
		if err != nil {
			t.Fatalf("конфиг не собрался: %v", err)
		}
		var k struct {
			Outbounds []struct {
				Tag     string `json:"tag"`
				Default string `json:"default"`
			} `json:"outbounds"`
		}
		if err := json.Unmarshal(telo, &k); err != nil {
			t.Fatal(err)
		}
		for _, o := range k.Outbounds {
			if o.Tag == genkonfig.TegSelector {
				return o.Default
			}
		}
		t.Fatal("селектора в конфиге нет")
		return ""
	}
	if got := defoltSelektora(); got != genkonfig.TegKandidata("nl") {
		t.Fatalf("без подмены default %q", got)
	}
	n, err := s.nabor()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := n.VybrannyyServer()
	if err != nil {
		t.Fatal(err)
	}
	s.postavitZapasnoy(srv)
	if got := defoltSelektora(); got != genkonfig.TegAvto {
		t.Fatalf("при подмене default %q", got)
	}
	// Человек выбрал другой сервер: подмена про прежний ему не указ.
	if err := s.pravitNabor(func(n *Nabor) error { n.Vybran = "de"; return nil }); err != nil {
		t.Fatal(err)
	}
	if got := defoltSelektora(); got != genkonfig.TegKandidata("de") {
		t.Fatalf("после выбора de default %q", got)
	}
}

func TestPodmenuSnimaetVyborCheloveka(t *testing.T) {
	t.Run("отключение", func(t *testing.T) {
		konfigVyboraDlyaTesta(t, konfigProby)
		s := sZhivymTunnelem(t)
		s.postavitZapasnoy(serverProby())
		s.Otklyuchit()
		if s.zapasnoyDlya() != "" {
			t.Fatal("подмена пережила отключение человеком")
		}
	})
	t.Run("выбор сервера", func(t *testing.T) {
		konfigVyboraDlyaTesta(t, konfigProby)
		s := sZhivymTunnelem(t)
		s.postavitZapasnoy(serverProby())
		if o := vypolnit(t, s, "setServer", map[string]string{"id": "de"}); o.Oshib != nil {
			t.Fatalf("переключение не прошло: %s", o.Oshib.Kod)
		}
		if s.zapasnoyDlya() != "" {
			t.Fatal("подмена пережила выбор человека")
		}
	})
	t.Run("смена режима", func(t *testing.T) {
		konfigVyboraDlyaTesta(t, konfigProby)
		s := sZhivymTunnelem(t)
		s.postavitZapasnoy(serverProby())
		if o := vypolnit(t, s, "setRouteMode", map[string]string{"rezhim": "avto"}); o.Oshib != nil {
			t.Fatalf("смена режима не прошла: %s", o.Oshib.Kod)
		}
		if s.zapasnoyDlya() != "" {
			t.Fatal("подмена пережила смену режима")
		}
	})
}

func TestVremennoVmestoTolkoKogdaNesyotDrugoy(t *testing.T) {
	s := podstavnaya(t, nil)
	s.postavitZapasnoy(serverProby())
	sluchai := []struct {
		sost       protokol.Sostoyanie
		nesushchiy string
		zhdali     string
	}{
		{protokol.SostPodnyat, "de", "Нидерланды"},
		// Группа сама выбрала nl: подмены на деле нет, и говорить о ней незачем.
		{protokol.SostPodnyat, "nl", ""},
		// Без туннеля «VPN идёт через» неправда.
		{protokol.SostVosstanavl, "de", ""},
		{protokol.SostVyklyuchen, "de", ""},
	}
	for _, sl := range sluchai {
		s.mu.Lock()
		s.sost, s.nesushchiyId = sl.sost, sl.nesushchiy
		s.mu.Unlock()
		if got := s.Status().VremennoVmesto; got != sl.zhdali {
			t.Errorf("%s, несёт %s: временно вместо %q, ждали %q", sl.sost, sl.nesushchiy, got, sl.zhdali)
		}
	}
}
