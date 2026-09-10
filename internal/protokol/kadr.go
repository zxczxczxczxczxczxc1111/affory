// The wire is dumber than it looks: 4 bytes of length, then UTF-8 JSON. No gRPC,
// no protobuf, no schema registry. One machine, one reader, one writer, and a
// version number for the day the shapes stop matching.
package protokol

import (
	"encoding/json"
	"time"
)

type Kadr struct {
	Tip  string `json:"tip"`  // "cmd" | "otvet" | "sobytie"
	Id   uint64 `json:"id"`   // echoed back in otvet; 0 for sobytie
	Imya string `json:"imya"` // command or event name
	// RawMessage, NOT []byte. A []byte field marshals to base64, so the body of
	// every command would travel as a string of gibberish and every debugging
	// session would start with decoding it by hand.
	Telo  json.RawMessage `json:"telo,omitempty"`
	Oshib *Oshibka        `json:"oshibka,omitempty"`
}

type Oshibka struct {
	Kod   string `json:"kod"`   // one of kody.go, never free text
	Tekst string `json:"tekst"` // human-facing, Russian, already localized
}

// Sostoyanie is what the tray icon, the main screen and the tests all agree on.
// The service owns this value. The UI never computes it from parts, because two
// places computing the same truth is how they start disagreeing.
type Sostoyanie string

const (
	SostSluzhbaMolchit Sostoyanie = "sluzhba-molchit"
	SostVyklyuchen     Sostoyanie = "vyklyuchen"
	SostPodnimaetsya   Sostoyanie = "podnimaetsya"
	SostPodnyat        Sostoyanie = "podnyat"
	SostNeNeset        Sostoyanie = "ne-neset"
	SostVosstanavl     Sostoyanie = "vosstanavlivaetsya"
	SostOtkaz          Sostoyanie = "otkaz"
)

// Rezhim это способ выбирать сервер, а не сам сервер. Значений ровно два, и
// пустая строка не третье: она означает «человек про режим ничего не говорил»,
// живёт только в хранилище и до провода не доезжает.
type Rezhim string

const (
	RezhimAvto    Rezhim = "avto"
	RezhimRuchnoy Rezhim = "ruchnoy"
)

type StatusOtvet struct {
	TrafikPoUmolchaniyu MarshrutTrafika `json:"trafik_po_umolchaniyu,omitempty"`
	Sostoyanie          Sostoyanie      `json:"sostoyanie"`
	// Выбранный и несущий это РАЗНЫЕ вопросы, и одно поле на оба врало сразу.
	// В ручном режиме ответ совпадает, в автоматическом расходится: человек не
	// выбирал ничего, а трафик несёт кто-то конкретный. Оба с omitempty, потому
	// что «выбора нет» это законное состояние, а не пустая строка на экране.
	VybranId     string `json:"vybran_id,omitempty"`
	NesushchiyId string `json:"nesushchiy_id,omitempty"`
	// Имя несущего, чтобы трей мог сказать «несёт vpn-pc-hy2» без похода за
	// списком: смена несущего в авто-режиме случается без команды человека.
	NesushchiyImya string `json:"nesushchiy_imya,omitempty"`
	// БЕЗ omitempty, и довод ровно один: поле службы не бывает пустым по
	// построению. omitempty на непустом поле это мёртвая разметка, которая
	// однажды прикроет настоящую пустоту. Ссылаться на соседей нельзя: из
	// восьми полей структуры omitempty несут три, и каждое по своей причине.
	RezhimMarshruta Rezhim `json:"rezhim_marshruta"`
	KillSwitch      bool   `json:"kill_switch"`
	Avtozapusk      bool   `json:"avtozapusk"`
	// Два решения, а не одно (задача 4.8): «программа поднимается при входе»
	// это Avtozapusk, «туннель поднимается сам» это это поле. Умолчания
	// разные (§9.2: первое включено, второе выключено), поэтому один флаг на
	// оба обещал бы человеку подключение, которого он не выбирал.
	PodklyuchatPriStarte bool `json:"podklyuchat_pri_starte"`
	Zhurnal              bool `json:"zhurnal"`
	Diagnostika          bool `json:"diagnostika"`
	// Полоса канала человека в мегабитах, добавлено 05.09.2026. Пара нулей это
	// «не измерена», и экран обязан отличать её от объявленной: без объявления
	// hysteria2 идёт на BBR, и это рабочее состояние, а не пустое поле.
	PolosaVverh int `json:"polosa_vverh,omitempty"`
	PolosaVniz  int `json:"polosa_vniz,omitempty"`
	// Порт локального прокси рядом с туннелем (задача 4.11). Ноль, и в JSON
	// отсутствие, означает «не поднят»: занятый порт молча отключает прокси,
	// и без этого поля человек об этом не узнаёт никак.
	PortProksi   int        `json:"port_proksi,omitempty"`
	VersiyaSluzh string     `json:"versiya_sluzhby"`
	PodnyatS     *time.Time `json:"podnyat_s,omitempty"`
	Oshib        *Oshibka   `json:"oshibka,omitempty"`
	// Версия программы (сборка выпуска), а не протокола: экран показывает её
	// в строке обновления. У сборки dev пустая.
	VersiyaProgrammy string `json:"versiya_programmy,omitempty"`
	// Найденное обновление и отметка последней проверки (план «шесть
	// удобств» §5). Obnovlenie есть только когда версия на сервере новее.
	Obnovlenie          *ObnovlenieOtvet `json:"obnovlenie,omitempty"`
	ObnovlenieProvereno *time.Time       `json:"obnovlenie_provereno,omitempty"`
}

// ObnovlenieOtvet это то, что служба знает о новой версии на сервере
// обновлений: номер, размер архива и когда это узнано.
type ObnovlenieOtvet struct {
	Versiya   string    `json:"versiya"`
	Razmer    int64     `json:"razmer"`
	Provereno time.Time `json:"provereno"`
}
