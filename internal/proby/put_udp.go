package proby

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"time"
)

// putUDP это путь пакетов пробы: напрямую или через прокси-вход ядра.
type putUDP interface {
	otpravit(paket []byte) error
	poluchit(srok time.Time) ([]byte, error)
	Close() error
}

// Запрос STUN: тип Binding Request, пустое тело, постоянное «магическое
// печенье» RFC 5389 и двенадцать байт номера.
const (
	stunZapros     = 0x0001
	stunUspeh      = 0x0101
	stunPechenye   = 0x2112A442
	dlinaZagolovka = 20
)

func zaprosSTUN() ([]byte, []byte, error) {
	z := make([]byte, dlinaZagolovka)
	binary.BigEndian.PutUint16(z[0:], stunZapros)
	binary.BigEndian.PutUint32(z[4:], stunPechenye)
	if _, err := rand.Read(z[8:]); err != nil {
		return nil, nil, fmt.Errorf("номер запроса STUN не выбран: %w", err)
	}
	return z, z[8:], nil
}

func otvetSTUN(p, nomer []byte) bool {
	return len(p) >= dlinaZagolovka &&
		binary.BigEndian.Uint16(p[0:]) == stunUspeh &&
		binary.BigEndian.Uint32(p[4:]) == stunPechenye &&
		bytes.Equal(p[8:dlinaZagolovka], nomer)
}

type pryamoyUDP struct{ c net.Conn }

func otkrytPryamoyUDP(ctx context.Context, cel string) (putUDP, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "udp", cel)
	if err != nil {
		return nil, err
	}
	return pryamoyUDP{c}, nil
}

func (p pryamoyUDP) otpravit(paket []byte) error { _, err := p.c.Write(paket); return err }

func (p pryamoyUDP) poluchit(srok time.Time) ([]byte, error) {
	if err := p.c.SetReadDeadline(srok); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	n, err := p.c.Read(buf)
	return buf[:n], err
}

func (p pryamoyUDP) Close() error { return p.c.Close() }

// socksUDP это пересылка UDP через SOCKS5 (RFC 1928, раздел 7). Пока открыто
// управляющее TCP-соединение, ядро держит пересылку; каждый пакет несёт
// заголовок с адресом цели.
type socksUDP struct {
	upr       net.Conn
	udp       *net.UDPConn
	zagolovok []byte
}

func otkrytSocksUDP(ctx context.Context, portProksi int, cel string) (putUDP, error) {
	zagolovok, err := zagolovokSocks(cel)
	if err != nil {
		return nil, err
	}
	var d net.Dialer
	upr, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(portProksi)))
	if err != nil {
		return nil, fmt.Errorf("прокси-вход ядра не отвечает: %w", err)
	}
	srok := time.Now().Add(SrokProby)
	if dl, est := ctx.Deadline(); est && dl.Before(srok) {
		srok = dl
	}
	if err := upr.SetDeadline(srok); err != nil {
		upr.Close()
		return nil, err
	}
	rele, err := socksAssociate(upr)
	if err != nil {
		upr.Close()
		return nil, err
	}
	udp, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(rele))
	if err != nil {
		upr.Close()
		return nil, fmt.Errorf("пересылка UDP ядра недоступна: %w", err)
	}
	if err := upr.SetDeadline(time.Time{}); err != nil {
		udp.Close()
		upr.Close()
		return nil, err
	}
	return &socksUDP{upr: upr, udp: udp, zagolovok: zagolovok}, nil
}

// socksAssociate договаривается о пересылке и отдаёт адрес, куда слать пакеты.
func socksAssociate(upr net.Conn) (netip.AddrPort, error) {
	if _, err := upr.Write([]byte{5, 1, 0}); err != nil {
		return netip.AddrPort{}, err
	}
	var otv [2]byte
	if _, err := io.ReadFull(upr, otv[:]); err != nil {
		return netip.AddrPort{}, fmt.Errorf("прокси-вход не ответил на приветствие SOCKS5: %w", err)
	}
	if otv != [2]byte{5, 0} {
		return netip.AddrPort{}, fmt.Errorf("прокси-вход не принял SOCKS5 без пароля: %x", otv)
	}
	// UDP ASSOCIATE с пустым адресом: слать будем с любого своего порта.
	if _, err := upr.Write([]byte{5, 3, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return netip.AddrPort{}, err
	}
	var h [4]byte
	if _, err := io.ReadFull(upr, h[:]); err != nil {
		return netip.AddrPort{}, fmt.Errorf("прокси-вход не ответил на просьбу о UDP: %w", err)
	}
	if h[0] != 5 || h[1] != 0 {
		return netip.AddrPort{}, fmt.Errorf("прокси-вход отказал в пересылке UDP: код %d", h[1])
	}
	adres, err := chitatAdresSocks(upr, h[3])
	if err != nil {
		return netip.AddrPort{}, err
	}
	// Пустой адрес в ответе значит «тот же, куда стучались».
	if adres.Addr().IsUnspecified() {
		adres = netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), adres.Port())
	}
	return adres, nil
}

func chitatAdresSocks(r io.Reader, tip byte) (netip.AddrPort, error) {
	var a []byte
	switch tip {
	case 1:
		a = make([]byte, 4+2)
	case 4:
		a = make([]byte, 16+2)
	default:
		return netip.AddrPort{}, fmt.Errorf("прокси-вход назвал адрес пересылки типом %d", tip)
	}
	if _, err := io.ReadFull(r, a); err != nil {
		return netip.AddrPort{}, err
	}
	ip, _ := netip.AddrFromSlice(a[:len(a)-2])
	return netip.AddrPortFrom(ip.Unmap(), binary.BigEndian.Uint16(a[len(a)-2:])), nil
}

// zagolovokSocks собирает заголовок пакета: имя цели уходит как есть, и
// разрешает его уже сервер туннеля.
func zagolovokSocks(cel string) ([]byte, error) {
	host, portS, err := net.SplitHostPort(cel)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portS, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("порт цели %q: %w", portS, err)
	}
	z := []byte{0, 0, 0}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.Is4() {
			z = append(append(z, 1), ip.AsSlice()...)
		} else {
			z = append(append(z, 4), ip.AsSlice()...)
		}
	} else {
		if len(host) > 255 {
			return nil, errors.New("имя цели длиннее 255 байт")
		}
		z = append(append(z, 3, byte(len(host))), host...)
	}
	return binary.BigEndian.AppendUint16(z, uint16(port)), nil
}

func (s *socksUDP) otpravit(paket []byte) error {
	_, err := s.udp.Write(append(append([]byte{}, s.zagolovok...), paket...))
	return err
}

func (s *socksUDP) poluchit(srok time.Time) ([]byte, error) {
	if err := s.udp.SetReadDeadline(srok); err != nil {
		return nil, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := s.udp.Read(buf)
		if err != nil {
			return nil, err
		}
		if telo, ok := snyatZagolovokSocks(buf[:n]); ok {
			return telo, nil
		}
	}
}

// snyatZagolovokSocks отрезает заголовок ответного пакета. Дроблёные пакеты
// (FRAG не ноль) ядро не шлёт, а чужие отбрасываются.
func snyatZagolovokSocks(p []byte) ([]byte, bool) {
	if len(p) < 4 || p[2] != 0 {
		return nil, false
	}
	dlina := 0
	switch p[3] {
	case 1:
		dlina = 4
	case 4:
		dlina = 16
	case 3:
		if len(p) < 5 {
			return nil, false
		}
		dlina = 1 + int(p[4])
	default:
		return nil, false
	}
	nachalo := 4 + dlina + 2
	if len(p) < nachalo {
		return nil, false
	}
	return p[nachalo:], true
}

func (s *socksUDP) Close() error {
	return errors.Join(s.udp.Close(), s.upr.Close())
}
