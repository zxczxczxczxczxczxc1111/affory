package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/diagnostika"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sboi"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Стратегия загрузки подписки (A8, 22.09.2026).
//
// Было так: пять попыток подряд ОДНОЙ И ТОЙ ЖЕ дорогой, каждая до тридцати
// секунд, общего срока нет вовсе. Дорога при этом всегда прямая: конфиг ядра
// уводит affory-svc.exe в direct правилом петли, и загрузка подписки идёт мимо
// туннеля даже тогда, когда туннель поднят и несёт. У человека, которому
// провайдер режет панель, все пять попыток были заведомо холостыми, а ждал он
// их до двух с половиной минут.
//
// Стало: общий бюджет на всё обновление, два НАЗВАННЫХ пути и запасной заход
// другим путём, ограниченный по смыслу. Запасной путь пробуется только тогда,
// когда первый сорвался ДО ответа: панель, которая ответила и отказала по праву
// доступа, ответит так же и с другой дороги, а лишний заход это ещё одна
// засветка пропуска в сети.
//
// Проверка TLS не ослабляется ни на одном пути: см. ssylki.NovyyZagruzchikCherez.

const (
	putNapryamuyu = "napryamuyu"
	putCherezVpn  = "cherez-vpn"
)

// budzhetPodpiski это ОБЩИЙ срок на обновление одной подписки: оба пути, все
// повторы и паузы между ними. Var, а не const, и только ради теста: честно
// ждущий полторы минуты тест через месяц закомментируют.
var budzhetPodpiski = 90 * time.Second

// Повторов на путь. Прямому пути длинная серия: служба стартует вместе с
// Windows, то есть раньше сети, и первая загрузка почти всегда приходится на
// этот момент. Пути через туннель хватает двух: туннель либо несёт, либо нет, и
// за пять заходов он не оживёт, зато съест бюджет прямого.
const (
	povtorovNapryamuyu = 5
	povtorovCherezVpn  = 2
)

// zagruzitPodpiskuStrategiey выбирает путь и держит общий срок.
func (s *Sluzhba) zagruzitPodpiskuStrategiey(ctx context.Context, adres string) (ssylki.Razbor, error) {
	ctx, otmena := context.WithTimeout(ctx, budzhetPodpiski)
	defer otmena()

	proksi := s.proksiCherezTunnel()
	if proksi == "" {
		// Туннель опущен или локального входа нет: дорога одна, и весь бюджет
		// её. Делить срок надвое там, где второй половине некуда идти, значит
		// вдвое сократить единственную попытку.
		return s.zagruzitPutyom(ctx, adres, putNapryamuyu, "", povtorovNapryamuyu)
	}

	// Половина срока каждому. Походу через туннель нельзя съедать время
	// прямого, иначе запасной путь существует только на бумаге.
	do, otm := polovinaSroka(ctx)
	r, err := s.zagruzitPutyom(do, adres, putCherezVpn, proksi, povtorovCherezVpn)
	otm()
	if err == nil || !stoitZapasnoyPut(ctx, err) {
		return r, err
	}
	r, err = s.zagruzitPutyom(ctx, adres, putNapryamuyu, "", povtorovNapryamuyu)
	if err != nil {
		// Обёртка, а не новая ошибка: код отказа и шаг разбираются у неё
		// прежними errors.Is и errors.As. Человеку важно, что дорог пробовали
		// две, иначе «подписка недоступна» читается как одна неудачная попытка.
		return r, fmt.Errorf("%w; через VPN тоже не вышло", err)
	}
	return r, nil
}

// zagruzitPutyom это одна дорога целиком: попытки, повторы и строка в журнале.
func (s *Sluzhba) zagruzitPutyom(ctx context.Context, adres, put, proksi string, popytok int) (ssylki.Razbor, error) {
	nachalo := s.seychas()
	r, err := s.zagruzitCherez(ctx, adres, proksi, popytok)
	itog, shag := "ok", ""
	if err != nil {
		itog, shag = "otkaz", string(shagZagruzki(err))
	}
	// Строка на КАЖДЫЙ путь, а не одна на обновление. Весь смысл запасного
	// пути в том, чтобы потом было видно, который из двух работает; одна
	// строка на итог этого не отвечает.
	_ = s.zhurnalDiag.SobytieOperatsii(diagnostika.Operatsiya{
		Vid:        "zagruzka-podpiski",
		Dlitelnost: s.seychas().Sub(nachalo),
		Itog:       itog,
		Shag:       shag,
		Istochnik:  diagnostika.Obezlichit(adres),
		Put:        put,
	})
	return r, err
}

// stoitZapasnoyPut отвечает, есть ли смысл во второй дороге.
func stoitZapasnoyPut(ctx context.Context, err error) bool {
	// Бюджет кончился или службу останавливают: второй заход уже некуда
	// уложить, и начинать его значит соврать про срок.
	if ctx.Err() != nil {
		return false
	}
	// Первый путь исчерпал СВОЮ половину, а общий срок ещё есть (проверено
	// строкой выше). Ровно ради этого случая половина и отрезана: ZagruzitSPovtorami
	// отдаёт здесь голый context.DeadlineExceeded, без обёртки «подписка
	// недоступна», и без этой ветки запасной путь не запускался бы именно
	// тогда, когда он нужен - при молчащем туннеле.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Содержательный ответ панели: пустая подписка, истекшая, слишком большое
	// тело, понижение до http. Другая дорога отдаст ровно то же самое.
	if !errors.Is(err, ssylki.ErrPodpiskaNedostupna) {
		return false
	}
	switch shagZagruzki(err) {
	case sboi.Dostup, sboi.Otvet:
		// Панель ответила и отказала: дело в ссылке, а не в дороге.
		return false
	}
	return true
}

// shagZagruzki достаёт шаг у самого отказа, если он его несёт.
//
// Загрузчик уже разобрался, на чём сорвалось, и повторная классификация
// обёрнутой ошибки дала бы тот же ответ более длинным путём. Общая функция,
// потому что ответ нужен трём местам: строке операции, выбору запасного пути и
// коду отказа для человека.
func shagZagruzki(err error) sboi.Vid {
	var zagruzka ssylki.OtkazZagruzki
	if errors.As(err, &zagruzka) {
		return zagruzka.Vid
	}
	return sboi.Klassifitsirovat(err)
}
