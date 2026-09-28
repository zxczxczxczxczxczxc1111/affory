package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/reklama"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Блокировка рекламы: файл списка, его целостность и суточное обновление
// (28.09.2026).
//
// Главное свойство: сбой или обновление списка никогда не трогает туннель. Фон
// только меняет файл reklama.srs, а ядро перечитывает local-набор само.
// Переподключение бывает только при включении, выключении и правке исключений,
// и идёт обычным путём setRules.

const (
	// Первый заход через минуту после старта, как у проверки обновлений:
	// подъём туннеля важнее списка.
	pervyyZahodReklamy = time.Minute
	otstupReklamy      = time.Hour
	zhdatSpisok        = 60 * time.Second // multi весит 4,6 МБ
	// multi собирается за 1,25 с; 30 с это защита от зависшего процесса.
	zhdatSborku = 30 * time.Second
)

func putReklamy() string     { return filepath.Join(katalogNaborov(), "reklama.srs") }
func putMetyReklamy() string { return filepath.Join(katalogNaborov(), "reklama.json") }

// Проверки собранного набора ядром: по одной российской и мировой рекламе и
// один свой хост. Остальное уже проверено на тексте (reklama.Razobrat).
var (
	dolzhnySovpast   = []string{"an.yandex.ru", "googleads.g.doubleclick.net"}
	neDolzhnySovpast = []string{"raw.githubusercontent.com"}
)

func (s *Sluzhba) sobratSpisokYadrom(ctx context.Context, tekst, vyhod string) error {
	return yadra.SobratSpisok(ctx, imyaYadraTun, tekst, vyhod, dolzhnySovpast, neDolzhnySovpast)
}

// zhdatIliTolchok ждёт срок, толчок или отмену. Толчок это действие человека:
// он не ждёт отступа после отказа.
func zhdatIliTolchok(ctx context.Context, d time.Duration, ch <-chan struct{}) (tolchok, zhiv bool) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false, false
	case <-ch:
		return true, true
	case <-t.C:
		return false, true
	}
}

// tolknutReklamu будит расписание. Две просьбы подряд значат то же, что одна.
func (s *Sluzhba) tolknutReklamu() {
	select {
	case s.reklamaTolchok <- struct{}{}:
	default:
	}
}

func (s *Sluzhba) raspisanieReklamy(ctx context.Context) {
	s.nachatReklamu()
	var otkazDo time.Time
	pauza := pervyyZahodReklamy
	for {
		tolchok, zhiv := s.zhdatReklamu(ctx, pauza)
		if !zhiv {
			return
		}
		r := s.nastroykaReklamy()
		if r == nil || !r.Vkl {
			pauza = reklama.Period // спит до толчка из setRules
			continue
		}
		if err := s.obespechitFaylReklamy(); err != nil {
			log.Printf("встроенный список рекламы не выложен: %v", err)
			s.zapomnitOtkazReklamy(err.Error())
		}
		u, _ := reklama.Privesti(r.Uroven)
		srok := reklama.SleduyushchiyZahod(s.metaReklamy(), u, s.seychas())
		// Толчок (действие человека) не ждёт отступа после отказа.
		if !tolchok && otkazDo.After(srok) {
			srok = otkazDo
		}
		if pauza = srok.Sub(s.seychas()); pauza > 0 {
			continue
		}
		povtor, err := s.obnovitReklamu(ctx, u)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("список рекламы не обновлён, остаётся прежний: %v", err)
			s.zapomnitOtkazReklamy(err.Error())
			// Отказ по падению числа правил ждёт свой срок, остальные час.
			otkazDo = s.seychas().Add(otstupReklamy)
			if povtor.After(otkazDo) {
				otkazDo = povtor
			}
			pauza = otkazDo.Sub(s.seychas())
			continue
		}
		otkazDo, pauza = time.Time{}, reklama.Period
	}
}

// nachatReklamu убирает хвосты прерванной сборки и поднимает в статус то, что
// лежит на диске.
func (s *Sluzhba) nachatReklamu() {
	s.muReklama.Lock()
	defer s.muReklama.Unlock()
	s.uborkaReklamy()
	if m := prochitatMetuReklamy(); m != nil {
		s.zapomnitMetuReklamy(*m)
	}
}

// uborkaReklamy зовётся под muReklama.
func (s *Sluzhba) uborkaReklamy() {
	hvosty, err := filepath.Glob(filepath.Join(katalogNaborov(), "reklama*.chast"))
	if err != nil {
		log.Printf("хвосты списка рекламы не найдены: %v", err)
		return
	}
	for _, h := range hvosty {
		if err := os.Remove(h); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("хвост списка рекламы %s не убран: %v", filepath.Base(h), err)
		}
	}
}

// nastroykaReklamy: отказ чтения набора значит «выключено», а не «включено»:
// неизвестность не повод ходить в сеть.
func (s *Sluzhba) nastroykaReklamy() *ReklamaPravila {
	n, err := s.nabor()
	if err != nil {
		log.Printf("набор не прочитан, список рекламы не обновляется: %v", err)
		return nil
	}
	return n.Pravila.Reklama
}

func (s *Sluzhba) metaReklamy() *reklama.Meta {
	s.muReklama.Lock()
	defer s.muReklama.Unlock()
	return prochitatMetuReklamy()
}

// prochitatMetuReklamy зовётся под muReklama. Нет файла или он битый: меты
// нет, и файл набора считается неподтверждённым.
func prochitatMetuReklamy() *reklama.Meta {
	b, err := os.ReadFile(putMetyReklamy())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("мета списка рекламы не прочитана: %v", err)
		}
		return nil
	}
	var m reklama.Meta
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("мета списка рекламы не разобрана: %v", err)
		return nil
	}
	return &m
}

// zapisatMetuReklamy атомарно, через временный файл. Зовётся под muReklama.
func zapisatMetuReklamy(m reklama.Meta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	chast := putMetyReklamy() + ".chast"
	if err := os.WriteFile(chast, b, 0o644); err != nil {
		return fmt.Errorf("мета списка не записана: %w", err)
	}
	return pereimenovatSPovtorom(chast, putMetyReklamy())
}

// zapomnitMetuReklamy кладёт в статус ЛЕЖАЩИЙ список. Удачная запись снимает
// прежний отказ.
func (s *Sluzhba) zapomnitMetuReklamy(m reklama.Meta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sostReklamy = &protokol.ReklamaSostoyanie{
		Uroven: string(m.Uroven), Pravil: m.Pravil, Versiya: m.Versiya,
		Sobran: m.Sobran, Proveren: m.Proveren, Vstroennyy: m.Vstroennyy,
	}
}

// Причина уходит в status каждые 5 с и в одну строку окна, а вывод ядра при
// отказе сборки бывает до 64 КБ.
const predelPrichinyReklamy = 300

// zapomnitOtkazReklamy: голая причина, без префикса. Фразу вокруг строит окно.
func (s *Sluzhba) zapomnitOtkazReklamy(prichina string) {
	if r := []rune(prichina); len(r) > predelPrichinyReklamy {
		prichina = string(r[:predelPrichinyReklamy]) + "…"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sostReklamy == nil {
		s.sostReklamy = &protokol.ReklamaSostoyanie{}
	}
	s.sostReklamy.Otkaz = prichina
}

// pereimenovatSPovtorom: ядро читает набор при подмене, и Windows на это время
// отказывает в замене. Пять попыток через 200 мс.
func pereimenovatSPovtorom(iz, v string) error {
	var err error
	for popytka := 0; popytka < 5; popytka++ {
		if err = os.Rename(iz, v); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("%s не заменён: %w", filepath.Base(v), err)
}

func sha256Fayla(put string) (string, error) {
	f, err := os.Open(put)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// obespechitFaylReklamy держит И5: в конфиг и в перезапуск ядра попадает только
// файл, сошедшийся по sha256 с метой, иначе на его место ложится встроенный.
// Local-набор без годного файла роняет весь конфиг (FATAL "initialize router:
// parse rule-set"), поэтому зовётся перед каждой сборкой и каждым перезапуском.
//
// Сухая сборка тоже зовёт её, и это не нарушает контракт «сухая ничего не
// запоминает»: он о памяти службы, а файлы наборов сухая сборка и так качает.
// Подпроцесса нет: sha256 файла в 1,5 МБ это миллисекунды.
func (s *Sluzhba) obespechitFaylReklamy() error {
	s.muReklama.Lock()
	defer s.muReklama.Unlock()
	m := prochitatMetuReklamy()
	if m != nil {
		sha, err := sha256Fayla(putReklamy())
		if err == nil && sha == m.Sha256 {
			return nil
		}
		prichina := "файла нет"
		if err == nil {
			prichina = "sha256 другой"
		} else if !errors.Is(err, os.ErrNotExist) {
			prichina = err.Error()
		}
		if err := s.vylozhitVstroennyy(); err != nil {
			return err
		}
		log.Printf("список рекламы не сошёлся с метой (%s), выложен встроенный", prichina)
		s.tolknutReklamu()
		return nil
	}
	return s.vylozhitVstroennyy()
}

// vylozhitVstroennyy кладёт начальный список побайтно. Зовётся под muReklama.
func (s *Sluzhba) vylozhitVstroennyy() error {
	b, m := reklama.Vstroennyy()
	if err := os.MkdirAll(katalogNaborov(), 0o755); err != nil {
		return fmt.Errorf("каталог наборов не создан: %w", err)
	}
	// Своё имя, а не reklama.srs.chast: фон мог бы собирать список прямо сейчас.
	chast := putReklamy() + ".vstr.chast"
	if err := os.WriteFile(chast, b, 0o644); err != nil {
		return fmt.Errorf("встроенный список не записан: %w", err)
	}
	if err := pereimenovatSPovtorom(chast, putReklamy()); err != nil {
		if errUd := os.Remove(chast); errUd != nil && !errors.Is(errUd, os.ErrNotExist) {
			log.Printf("хвост встроенного списка не убран: %v", errUd)
		}
		return err
	}
	m.Affory = versiyaProgrammy
	if err := zapisatMetuReklamy(m); err != nil {
		return err
	}
	s.zapomnitMetuReklamy(m)
	return nil
}

// obnovitReklamu: скачать, проверить текст, собрать ядром, проверить набор,
// подменить файл. Отказ на любом шаге оставляет прежний reklama.srs.
func (s *Sluzhba) obnovitReklamu(ctx context.Context, u reklama.Uroven) (povtorPosle time.Time, err error) {
	if err := os.MkdirAll(katalogNaborov(), 0o755); err != nil {
		return time.Time{}, fmt.Errorf("каталог наборов не создан: %w", err)
	}
	tekst, nabor := putReklamy()+".txt.chast", putReklamy()+".chast"
	defer func() {
		for _, h := range []string{tekst, nabor} {
			if errUd := os.Remove(h); errUd != nil && !errors.Is(errUd, os.ErrNotExist) {
				log.Printf("хвост списка рекламы %s не убран: %v", filepath.Base(h), errUd)
			}
		}
	}()

	sctx, otm := context.WithTimeout(ctx, zhdatSpisok)
	telo, err := s.skachatSpisok(sctx, u.Adres())
	otm()
	if err != nil {
		return time.Time{}, fmt.Errorf("список не скачан: %w", err)
	}
	sp, err := reklama.Razobrat(telo, u)
	if err != nil {
		return time.Time{}, err
	}
	prezhniy := s.metaReklamy()
	prinyat, padenie, povtor := reklama.SverkaSPrezhnim(sp.Pravil, prezhniy, u, s.seychas())
	if !prinyat {
		s.zapomnitPadenie(padenie)
		return povtor, fmt.Errorf("в списке %d правил против %d у действующего, похоже на урезанный список; проверю снова после %s",
			sp.Pravil, prezhniy.Pravil, povtor.Local().Format("02.01 15:04"))
	}
	if err := os.WriteFile(tekst, sp.Tekst, 0o600); err != nil {
		return time.Time{}, fmt.Errorf("текст списка не записан: %w", err)
	}
	bctx, otm := context.WithTimeout(ctx, zhdatSborku)
	err = s.sobratSpisok(bctx, tekst, nabor)
	otm()
	if err != nil {
		return time.Time{}, err
	}
	sha, err := sha256Fayla(nabor)
	if err != nil {
		return time.Time{}, fmt.Errorf("собранный список не прочитан: %w", err)
	}
	seychas := s.seychas()
	m := reklama.Meta{Uroven: u, Pravil: sp.Pravil, Versiya: sp.Versiya, Sobran: sp.Sobran,
		Sha256: sha, Affory: versiyaProgrammy, Proveren: &seychas}
	if err := s.postavitFayl(nabor, m); err != nil {
		return time.Time{}, err
	}
	log.Printf("список рекламы %s поставлен: %d правил, версия %s", u, sp.Pravil, sp.Versiya)
	return time.Time{}, nil
}

// zapomnitPadenie пишет в мету отложенное подозрение, файл не трогает.
func (s *Sluzhba) zapomnitPadenie(p *reklama.Padenie) {
	s.muReklama.Lock()
	defer s.muReklama.Unlock()
	m := prochitatMetuReklamy()
	if m == nil {
		return
	}
	m.Padenie = p
	if err := zapisatMetuReklamy(*m); err != nil {
		log.Printf("подозрение на урезанный список не записано: %v", err)
	}
}

func (s *Sluzhba) postavitFayl(chast string, m reklama.Meta) error {
	s.muReklama.Lock()
	defer s.muReklama.Unlock()
	if err := pereimenovatSPovtorom(chast, putReklamy()); err != nil {
		return err
	}
	if err := zapisatMetuReklamy(m); err != nil {
		return err
	}
	s.zapomnitMetuReklamy(m)
	return nil
}

// skachatSpisokReklamy: две дороги, как у выпуска. При поднятом туннеле сначала
// через него, при отказе напрямую; половина срока каждой. GitHub рвёт
// соединения по каналу провайдера, а служба ходит мимо туннеля.
func (s *Sluzhba) skachatSpisokReklamy(ctx context.Context, adres string) ([]byte, error) {
	proksi := s.proksiCherezTunnel()
	if proksi == "" {
		return zaprositSpisok(ctx, "", adres)
	}
	do, otm := polovinaSroka(ctx)
	b, err := zaprositSpisok(do, proksi, adres)
	otm()
	if err == nil {
		return b, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	log.Printf("список рекламы через туннель не пришёл (%v), иду напрямую", err)
	return zaprositSpisok(ctx, "", adres)
}

func zaprositSpisok(ctx context.Context, proksi, adres string) ([]byte, error) {
	z, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
	if err != nil {
		return nil, err
	}
	z.Header.Set("User-Agent", "affory/"+versiyaProgrammy)
	klient := &http.Client{}
	if proksi != "" {
		u, err := url.Parse("http://" + proksi)
		if err != nil {
			return nil, fmt.Errorf("адрес локального входа не разобран: %w", err)
		}
		klient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(u)}}
	}
	o, err := klient.Do(z)
	if err != nil {
		return nil, err
	}
	defer o.Body.Close()
	if o.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("код ответа %d", o.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(o.Body, potolokNabora+1))
	if err != nil {
		return nil, err
	}
	if len(b) > potolokNabora {
		return nil, errNaborVelik
	}
	return b, nil
}

// reklamaDlyaKonfiga: nil, если блокировка выключена или сборка без неё. Отказ
// выложить файл это строка в журнал и подъём без блока, а не отказ подъёма.
func (s *Sluzhba) reklamaDlyaKonfiga(n Nabor, bezReklamy bool) *genkonfig.Reklama {
	r := n.Pravila.Reklama
	if bezReklamy || r == nil || !r.Vkl {
		return nil
	}
	if err := s.obespechitFaylReklamy(); err != nil {
		log.Printf("список рекламы не выложен, подъём без блокировки: %v", err)
		s.zapomnitOtkazReklamy(err.Error())
		return nil
	}
	return &genkonfig.Reklama{Fayl: putReklamy(), Razresheno: r.Razresheno, Svoi: s.svoiHostyReklamy(n)}
}

// svoiHostyReklamy собирает имена, к которым ходит сама служба. Из констант
// кода, а не списком здесь: список разошёлся бы с кодом молча. Хосты замера
// скорости не нужны: при поднятом туннеле замер идёт через proksi-in, а там
// службу исключает правило по process_path.
func (s *Sluzhba) svoiHostyReklamy(n Nabor) []string {
	s.mu.Lock()
	adresa := []string{s.adresOtklika, s.adresProverki, s.adresObnovleniy}
	s.mu.Unlock()
	adresa = append(adresa, reklama.VseAdresa()...)
	adresa = append(adresa, s.adresaNaborov()...)
	adresa = append(adresa, n.AdresAktivnoy())
	for _, p := range n.Podpiski {
		adresa = append(adresa, p.Adres)
	}
	imena := []string{imyaProbyImeni, imyaProbyMimoVPN}
	for _, a := range adresa {
		if u, err := url.Parse(a); err == nil {
			imena = append(imena, u.Hostname())
		}
	}
	for _, srv := range n.Servery {
		imena = append(imena, srv.Host, srv.Sni)
	}
	itog := make([]string, 0, len(imena))
	for _, d := range imena {
		d = strings.Trim(strings.ToLower(d), ".")
		if d == "" || net.ParseIP(d) != nil || !reDomen.MatchString(d) {
			continue
		}
		itog = append(itog, d)
	}
	slices.Sort(itog)
	return slices.Compact(itog)
}

// posleSmenyReklamy: смена того, что в конфиге (включение, выключение,
// исключения), сбрасывает кэш DNS Windows, иначе запомненный NXDOMAIN или
// адрес рекламы живёт до своего TTL. Включённая блокировка будит фон: смена
// уровня применяется заменой файла.
func (s *Sluzhba) posleSmenyReklamy(bylo, stalo *ReklamaPravila) {
	if otpechatokReklamy(bylo) != otpechatokReklamy(stalo) {
		if err := s.sbrositKeshDNS(); err != nil {
			log.Printf("кэш DNS Windows не сброшен: %v", err)
		}
	}
	if stalo != nil && stalo.Vkl {
		s.tolknutReklamu()
	}
}

func otpechatokReklamy(r *ReklamaPravila) string {
	b, err := json.Marshal(reklamaDlyaOtpechatka(r))
	if err != nil {
		return ""
	}
	return string(b)
}

// otlozhitFaylReklamy убирает в сторону файл, с которым ядро не приняло
// конфиг. Он лежит для разбора, фон соберёт новый.
func (s *Sluzhba) otlozhitFaylReklamy(prichina string) {
	s.muReklama.Lock()
	if err := os.Rename(putReklamy(), putReklamy()+".otvergnut"); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("отвергнутый файл списка рекламы не отложен: %v", err)
	}
	if err := os.Remove(putMetyReklamy()); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("мета отвергнутого списка рекламы не удалена: %v", err)
	}
	s.muReklama.Unlock()
	log.Printf("ядро не приняло конфиг с блокировкой рекламы, конфиг собран без неё: %s", prichina)
	s.zapomnitOtkazReklamy("ядро не приняло файл списка")
	s.tolknutReklamu()
}

// sbrositKeshDNSWindows это ipconfig /flushdns без подпроцесса.
var procDnsFlushResolverCache = windows.NewLazySystemDLL("dnsapi.dll").NewProc("DnsFlushResolverCache")

func sbrositKeshDNSWindows() error {
	if err := procDnsFlushResolverCache.Find(); err != nil {
		return err
	}
	r, _, err := procDnsFlushResolverCache.Call()
	if r == 0 {
		return fmt.Errorf("DnsFlushResolverCache: %w", err)
	}
	return nil
}
