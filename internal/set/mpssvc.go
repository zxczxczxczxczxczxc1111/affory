package set

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// imyaMpsSvc это служба «Брандмауэр Защитника Windows».
const imyaMpsSvc = "MpsSvc"

// sostoyanieMpsSvc это шов: настоящий спрашивает диспетчер служб.
var sostoyanieMpsSvc = sostoyanieMpsSvcSistemnoe

// sveritSMpsSvc уточняет отказ netsh по состоянию службы брандмауэра (L16
// аудита 1.8.0). «Оптимизаторы» останавливают и отключают MpsSvc, netsh тогда
// отказывает на всём, и человек видел общий firewall-failed вместо понятной
// причины. Остановленная или отключённая служба даёт ErrBrandmauerVyklyuchen.
//
// Состояние не прочиталось: отказ остаётся прежним, гадать о причине нельзя.
func sveritSMpsSvc(err error) error {
	if err == nil || errors.Is(err, ErrBrandmauerVyklyuchen) || errors.Is(err, ErrNetTunnelya) {
		return err
	}
	rabotaet, otklyuchena, oshibka := sostoyanieMpsSvc()
	if oshibka != nil {
		return err
	}
	switch {
	case otklyuchena:
		return fmt.Errorf("%w: служба «Брандмауэр Защитника Windows» (MpsSvc) отключена: %v", ErrBrandmauerVyklyuchen, err)
	case !rabotaet:
		return fmt.Errorf("%w: служба «Брандмауэр Защитника Windows» (MpsSvc) остановлена: %v", ErrBrandmauerVyklyuchen, err)
	}
	return err
}

// sostoyanieMpsSvcSistemnoe читает состояние и тип запуска MpsSvc. Права
// только на чтение: полный доступ, который просит mgr.OpenService, служба
// брандмауэра не даёт даже администратору.
func sostoyanieMpsSvcSistemnoe() (rabotaet, otklyuchena bool, err error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, false, fmt.Errorf("диспетчер служб не открылся: %w", err)
	}
	defer windows.CloseServiceHandle(m)
	imya, err := windows.UTF16PtrFromString(imyaMpsSvc)
	if err != nil {
		return false, false, err
	}
	s, err := windows.OpenService(m, imya, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return false, false, fmt.Errorf("служба %s не открылась: %w", imyaMpsSvc, err)
	}
	defer windows.CloseServiceHandle(s)

	var st windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(s, &st); err != nil {
		return false, false, fmt.Errorf("состояние %s не прочитано: %w", imyaMpsSvc, err)
	}
	var nuzhno uint32
	err = windows.QueryServiceConfig(s, nil, 0, &nuzhno)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return false, false, fmt.Errorf("настройка %s не прочитана: %w", imyaMpsSvc, err)
	}
	buf := make([]byte, nuzhno)
	cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
	if err := windows.QueryServiceConfig(s, cfg, nuzhno, &nuzhno); err != nil {
		return false, false, fmt.Errorf("настройка %s не прочитана: %w", imyaMpsSvc, err)
	}
	return st.CurrentState == windows.SERVICE_RUNNING, cfg.StartType == windows.SERVICE_DISABLED, nil
}
