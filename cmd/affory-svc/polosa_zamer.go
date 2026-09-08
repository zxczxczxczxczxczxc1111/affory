package main

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Команда measureBandwidth: полоса канала ИЗМЕРЕНИЕМ, а не полем ввода.
//
// Задача 8, часть В. Число нужно ровно затем, чтобы объявлять его hysteria2:
// объявление полосы это единственный переключатель Brutal, а Brutal шлёт РОВНО
// с объявленной скоростью. Завышение на 30% в замере на стенде стоило 10%
// полосы и 30% задержки, восемь провалов против нуля. Поле ввода перекладывает
// эту цену на человека, который своего канала не знает.
//
// Мишень задаётся ЯВНО, умолчания нет. Клиент не ходит самовольно на чужой хост
// и не тратит трафик мобильного тарифа без спроса: минута скачивания это
// десятки мегабайт.
//
// Путь через НАШ входящий прокси, если он поднят, и напрямую, если нет. Оба
// ответа осмысленны: через туннель это то, что получит человек, напрямую это
// потолок, с которым сравнивают.

const srokZameraPolosyPoUmolchaniyu = 10 * time.Second

type vhodZameraPolosy struct {
	Adres string `json:"adres"`
	// AdresVverh это ОТДЕЛЬНАЯ мишень: направления живут по разным адресам.
	// Файл на CDN отдаёт байты всякому и на POST отвечает 405, а принимающий
	// сервер редок (у speed.cloudflare.com это __down и __up). Пусто значит
	// «отдачу не мерим», и это не поломка: замер приёма всё равно состоится.
	AdresVverh string `json:"adres_vverh"`
	Potokov    int    `json:"potokov"`
	Sekund     int    `json:"sekund"`
	// MimoTunnelya меряет канал БЕЗ туннеля, даже когда он поднят. Это не
	// прихоть: без потолка измеренная через туннель полоса не с чем сравнить.
	MimoTunnelya bool `json:"mimo_tunnelya"`
}

func (s *Sluzhba) measureBandwidth(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	var v vhodZameraPolosy
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &v); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "тело команды не разбирается")
		}
	}
	if v.Potokov == 0 {
		v.Potokov = 4
	}
	srok := srokZameraPolosyPoUmolchaniyu
	if v.Sekund > 0 {
		srok = time.Duration(v.Sekund) * time.Second
	}

	proksi := ""
	if !v.MimoTunnelya {
		s.mu.Lock()
		port := s.portProksiNash
		s.mu.Unlock()
		// Порт ноль это законный случай: прокси надстройка, и туннель поднимают
		// без него, когда 10809 занят чужим клиентом. Тогда замер идёт напрямую
		// и ЧЕСТНО об этом говорит, а не выдаёт канал за туннель.
		if port > 0 {
			proksi = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}

	vhod := yadra.VhodPolosy{Adres: v.Adres, Proksi: proksi, Potokov: v.Potokov, Srok: srok}
	vniz, err := yadra.ZamerPolosy(ctx, vhod)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodPolosaNeIzmerena, err.Error())
	}

	otvetTelo := map[string]any{
		"mbit_vniz": vniz.Mbit,
		"bayt_vniz": vniz.Bayt,
		"potokov":   vniz.Potok,
		// Округление ВНИЗ и с запасом: объявленная полоса больше настоящей
		// ломает Brutal, меньше только ограничивает. Десять процентов это тот
		// же запас, что кладёт сборщик подписки.
		"sovet_vniz":    sovetPoPolose(vniz.Mbit),
		"cherez_tunnel": proksi != "",
	}

	// Отдача мерится ПОСЛЕ приёма, а не рядом с ним: два замера разом делят
	// один канал и занижают оба числа. Полторы минуты вместо минуты это цена
	// того, чтобы объявить настоящее число, а не половину настоящего.
	switch {
	case v.AdresVverh == "":
		// Молчание тут читалось бы как ноль, а ноль это Brutal с нулевой
		// оценкой канала. Поэтому причина называется вслух даже когда её
		// назвал сам человек, не заполнив поле.
		otvetTelo["otkaz_vverh"] = "мишень отдачи не задана: заливать некуда"
	default:
		vhod.Adres = v.AdresVverh
		vverh, err := yadra.ZamerOtdachi(ctx, vhod)
		if err != nil {
			// Отказ ОДНОЙ стороны не уносит другую. Обычная мишень скачивания
			// на POST отвечает 405, и потерять из-за этого измеренный приём
			// значит заставить человека мерить дважды.
			otvetTelo["otkaz_vverh"] = err.Error()
			break
		}
		otvetTelo["mbit_vverh"] = vverh.Mbit
		otvetTelo["bayt_vverh"] = vverh.Bayt
		otvetTelo["sovet_vverh"] = sovetPoPolose(vverh.Mbit)
	}
	return otvet(k.Id, k.Imya, otvetTelo)
}

// sovetPoPolose переводит замер в число, годное для объявления.
//
// Запас ВНИЗ, потому что асимметрия ошибки: заниженное объявление работает
// ограничителем и не ломает ничего, завышенное рвёт соединение. Ноль не
// советуется никогда: объявленный ноль это Brutal с нулевой оценкой канала.
func sovetPoPolose(mbit float64) int {
	sovet := int(mbit * 0.9)
	if sovet < 1 {
		sovet = 1
	}
	if sovet > PotolokPolosy {
		sovet = PotolokPolosy
	}
	return sovet
}
