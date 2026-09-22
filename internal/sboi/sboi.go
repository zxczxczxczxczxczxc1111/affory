// Пакет sboi отвечает на один вопрос: на каком шаге сорвалась сетевая
// операция.
//
// Зачем отдельно. До 22.09.2026 всё, что мешало загрузить подписку, уезжало
// человеку одной строкой «подписка недоступна», а таймаут пробы через clash_api
// (код 504) назывался «сервер не принял ключ, проверь подписку». В разборе
// журнала живой машины 22.09.2026 это стоило отдельного захода: по записи
// нельзя было отличить не отвечающий сервер от неверного ключа, а совет вёл
// чинить исправную ссылку.
//
// Шаги названы так, как они ведут человека в РАЗНЫЕ стороны:
//
//	dns     - имя не разрешилось: чинить сеть или DNS, сервер ни при чём;
//	tcp     - до порта не достучались: сервер или путь до него;
//	tls     - канал не установился: сертификат, подмена, обрыв рукопожатия;
//	dostup  - ответили и отказали по праву доступа: ключ или подписка;
//	srok    - никто не отказал, просто не успели;
//	otmena  - операцию свернули мы сами: это не отказ и лечить нечего;
//	otvet   - ответ пришёл, но не тот, что ждали.
//
// Классификация идёт по ТИПАМ ошибок Go, а не по подстрокам текста: тексты
// меняются от версии к версии и переводятся, а типы нет.
package sboi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"syscall"
)

// Vid это шаг, на котором сорвалась операция. Пустое значение значит «не
// разобрались», и это честный ответ: выдумывать шаг хуже, чем не называть его.
type Vid string

const (
	Neyasno Vid = ""
	DNS     Vid = "dns"
	TCP     Vid = "tcp"
	TLS     Vid = "tls"
	Dostup  Vid = "dostup"
	Srok    Vid = "srok"
	Otmena  Vid = "otmena"
	Otvet   Vid = "otvet"
)

// Opisanie даёт короткую строку для журнала и для текста отказа.
func (v Vid) Opisanie() string {
	switch v {
	case DNS:
		return "имя сервера не разрешилось"
	case TCP:
		return "соединение не установилось"
	case TLS:
		return "защищённый канал не установился"
	case Dostup:
		return "доступ не подтверждён"
	case Srok:
		return "ответа не дождались"
	case Otmena:
		return "операция отменена"
	case Otvet:
		return "ответ не тот, что ожидался"
	}
	return "причина не определена"
}

// Klassifitsirovat разбирает ошибку сетевой операции.
//
// Порядок проверок НЕ случайный. Отмена и срок идут первыми: обёрнутый
// context.Canceled внутри *url.Error одновременно похож и на сетевую ошибку, и
// на таймаут, а человеку важно, что операцию свернули мы. DNS раньше TCP:
// *net.DNSError это тоже *net.OpError по вложенности. TLS раньше TCP по той же
// причине - рукопожатие рвётся поверх уже открытого сокета.
func Klassifitsirovat(err error) Vid {
	if err == nil {
		return Neyasno
	}
	switch {
	case errors.Is(err, context.Canceled):
		return Otmena
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
		return Srok
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return DNS
	}
	var zapisTLS tls.RecordHeaderError
	var sertifikat *x509.CertificateInvalidError
	var imyaSertifikata *x509.HostnameError
	var nepodpisan *x509.UnknownAuthorityError
	var trevogaTLS *tls.CertificateVerificationError
	if errors.As(err, &zapisTLS) || errors.As(err, &sertifikat) || errors.As(err, &imyaSertifikata) ||
		errors.As(err, &nepodpisan) || errors.As(err, &trevogaTLS) {
		return TLS
	}
	// Таймаут сети приходит не только контекстом: свой срок у http.Client и у
	// набора соединения даёт net.Error с Timeout.
	var setevaya net.Error
	if errors.As(err, &setevaya) && setevaya.Timeout() {
		return Srok
	}
	switch {
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.ECONNABORTED), errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.ETIMEDOUT):
		return TCP
	}
	var operatsiya *net.OpError
	if errors.As(err, &operatsiya) {
		return TCP
	}
	return Neyasno
}

// PoKoduOtveta разбирает ответ, который всё-таки пришёл.
//
// 401 и 403 это доступ: отвечает живой сервер и отказывает по праву, а не по
// сети. 408 и 504 это срок: никто не отказывал, просто не успели, и советовать
// тут проверку ключей - ровно тот дефект, из-за которого пакет и появился.
func PoKoduOtveta(kod int) Vid {
	switch {
	case kod == 401 || kod == 403 || kod == 407:
		return Dostup
	case kod == 408 || kod == 504 || kod == 598 || kod == 599:
		return Srok
	case kod >= 200 && kod < 300:
		return Neyasno
	}
	return Otvet
}
