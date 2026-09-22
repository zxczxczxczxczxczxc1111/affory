package sboi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
	"testing"
)

// Вложенность здесь не декорация: настоящие ошибки приезжают завёрнутыми в
// *url.Error поверх *net.OpError, и классификация по верхнему слою называла бы
// всё «сетью». Каждый случай проверяется в той обёртке, в какой приходит.
func TestKlassifikatsiyaPoTipamOshibok(t *testing.T) {
	sluchai := []struct {
		imya string
		err  error
		zhdu Vid
	}{
		{"пусто", nil, Neyasno},
		{"отмена", &url.Error{Op: "Get", URL: "https://x", Err: context.Canceled}, Otmena},
		{"срок контекста", fmt.Errorf("обёртка: %w", context.DeadlineExceeded), Srok},
		{"срок сокета", &url.Error{Err: os.ErrDeadlineExceeded}, Srok},
		{"имя не разрешилось", &url.Error{Err: &net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host", IsNotFound: true}}}, DNS},
		{"сертификат не проверился", &url.Error{Err: &tls.CertificateVerificationError{Err: errors.New("x509")}}, TLS},
		{"имя в сертификате чужое", &url.Error{Err: &x509.HostnameError{Host: "x"}}, TLS},
		{"подписан неизвестным", &url.Error{Err: &x509.UnknownAuthorityError{}}, TLS},
		{"не тот заголовок записи TLS", &url.Error{Err: tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}}, TLS},
		{"порт закрыт", &url.Error{Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, TCP},
		{"соединение сброшено", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, TCP},
		{"сеть недоступна", fmt.Errorf("обёртка: %w", syscall.ENETUNREACH), TCP},
		{"чужая ошибка", errors.New("что-то своё"), Neyasno},
	}
	for _, s := range sluchai {
		if got := Klassifitsirovat(s.err); got != s.zhdu {
			t.Errorf("%s: получил %q, жду %q", s.imya, got, s.zhdu)
		}
	}
}

// Отдельная проверка приоритета: таймаут сетевого набора это net.Error с
// Timeout поверх *net.OpError, и без порядка он читался бы как TCP.
func TestSrokSetiSilneeSlovaOSokete(t *testing.T) {
	err := &url.Error{Err: &net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}}
	if got := Klassifitsirovat(err); got != Srok {
		t.Fatalf("таймаут набора назван %q", got)
	}
}

func TestKodyOtvetaVedutVRaznyeStorony(t *testing.T) {
	sluchai := map[int]Vid{
		200: Neyasno, 204: Neyasno,
		401: Dostup, 403: Dostup, 407: Dostup,
		// 504 приходит от clash_api на не уложившуюся в срок пробу. Именно он
		// назывался «сервер не принял ключ», отправляя чинить исправную ссылку.
		408: Srok, 504: Srok,
		404: Otvet, 500: Otvet, 502: Otvet,
	}
	for kod, zhdu := range sluchai {
		if got := PoKoduOtveta(kod); got != zhdu {
			t.Errorf("код %d: получил %q, жду %q", kod, got, zhdu)
		}
	}
}

func TestUKazhdogoVidaEstOpisanie(t *testing.T) {
	for _, v := range []Vid{DNS, TCP, TLS, Dostup, Srok, Otmena, Otvet, Neyasno} {
		if Vid.Opisanie(v) == "" {
			t.Errorf("вид %q без описания", v)
		}
	}
}
