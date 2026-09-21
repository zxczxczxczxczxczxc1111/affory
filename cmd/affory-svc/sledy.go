package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/udaleniye"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Следы программы ВНЕ её двух каталогов.
//
// Снятие из окна зовёт `affory-svc.exe uninstall`, а ярлыки и запись в
// «Программах и компонентах» ставил установщик NSIS и снимал только его
// собственный Uninstall.exe. К моменту снятия из окна тот уже уезжает вместе с
// каталогом программы, и на машине оставались: битая запись деинсталляции с
// путём в никуда, папка мёртвых ярлыков в меню Пуск, иконка уведомлений в
// профиле человека и два ключа реестра, которые окно завело под себя.
//
// Жалоба 21.09.2026: «удалил, а профили и папки остались». Проверено на живой
// машине, все пять следов нашлись.
//
// Куда НЕ лезем: каталог данных (C:\ProgramData\Affory) снимается только по
// явному согласию человека, это его ключи, и правило про них не меняется.
const (
	klyuchUdaleniya    = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Affory`
	klyuchOknaVKuste   = `Software\Affory`
	klyuchIkonkiVKuste = `Software\Classes\AppUserModelId\Affory`
	// Тот же ключ, но в отдельном кусте классов: HKEY_USERS держит его вторым
	// именем, и на части машин видно только его.
	klyuchIkonkiVKlassah = `AppUserModelId\Affory`
	papkaYarlykovImya    = "Affory"
	katalogVProfile      = `AppData\Local\Affory`
	putProfiley          = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`
)

// Швы для тестов: настоящие функции ходят в реестр и меню Пуск живой машины.
var (
	papkaYarlykov        = papkaYarlykovSistemnaya
	profiliLyudey        = profiliLyudeySistemnye
	udalitReestrovyySled = udalitKlyuchSPotomkami
)

func papkaYarlykovSistemnaya() (string, error) {
	koren, err := windows.KnownFolderPath(windows.FOLDERID_CommonPrograms, 0)
	if err != nil {
		return "", fmt.Errorf("папка меню Пуск не определена: %w", err)
	}
	return filepath.Join(koren, papkaYarlykovImya), nil
}

// profiliLyudeySistemnye отдаёт каталоги профилей настоящих людей.
//
// Берётся ProfileList, а не перечисление C:\Users: там лежат и шаблоны, и
// каталоги уволенных, и «Public». Служебные учётки (S-1-5-18, -19, -20)
// отсеиваются тем же правилом, что в set.ProksiLyudey.
func profiliLyudeySistemnye() (map[string]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, putProfiley, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, fmt.Errorf("список профилей не открылся: %w", err)
	}
	defer k.Close()
	sidy, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("профили не перечислены: %w", err)
	}

	itog := make(map[string]string)
	for _, sid := range sidy {
		if !chelovecheskiySid(sid) {
			continue
		}
		p, err := registry.OpenKey(registry.LOCAL_MACHINE, putProfiley+`\`+sid, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		put, _, err := p.GetStringValue("ProfileImagePath")
		p.Close()
		if err != nil || put == "" {
			continue
		}
		itog[sid] = put
	}
	return itog, nil
}

// chelovecheskiySid повторяет правило set.sidCheloveka. Копия, а не импорт:
// пакет set про удаление ничего не знает, а тащить туда знание про снятие
// программы значит связать два несвязанных дела ради трёх строк.
func chelovecheskiySid(sid string) bool {
	if strings.HasSuffix(sid, "_Classes") {
		return false
	}
	return strings.HasPrefix(sid, "S-1-5-21-") || strings.HasPrefix(sid, "S-1-12-1-")
}

// snyatSledy убирает всё перечисленное выше и возвращает жалобы на то, что не
// вышло. Ни одна из них не повод прервать снятие: программа уже снимается, и
// оставшийся ярлык хуже удалённого, но лучше службы, которую не сняли.
func snyatSledy() []string {
	var zhaloby []string

	if put, err := papkaYarlykov(); err != nil {
		zhaloby = append(zhaloby, err.Error())
	} else if err := udaleniye.Katalog(put); err != nil {
		zhaloby = append(zhaloby, fmt.Sprintf("ярлыки %s не удалены: %v", put, err))
	}

	if err := udalitReestrovyySled(registry.LOCAL_MACHINE, klyuchUdaleniya); err != nil {
		zhaloby = append(zhaloby, fmt.Sprintf("запись в «Программах и компонентах» не снята: %v", err))
	}

	zhaloby = append(zhaloby, snyatSledyLyudey()...)
	return zhaloby
}

// snyatSledyLyudey чистит следы в профилях людей.
//
// Реестр берётся из HKEY_USERS: там загружены кусты тех, кто сейчас в системе.
// У человека, который не вошёл, куст лежит в NTUSER.DAT его профиля, и грузить
// его ради двух ключей мы не станем: чужой куст, поднятый службой, это цена
// выше следа, который он уберёт. Файлы при этом чистятся у ВСЕХ: путь профиля
// известен из ProfileList и вход человека для него не нужен.
func snyatSledyLyudey() []string {
	var zhaloby []string

	profili, err := profiliLyudey()
	if err != nil {
		return append(zhaloby, err.Error())
	}

	for sid, profil := range profili {
		katalog := filepath.Join(profil, katalogVProfile)
		if err := udaleniye.Katalog(katalog); err != nil {
			zhaloby = append(zhaloby, fmt.Sprintf("каталог %s не удалён: %v", katalog, err))
		}

		for _, put := range []string{
			sid + `\` + klyuchOknaVKuste,
			sid + `\` + klyuchIkonkiVKuste,
			sid + `_Classes\` + klyuchIkonkiVKlassah,
		} {
			if err := udalitReestrovyySled(registry.USERS, put); err != nil {
				zhaloby = append(zhaloby, fmt.Sprintf("ключ %s не снят: %v", put, err))
			}
		}
	}
	return zhaloby
}

// udalitKlyuchSPotomkami удаляет ветку целиком: registry.DeleteKey отказывается
// от ключа с подключами, а `Software\Affory` несёт внутри `Okno`.
// Отсутствие ключа это успех: удаление обязано быть идемпотентным, его зовут и
// после ручного снятия, и после установщика, который часть следов уже убрал.
func udalitKlyuchSPotomkami(kust registry.Key, put string) error {
	k, err := registry.OpenKey(kust, put, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	deti, err := k.ReadSubKeyNames(-1)
	k.Close()
	if err != nil {
		return err
	}
	for _, d := range deti {
		if err := udalitKlyuchSPotomkami(kust, put+`\`+d); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(kust, put); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
