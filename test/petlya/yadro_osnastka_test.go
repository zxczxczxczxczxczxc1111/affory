package petlya

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Запуск ядра рядом с тестом.
//
// Ни TUN, ни маршрутов, ни брандмауэра, ни службы: вход mixed на петле и выход.
// Этого хватает, чтобы спросить «несёт ли транспорт», и не хватает, чтобы
// испортить рабочую машину, что здесь важнее удобства.
type Yadro struct {
	PortProksi int
	protsess   *os.Process
	zhurnal    string
	konchilos  chan struct{}
}

// Pogasit останавливает ядро и ДОЖИДАЕТСЯ его конца.
//
// Нужен там, где второе ядро поднимается на том же кэше: не дождавшись конца
// первого, второе читало бы файл, который ещё пишут, и разбор такого прогона
// стоил бы больше, чем весь тест.
func (y *Yadro) Pogasit() {
	if y == nil || y.protsess == nil {
		return
	}
	_ = y.protsess.Kill()
	<-y.konchilos
}

const putYadraPoUmolchaniyu = `ustanovka\yadro\sing-box.exe`

// PutKYadru берёт ту же переменную, что и ворота: AFFORY_SINGBOX.
func PutKYadru(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("AFFORY_SINGBOX"); p != "" {
		return p
	}
	// Тесты живут в test/petlya, продукт двумя уровнями выше.
	p, err := filepath.Abs(filepath.Join("..", "..", putYadraPoUmolchaniyu))
	if err != nil {
		t.Fatalf("путь к ядру не собрался: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("ядра нет по пути %s, задать AFFORY_SINGBOX: %v", p, err)
	}
	return p
}

// SvobodnyyPort отдаёт порт, который СЕЙЧАС свободен.
//
// Постоянных портов здесь нет намеренно: у владельца на машине живёт свой прокси
// на 10809, и занять его значило бы отобрать у человека интернет посреди
// прогона тестов.
func SvobodnyyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("свободный порт не нашёлся: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// PodnyatYadro запускает ядро и ждёт, пока оно ПРИМЕТ соединение.
//
// По признаку, а не по часам: сон на две секунды это лотерея, которая на
// медленной машине обвиняет продукт, а на быстрой тратит время впустую. Этот
// класс ошибок стоил стенду шести ложных провалов за один день 04.09.2026.
func PodnyatYadro(t *testing.T, konfig map[string]any) *Yadro {
	t.Helper()
	y, oshibka := podnyat(t, konfig)
	if oshibka != "" {
		t.Fatalf("ядро не поднялось: %s", oshibka)
	}
	return y
}

// PodnyatYadroSOshibkoy нужна контролю: он проверяет, что негодный конфиг
// ОТВЕРГАЕТСЯ, и падение здесь ожидаемо.
func PodnyatYadroSOshibkoy(t *testing.T, konfig map[string]any) string {
	t.Helper()
	y, oshibka := podnyat(t, konfig)
	if y != nil {
		t.Fatal("ядро приняло конфиг, который обязано было отвергнуть")
	}
	return oshibka
}

func podnyat(t *testing.T, konfig map[string]any) (*Yadro, string) {
	t.Helper()
	yadro := PutKYadru(t)

	// Уровень журнала ставим сами: признак подъёма («sing-box started») ядро
	// печатает на info, и конфиг с warn выглядел бы как не поднявшееся ядро.
	konfig["log"] = map[string]any{"level": "info", "timestamp": true}

	telo, err := json.MarshalIndent(konfig, "", "  ")
	if err != nil {
		t.Fatalf("конфиг не собрался: %v", err)
	}
	rab := t.TempDir()
	putKonfiga := filepath.Join(rab, "konfig.json")
	// БЕЗ сигнатуры: ядро отвергает конфиг с BOM словами `invalid character`,
	// а выглядит это как молчащий сервер.
	if err := os.WriteFile(putKonfiga, telo, 0o600); err != nil {
		t.Fatalf("конфиг не записался: %v", err)
	}

	port := portVhoda(konfig)
	putZhurnala := filepath.Join(rab, "yadro.log")
	zhurnal, err := os.Create(putZhurnala)
	if err != nil {
		t.Fatalf("журнал не создался: %v", err)
	}
	defer zhurnal.Close()

	kom := exec.Command(yadro, "run", "-c", putKonfiga)
	kom.Dir = rab
	kom.Stdout = zhurnal
	kom.Stderr = zhurnal
	if err := kom.Start(); err != nil {
		t.Fatalf("ядро не запустилось: %v", err)
	}

	// Wait зовётся ровно один раз и в своей горутине: без него мёртвое ядро
	// приходится ждать по часам, а ожидание по часам это ложный провал на
	// медленной машине.
	konchilos := make(chan struct{})
	go func() { _, _ = kom.Process.Wait(); close(konchilos) }()

	y := &Yadro{PortProksi: port, protsess: kom.Process, zhurnal: putZhurnala, konchilos: konchilos}
	// Гашение строго по СВОЕМУ процессу. Убийство по имени погасило бы и личное
	// ядро владельца, это запрещено навсегда.
	t.Cleanup(y.Pogasit)

	// Признак подъёма это строка ядра, а не открытый TCP-порт: у hysteria2 и
	// tuic вход живёт на UDP, и TCP там не откроется никогда.
	if !zhdatStroku(putZhurnala, "sing-box started", konchilos, 15*time.Second) {
		// Пустой журнал это тоже отказ, и назвать его надо словами: пустая
		// строка ошибки читается вызывающим как «всё сошлось».
		prichina := y.Zhurnal()
		if prichina == "" {
			prichina = "ядро не сказало ни слова и не поднялось"
		}
		return nil, prichina
	}
	if port > 0 && !zhdatPort(port, 5*time.Second) {
		return nil, "порт " + fmt.Sprint(port) + " не открылся: " + y.Zhurnal()
	}
	return y, ""
}

// zhdatStroku ждёт признак в журнале, смерть процесса или срок, что раньше.
func zhdatStroku(put, priznak string, konchilos <-chan struct{}, srok time.Duration) bool {
	do := time.Now().Add(srok)
	for {
		telo, _ := os.ReadFile(put)
		if strings.Contains(string(telo), priznak) {
			return true
		}
		select {
		case <-konchilos:
			// Ядро умерло. Последний взгляд на журнал: строка могла лечь
			// между чтением и смертью.
			telo, _ := os.ReadFile(put)
			return strings.Contains(string(telo), priznak)
		default:
		}
		if time.Now().After(do) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// portVhoda ищет ТОЛЬКО вход mixed: у серверных ролей вход живёт на UDP либо
// принимает не HTTP, и ожидание TCP-соединения на нём никогда не сойдётся.
func portVhoda(konfig map[string]any) int {
	vhody, _ := konfig["inbounds"].([]any)
	for _, v := range vhody {
		m, _ := v.(map[string]any)
		if m["type"] != "mixed" {
			continue
		}
		if p, ok := m["listen_port"].(int); ok {
			return p
		}
	}
	return 0
}

func zhdatPort(port int, srok time.Duration) bool {
	do := time.Now().Add(srok)
	for time.Now().Before(do) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err == nil {
			c.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// Zhurnal отдаёт последние строки журнала ядра. Нужен там, где отказ надо
// НАЗВАТЬ, а не просто зафиксировать.
func (y *Yadro) Zhurnal() string {
	telo, err := os.ReadFile(y.zhurnal)
	if err != nil {
		return "журнал ядра не прочитан: " + err.Error()
	}
	stroki := strings.Split(strings.TrimSpace(string(telo)), "\n")
	if len(stroki) > 4 {
		stroki = stroki[len(stroki)-4:]
	}
	return strings.TrimSpace(strings.Join(stroki, " | "))
}

// SprositCherezProksi ходит в мишень ЧЕРЕЗ ядро и возвращает её имя.
func (y *Yadro) SprositCherezProksi(t *testing.T, adresMisheni string) string {
	t.Helper()
	imya, err := y.sprosit(adresMisheni, 12*time.Second)
	if err != nil {
		t.Fatalf("через ядро не прошло: %v (журнал: %s)", err, y.Zhurnal())
	}
	return imya
}

// SprositTiho отличается от SprositCherezProksi одним: молчаливым отказом.
// Нужна там, где отказ ОЖИДАЕТСЯ (испорченный ключ, мёртвый сервер).
func (y *Yadro) SprositTiho(adresMisheni string, srok time.Duration) (string, error) {
	return y.sprosit(adresMisheni, srok)
}

func (y *Yadro) sprosit(adresMisheni string, srok time.Duration) (string, error) {
	adresProksi, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", y.PortProksi))
	if err != nil {
		return "", err
	}
	klient := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(adresProksi)},
		Timeout:   srok,
	}
	otvet, err := klient.Get(adresMisheni)
	if err != nil {
		return "", err
	}
	defer otvet.Body.Close()
	// Код ответа проверяется ЗДЕСЬ. Вход mixed на сорванном исходящем отвечает
	// 502 с пустым телом, то есть без этой проверки «сервер с чужим пином» и
	// «сервер отдал пустую страницу» выглядят одинаково, и контроль подлинности
	// зеленеет при полностью сломанном транспорте (поймано 07.09.2026).
	if otvet.StatusCode != http.StatusOK {
		return "", fmt.Errorf("мишень ответила кодом %d", otvet.StatusCode)
	}
	telo, err := io.ReadAll(otvet.Body)
	if err != nil {
		return "", err
	}
	return string(telo), nil
}
