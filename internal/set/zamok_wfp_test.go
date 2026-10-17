package set

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/wfp"
)

// zamokVPamyati держит фильтры в памяти. Постоянный запрет живого BFE на
// машине разработки пережил бы и тест, и перезагрузку, поэтому TestMain
// ставит этот шов на весь прогон.
type zamokVPamyati struct {
	filtry map[windows.GUID]wfp.Filtr
	// otkaz ломает постановку и замену, otkazSnyatiya снятие. Отказ, как и у
	// настоящей транзакции, не меняет ничего.
	otkaz         error
	otkazSnyatiya error
	vyzovy        []string
	// priPostanovke видит мир в момент постановки, до её исхода.
	priPostanovke func()
}

func (z *zamokVPamyati) Postavit(nabor []wfp.Filtr) error {
	z.vyzovy = append(z.vyzovy, "postavit")
	if z.priPostanovke != nil {
		z.priPostanovke()
	}
	if z.otkaz != nil {
		return z.otkaz
	}
	z.filtry = map[windows.GUID]wfp.Filtr{}
	for _, f := range nabor {
		z.filtry[f.Klyuch] = f
	}
	return nil
}

func (z *zamokVPamyati) Zamenit(klyuchi []windows.GUID, nabor []wfp.Filtr) (bool, error) {
	z.vyzovy = append(z.vyzovy, "zamenit")
	if z.otkaz != nil {
		return false, z.otkaz
	}
	if !z.stoit() {
		return false, nil
	}
	for _, k := range klyuchi {
		delete(z.filtry, k)
	}
	for _, f := range nabor {
		z.filtry[f.Klyuch] = f
	}
	return true, nil
}

func (z *zamokVPamyati) Snyat() (bool, error) {
	z.vyzovy = append(z.vyzovy, "snyat")
	if z.otkazSnyatiya != nil {
		return false, z.otkazSnyatiya
	}
	bylo := len(z.filtry) > 0
	z.filtry = nil
	return bylo, nil
}

func (z *zamokVPamyati) Stoit() (bool, error) { return z.stoit(), nil }

func (z *zamokVPamyati) stoit() bool {
	return zapretSredi(slices.Collect(func(yield func(windows.GUID) bool) {
		for k := range z.filtry {
			if !yield(k) {
				return
			}
		}
	}))
}

func (z *zamokVPamyati) vid(vid vidFiltra, v6 bool) (wfp.Filtr, bool) {
	f, est := z.filtry[klyuchFiltra(vid, v6)]
	return f, est
}

func podstavitZamok(t *testing.T) *zamokVPamyati {
	t.Helper()
	z := &zamokVPamyati{}
	prezhniy := zamok
	zamok = z
	t.Cleanup(func() { zamok = prezhniy })
	return z
}

// programmy кладёт настоящие файлы: WFP опознаёт программу по файлу, и путь
// без файла в запрет не попадает.
func programmy(t *testing.T, imena ...string) []string {
	t.Helper()
	kat := t.TempDir()
	var puti []string
	for _, imya := range imena {
		p := filepath.Join(kat, imya)
		if err := os.WriteFile(p, []byte("MZ"), 0o600); err != nil {
			t.Fatal(err)
		}
		puti = append(puti, p)
	}
	return puti
}

func naborPoVidam(t *testing.T, r Razreshyonnoe) map[windows.GUID]wfp.Filtr {
	t.Helper()
	nabor, err := naborFiltrov(r)
	if err != nil {
		t.Fatal(err)
	}
	itog := map[windows.GUID]wfp.Filtr{}
	for _, f := range nabor {
		if _, povtor := itog[f.Klyuch]; povtor {
			t.Fatalf("ключ фильтра %s повторяется: замена по ключу снесла бы чужой фильтр", f.Imya)
		}
		itog[f.Klyuch] = f
	}
	return itog
}

func estUslovie(f wfp.Filtr, u wfp.Uslovie) bool { return slices.Contains(f.Usloviya, u) }

func usSet(s string) wfp.Uslovie { return wfp.UdalyonnyySet(netip.MustParsePrefix(s)) }

// Запрет без условий стоит в обоих семействах ниже всех разрешений. Без
// IPv6 весь трафик по нему шёл бы мимо туннеля, а равный вес отдал бы исход
// на волю порядка.
func TestZapretVOboihSemeystvahNizheRazresheniy(t *testing.T) {
	nabor := naborPoVidam(t, obraztsovoeRazreshyonnoe())
	for _, v6 := range []bool{false, true} {
		z, est := nabor[klyuchFiltra(vidZapret, v6)]
		if !est || z.Razreshit || len(z.Usloviya) != 0 || z.V6 != v6 {
			t.Fatalf("запрет v6=%v: %+v", v6, z)
		}
		for _, f := range nabor {
			if f.Razreshit && f.Ves <= z.Ves {
				t.Errorf("разрешение %s не выше запрета: %d против %d", f.Imya, f.Ves, z.Ves)
			}
			if f.Razreshit && len(f.Usloviya) == 0 {
				t.Errorf("разрешение %s без условий пускает всё", f.Imya)
			}
		}
		if p, est := nabor[klyuchFiltra(vidPetlya, v6)]; !est || !estUslovie(p, wfp.Petlya()) {
			t.Errorf("внутри машины не разрешено v6=%v: прокси на 127.0.0.1 умер бы", v6)
		}
	}
}

func TestTunnelRazreshyonPoSvoemuAdresu(t *testing.T) {
	nabor := naborPoVidam(t, obraztsovoeRazreshyonnoe())
	tun, est := nabor[klyuchFiltra(vidTun, false)]
	if !est || !estUslovie(tun, wfp.LokalnyyAdres(netip.MustParseAddr("172.19.0.1"))) || len(tun.Usloviya) != 1 {
		t.Fatalf("туннель: %+v", tun)
	}
	if _, est := nabor[klyuchFiltra(vidTun, true)]; est {
		t.Fatal("у туннеля нет IPv6, а разрешение по нему есть")
	}
}

// С2 аудита 1.6.1. Разрешение серверов без порта пускало любую программу на
// любой порт этих адресов. Без портов разрешения нет вовсе.
func TestServeryTolkoNaIhPortahIObaProtokola(t *testing.T) {
	r := obraztsovoeRazreshyonnoe()
	r.Kandidaty = append(r.Kandidaty, netip.MustParseAddr("2001:db8::7"))
	r.Porty = []string{"443", "20000-30000"}
	nabor := naborPoVidam(t, r)
	v4, est4 := nabor[klyuchFiltra(vidServery, false)]
	v6, est6 := nabor[klyuchFiltra(vidServery, true)]
	if !est4 || !est6 {
		t.Fatalf("разрешений серверов v4=%v v6=%v", est4, est6)
	}
	for _, sl := range []struct {
		f       wfp.Filtr
		adres   string
		chuzhoy string
	}{{v4, "192.0.2.225/32", "2001:db8::7/128"}, {v6, "2001:db8::7/128", "192.0.2.225/32"}} {
		for _, u := range []wfp.Uslovie{usSet(sl.adres), wfp.UdalyonnyePorty(443, 443),
			wfp.UdalyonnyePorty(20000, 30000), wfp.Protokol(protokolTcp), wfp.Protokol(protokolUdp)} {
			if !estUslovie(sl.f, u) {
				t.Errorf("%s: нет условия %+v", sl.f.Imya, u)
			}
		}
		if estUslovie(sl.f, usSet(sl.chuzhoy)) {
			t.Errorf("%s: адрес чужого семейства", sl.f.Imya)
		}
	}

	r.Porty = nil
	nabor = naborPoVidam(t, r)
	for _, v6 := range []bool{false, true} {
		if _, est := nabor[klyuchFiltra(vidServery, v6)]; est {
			t.Fatal("разрешение серверов без портов")
		}
	}
	r.Porty = []string{"443-80"}
	if _, err := naborFiltrov(r); err == nil {
		t.Fatal("перевёрнутый диапазон портов принят")
	}
}

func TestMestnyeNesutShlyuzISetiChuzhihTunneley(t *testing.T) {
	r := obraztsovoeRazreshyonnoe()
	r.Shlyuz = netip.MustParseAddr("100.70.0.1")
	r.ChuzhieSeti = []string{"26.0.0.0/8"}
	nabor := naborPoVidam(t, r)
	v4 := nabor[klyuchFiltra(vidMestnye, false)]
	for _, s := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
		"224.0.0.0/4", "255.255.255.255/32", "100.70.0.1/32", "26.0.0.0/8"} {
		if !estUslovie(v4, usSet(s)) {
			t.Errorf("в местных нет %s", s)
		}
	}
	v6 := nabor[klyuchFiltra(vidMestnye, true)]
	for _, s := range []string{"fe80::/10", "fc00::/7", "ff00::/8"} {
		if !estUslovie(v6, usSet(s)) {
			t.Errorf("в местных IPv6 нет %s", s)
		}
	}
	r.ChuzhieSeti = []string{"не сеть"}
	if _, err := naborFiltrov(r); err == nil {
		t.Fatal("неразборчивая сеть чужого туннеля принята")
	}
}

// Встроенное правило Windows для DHCP запрет WFP бьёт так же, как остальные:
// без своего разрешения через сутки истекает аренда адреса.
func TestDhcpRazreshyon(t *testing.T) {
	nabor := naborPoVidam(t, obraztsovoeRazreshyonnoe())
	v4 := nabor[klyuchFiltra(vidDhcp, false)]
	v6 := nabor[klyuchFiltra(vidDhcp, true)]
	if !estUslovie(v4, wfp.LokalnyyPort(68)) || !estUslovie(v4, wfp.UdalyonnyePorty(67, 67)) || !estUslovie(v4, wfp.Protokol(protokolUdp)) {
		t.Errorf("DHCP: %+v", v4)
	}
	if !estUslovie(v6, wfp.LokalnyyPort(546)) || !estUslovie(v6, wfp.UdalyonnyePorty(547, 547)) {
		t.Errorf("DHCPv6: %+v", v6)
	}
}

// С4 аудита 1.6.1: порт 53 к резолверу только хосту системных служб, и по
// обоим протоколам: резолвер уходит на TCP при усечённом ответе.
func TestDnsTolkoSistemnymSluzhbamIPoOboimProtokolam(t *testing.T) {
	nabor := naborPoVidam(t, obraztsovoeRazreshyonnoe())
	dns, est := nabor[klyuchFiltra(vidDns, false)]
	if !est {
		t.Fatal("разрешения DNS нет")
	}
	svchost, err := putSvchost()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []wfp.Uslovie{wfp.Prilozhenie(svchost), usSet("10.7.0.1/32"), wfp.UdalyonnyePorty(53, 53),
		wfp.Protokol(protokolTcp), wfp.Protokol(protokolUdp)} {
		if !estUslovie(dns, u) {
			t.Errorf("в DNS нет условия %+v", u)
		}
	}
	r := obraztsovoeRazreshyonnoe()
	r.Resolver = netip.Addr{}
	if _, est := naborPoVidam(t, r)[klyuchFiltra(vidDns, false)]; est {
		t.Fatal("разрешение DNS без резолвера")
	}
}

func TestProgrammyRazresheny(t *testing.T) {
	r := obraztsovoeRazreshyonnoe()
	r.Protsessy = programmy(t, "sing-box.exe", "affory-svc.exe")
	nabor := naborPoVidam(t, r)
	for _, v6 := range []bool{false, true} {
		f, est := nabor[klyuchFiltra(vidProtsessy, v6)]
		if !est || len(f.Usloviya) != 2 || !estUslovie(f, wfp.Prilozhenie(r.Protsessy[0])) || !estUslovie(f, wfp.Prilozhenie(r.Protsessy[1])) {
			t.Fatalf("программы v6=%v: %+v", v6, f)
		}
	}
	// Пути без файла WFP не опознаёт, и фильтр с ним не встал бы вовсе.
	r.Protsessy = append(r.Protsessy, filepath.Join(t.TempDir(), "net-takogo.exe"))
	if f := naborPoVidam(t, r)[klyuchFiltra(vidProtsessy, false)]; len(f.Usloviya) != 2 {
		t.Fatalf("путь без файла попал в запрет: %+v", f)
	}
}

func TestServeryVZapreteSoVhoda(t *testing.T) {
	r := obraztsovoeRazreshyonnoe()
	r.Kandidaty = append(r.Kandidaty, netip.MustParseAddr("2001:db8::7"))
	got, err := ServeryVZaprete(r)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"192.0.2.225", "2001:db8::7"}) {
		t.Fatalf("адреса серверов %v", got)
	}
}
