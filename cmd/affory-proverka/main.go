// Команда affory-proverka читает ссылки и говорит, поднимутся ли они.
//
// Зачем отдельный инструмент. Генератор подписки собирает ссылки ТЕКСТОМ из
// живого конфига сервера, и до публикации их никто не читал нашим разбором.
// Опечатка в параметре, потерянный пин, транспорт, которого ядро не несёт, -
// всё это доезжает до человека и выясняется на его машине. Хуже того, ядро
// отвергает ВЕСЬ конфиг из-за одной негодной записи, то есть одна кривая
// ссылка уносит вместе с собой исправные серверы.
//
// Проверка идёт в два захода, и оба нужны:
//
//	по одной   - негодная ссылка названа поимённо, а не «что-то в наборе»;
//	все вместе - так конфиг собирается в бою, и только здесь видно то, что
//	             ломается набором: совпавшие идентификаторы, кандидат без
//	             адреса в правиле петли.
//
// Запускается на той машине, где ссылки уже есть. На локальный диск они не
// пишутся: читаются со стандартного ввода, конфиг кладётся во временный файл
// с правами 0600 и стирается сразу после проверки.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// ErrYadroNeZapustilos отделяет отказ прибора от приговора ссылке.
var ErrYadroNeZapustilos = errors.New("ядро не запустилось")

// Пути процессов и адрес Clash нужны генератору, но к годности ссылки
// отношения не имеют: это обвязка, одинаковая для всех проверок.
var (
	putiProtsessov = []string{
		`C:\Program Files\Affory\sing-box.exe`,
		`C:\Program Files\Affory\affory-svc.exe`,
	}
	klash = genkonfig.ClashApi{Adres: "127.0.0.1", Port: 9090, Sekret: "proverka"}
)

func main() {
	yadro := flag.String("yadro", "sing-box", "путь к ядру sing-box")
	tiho := flag.Bool("tiho", false, "печатать только негодные")
	flag.Parse()

	stroki, err := prochitat(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "не прочитать ввод:", err)
		os.Exit(2)
	}
	if len(stroki) == 0 {
		fmt.Fprintln(os.Stderr, "на входе нет ни одной ссылки")
		os.Exit(2)
	}

	plohih := 0
	for _, s := range stroki {
		imya := imyaSsylki(s)
		srv, err := ssylki.Razobrat(s)
		if err != nil {
			fmt.Printf("НЕГОДНА  %-28s разбор: %v\n", imya, err)
			plohih++
			continue
		}
		if !genkonfig.Izvestnyy(srv.Transport) {
			fmt.Printf("НЕГОДНА  %-28s транспорт %q ядро не несёт\n", imya, srv.Transport)
			plohih++
			continue
		}
		if err := proveritYadrom(*yadro, []protokol.Server{srv}, srv); err != nil {
			if errors.Is(err, ErrYadroNeZapustilos) {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			fmt.Printf("НЕГОДНА  %-28s ядро: %v\n", imya, err)
			plohih++
			continue
		}
		if !*tiho {
			fmt.Printf("годна    %-28s %s %s:%d\n", imya, srv.Transport, srv.Host, srv.Port)
		}
	}

	// Набор целиком собирается ТЕМ ЖЕ разбором подписки, что и в клиенте, а
	// не склейкой одиночных. Разница не формальная: клиент схлопывает
	// повторяющиеся идентификаторы молча, и прибор, который этого не делает,
	// обвиняет исправный набор. Первый же прогон на живых ссылках 19.09.2026
	// так и сказал «идентификатор встречается дважды» про подписку, которая в
	// бою поднимается: два профиля reality на одном входе, для ПК и для
	// телефона, и в саму подписку они уезжают порознь.
	if plohih == len(stroki) {
		// Годных нет вовсе, и набор из них не соберётся по определению.
		// Считать это ВТОРЫМ отказом значит печатать «негодных 2» там, где
		// негодная ссылка одна.
		fmt.Printf("\nссылок %d, негодных %d\n", len(stroki), plohih)
		os.Exit(1)
	}
	razbor, err := ssylki.RazobratSpisok([]byte(strings.Join(stroki, "\n")))
	if err != nil {
		fmt.Printf("НЕГОДЕН  %-28s разбор подписки: %v\n", "набор целиком", err)
		plohih++
	} else if len(razbor.Servery) > 0 {
		if shlopnuto := len(stroki) - len(razbor.Servery) - len(razbor.Otkazy) - len(razbor.Uvedomleniya); shlopnuto > 0 {
			// Не отказ: так же поступит и клиент. Но сказать надо, иначе
			// «опубликовано 9 ссылок» означает восемь серверов на экране.
			fmt.Printf("внимание %-28s %d строк схлопнуто: тот же адрес, порт и транспорт\n", "набор целиком", shlopnuto)
		}
		if err := proveritYadrom(*yadro, razbor.Servery, razbor.Servery[0]); err != nil {
			if errors.Is(err, ErrYadroNeZapustilos) {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			fmt.Printf("НЕГОДЕН  %-28s ядро: %v\n", "набор целиком", err)
			plohih++
		} else if !*tiho {
			fmt.Printf("годен    %-28s %d серверов\n", "набор целиком", len(razbor.Servery))
		}
	}

	fmt.Printf("\nссылок %d, негодных %d\n", len(stroki), plohih)
	if plohih > 0 {
		os.Exit(1)
	}
}

// prochitat берёт со входа строки, похожие на ссылки. Шапка файла ссылок,
// пустые строки и хвост с публичным ключом отбрасываются здесь, чтобы
// вызывающему не приходилось их вырезать sed-ом и ошибаться в этом.
func prochitat(f *os.File) ([]string, error) {
	var itog []string
	sk := bufio.NewScanner(f)
	sk.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sk.Scan() {
		s := strings.TrimSpace(sk.Text())
		if s == "" || !strings.Contains(s, "://") {
			continue
		}
		itog = append(itog, s)
	}
	return itog, sk.Err()
}

// imyaSsylki отдаёт имя профиля из фрагмента. В отчёт идёт оно, а не сама
// ссылка: в ссылке ключ, а отчёт попадает в журнал прогона.
func imyaSsylki(s string) string {
	if i := strings.LastIndex(s, "#"); i >= 0 && i+1 < len(s) {
		return s[i+1:]
	}
	if i := strings.Index(s, "://"); i > 0 {
		return s[:i] + "://…"
	}
	return "без имени"
}

func proveritYadrom(yadro string, nabor []protokol.Server, vybrannyy protokol.Server) error {
	kandidaty := adresaLiteraly(nabor)
	if len(kandidaty) == 0 {
		// Все адреса набора это имена, а не литералы. Правилу петли тогда
		// нечего перечислять, но генератор требует непустой список.
		kandidaty = []netip.Addr{netip.MustParseAddr("192.0.2.1")}
	}
	v := genkonfig.Vhod{
		Server:         vybrannyy,
		Kandidaty:      kandidaty,
		Resolver:       netip.MustParseAddr("10.7.0.1"),
		PutiProtsessov: putiProtsessov,
		ClashApi:       klash,
	}
	if len(nabor) > 1 {
		v.Servery = nabor
	}
	telo, err := genkonfig.SingBox(v)
	if err != nil {
		return err
	}
	return check(yadro, telo)
}

func adresaLiteraly(nabor []protokol.Server) []netip.Addr {
	vidno := map[string]bool{}
	var itog []netip.Addr
	for _, s := range nabor {
		a, err := netip.ParseAddr(s.Host)
		if err != nil || vidno[a.String()] {
			continue
		}
		vidno[a.String()] = true
		itog = append(itog, a)
	}
	return itog
}

// check отдаёт конфиг ядру. Файл кладётся с правами 0600 и стирается сразу:
// в нём ключи, а проверка часто идёт на общей машине.
func check(yadro string, telo []byte) error {
	dir, err := os.MkdirTemp("", "affory-proverka")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	put := filepath.Join(dir, "konfig.json")
	if err := os.WriteFile(put, telo, 0o600); err != nil {
		return err
	}
	cmd := exec.Command(yadro, "check", "-c", put)
	vyvod, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// Ядро, которое не запустилось, это отказ ПРИБОРА, а не приговор ссылке.
	// Разница не косметическая: без неё отчёт «негодны все» читается как
	// «ключи испорчены», и на поиск несуществующей поломки уходит вечер.
	var vyhod *exec.ExitError
	if !errors.As(err, &vyhod) {
		return fmt.Errorf("%w: %s: %v", ErrYadroNeZapustilos, yadro, err)
	}
	soobshchenie := strings.TrimSpace(string(vyvod))
	if soobshchenie == "" {
		return err
	}
	// Ядро печатает путь к временному файлу в каждой жалобе. В отчёте он
	// шум: файла уже нет, а строка из-за него не совпадает между прогонами.
	return errors.New(strings.ReplaceAll(soobshchenie, put, "конфиг"))
}
