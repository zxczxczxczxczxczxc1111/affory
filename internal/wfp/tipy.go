// Пакет wfp это тонкие привязки к Windows Filtering Platform: ровно то, что
// нужно режиму «весь трафик», и ничего сверх.
//
// Раскладка структур повторяет fwpmtypes.h и сверена с wireguard-windows
// (лицензия MIT), где смещения выписаны константами. Тест размеров и смещений
// держит её без прав администратора: съехавшее поле здесь не роняет вызов, а
// молча ставит фильтр не на тот адрес.
package wfp

import "golang.org/x/sys/windows"

// FWP_VALUE0. Значение лежит в объединении: число прямо в нём, всё длиннее
// указателем.
type fwpValue0 struct {
	tip       uint32
	znachenie uintptr
}

// FWP_BYTE_BLOB.
type fwpByteBlob struct {
	razmer uint32
	dannye *byte
}

// FWPM_DISPLAY_DATA0.
type fwpmDisplayData0 struct {
	imya     *uint16
	opisanie *uint16
}

// FWPM_FILTER_CONDITION0.
type fwpmFilterCondition0 struct {
	pole      windows.GUID
	sravnenie uint32
	znachenie fwpValue0
}

// FWPM_ACTION0. Второе поле это filterType или calloutKey, нам не нужно ни то,
// ни другое.
type fwpmAction0 struct {
	tip   uint32
	vyzov windows.GUID
}

// FWPM_FILTER0.
type fwpmFilter0 struct {
	klyuch           windows.GUID
	podpis           fwpmDisplayData0
	flagi            uint32
	provayder        *windows.GUID
	dannyeProvaydera fwpByteBlob
	sloy             windows.GUID
	podsloy          windows.GUID
	ves              fwpValue0
	chisloUsloviy    uint32
	usloviya         *fwpmFilterCondition0
	deystvie         fwpmAction0
	// Выравнивание: у Си объединение за действием начинается с 8 байт.
	_ [4]byte
	// providerContextKey или rawContext, объединение.
	kontekst           windows.GUID
	zarezervirovano    *windows.GUID
	id                 uint64
	deystvuyushchiyVes fwpValue0
}

// FWPM_SUBLAYER0.
type fwpmSublayer0 struct {
	klyuch           windows.GUID
	podpis           fwpmDisplayData0
	flagi            uint32
	provayder        *windows.GUID
	dannyeProvaydera fwpByteBlob
	ves              uint16
}

// FWPM_PROVIDER0.
type fwpmProvider0 struct {
	klyuch      windows.GUID
	podpis      fwpmDisplayData0
	flagi       uint32
	dannye      fwpByteBlob
	imyaSluzhby *uint16
}

// FWPM_SESSION0.
type fwpmSession0 struct {
	klyuch           windows.GUID
	podpis           fwpmDisplayData0
	flagi            uint32
	zhdatTranzakciyu uint32
	idProtsessa      uint32
	sid              *windows.SID
	polzovatel       *uint16
	rezhimYadra      int32
}

// FWPM_FILTER_ENUM_TEMPLATE0.
type fwpmFilterEnumTemplate0 struct {
	provayder     *windows.GUID
	sloy          windows.GUID
	tipPerebora   uint32
	flagi         uint32
	kontekst      uintptr
	chisloUsloviy uint32
	usloviya      *fwpmFilterCondition0
	maskaDeystviy uint32
	vyzov         *windows.GUID
}

// FWP_V4_ADDR_AND_MASK. Адрес и маска в порядке байт машины, а не сети.
type fwpV4AddrAndMask struct {
	adres uint32
	maska uint32
}

// FWP_V6_ADDR_AND_MASK.
type fwpV6AddrAndMask struct {
	adres   [16]byte
	prefiks uint8
}

// FWP_RANGE0.
type fwpRange0 struct {
	ot fwpValue0
	do fwpValue0
}

// FWP_DATA_TYPE.
const (
	fwpUint8       = 1
	fwpUint16      = 2
	fwpUint32      = 3
	fwpByteBlobTip = 12
	fwpV4AddrMask  = 0x100
	fwpV6AddrMask  = 0x101
	fwpRangeTip    = 0x102
)

// FWP_MATCH_TYPE.
const (
	fwpMatchEqual       = 0
	fwpMatchRange       = 5
	fwpMatchFlagsAllSet = 6
)

// FWP_ACTION_TYPE: терминальные запрет и разрешение.
const (
	fwpActionBlock  = 0x1001
	fwpActionPermit = 0x1002
)

const (
	// FWPM_PROVIDER_FLAG_PERSISTENT, FWPM_SUBLAYER_FLAG_PERSISTENT и
	// FWPM_FILTER_FLAG_PERSISTENT равны одному числу.
	flagPostoyannyy = 0x1

	// FWP_CONDITION_FLAG_IS_LOOPBACK.
	flagPetlya = 0x1

	// FWP_FILTER_ENUM_OVERLAPPING: без условий в шаблоне отдаёт все фильтры
	// слоя.
	fwpFilterEnumOverlapping = 1

	// RPC_C_AUTHN_DEFAULT.
	rpcCAuthnDefault = 0xFFFFFFFF
)

// Коды отказов, которые здесь различаются по смыслу.
const (
	fwpEFilterNotFound   = 0x80320003
	fwpEProviderNotFound = 0x80320005
	fwpESublayerNotFound = 0x80320007
	fwpEAlreadyExists    = 0x80320009

	// Служба BFE остановлена или ещё не поднялась.
	eptSNotRegistered     = 1753
	rpcSServerUnavailable = 1722
)

// Слои: исходящее соединение и первая отправка UDP, отдельно для IPv4 и IPv6.
var (
	sloyConnectV4 = windows.GUID{Data1: 0xc38d57d1, Data2: 0x05a7, Data3: 0x4c33,
		Data4: [8]byte{0x90, 0x4f, 0x7f, 0xbc, 0xee, 0xe6, 0x0e, 0x82}}
	sloyConnectV6 = windows.GUID{Data1: 0x4a72393b, Data2: 0x319f, Data3: 0x44bc,
		Data4: [8]byte{0x84, 0xc3, 0xba, 0x54, 0xdc, 0xb3, 0xb6, 0xb4}}
)

// Поля условий.
var (
	poleUdalyonnyyAdres = windows.GUID{Data1: 0xb235ae9a, Data2: 0x1d64, Data3: 0x49b8,
		Data4: [8]byte{0xa4, 0x4c, 0x5f, 0xf3, 0xd9, 0x09, 0x50, 0x45}}
	poleLokalnyyAdres = windows.GUID{Data1: 0xd9ee00de, Data2: 0xc1ef, Data3: 0x4617,
		Data4: [8]byte{0xbf, 0xe3, 0xff, 0xd8, 0xf5, 0xa0, 0x89, 0x57}}
	poleUdalyonnyyPort = windows.GUID{Data1: 0xc35a604d, Data2: 0xd22b, Data3: 0x4e1a,
		Data4: [8]byte{0x91, 0xb4, 0x68, 0xf6, 0x74, 0xee, 0x67, 0x4b}}
	poleLokalnyyPort = windows.GUID{Data1: 0x0c1ba1af, Data2: 0x5765, Data3: 0x453f,
		Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b}}
	poleProtokol = windows.GUID{Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a,
		Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7}}
	polePrilozhenie = windows.GUID{Data1: 0xd78e1e87, Data2: 0x8644, Data3: 0x4ea5,
		Data4: [8]byte{0x94, 0x37, 0xd8, 0x09, 0xec, 0xef, 0xc9, 0x71}}
	poleFlagi = windows.GUID{Data1: 0x632ce23b, Data2: 0x5167, Data3: 0x435c,
		Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c}}
)
