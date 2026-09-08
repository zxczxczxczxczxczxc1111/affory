package petlya

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Maska это посторонний TLS-сервер, за который прячется REALITY.
//
// Настоящий REALITY уводит чужое рукопожатие на живой сайт вроде
// www.microsoft.com и его же сертификатом отвечает тому, кто пришёл без ключа.
// В петле такой сайт свой: тест, зависящий от чужой сети, обвиняет продукт за
// то, чего продукт не делал, и этот класс ошибок уже стоил разбора целого дня
// 06.09.2026, когда reality «не нёс» из-за сети, а не из-за клиента.
type Maska struct {
	// Imya это server_name, он же SNI в ссылке.
	Imya string
	// Port петлевой: сервер REALITY ходит на маску сам, изнутри той же машины.
	Port int
}

// Имя маски намеренно НЕ совпадает с именем узла: SNI рукопожатия REALITY это
// имя постороннего сайта, а не имя нашего сервера, и совпадение скрыло бы
// путаницу между ними.
const imyaMaski = "maska.example"

// NovayaMaska поднимает TLS-сервер маски и гасит его сам.
func NovayaMaska(t *testing.T) Maska {
	t.Helper()
	sert := NovyySertifikat(t, imyaMaski)
	para, err := tls.LoadX509KeyPair(sert.PutSert, sert.PutKlyuch)
	if err != nil {
		t.Fatalf("сертификат маски не загрузился: %v", err)
	}

	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "maska")
	}))
	s.TLS = &tls.Config{Certificates: []tls.Certificate{para}}
	s.StartTLS()
	t.Cleanup(s.Close)

	_, port, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatalf("адрес маски не разобрался: %v", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("порт маски не число: %v", err)
	}
	return Maska{Imya: imyaMaski, Port: n}
}
