package petlya

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"os"
	"testing"
)

func TestSertifikatPodpisanSvoimKornem(t *testing.T) {
	s := NovyySertifikat(t, "opyt.example")

	list := prochitatSert(t, s.PutSert)
	koren := prochitatSertIzPEM(t, s.CaPEM)

	// Именно CheckSignatureFrom, а не Verify с пулом: пул, в который положили
	// сам лист, принимает его как доверенный корень, и самоподписанный лист
	// прошёл бы такую проверку молча (поймано мутацией 07.09.2026).
	if err := list.CheckSignatureFrom(koren); err != nil {
		t.Fatalf("лист подписан не этим корнем: %v", err)
	}
	if !koren.IsCA {
		t.Error("корень не помечен как CA")
	}
	if koren.Equal(list) {
		t.Fatal("корень и лист это один сертификат")
	}

	pul := x509.NewCertPool()
	pul.AddCert(koren)
	if _, err := list.Verify(x509.VerifyOptions{Roots: pul, DNSName: "opyt.example"}); err != nil {
		t.Fatalf("сертификат не проверился своим же корнем: %v", err)
	}
}

func TestSertifikatSluzhitPetle(t *testing.T) {
	s := NovyySertifikat(t, "opyt.example")
	list := prochitatSert(t, s.PutSert)

	// Сервер в петле стоит на 127.0.0.1, и без этого адреса в SAN клиент
	// откажется молча ещё до транспорта.
	nashla := false
	for _, ip := range list.IPAddresses {
		if ip.Equal(net.IPv4(127, 0, 0, 1)) {
			nashla = true
		}
	}
	if !nashla {
		t.Errorf("в SAN нет 127.0.0.1: %v", list.IPAddresses)
	}
	if _, err := os.Stat(s.PutKlyuch); err != nil {
		t.Errorf("ключ не записан: %v", err)
	}
}

func TestPinSchitaetsyaPoOtkrytomuKlyuchu(t *testing.T) {
	s := NovyySertifikat(t, "opyt.example")
	list := prochitatSert(t, s.PutSert)

	summa := sha256.Sum256(list.RawSubjectPublicKeyInfo)
	zhdyom := base64.StdEncoding.EncodeToString(summa[:])
	if s.Pin != zhdyom {
		t.Errorf("пин %q, а по открытому ключу выходит %q", s.Pin, zhdyom)
	}
}

func TestDvaSertifikataRazlichayutsyaPinom(t *testing.T) {
	// Контроль на испорченный пин имеет смысл только если пины вообще разные.
	a := NovyySertifikat(t, "opyt.example")
	b := NovyySertifikat(t, "opyt.example")
	if a.Pin == b.Pin {
		t.Fatalf("два сертификата с одним пином %q: ключ не случайный", a.Pin)
	}
}

func prochitatSert(t *testing.T, put string) *x509.Certificate {
	t.Helper()
	telo, err := os.ReadFile(put)
	if err != nil {
		t.Fatalf("сертификат не прочитан: %v", err)
	}
	return prochitatSertIzPEM(t, string(telo))
}

func prochitatSertIzPEM(t *testing.T, telo string) *x509.Certificate {
	t.Helper()
	blok, _ := pem.Decode([]byte(telo))
	if blok == nil {
		t.Fatalf("PEM не разобрался: %.60q", telo)
	}
	list, err := x509.ParseCertificate(blok.Bytes)
	if err != nil {
		t.Fatalf("сертификат не разобрался: %v", err)
	}
	return list
}
