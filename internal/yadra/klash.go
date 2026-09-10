package yadra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Цель замера и срок, который мы даём ядру на её проверку.
//
// Адрес чужой и постоянный, и это осознанно: 204 без тела, отвечает быстро, и
// именно по нему меряют задержку все клиенты этого семейства. Свой сервер сюда
// не годится: мы проверяем, что трафик выходит В МИР, а не то, что мы дозвонились
// до узла, через который сами же и идём.
const (
	// Как часто спрашивать живой туннель, несёт ли он ещё. Константа пережила
	// SOCKS-пробу, ради которой была заведена, и это оказалось единственным её
	// следом: вызывающих у неё не было ни одного, то есть регулярной проверки в
	// продукте не существовало вовсе.
	PeriodProby = 30 * time.Second

	CelZamera   = "https://www.gstatic.com/generate_204"
	SrokZamera  = 5 * time.Second
	srokGotov   = 5 * time.Second
	shagGotovn  = 200 * time.Millisecond
	srokZaprosa = 10 * time.Second
)

// Zaderzhka спрашивает у clash_api задержку КОНКРЕТНОГО исходящего.
//
// Это замена SOCKS-пробе, и разница не в удобстве. Проба через локальный SOCKS
// меряла участок «мы -> второе ядро -> сервер», минуя туннель целиком, и на
// сервере, который не несёт ничего, отвечала успехом. Здесь спрашивается то же
// ядро, которое ведёт трафик, про тот же исходящий, которым он пойдёт.
func Zaderzhka(ctx context.Context, adres, sekret, teg string) (time.Duration, error) {
	// Тег приезжает из подписки, то есть его пишет чужой человек. Пробел или
	// слэш в имени без экранирования это запрос по другому адресу.
	u := fmt.Sprintf("http://%s/proxies/%s/delay?url=%s&timeout=%d",
		adres, url.PathEscape(teg), url.QueryEscape(CelZamera), SrokZamera.Milliseconds())

	telo, kod, err := sprositKlash(ctx, u, sekret)
	if err != nil {
		return 0, err
	}
	switch {
	case kod == http.StatusUnauthorized || kod == http.StatusForbidden:
		return 0, sekretNePrinyat(adres, kod)
	case kod != http.StatusOK:
		return 0, fmt.Errorf("%w: исходящий %s не отвечает (код %d)",
			ErrServerOtvergKlyuchi, teg, kod)
	}

	var o struct {
		Zaderzhka *int `json:"delay"`
	}
	if err := json.Unmarshal(telo, &o); err != nil {
		return 0, fmt.Errorf("ответ clash_api не разбирается: %w", err)
	}
	// Код 200 без числа это не успех. Пустой ответ, принятый за ноль, дал бы
	// «задержка 0 мс» вместо «непонятно», а на этом решении держится подъём.
	if o.Zaderzhka == nil {
		return 0, fmt.Errorf("в ответе clash_api нет задержки")
	}
	return time.Duration(*o.Zaderzhka) * time.Millisecond, nil
}

// ErrServerOtvergKlyuchi значит: ядро живо, ответило, а исходящий не отработал
// пробу. Так выглядит отвергнутое сервером рукопожатие: несовпавший uuid, чужой
// публичный ключ reality, неверный shortId.
//
// Отдельным признаком, а не текстом: без него отказ схлопывался в
// all-servers-down при подъёме и в switch-target-not-carrying при переключении,
// то есть человека отправляли проверять сеть при исправной сети.
//
// НЕ покрывает 401 и 403: там отвечает не наш сервер, а не принявший секрет
// clash_api или чужой клиент на нашем порту, и человеку туда идти незачем.
var ErrServerOtvergKlyuchi = errors.New("сервер не принял рукопожатие")

// sekretNePrinyat объясняет 401 и 403 от clash_api.
//
// Отвечает HTTP-сервер, но не нашим секретом. Причин ровно две, и они ведут
// человека в РАЗНЫЕ стороны: либо секрет собрали не тот (наш дефект), либо порт,
// выданный системой как свободный, успел занять чужой клиент. На этой машине
// жили nekoray, Throne, Amnezia и Sota, так что второе не теория. Спрашиваем
// владельца порта и называем его.
//
// Общая, а не по копии в каждом вызывающем: копия разошлась бы с оригиналом
// молча, и один из путей перестал бы называть виновника.
func sekretNePrinyat(adres string, kod int) error {
	if err := chuzhoyNaPortu(adres); err != nil {
		return err
	}
	return fmt.Errorf("clash_api не принял секрет (код %d)", kod)
}

// ErrTegaNetVYadre означает, что тега нет в конфиге ЖИВОГО ядра. Единственная
// причина отказа переключения, которую лечит переподключение: конфиг ядра
// собран при подъёме, а сервер мог быть добавлен уже после.
var ErrTegaNetVYadre = errors.New("тега нет в конфиге живого ядра")

// PostavitVybor переключает группу на другой исходящий в ПАМЯТИ живого ядра.
//
// Успех это 204, и только он. Двести означает, что отвечает чужой клиент на
// нашем порту, и принять его за успех значит объявить переключение, которого не
// было.
func PostavitVybor(ctx context.Context, adres, sekret, gruppa, teg string) error {
	// Через json.Marshal, а не форматированием строки: тег приезжает из
	// подписки, то есть его пишет чужой человек, и кавычка в имени означала бы
	// сломанный JSON.
	telo, err := json.Marshal(struct {
		Imya string `json:"name"`
	}{teg})
	if err != nil {
		return fmt.Errorf("тело запроса не собрано: %w", err)
	}
	u := fmt.Sprintf("http://%s/proxies/%s", adres, url.PathEscape(gruppa))
	otvet, kod, err := zapros(ctx, http.MethodPut, u, sekret, bytes.NewReader(telo))
	if err != nil {
		return err
	}
	switch {
	case kod == http.StatusNoContent:
		return nil
	case kod == http.StatusUnauthorized || kod == http.StatusForbidden:
		return sekretNePrinyat(adres, kod)
	}
	// Замерено: 400 приходит на три разные причины, по коду они неразличимы, по
	// телу различимы, и действия у человека разные.
	//
	// Предел на текст не украшение: тело приходит из сокета, и складывать его
	// целиком в ошибку, которая уедет человеку на экран, значит однажды показать
	// ему чужой HTML-ответ на всю страницу.
	hvost := string(otvet)
	if len(hvost) > 200 {
		hvost = hvost[:200]
	}
	if kod == http.StatusBadRequest && strings.Contains(hvost, "not found") {
		return fmt.Errorf("%w: %s", ErrTegaNetVYadre, teg)
	}
	return fmt.Errorf("ядро не переключило группу %s на %s (код %d): %s", gruppa, teg, kod, hvost)
}

// ErrNeVybrala отделяет «группа ещё не выбрала» от «ответ не тот».
//
// Разница не косметическая. У селектора выбор есть всегда, хотя бы умолчанием
// из конфига, и пустой now там это поломка. У urltest пустой now это ФАЗА:
// группа не закончила первую пробу задержки. Замерено 06.09.2026 на живом
// ядре 1.14.0-rc.5: через 1.23 с после подъёма now пуст, через 5.63 с назван.
//
// Пока разницы не было, служба спрашивала ядро один раз при подъёме, глотала
// отказ строкой в журнал и оставляла поле несущего пустым до следующего опроса
// наблюдателя, то есть на тридцать секунд.
var ErrNeVybrala = errors.New("группа ещё не назвала выбор")

// VyborGruppy отвечает, что группа выбрала СЕЙЧАС, одним этажом.
//
// Экспортирована не про запас: живому переключению она нужна именно
// одноэтажной. Откат неудавшегося переключения обязан вернуть прежний ВЫБОР
// верхней группы, а он в авто равен строке «avto», то есть ровно то значение,
// которое Nesyot отбрасывает как имя группы.
func VyborGruppy(ctx context.Context, adres, sekret, gruppa string) (string, error) {
	// Имя группы наше собственное, но экранируется по той же причине, что и тег
	// в Zaderzhka: незаэкранированное имя это запрос по другому адресу.
	u := fmt.Sprintf("http://%s/proxies/%s", adres, url.PathEscape(gruppa))
	telo, kod, err := sprositKlash(ctx, u, sekret)
	if err != nil {
		return "", err
	}
	if kod != http.StatusOK {
		return "", fmt.Errorf("группа %s не отвечает (код %d)", gruppa, kod)
	}
	var o struct {
		Seychas string `json:"now"`
	}
	if err := json.Unmarshal(telo, &o); err != nil {
		return "", fmt.Errorf("ответ clash_api не разбирается: %w", err)
	}
	// Пустой now при коде 200 значит РАЗНОЕ у разных групп, поэтому отдаётся
	// отличимой ошибкой, а вызывающий решает сам: у селектора это поломка, у
	// urltest фаза до первой пробы (см. ErrNeVybrala).
	if o.Seychas == "" {
		return "", fmt.Errorf("%w: %s", ErrNeVybrala, gruppa)
	}
	return o.Seychas, nil
}

// Nesyot отвечает, через какой ИМЕННО сервер ядро несёт трафик сейчас.
//
// Два этажа, и это не перестраховка. Замерено на живом ядре: в авто «now»
// верхней группы равно имени вложенной группы, а не тегу сервера. Читать один
// этаж значит получить строку «avto» ровно в том режиме, ради которого функция
// написана.
//
// Имена групп параметрами: пакет не знает про генератор конфига и знать не
// должен.
func Nesyot(ctx context.Context, adres, sekret, gruppa, tegAvto string) (string, error) {
	teg, err := VyborGruppy(ctx, adres, sekret, gruppa)
	if err != nil {
		return "", err
	}
	if teg != tegAvto {
		return teg, nil
	}
	teg, err = VyborGruppy(ctx, adres, sekret, tegAvto)
	if err != nil {
		return "", err
	}
	// Спускаться дальше некуда: групп у нас ровно две. Третий этаж означает
	// чужой конфиг или наш дефект, и в обоих случаях честный ответ это отказ,
	// а не ещё один круг.
	if teg == tegAvto || teg == gruppa {
		return "", fmt.Errorf("группы ядра ссылаются по кругу: %s -> %s", gruppa, teg)
	}
	return teg, nil
}

// ZhdatKlash ждёт, пока clash_api начнёт отвечать после старта ядра.
//
// Ядро поднимается не мгновенно. Спрашивать его сразу после запуска значит
// измерить пустоту и объявить отказ: ровно это уже случилось со сверкой
// владельца порта, где проверка «сразу после старта» не проходила никогда.
func ZhdatKlash(ctx context.Context, adres, sekret string) error {
	return zhdatKlashS(ctx, adres, sekret, srokGotov, shagGotovn)
}

func zhdatKlashS(ctx context.Context, adres, sekret string, srok, shag time.Duration) error {
	do, otm := context.WithTimeout(ctx, srok)
	defer otm()
	u := fmt.Sprintf("http://%s/version", adres)
	var posledn error
	for {
		_, kod, err := sprositKlash(do, u, sekret)
		switch {
		case err != nil:
			posledn = err
		case kod == http.StatusOK:
			return nil
		default:
			posledn = fmt.Errorf("clash_api ответил кодом %d", kod)
		}
		select {
		case <-do.Done():
			return fmt.Errorf("clash_api не отвечает за %v: %w", srok, posledn)
		case <-time.After(shag):
		}
	}
}

// sprositKlash это GET, тонкая обёртка над zapros. Своё имя у неё осталось
// потому, что вызывающих и тестов у неё уже много, и переписывать их ради
// появления второго метода незачем.
func sprositKlash(ctx context.Context, u, sekret string) ([]byte, int, error) {
	return zapros(ctx, http.MethodGet, u, sekret, nil)
}

func zapros(ctx context.Context, metod, u, sekret string, telo io.Reader) ([]byte, int, error) {
	return zaprosS(ctx, metod, u, sekret, telo, predelTela)
}

// Потолок на тело по умолчанию. /connections перечисляет ВСЕ живые соединения
// и в него не умещается, поэтому предел параметром, а не константой в теле.
const predelTela = 64 * 1024

// klientKlash это ОДИН клиент на пакет, а не клиент на запрос.
//
// Свой, а не http.DefaultClient: у общего клиента чужие настройки, и однажды
// кто-то поменяет их не думая про нас. Прокси не берём из окружения намеренно:
// адрес петлевой.
//
// Почему именно один. Транспорт, собранный на каждый вызов, уносит соединение
// в СВОЙ пул простоя и хоронит его там: у нулевого транспорта IdleConnTimeout
// это «никогда», а сам он не собирается сборщиком мусора, потому что на него
// ссылается живая горутина readLoop. Замер владельца 10.09.2026: 46 156
// дескрипторов у службы, 3155 соединений с локальным ядром, +2.93 дескриптора
// в секунду при 2.9 запроса в секунду. Совпадение до второго знака и есть
// доказательство, что источник тут.
//
// Числа маленькие намеренно: хост один и петлевой, больше горстки соединений к
// нему не нужно никогда. IdleConnTimeout не защита, а второй рубеж: он
// превращает будущую ошибку того же класса из тихой утечки в закрытое
// соединение. MaxConnsPerHost сознательно НЕ ставится: потолок на
// одновременные соединения превратил бы такую ошибку в зависший запрос, а
// зависший запрос службы хуже лишнего сокета.
var klientKlash = &http.Client{
	Transport: &http.Transport{
		Proxy:               nil,
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     30 * time.Second,
	},
}

func zaprosS(ctx context.Context, metod, u, sekret string, telo io.Reader, predel int64) ([]byte, int, error) {
	do, otm := context.WithTimeout(ctx, srokZaprosa)
	defer otm()
	z, err := http.NewRequestWithContext(do, metod, u, telo)
	if err != nil {
		return nil, 0, fmt.Errorf("запрос к clash_api не собран: %w", err)
	}
	z.Header.Set("Authorization", "Bearer "+sekret)

	o, err := klientKlash.Do(z)
	if err != nil {
		return nil, 0, fmt.Errorf("clash_api не отвечает: %w", err)
	}
	defer o.Body.Close()
	// Потолок на тело: отвечает наше же ядро, но читать без предела из сокета
	// это привычка, которая однажды встретит не наше ядро.
	otvet, err := io.ReadAll(io.LimitReader(o.Body, predel))
	if err != nil {
		return nil, o.StatusCode, fmt.Errorf("ответ clash_api не дочитан: %w", err)
	}
	return otvet, o.StatusCode, nil
}

// ImyaYadra это имя файла нашего ядра. Переменная, а не константа, ровно ради
// теста: подставного sing-box.exe у теста нет, а слушателя он поднимает своим
// же процессом.
var ImyaYadra = "sing-box.exe"

// chuzhoyNaPortu отвечает ошибкой ТОЛЬКО когда владелец порта заведомо чужой.
// Молчание здесь означает «виновник не установлен», а не «всё в порядке»:
// таблица TCP может не прочитаться, и превращать это в обвинение нельзя.
func chuzhoyNaPortu(adres string) error {
	_, port, err := net.SplitHostPort(adres)
	if err != nil {
		return nil
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return nil
	}
	imya, err := VladelecPorta(n)
	if err != nil || imya == "" {
		return nil
	}
	if strings.EqualFold(filepath.Base(imya), ImyaYadra) {
		return nil
	}
	return fmt.Errorf("%s: порт %d занят процессом %s, а не ядром %s",
		protokol.KodForeignProxyHijack, n, imya, ImyaYadra)
}
