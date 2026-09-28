package proby

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// zapustitSTUN поднимает местный STUN: на каждый запрос отвечает успехом с тем
// же номером. molchat заставляет его не отвечать вовсе.
func zapustitSTUN(t *testing.T, molchat bool) (netip.AddrPort, *atomic.Int32) {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	var zaprosov atomic.Int32
	go func() {
		buf := make([]byte, 1500)
		for {
			n, ot, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n < dlinaZagolovka || binary.BigEndian.Uint16(buf) != stunZapros {
				continue
			}
			zaprosov.Add(1)
			if molchat {
				continue
			}
			otv := make([]byte, dlinaZagolovka)
			copy(otv, buf[:dlinaZagolovka])
			binary.BigEndian.PutUint16(otv, stunUspeh)
			c.WriteToUDP(otv, ot)
		}
	}()
	return c.LocalAddr().(*net.UDPAddr).AddrPort(), &zaprosov
}

// zapustitProksi поднимает SOCKS5 с пересылкой UDP: пакет на любое имя уходит
// на stun, а имя запоминается.
func zapustitProksi(t *testing.T, stun netip.AddrPort) (int, func() []string) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	var mu sync.Mutex
	var imena []string
	go func() {
		upr, err := l.Accept()
		if err != nil {
			return
		}
		defer upr.Close()
		buf := make([]byte, 10)
		if _, err := io.ReadFull(upr, buf[:3]); err != nil {
			return
		}
		upr.Write([]byte{5, 0})
		if _, err := io.ReadFull(upr, buf[:10]); err != nil || buf[1] != 3 {
			return
		}
		rele, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			return
		}
		defer rele.Close()
		otv := binary.BigEndian.AppendUint16([]byte{5, 0, 0, 1, 0, 0, 0, 0}, uint16(rele.LocalAddr().(*net.UDPAddr).Port))
		upr.Write(otv)
		k, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(stun))
		if err != nil {
			return
		}
		defer k.Close()
		p := make([]byte, 1500)
		for {
			n, klient, err := rele.ReadFromUDP(p)
			if err != nil {
				return
			}
			if n < 5 || p[3] != 3 {
				continue
			}
			dl := int(p[4])
			mu.Lock()
			imena = append(imena, string(p[5:5+dl]))
			mu.Unlock()
			k.Write(p[5+dl+2 : n])
			k.SetReadDeadline(time.Now().Add(time.Second))
			m, err := k.Read(p)
			if err != nil {
				continue
			}
			zag := []byte{0, 0, 0, 1}
			zag = append(zag, stun.Addr().AsSlice()...)
			zag = binary.BigEndian.AppendUint16(zag, stun.Port())
			rele.WriteToUDP(append(zag, p[:m]...), klient)
		}
	}()
	return l.Addr().(*net.TCPAddr).Port, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, imena...)
	}
}

// С5 аудита 1.6.1. Проба «Голос и видео» слала пакеты прямо из службы, а свои
// процессы службы правило процессов ведёт мимо туннеля: мерился путь без VPN.
// Через прокси-вход пакеты несут имя цели, и все до одного идут через него.
func TestUDPCherezProksiIdyotCherezProksi(t *testing.T) {
	stun, zaprosov := zapustitSTUN(t, false)
	port, imena := zapustitProksi(t, stun)
	u := UDP(context.Background(), "stun.example:3478", port)
	if !u.Proshlo || u.Poluchheno != PaketovUDP {
		t.Fatalf("проба через прокси: %+v", u)
	}
	vidennye := imena()
	if len(vidennye) != PaketovUDP || vidennye[0] != "stun.example" {
		t.Fatalf("прокси видел цели %v", vidennye)
	}
	if zaprosov.Load() != PaketovUDP {
		t.Fatalf("STUN получил %d запросов, отправлено %d", zaprosov.Load(), PaketovUDP)
	}
}

func TestUDPNapryamuyuBezProksi(t *testing.T) {
	stun, _ := zapustitSTUN(t, false)
	if u := UDP(context.Background(), stun.String(), 0); !u.Proshlo || u.Poluchheno != PaketovUDP {
		t.Fatalf("прямая проба: %+v", u)
	}
}

// Молчащий путь это красная проба, а не зелёная с нулём потерь.
func TestUDPBezOtvetovKrasnaya(t *testing.T) {
	stun, zaprosov := zapustitSTUN(t, true)
	ctx, otmena := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer otmena()
	u := UDP(ctx, stun.String(), 0)
	if u.Proshlo || u.Poluchheno != 0 || u.Poter != PaketovUDP {
		t.Fatalf("проба без ответов: %+v", u)
	}
	if zaprosov.Load() == 0 {
		t.Fatal("запросы не ушли вовсе: красная проба ничего не доказала")
	}
}

// Опоздавший ответ на прошлый запрос не засчитывается следующему.
func TestChuzhoyNomerSTUNNeZaschityvaetsya(t *testing.T) {
	z, nomer, err := zaprosSTUN()
	if err != nil {
		t.Fatal(err)
	}
	otv := append([]byte{}, z...)
	binary.BigEndian.PutUint16(otv, stunUspeh)
	if !otvetSTUN(otv, nomer) {
		t.Fatal("свой ответ не узнан")
	}
	otv[19] ^= 0xff
	if otvetSTUN(otv, nomer) {
		t.Fatal("ответ с чужим номером засчитан")
	}
}
