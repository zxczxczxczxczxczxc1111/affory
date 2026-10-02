package sboi

import (
	"regexp"
	"strings"
)

// Причина отказа сервера, которую назвало ядро (02.10.2026).
//
// Повод: у машины истёк сертификат входов, пять ключей из шести перестали
// подключаться, а окно говорило «сервер не принял ключ» и вело чинить
// подписку. Настоящая причина была только в журнале ядра строкой
// «x509: certificate has expired or is not yet valid».
//
// Здесь, в отличие от Klassifitsirovat, разбор идёт по ПОДСТРОКАМ: ядро это
// чужой процесс, и типов его ошибок у нас нет, есть только текст. Подстроки
// взяты из исходников, а не из догадок: crypto/x509/verify.go (сроки, имя,
// подписант), crypto/x509/root_windows.go (под Windows истёкший сертификат
// приходит с пустой подробностью, ровно так он и выглядел в журнале),
// crypto/tls/conn.go («remote error» на предупреждение с той стороны),
// quic-go internal/qerr/errors.go («CRYPTO_ERROR 0x1.. (remote)» у hy2 и tuic).
// Сменится текст в новой версии Go или ядра - причина просто перестанет
// называться, а тесты на эти строки покраснеют.

// PrichinaYadra это класс отказа рукопожатия с сервером. Пустое значение
// значит «ядро не назвало ничего из известного», и тогда человек видит
// прежний текст, а не догадку.
type PrichinaYadra string

const (
	NeNazvana PrichinaYadra = ""
	// Истёк или ещё не начал действовать. Под Windows ядро эти два случая не
	// различает: подробности со сроками в тексте нет.
	SrokSertifikata PrichinaYadra = "srok-sertifikata"
	// Выписан на другое имя или адрес, чем записан в ключе.
	ChuzhoeImya PrichinaYadra = "chuzhoe-imya"
	// Цепочку не подтвердила Windows: самодельный сертификат, чужой
	// удостоверяющий центр или перехват по пути.
	NeizvestnyyPodpisant PrichinaYadra = "neizvestnyy-podpisant"
	// Сервер сам оборвал рукопожатие предупреждением TLS.
	ObryvRukopozhatiya PrichinaYadra = "obryv-rukopozhatiya"
)

// Порядок важен: строка QUIC несёт и «CRYPTO_ERROR», и текст x509, и
// сертификат должен победить, когда отказал он, а не сервер.
var priznakiYadra = []struct {
	prichina  PrichinaYadra
	podstroki []string
}{
	{SrokSertifikata, []string{"x509: certificate has expired or is not yet valid"}},
	{ChuzhoeImya, []string{
		"x509: certificate is valid for ",
		"x509: certificate is not valid for any names",
		"because it doesn't contain any IP SANs",
	}},
	{NeizvestnyyPodpisant, []string{"x509: certificate signed by unknown authority"}},
	{ObryvRukopozhatiya, []string{"remote error: tls: "}},
}

// Код QUIC от 0x100 до 0x1ff это предупреждение TLS, «remote» значит, что его
// прислал сервер.
var obryvQUIC = regexp.MustCompile(`CRYPTO_ERROR 0x1[0-9a-f]{2} \(remote\)`)

// PoStrokeYadra называет причину по тексту ошибки ядра.
func PoStrokeYadra(tekst string) PrichinaYadra {
	for _, p := range priznakiYadra {
		for _, s := range p.podstroki {
			if strings.Contains(tekst, s) {
				return p.prichina
			}
		}
	}
	if obryvQUIC.MatchString(tekst) {
		return ObryvRukopozhatiya
	}
	return NeNazvana
}

// Korotko это надпись в строке сервера, на месте «недоступен». Короткая
// намеренно: строка списка одна, и длинный текст наехал бы на имя сервера.
func (p PrichinaYadra) Korotko() string {
	switch p {
	case SrokSertifikata:
		return "сертификат не действует"
	case ChuzhoeImya:
		return "сертификат на другое имя"
	case NeizvestnyyPodpisant:
		return "сертификат не подтверждён"
	case ObryvRukopozhatiya:
		return "сервер оборвал связь"
	}
	return ""
}

// Tekst это причина целиком: в подсказке у строки сервера и в отказе
// подключения. Говорит, что случилось и на чьей стороне чинить.
func (p PrichinaYadra) Tekst() string {
	switch p {
	case SrokSertifikata:
		return "сертификат сервера просрочен или ещё не начал действовать: если часы на компьютере верные, его должен продлить владелец сервера"
	case ChuzhoeImya:
		return "у сервера сертификат на другое имя: адрес в ключе мог устареть, или связь перехватывают по пути"
	case NeizvestnyyPodpisant:
		return "сертификат сервера подписал тот, кому Windows не доверяет: так бывает у самодельного сертификата или когда связь перехватывают по пути"
	case ObryvRukopozhatiya:
		return "сервер оборвал защищённое соединение в самом начале, ключ мог устареть"
	}
	return ""
}
