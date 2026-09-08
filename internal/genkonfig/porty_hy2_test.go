package genkonfig

import (
	"reflect"
	"testing"
)

// Одиночный порт в mport ссылки ядро отвергает, если написать его числом:
// «bad port range: 443», и падает ВЕСЬ конфиг, вместе с исправными серверами.
// Найдено 03.09.2026 прогоном инварианта 8 настоящим ядром; обычный go test
// этого не видел, потому что без ядра профили пропускаются, а тест печатает ok.
func TestPortyHy2OdinochnyyPortEtoTozheDiapazon(t *testing.T) {
	sluchai := []struct {
		mport  string
		zhdyom []string
	}{
		{"20000-21000,443", []string{"20000:21000", "443:443"}},
		{"443", []string{"443:443"}},
		{" 443 , 20000-21000 ", []string{"443:443", "20000:21000"}},
		{"20000:21000", []string{"20000:21000"}},
	}
	for _, c := range sluchai {
		if got := portyHy2(c.mport); !reflect.DeepEqual(got, c.zhdyom) {
			t.Errorf("portyHy2(%q) = %v, ждали %v", c.mport, got, c.zhdyom)
		}
	}
}
