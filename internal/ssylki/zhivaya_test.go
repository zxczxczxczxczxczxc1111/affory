package ssylki_test

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Прогон по НАСТОЯЩЕМУ телу подписки.
//
// Файл в репозиторий не кладётся никогда: там чужие ключи. Путь передаётся
// переменной AFFORY_PODPISKA_FAYL, и без неё тест пропускается. Пропуск здесь
// честен, в отличие от пропуска инварианта 8: это проверка чужими данными,
// которых у сборки может не быть, а не проверка нашего кода нашим же ядром.
//
// Смысл в том, что фикстуры пишет тот же человек, что и разбор, и потому
// содержат ровно те формы, о которых он подумал. Живая подписка про наши
// фикстуры не знает. Первый же её прогон нашёл четыре дыры.
func TestZhivayaPodpiska(t *testing.T) {
	put := os.Getenv("AFFORY_PODPISKA_FAYL")
	if put == "" {
		t.Skip("не задан AFFORY_PODPISKA_FAYL: живого тела подписки нет")
	}
	syroe, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}

	telo := string(syroe)
	if b, ok := dekodirovatLyuboe(strings.TrimSpace(telo)); ok {
		telo = string(b)
	}
	stroki := []string{}
	for _, s := range strings.Split(strings.ReplaceAll(telo, "\r\n", "\n"), "\n") {
		if s = strings.TrimSpace(s); s != "" {
			stroki = append(stroki, s)
		}
	}
	if len(stroki) == 0 {
		t.Fatal("в теле нет ни одной строки: тело не то, что мы думаем")
	}
	t.Logf("строк в подписке: %d", len(stroki))

	razobrano, chuzhih, uvedomleniy := 0, 0, 0
	for i, s := range stroki {
		srv, err := ssylki.Razobrat(s)
		switch {
		case err == nil:
			razobrano++
			// Ключи и адреса НЕ печатаются: тело чужое. Печатаем только форму.
			t.Logf("строка %d: транспорт=%s порт=%d имя=%q",
				i+1, srv.Transport, srv.Port, srv.Imya)
			if srv.Host == "" || srv.Port == 0 {
				t.Errorf("строка %d разобралась без адреса", i+1)
			}
			if srv.Id == "" {
				t.Errorf("строка %d разобралась без идентификатора", i+1)
			}
			if strings.Contains(srv.Imya, "%") {
				t.Errorf("строка %d: имя осталось в процентном кодировании: %q", i+1, srv.Imya)
			}
			if srv.Transport == "reality-tcp" && srv.PublicKey == "" {
				t.Errorf("строка %d: reality без ключа проехала как годная", i+1)
			}
		case errors.Is(err, ssylki.ErrUvedomleniePodpiski):
			// Истекшая подписка приезжает кодом 200 и исправными ссылками на
			// 0.0.0.0. Это не поломка разбора, а ответ панели, и текст его
			// печатаем целиком: он единственное, что тут полезно человеку.
			uvedomleniy++
			t.Logf("строка %d: УВЕДОМЛЕНИЕ ПАНЕЛИ: %v", i+1, err)
		case strings.Contains(err.Error(), "не поддерживается"):
			chuzhih++
			t.Logf("строка %d: не наш транспорт (%v)", i+1, err)
		default:
			t.Errorf("строка %d НЕ разобрана: %v", i+1, err)
		}
	}
	t.Logf("итого: разобрано %d, чужих транспортов %d, уведомлений %d, из %d",
		razobrano, chuzhih, uvedomleniy, len(stroki))
	// Подписка, состоящая ТОЛЬКО из уведомлений, это законный ответ истекшей
	// подписки, а не провал разбора. Требовать от неё серверов значило бы
	// объявить сломанным ровно тот случай, ради которого канал и заводился.
	if razobrano == 0 && uvedomleniy == 0 {
		t.Fatal("не разобралась ни одна строка: разбор бесполезен для чужих подписок")
	}
	if uvedomleniy == len(stroki) {
		t.Logf("подписка целиком состоит из уведомлений: она истекла")
	}
}

func dekodirovatLyuboe(s string) ([]byte, bool) {
	for _, k := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := k.DecodeString(s); err == nil && strings.Contains(string(b), "://") {
			return b, true
		}
	}
	return nil, false
}

// Тот же файл, но через РАЗБОР СПИСКА, а не построчно.
//
// Смысл отдельного прогона в том, что список делает своё: снимает base64, чинит
// переносы, считает номера строк и решает, отказ это или нет. Построчный тест
// выше ничего из этого не проверяет, потому что строки ему уже нарезали.
func TestZhivayaPodpiskaSpiskom(t *testing.T) {
	put := os.Getenv("AFFORY_PODPISKA_FAYL")
	if put == "" {
		t.Skip("не задан AFFORY_PODPISKA_FAYL: живого тела подписки нет")
	}
	syroe, err := os.ReadFile(put)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ssylki.RazobratSpisok(syroe)
	t.Logf("серверов %d, отказов %d, уведомлений %d, ошибка %v",
		len(r.Servery), len(r.Otkazy), len(r.Uvedomleniya), err)
	for _, u := range r.Uvedomleniya {
		t.Logf("строка %d: %s", u.Stroka, u.Tekst)
	}
	for _, o := range r.Otkazy {
		t.Logf("строка %d НЕ разобрана: %s", o.Stroka, o.Prichina)
	}
	switch {
	case err == nil:
		if len(r.Servery) == 0 {
			t.Fatal("успех без единого сервера: так не бывает")
		}
		// Ключи и адреса не печатаем: тело чужое.
		for _, s := range r.Servery {
			if !s.IzPodpiski {
				t.Errorf("сервер %q не помечен как пришедший из подписки", s.Imya)
			}
			if s.Id == "" || s.Host == "" || s.Port == 0 {
				t.Errorf("сервер %q без адреса или идентификатора", s.Imya)
			}
		}
	case errors.Is(err, ssylki.ErrPodpiskaIstekla):
		if len(r.Uvedomleniya) == 0 {
			t.Fatal("истекшая подписка без единого текста: показывать нечего")
		}
	default:
		t.Fatalf("живое тело отвергнуто: %v", err)
	}
}
