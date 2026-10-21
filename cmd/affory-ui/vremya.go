package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Настройки даты и времени Windows для отказа clock-skew (1.10.0). Окно, а не
// служба: служба сидит в нулевом сеансе, и открытое ею окно настроек человек
// бы не увидел.
const ssylkaNastroekVremeni = "ms-settings:dateandtime"

func (m *most) OtkrytNastroykiVremeni() error {
	otkryt, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	ssylka, err := windows.UTF16PtrFromString(ssylkaNastroekVremeni)
	if err != nil {
		return err
	}
	if err := windows.ShellExecute(0, otkryt, ssylka, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("настройки даты и времени не открылись: %w", err)
	}
	return nil
}
