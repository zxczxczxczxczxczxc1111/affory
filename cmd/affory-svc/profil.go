package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// vBase64 нужен и обработчику, и тесту: профиль едет в JSON, а он двоичный.
func vBase64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

type teloProfilya struct {
	Parol  string `json:"parol"`
	Profil string `json:"profil,omitempty"`
}

// trebuetAdmina это отдельная проверка поверх допуска канала.
//
// Канал пускает INTERACTIVE намеренно, чтобы интерфейс не требовал админа на
// каждый запуск. Но экспорт отдаёт ВСЕ ключи и адрес подписки, а импорт уводит
// весь трафик машины на чужой выход, и не-админ по RDP либо вторая учётка
// получали бы и то и другое.
func (s *Sluzhba) trebuetAdmina(ctx context.Context, k protokol.Kadr) *protokol.Kadr {
	// Отсутствие допуска трактуется как НЕ админ. Контекст без значения бывает
	// только там, где проверку не проводили, и толковать это в пользу
	// вызывающего значит раздавать права по недосмотру.
	if kanal.DopuskIz(ctx).Admin {
		return nil
	}
	o := otkaz(k.Id, k.Imya, protokol.KodTrebuetsyaAdmin,
		"команда доступна только администратору этой машины")
	return &o
}

func (s *Sluzhba) eksportProfilya(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	if o := s.trebuetAdmina(ctx, k); o != nil {
		return *o
	}
	var t teloProfilya
	if err := json.Unmarshal(k.Telo, &t); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}

	telo, err := s.sekretyChitat()
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, err.Error())
	}
	if len(telo) == 0 {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable,
			"выгружать нечего: серверов на этой машине нет")
	}

	blob, err := hranenie.Eksport(t.Parol, telo)
	if err != nil {
		// Текст ошибки строим САМИ и без подстановки тела: подставить сюда
		// err.Error() безопасно сегодня и перестанет быть безопасным ровно в тот
		// день, когда в ошибку попадёт что-нибудь из входа.
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, prichinaProfilya(err))
	}
	return otvet(k.Id, k.Imya, map[string]string{"profil": vBase64(blob)})
}

func (s *Sluzhba) importProfilya(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	if o := s.trebuetAdmina(ctx, k); o != nil {
		return *o
	}
	var t teloProfilya
	if err := json.Unmarshal(k.Telo, &t); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "тело команды не разбирается")
	}
	blob, err := base64.StdEncoding.DecodeString(t.Profil)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodProtocolMismatch, "профиль не декодируется")
	}

	// Сначала расшифровать, ПОТОМ писать. Обратный порядок означал бы, что
	// неверный пароль стирает собственные серверы машины: человек ошибся
	// паролем, а не согласился всё потерять.
	telo, err := hranenie.Import(t.Parol, blob)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, prichinaProfilya(err))
	}
	// Запись идёт через ту же дверь, что и остальные семь путей правки набора
	// (komandy_serverov.go): под тем же замком и с той же пересборкой правил.
	// Импорт меняет список серверов ЦЕЛИКОМ, значит правила брандмауэра обязаны
	// пересобраться. Без этого при включённом режиме «весь трафик»
	// импортированные серверы остаются заблокированы собственным kill-switch:
	// человек видит их на экране и не может подключиться ни к одному.
	oshibPeresborki := s.zamenitNaborBlobom(telo)
	if oshibPeresborki != nil && !errors.Is(oshibPeresborki, errPravilaOtstali) {
		// Записать не смогли вовсе: набор не тронут, опускать нечего.
		return otkaz(k.Id, k.Imya, protokol.KodSecretsUnreadable, oshibPeresborki.Error())
	}

	// Туннель опускается ПОСЛЕ пересборки, и порядок здесь не вкусовой. Импорт
	// заменил список целиком, значит ядро продолжает нести трафик через сервер,
	// которого в наборе больше нет. Заслон spisokNeTeryaetZhivyh сюда не годится:
	// он означал бы «нельзя импортировать профиль, пока подключён», то есть
	// запрет вместо починки.
	//
	// Обратный порядок ломает продукт: PeresobratRazresheniya выходит первой
	// строкой при не-podnyat, а пересборка на подъёме наступает ПОСЛЕ удачной
	// пробы. Опустив туннель первым, мы оставили бы запертую машину с
	// разрешающими правилами под СТАРЫЕ адреса, проба нового сервера через них
	// не прошла бы, и подключиться после импорта стало бы нельзя вовсе.
	//
	// Опускается и по ВЕТКЕ ОТКАЗА пересборки тоже, и это важнее удачного пути.
	// Секреты уже переписаны, список заменён целиком, а ранний return оставлял
	// машину с поднятым туннелем на сервере, которого в наборе больше нет, и с
	// разрешающими правилами под старые адреса. Отказ говорится ПОСЛЕ, вместе с
	// доведённым до конца делом, а не вместо него.
	//
	// Критерий тот же, что у заслона: живо ли ядро, а не что говорит состояние.
	if adres, _ := s.dostupKKlash(); adres != "" {
		s.Otklyuchit()
	}
	if oshibPeresborki != nil {
		// Отказ пересборки НЕ отменяет импорт: секреты уже записаны и верны, а
		// правила лишь отстали. Сообщить об этом надо, соврать про успех нельзя.
		return otkaz(k.Id, k.Imya, protokol.KodFirewallFailed,
			"профиль принят, туннель опущен, но правила брандмауэра отстали: "+oshibPeresborki.Error())
	}
	return otvet(k.Id, k.Imya, map[string]bool{"prinyato": true})
}

// prichinaProfilya переводит ошибку в текст, в котором заведомо нет ни пароля,
// ни содержимого профиля.
func prichinaProfilya(err error) string {
	switch {
	case errors.Is(err, hranenie.ErrParolPust):
		return "пароль пуст"
	case errors.Is(err, hranenie.ErrProfilNeNash):
		return "это не файл профиля Affory"
	case errors.Is(err, hranenie.ErrParametrySlaby):
		return "защита профиля понижена: файлу нельзя доверять"
	case errors.Is(err, hranenie.ErrParametryDiki):
		return "параметры защиты профиля неправдоподобно велики"
	case errors.Is(err, hranenie.ErrProfilIsporchen):
		return "пароль неверен либо файл повреждён"
	default:
		return fmt.Sprintf("профиль не обработан: %v", err)
	}
}
