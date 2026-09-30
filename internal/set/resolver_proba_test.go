package set

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// podstavnoyDNS отвечает на первый вопрос заданным кодом. Пустой код
// значит «молчать».
func podstavnoyDNS(t *testing.T, kod *dnsmessage.RCode) netip.AddrPort {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 1500)
		n, otkuda, err := pc.ReadFrom(buf)
		if err != nil || kod == nil {
			return
		}
		var p dnsmessage.Parser
		h, err := p.Start(buf[:n])
		if err != nil {
			return
		}
		// Сначала чужой пакет: проба обязана его пропустить.
		for _, zagolovok := range []dnsmessage.Header{
			{ID: h.ID + 1, Response: true},
			{ID: h.ID, Response: true, RCode: *kod},
		} {
			b := dnsmessage.NewBuilder(nil, zagolovok)
			paket, err := b.Finish()
			if err != nil {
				t.Errorf("ответ не собран: %v", err)
				return
			}
			if _, err := pc.WriteTo(paket, otkuda); err != nil {
				t.Errorf("ответ не отправлен: %v", err)
				return
			}
		}
	}()
	return netip.MustParseAddrPort(pc.LocalAddr().String())
}

func TestProbaResolvera(t *testing.T) {
	kod := func(k dnsmessage.RCode) *dnsmessage.RCode { return &k }
	for _, sl := range []struct {
		imya  string
		kod   *dnsmessage.RCode
		godno bool
	}{
		{"отвечает", kod(dnsmessage.RCodeSuccess), true},
		{"имени нет", kod(dnsmessage.RCodeNameError), true},
		{"отказывает", kod(dnsmessage.RCodeRefused), false},
		{"сбоит", kod(dnsmessage.RCodeServerFailure), false},
		{"молчит", nil, false},
	} {
		t.Run(sl.imya, func(t *testing.T) {
			ctx, otmena := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer otmena()
			err := sprositNa(ctx, podstavnoyDNS(t, sl.kod))
			if sl.godno && err != nil {
				t.Fatalf("годный резолвер забракован: %v", err)
			}
			if !sl.godno && err == nil {
				t.Fatal("негодный резолвер принят")
			}
			if sl.kod != nil && !sl.godno && !errors.Is(err, errResolverOtkazal) {
				t.Fatalf("отказ сервера назван иначе: %v", err)
			}
		})
	}
}
