package ssylki

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Vychistit готовит текст журнала к выгрузке из машины (О6 аудита 1.6.1).
//
// Два слоя. Первый тот же, что у отказов подписки: известные секреты набора и
// адреса подписок, целиком и по частям. Второй ловит то, чего в наборе уже нет
// (удалённый сервер, ссылка, вставленная и отвергнутая): любая ссылка со
// схемой ключа, путь и запрос любого http(s) адреса, имя пользователя в пути
// к профилю. Адреса узлов остаются: по ним и разбирается сетевой дефект.
func Vychistit(tekst string, servery []protokol.Server, adresa []string) string {
	var kuski []string
	for _, s := range servery {
		kuski = append(kuski, s.Uuid, s.Parol, s.ObfsParol, s.PublicKey, s.ShortId)
		if s.Put != "/" {
			kuski = append(kuski, s.Put)
		}
	}
	tekst = zamenitKuski(tekst, kuski, "<секрет>")
	for _, a := range adresa {
		tekst = zamenitAdres(tekst, a)
	}
	tekst = reSsylka.ReplaceAllStringFunc(tekst, bezPutiSsylki)
	return reDomashniy.ReplaceAllString(tekst, "${1}<пользователь>")
}

var (
	// Схема до шестнадцати знаков, как в pohozheNaSkhemu, и всё до пробела
	// или кавычки.
	reSsylka = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]{1,15}://[^\s"'<>]+`)
	// C:\Users\<имя>, в том числе в JSON, где обратная косая удвоена.
	reDomashniy = regexp.MustCompile(`(?i)([a-z]:(?:\\\\|\\|/)users(?:\\\\|\\|/))([^\\/"'\n]+)`)
)

// bezPutiSsylki оставляет от http(s) адреса схему и узел, а ссылку ключа
// убирает целиком: в ней узел рядом с паролем.
func bezPutiSsylki(s string) string {
	n := strings.ToLower(s)
	if !strings.HasPrefix(n, "http://") && !strings.HasPrefix(n, "https://") {
		return "<ссылка>"
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "<ссылка>"
	}
	golyy := u.Scheme + "://" + u.Host + "/"
	if (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == "" {
		return golyy
	}
	return golyy + "<путь скрыт>"
}
