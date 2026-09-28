// Command obnovit освежает встроенный начальный список рекламы: исходник
// hagezi-light.txt, собранный из него vstroennyy-light.srs и паспорт
// vstroennyy.json. Работает на машине разработчика перед выпуском, отдельным
// коммитом:
//
//	go run ./internal/reklama/obnovit -yadro <sing-box выпуска> [-iz <light.txt>]
//
// Ядро то же, что едет в выпуске: набор собирается им байт-в-байт, и тест
// пакета это сверяет.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/reklama"
)

// Потолок ответа: light весит 1 МБ, в шестнадцать раз больше это уже не список.
const potolok = 16 << 20

type pasport struct {
	Uroven    reklama.Uroven `json:"uroven"`
	Pravil    int            `json:"pravil"`
	Versiya   string         `json:"versiya"`
	Sobran    *time.Time     `json:"sobran"`
	Sha256    string         `json:"sha256"`
	Istochnik string         `json:"istochnik"`
	Yadro     string         `json:"yadro"`
}

func main() {
	yadro := flag.String("yadro", "", "путь к sing-box той версии, что едет в выпуске")
	iz := flag.String("iz", "", "взять light.txt с диска, а не из сети")
	kuda := flag.String("kuda", filepath.Join("internal", "reklama"), "каталог пакета reklama")
	flag.Parse()
	if err := obnovit(*yadro, *iz, *kuda); err != nil {
		fmt.Fprintln(os.Stderr, "встроенный список не обновлён:", err)
		os.Exit(1)
	}
}

func obnovit(yadro, iz, kuda string) error {
	if yadro == "" {
		return errors.New("не задан -yadro")
	}
	telo, err := prochitat(iz)
	if err != nil {
		return err
	}
	sp, err := reklama.Razobrat(telo, reklama.Bazovyy)
	if err != nil {
		return fmt.Errorf("список не прошёл разбор: %w", err)
	}
	vremennyy, err := os.MkdirTemp("", "affory-reklama-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(vremennyy)
	tekst, srs := filepath.Join(vremennyy, "light.txt"), filepath.Join(vremennyy, "light.srs")
	if err := os.WriteFile(tekst, sp.Tekst, 0o600); err != nil {
		return err
	}
	if v, err := zapustit(yadro, "rule-set", "convert", "--type", "adguard", "--output", srs, tekst); err != nil {
		return fmt.Errorf("ядро не собрало набор: %v: %s", err, v)
	}
	// Совпадение ядро печатает строкой «match rules.[N]: ...», код выхода 0 и
	// без него (замер 28.09.2026).
	if v, err := zapustit(yadro, "rule-set", "match", "--format", "binary", srs, "an.yandex.ru"); err != nil || !strings.Contains(v, "match rules") {
		return fmt.Errorf("собранный набор не узнал an.yandex.ru: %v: %s", err, v)
	}
	versiyaYadra, err := zapustit(yadro, "version")
	if err != nil {
		return fmt.Errorf("версия ядра не прочитана: %v: %s", err, versiyaYadra)
	}
	nabor, err := os.ReadFile(srs)
	if err != nil {
		return err
	}
	h := sha256.Sum256(nabor)
	p := pasport{
		Uroven: reklama.Bazovyy, Pravil: sp.Pravil, Versiya: sp.Versiya, Sobran: sp.Sobran,
		Sha256: hex.EncodeToString(h[:]), Istochnik: reklama.Bazovyy.Adres(),
		Yadro: strings.Replace(strings.SplitN(versiyaYadra, "\n", 2)[0], " version ", " ", 1),
	}
	pasportJSON, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	// Исходник пишется как пришёл, байт-в-байт: это Corresponding Source.
	for imya, dannye := range map[string][]byte{
		"hagezi-light.txt":     telo,
		"vstroennyy-light.srs": nabor,
		"vstroennyy.json":      append(pasportJSON, '\n'),
	} {
		if err := os.WriteFile(filepath.Join(kuda, imya), dannye, 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("встроенный список обновлён: %d правил, версия %s, %d Б, %s\n", sp.Pravil, sp.Versiya, len(nabor), p.Yadro)
	return nil
}

func prochitat(iz string) ([]byte, error) {
	if iz != "" {
		return os.ReadFile(iz)
	}
	ctx, otmena := context.WithTimeout(context.Background(), 60*time.Second)
	defer otmena()
	zapros, err := http.NewRequestWithContext(ctx, http.MethodGet, reklama.Bazovyy.Adres(), nil)
	if err != nil {
		return nil, err
	}
	otvet, err := http.DefaultClient.Do(zapros)
	if err != nil {
		return nil, err
	}
	defer otvet.Body.Close()
	if otvet.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("список не скачан: %s", otvet.Status)
	}
	telo, err := io.ReadAll(io.LimitReader(otvet.Body, potolok+1))
	if err != nil {
		return nil, err
	}
	if len(telo) > potolok {
		return nil, fmt.Errorf("ответ больше %d Б, это не список", potolok)
	}
	return telo, nil
}

func zapustit(yadro string, arg ...string) (string, error) {
	var v bytes.Buffer
	cmd := exec.Command(yadro, append([]string{"--disable-color"}, arg...)...)
	cmd.Stdout, cmd.Stderr = &v, &v
	err := cmd.Run()
	return strings.TrimSpace(v.String()), err
}
