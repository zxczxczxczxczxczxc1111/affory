package wfp

import (
	"net/netip"
	"os"
	"strings"
	"testing"
	"unsafe"
)

// Размеры и смещения из fwpmtypes.h для 64 бит. Съехавшее поле не роняет
// вызов: WFP читает соседние байты и ставит фильтр не на тот адрес.
func TestRaskladkaStrukturKakVZagolovkah(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("раскладка выписана для 64 бит")
	}
	var f fwpmFilter0
	var s fwpmSublayer0
	var p fwpmProvider0
	var se fwpmSession0
	var sh fwpmFilterEnumTemplate0
	var u fwpmFilterCondition0
	for _, k := range []struct {
		imya        string
		est, nuzhno uintptr
	}{
		{"FWP_VALUE0", unsafe.Sizeof(fwpValue0{}), 16},
		{"FWP_BYTE_BLOB", unsafe.Sizeof(fwpByteBlob{}), 16},
		{"FWPM_DISPLAY_DATA0", unsafe.Sizeof(fwpmDisplayData0{}), 16},
		{"FWPM_ACTION0", unsafe.Sizeof(fwpmAction0{}), 20},
		{"FWP_V4_ADDR_AND_MASK", unsafe.Sizeof(fwpV4AddrAndMask{}), 8},
		{"FWP_V6_ADDR_AND_MASK", unsafe.Sizeof(fwpV6AddrAndMask{}), 17},
		{"FWP_RANGE0", unsafe.Sizeof(fwpRange0{}), 32},

		{"FWPM_FILTER_CONDITION0", unsafe.Sizeof(u), 40},
		{"FWPM_FILTER_CONDITION0.matchType", unsafe.Offsetof(u.sravnenie), 16},
		{"FWPM_FILTER_CONDITION0.conditionValue", unsafe.Offsetof(u.znachenie), 24},

		{"FWPM_FILTER0", unsafe.Sizeof(f), 200},
		{"FWPM_FILTER0.displayData", unsafe.Offsetof(f.podpis), 16},
		{"FWPM_FILTER0.flags", unsafe.Offsetof(f.flagi), 32},
		{"FWPM_FILTER0.providerKey", unsafe.Offsetof(f.provayder), 40},
		{"FWPM_FILTER0.providerData", unsafe.Offsetof(f.dannyeProvaydera), 48},
		{"FWPM_FILTER0.layerKey", unsafe.Offsetof(f.sloy), 64},
		{"FWPM_FILTER0.subLayerKey", unsafe.Offsetof(f.podsloy), 80},
		{"FWPM_FILTER0.weight", unsafe.Offsetof(f.ves), 96},
		{"FWPM_FILTER0.numFilterConditions", unsafe.Offsetof(f.chisloUsloviy), 112},
		{"FWPM_FILTER0.filterCondition", unsafe.Offsetof(f.usloviya), 120},
		{"FWPM_FILTER0.action", unsafe.Offsetof(f.deystvie), 128},
		{"FWPM_FILTER0.providerContextKey", unsafe.Offsetof(f.kontekst), 152},
		{"FWPM_FILTER0.reserved", unsafe.Offsetof(f.zarezervirovano), 168},
		{"FWPM_FILTER0.filterId", unsafe.Offsetof(f.id), 176},
		{"FWPM_FILTER0.effectiveWeight", unsafe.Offsetof(f.deystvuyushchiyVes), 184},

		{"FWPM_SUBLAYER0", unsafe.Sizeof(s), 72},
		{"FWPM_SUBLAYER0.providerKey", unsafe.Offsetof(s.provayder), 40},
		{"FWPM_SUBLAYER0.weight", unsafe.Offsetof(s.ves), 64},

		{"FWPM_PROVIDER0", unsafe.Sizeof(p), 64},
		{"FWPM_PROVIDER0.providerData", unsafe.Offsetof(p.dannye), 40},
		{"FWPM_PROVIDER0.serviceName", unsafe.Offsetof(p.imyaSluzhby), 56},

		{"FWPM_SESSION0", unsafe.Sizeof(se), 72},
		{"FWPM_SESSION0.txnWaitTimeoutInMSec", unsafe.Offsetof(se.zhdatTranzakciyu), 36},
		{"FWPM_SESSION0.sid", unsafe.Offsetof(se.sid), 48},
		{"FWPM_SESSION0.kernelMode", unsafe.Offsetof(se.rezhimYadra), 64},

		{"FWPM_FILTER_ENUM_TEMPLATE0", unsafe.Sizeof(sh), 72},
		{"FWPM_FILTER_ENUM_TEMPLATE0.layerKey", unsafe.Offsetof(sh.sloy), 8},
		{"FWPM_FILTER_ENUM_TEMPLATE0.enumType", unsafe.Offsetof(sh.tipPerebora), 24},
		{"FWPM_FILTER_ENUM_TEMPLATE0.providerContextTemplate", unsafe.Offsetof(sh.kontekst), 32},
		{"FWPM_FILTER_ENUM_TEMPLATE0.numFilterConditions", unsafe.Offsetof(sh.chisloUsloviy), 40},
		{"FWPM_FILTER_ENUM_TEMPLATE0.filterCondition", unsafe.Offsetof(sh.usloviya), 48},
		{"FWPM_FILTER_ENUM_TEMPLATE0.actionMask", unsafe.Offsetof(sh.maskaDeystviy), 56},
		{"FWPM_FILTER_ENUM_TEMPLATE0.calloutKey", unsafe.Offsetof(sh.vyzov), 64},
	} {
		if k.est != k.nuzhno {
			t.Errorf("%s: %d, а в заголовке %d", k.imya, k.est, k.nuzhno)
		}
	}
}

// Адрес и маска IPv4 у WFP в порядке байт машины: 192.168.0.0/16 это
// 0xC0A80000 и 0xFFFF0000, а не перевёрнутые байты.
func TestSetIPv4VPoryadkeMashiny(t *testing.T) {
	var d derzhatel
	z, err := d.set(netip.MustParsePrefix("192.168.0.0/16"), false)
	if err != nil {
		t.Fatal(err)
	}
	if z.tip != fwpV4AddrMask {
		t.Fatalf("тип %#x", z.tip)
	}
	m := d.zhivye[len(d.zhivye)-1].(*fwpV4AddrAndMask)
	if m.adres != 0xC0A80000 || m.maska != 0xFFFF0000 {
		t.Errorf("адрес %#x маска %#x", m.adres, m.maska)
	}
	for bity, nuzhno := range map[int]uint32{0: 0, 1: 0x80000000, 8: 0xFF000000, 32: 0xFFFFFFFF} {
		if got := maskaV4(bity); got != nuzhno {
			t.Errorf("маска /%d: %#x", bity, got)
		}
	}
}

func TestSetIPv6SPrefiksom(t *testing.T) {
	var d derzhatel
	z, err := d.set(netip.MustParsePrefix("fe80::/10"), true)
	if err != nil {
		t.Fatal(err)
	}
	m := d.zhivye[len(d.zhivye)-1].(*fwpV6AddrAndMask)
	if z.tip != fwpV6AddrMask || m.prefiks != 10 || m.adres[0] != 0xfe || m.adres[1] != 0x80 {
		t.Errorf("тип %#x, %x/%d", z.tip, m.adres, m.prefiks)
	}
}

// Адрес не того семейства WFP принял бы молча, и фильтр не сработал бы
// никогда. Здесь это отказ.
func TestSetNeTogoSemeystvaOtkaz(t *testing.T) {
	var d derzhatel
	if _, err := d.set(netip.MustParsePrefix("10.0.0.0/8"), true); err == nil {
		t.Error("IPv4 в слое IPv6 принят")
	}
	if _, err := d.set(netip.MustParsePrefix("fc00::/7"), false); err == nil {
		t.Error("IPv6 в слое IPv4 принят")
	}
	if _, err := d.set(netip.Prefix{}, false); err == nil {
		t.Error("пустая сеть принята")
	}
}

func TestPortyOdinIDiapazon(t *testing.T) {
	var d derzhatel
	odin, err := d.uslovie(UdalyonnyePorty(443, 443), false)
	if err != nil {
		t.Fatal(err)
	}
	if odin.sravnenie != fwpMatchEqual || odin.znachenie.tip != fwpUint16 || odin.znachenie.znachenie != 443 {
		t.Errorf("один порт: %+v", odin)
	}
	diapazon, err := d.uslovie(UdalyonnyePorty(20000, 30000), false)
	if err != nil {
		t.Fatal(err)
	}
	r := d.zhivye[len(d.zhivye)-1].(*fwpRange0)
	if diapazon.sravnenie != fwpMatchRange || diapazon.znachenie.tip != fwpRangeTip ||
		r.ot.znachenie != 20000 || r.do.znachenie != 30000 || r.ot.tip != fwpUint16 {
		t.Errorf("диапазон: %+v %+v", diapazon, r)
	}
	if _, err := d.uslovie(UdalyonnyePorty(30000, 20000), false); err == nil {
		t.Error("перевёрнутый диапазон принят")
	}
	lok, err := d.uslovie(LokalnyyPort(68), false)
	if err != nil {
		t.Fatal(err)
	}
	if lok.pole != poleLokalnyyPort || lok.znachenie.znachenie != 68 {
		t.Errorf("локальный порт: %+v", lok)
	}
}

func TestPetlyaIProtokol(t *testing.T) {
	var d derzhatel
	p, err := d.uslovie(Petlya(), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.pole != poleFlagi || p.sravnenie != fwpMatchFlagsAllSet || p.znachenie.znachenie != flagPetlya {
		t.Errorf("петля: %+v", p)
	}
	u, err := d.uslovie(Protokol(17), true)
	if err != nil {
		t.Fatal(err)
	}
	if u.pole != poleProtokol || u.znachenie.tip != fwpUint8 || u.znachenie.znachenie != 17 {
		t.Errorf("протокол: %+v", u)
	}
}

// Опознание программы это путь через имя устройства. Вызов только читает, на
// машине ничего не меняет и прав администратора не требует.
func TestOpoznaniePrilozheniyaCherezUstroystvo(t *testing.T) {
	put, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	id, err := idPrilozheniya(put)
	if err != nil {
		t.Fatal(err)
	}
	// UTF-16 с нулём в конце.
	stroka := strings.ToLower(string(utf16VStroku(id)))
	if !strings.HasPrefix(stroka, `\device\`) {
		t.Errorf("опознание %q не через устройство", stroka)
	}
	if _, err := idPrilozheniya(put + ".net-takogo"); err == nil {
		t.Error("опознание несуществующего файла выдано")
	}
}

func utf16VStroku(b []byte) []rune {
	var r []rune
	for i := 0; i+1 < len(b); i += 2 {
		c := rune(b[i]) | rune(b[i+1])<<8
		if c == 0 {
			break
		}
		r = append(r, c)
	}
	return r
}
