package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// Номер версии в «Программах и компонентах».
//
// Запись деинсталляции заводит установщик NSIS, и DisplayVersion он берёт из
// своего /DVERSIYA. Обновление из окна идёт мимо установщика: служба качает
// архив, подменщик кладёт файлы и зовёт `install` у НОВОГО бинаря. Реестр при
// этом не трогал никто, и на живой машине после обновления 1.4.2 на 1.5.0
// система продолжала считать установленной 1.4.2 (найдено 26.09.2026).
//
// Поэтому номер пишет install: через неё проходят оба пути. При откате
// подменщик зовёт ту же install у прежнего бинаря, и номер возвращается вместе
// с файлами.
//
// Ключ НЕ создаётся. Программа, поставленная без установщика (стенд, сборка
// разработчика), в «Программах и компонентах» не числится, и завести ей запись
// значит завести запись без деинсталлятора. На первой установке ключа ещё нет,
// и это тоже не отказ: NSIS пишет его сразу после install.
var klyuchUdaleniyaDlyaZapisi = func() (registry.Key, error) {
	// Вид 64 явно: установщик пишет с SetRegView 64, и сборка под другую
	// разрядность без этого флага правила бы не тот ключ.
	return registry.OpenKey(registry.LOCAL_MACHINE, klyuchUdaleniya, registry.SET_VALUE|registry.WOW64_64KEY)
}

// zapisatVersiyuVReestr отдаёт ошибку только на настоящий отказ. Отсутствие
// ключа и сборка без номера (dev) отказом не считаются.
func zapisatVersiyuVReestr(versiya string) error {
	if _, ok := razobratVersiyu(versiya); !ok {
		return nil
	}
	k, err := klyuchUdaleniyaDlyaZapisi()
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("запись в «Программах и компонентах» не открылась: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("DisplayVersion", versiya); err != nil {
		return fmt.Errorf("номер версии в «Программах и компонентах» не записан: %w", err)
	}
	return nil
}
