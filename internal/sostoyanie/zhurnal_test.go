package sostoyanie

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Задача 6.2. Журнал соединений выключен по умолчанию и живёт не дольше
// суток: на сервере журнал посещений выключен намеренно, и завести его на
// клиенте без срока значило бы подменить решение, а не перенести его.

func zapisV(t *testing.T, vremya time.Time, host string) Zapis {
	t.Helper()
	return Zapis{Vremya: vremya, Protsess: `C:\x\a.exe`, Host: host, Adres: "192.0.2.1", Port: 443, Vyhod: "srv-nl", Pravilo: "final"}
}

func TestZhurnalDopisyvaetIPodrezaetPoSutkam(t *testing.T) {
	d := t.TempDir()
	zh := ZhurnalSoedineniyV(d)
	seychas := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	if err := zh.Dopisat([]Zapis{
		zapisV(t, seychas.Add(-25*time.Hour), "staryy.example"),
		zapisV(t, seychas.Add(-23*time.Hour), "svezhiy.example"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := zh.Dopisat([]Zapis{zapisV(t, seychas, "noviy.example")}); err != nil {
		t.Fatal(err)
	}
	if err := zh.Podrezat(seychas); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "soedineniya.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "staryy.example") {
		t.Fatal("запись старше суток пережила подрезку")
	}
	if !strings.Contains(s, "svezhiy.example") || !strings.Contains(s, "noviy.example") {
		t.Fatalf("свежие записи потеряны: %q", s)
	}
	if strings.Count(s, "\n") != 2 {
		t.Fatalf("строк %d, ожидалось 2: %q", strings.Count(s, "\n"), s)
	}
}

func TestZhurnalOchistkaSnosiFayl(t *testing.T) {
	d := t.TempDir()
	zh := ZhurnalSoedineniyV(d)
	if err := zh.Dopisat([]Zapis{zapisV(t, time.Now(), "a")}); err != nil {
		t.Fatal(err)
	}
	if err := zh.Ochistit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "soedineniya.jsonl")); !os.IsNotExist(err) {
		t.Fatal("файл остался после очистки")
	}
	// Повторная очистка и подрезка без файла не ошибка: кнопку жмут дважды.
	if err := zh.Ochistit(); err != nil {
		t.Fatal(err)
	}
	if err := zh.Podrezat(time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestZhurnalVybrasyvaetBituyuStrokuPriPodrezke(t *testing.T) {
	// Строка, которая не разбирается (обрыв записи при падении службы),
	// выбрасывается, а не валит подрезку целиком: иначе один битый хвост
	// оставил бы журнал расти вечно.
	d := t.TempDir()
	p := filepath.Join(d, "soedineniya.jsonl")
	if err := os.WriteFile(p, []byte("{\"vremya\":\"2026-09-03T11:00:00Z\",\"host\":\"ok\"}\n{обрыв"), 0o600); err != nil {
		t.Fatal(err)
	}
	zh := ZhurnalSoedineniyV(d)
	if err := zh.Podrezat(time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"host":"ok"`) || strings.Contains(string(b), "обрыв") {
		t.Fatalf("после подрезки: %q", string(b))
	}
}
