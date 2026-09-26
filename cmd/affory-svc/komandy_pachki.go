package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Пачка ключей, 26.09.2026: добавить шесть или сто ссылок одной вставкой и
// выгрузить свои серверы одной строкой или QR на другой ПК или телефон.

// addServers добавляет всё, что разобралось, ОДНОЙ правкой набора: сто
// пересборок правил брандмауэра подряд ради ста ключей человек ждал бы минуту.
//
// Ответ считает три исхода порознь. «Добавлено 0» после повторной вставки тех
// же ключей без «уже были» читалось бы как отказ.
func (s *Sluzhba) addServers(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Tekst string `json:"tekst"`
	}
	if err := json.Unmarshal(k.Telo, &telo); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	r, err := ssylki.RazobratPachku(telo.Tekst)
	if err != nil {
		// Все три отказа пачки это наши собственные фразы без куска входа,
		// поэтому их можно отдавать как есть.
		return otkaz(k.Id, k.Imya, protokol.KodSubscriptionMalformed, err.Error())
	}

	var dobavleno, obnovleno, uzheBylo int
	if len(r.Servery) > 0 {
		if err := s.pravitNabor(func(n *Nabor) error {
			dobavleno, obnovleno, uzheBylo = 0, 0, 0
			for _, srv := range r.Servery {
				bylo := make(map[string]protokol.Server, len(n.Servery))
				for _, x := range n.Servery {
					bylo[x.Id] = x
				}
				var itog protokol.Server
				n.Servery, itog = ssylki.DobavitProfil(n.Servery, srv)
				staryy, est := bylo[itog.Id]
				switch {
				case !est:
					dobavleno++
				case staryy.Imya != itog.Imya:
					obnovleno++
				default:
					uzheBylo++
				}
			}
			return nil
		}); err != nil {
			return otkaz(k.Id, k.Imya, kodSohraneniya(err), err.Error())
		}
	}
	return otvet(k.Id, k.Imya, map[string]any{
		"dobavleno": dobavleno,
		"obnovleno": obnovleno,
		"uzhe_bylo": uzheBylo,
		"otkazy":    r.Otkazy,
	})
}

// exportServers отдаёт ссылки на серверы набора. Требует администратора (см.
// komandyDlyaAdmina): ответ это все ключи машины открытым текстом.
//
// ids пустой значит «все». Удержанные записи не выгружаются: подписка про них
// уже не знает, и на другом устройстве они стали бы вечными.
func (s *Sluzhba) exportServers(k protokol.Kadr) protokol.Kadr {
	var telo struct {
		Ids []string `json:"ids"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &telo); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
		}
	}
	n, err := s.nabor()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	nuzhny := make(map[string]bool, len(telo.Ids))
	for _, id := range telo.Ids {
		nuzhny[id] = true
	}
	var otobrany []protokol.Server
	for _, srv := range n.Servery {
		if srv.Uderzhan || (len(nuzhny) > 0 && !nuzhny[srv.Id]) {
			continue
		}
		otobrany = append(otobrany, srv)
	}
	if len(otobrany) == 0 {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, errVygruzhatNechego.Error())
	}
	spisok, propushcheny := ssylki.SobratSpisok(otobrany)
	tekst := strings.Join(spisok, "\n")
	return otvet(k.Id, k.Imya, map[string]any{
		"tekst":        tekst,
		"base64":       base64.StdEncoding.EncodeToString([]byte(tekst)),
		"vsego":        len(spisok),
		"propushcheny": propushcheny,
	})
}

var errVygruzhatNechego = errors.New("выгружать нечего: серверов на этой машине нет")
