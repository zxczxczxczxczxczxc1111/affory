package set

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"

	"golang.org/x/net/dns/dnsmessage"
)

// SprositResolver задаёт DNS-серверу один короткий вопрос и отвечает, годен
// ли он (M9 аудита 1.8.0). Вопрос про корневую зону: на него отвечает любой
// рекурсивный резолвер, и ответ не зависит от того, что за сайты у человека.
//
// Годным считается ответ «есть» или «такого имени нет». Отказ обслуживать и
// сбой на стороне сервера значат, что имена серверов через него не
// разрешатся, то есть для ядра он тот же молчащий.
func SprositResolver(ctx context.Context, r netip.Addr) error {
	return sprositNa(ctx, netip.AddrPortFrom(r, 53))
}

// sprositNa отдельно от SprositResolver ради теста: слушать 53 порт на
// машине разработчика нельзя.
func sprositNa(ctx context.Context, adres netip.AddrPort) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", adres.String())
	if err != nil {
		return err
	}
	defer conn.Close()
	if srok, est := ctx.Deadline(); est {
		if err := conn.SetDeadline(srok); err != nil {
			return err
		}
	}

	id := uint16(rand.Uint32())
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	if err := b.StartQuestions(); err != nil {
		return err
	}
	if err := b.Question(dnsmessage.Question{
		Name: dnsmessage.MustNewName("."), Type: dnsmessage.TypeNS, Class: dnsmessage.ClassINET,
	}); err != nil {
		return err
	}
	zapros, err := b.Finish()
	if err != nil {
		return err
	}
	if _, err := conn.Write(zapros); err != nil {
		return err
	}

	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return err
		}
		var p dnsmessage.Parser
		h, err := p.Start(buf[:n])
		if err != nil || h.ID != id || !h.Response {
			// Чужой или битый пакет: ждём свой до срока.
			continue
		}
		switch h.RCode {
		case dnsmessage.RCodeSuccess, dnsmessage.RCodeNameError:
			return nil
		default:
			return fmt.Errorf("%w: код ответа %s", errResolverOtkazal, h.RCode)
		}
	}
}

var errResolverOtkazal = errors.New("DNS-сервер ответил отказом")
