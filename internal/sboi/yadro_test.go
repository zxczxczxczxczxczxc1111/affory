package sboi

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Строки 1-3 это хвосты ядра из журнала живой машины 02.10.2026 (адреса
// заменены), по одному на семью протоколов: anytls, hy2 и tuic поверх QUIC,
// trojan. Остальные собраны по исходникам Go и quic-go, откуда эти тексты и
// берутся.
func TestPoStrokeYadra(t *testing.T) {
	sluchai := []struct {
		tekst  string
		zhdyom PrichinaYadra
	}{
		{"failed to create session: tls: failed to verify certificate: x509: certificate has expired or is not yet valid: ", SrokSertifikata},
		{"open connection: CRYPTO_ERROR 0x12a (local): tls: failed to verify certificate: x509: certificate has expired or is not yet valid: ", SrokSertifikata},
		{"tls: failed to verify certificate: x509: certificate has expired or is not yet valid: ", SrokSertifikata},
		// Свой проверяющий Go (не Windows) кладёт сроки в подробность.
		{"x509: certificate has expired or is not yet valid: current time 2026-10-02T20:12:53+03:00 is after 2026-10-02T15:14:00Z", SrokSertifikata},

		{"tls: failed to verify certificate: x509: certificate is valid for vpn.example.net, not drugoy.example.net", ChuzhoeImya},
		{"x509: certificate is valid for 198.51.100.8, not 198.51.100.7", ChuzhoeImya},
		{"x509: certificate is valid for 120 names, but none matched vpn.example.net", ChuzhoeImya},
		{"x509: certificate is not valid for any names, but wanted to match vpn.example.net", ChuzhoeImya},
		{"x509: cannot validate certificate for 198.51.100.7 because it doesn't contain any IP SANs", ChuzhoeImya},

		{"tls: failed to verify certificate: x509: certificate signed by unknown authority", NeizvestnyyPodpisant},
		{`x509: certificate signed by unknown authority (possibly because of "x509: invalid signature" while trying to verify candidate authority certificate "Primer CA")`, NeizvestnyyPodpisant},
		{"open connection: CRYPTO_ERROR 0x12a (local): tls: failed to verify certificate: x509: certificate signed by unknown authority", NeizvestnyyPodpisant},

		{"remote error: tls: handshake failure", ObryvRukopozhatiya},
		{"failed to create session: remote error: tls: no application protocol", ObryvRukopozhatiya},
		{"open connection: CRYPTO_ERROR 0x128 (remote): tls: handshake failure", ObryvRukopozhatiya},
		{"CRYPTO_ERROR 0x178 (remote)", ObryvRukopozhatiya},

		// Не сертификат и не рукопожатие: тут причину называть нечем.
		{"context deadline exceeded", NeNazvana},
		{"failed to create session: context deadline exceeded", NeNazvana},
		{"dial tcp 198.51.100.7:995: i/o timeout", NeNazvana},
		{"dial udp 198.51.100.7:443: An invalid argument was supplied.", NeNazvana},
		{"open connection: CRYPTO_ERROR 0x150 (local): tls: unexpected message", NeNazvana},
		{"Application error 0x0 (remote)", NeNazvana},
		// Имя совпало, сертификат выписан по-старому: «на другое имя» было бы
		// неправдой.
		{"x509: certificate relies on legacy Common Name field, use SANs instead", NeNazvana},
		{"", NeNazvana},
	}
	for _, sl := range sluchai {
		if got := PoStrokeYadra(sl.tekst); got != sl.zhdyom {
			t.Errorf("PoStrokeYadra(%q) = %q, ждали %q", sl.tekst, got, sl.zhdyom)
		}
	}
}

// Короткая надпись встаёт в строку сервера вместо «недоступен», полная фраза
// в подсказку и в отказ подключения. Обе по правилам окна: с маленькой
// буквы, без точки в конце, короткая не длиннее строки списка.
func TestTekstyPrichinYadra(t *testing.T) {
	for _, p := range []PrichinaYadra{SrokSertifikata, ChuzhoeImya, NeizvestnyyPodpisant, ObryvRukopozhatiya} {
		korotko, tekst := p.Korotko(), p.Tekst()
		if korotko == "" || tekst == "" {
			t.Errorf("%s: пустой текст (коротко %q, целиком %q)", p, korotko, tekst)
			continue
		}
		if n := utf8.RuneCountInString(korotko); n > 25 {
			t.Errorf("%s: надпись %q длиной %d не уместится в строке сервера", p, korotko, n)
		}
		for _, s := range []string{korotko, tekst} {
			if strings.HasSuffix(s, ".") {
				t.Errorf("%s: точка в конце %q", p, s)
			}
			if r, _ := utf8.DecodeRuneInString(s); r >= 'А' && r <= 'Я' {
				t.Errorf("%s: %q с большой буквы, а встаёт в середину фразы", p, s)
			}
			if ObrezatTehniku(s) != s {
				t.Errorf("%s: %q режется как чужой текст до %q", p, s, ObrezatTehniku(s))
			}
		}
	}
	if NeNazvana.Korotko() != "" || NeNazvana.Tekst() != "" {
		t.Error("у ненайденной причины есть текст: окно показало бы догадку")
	}
}
