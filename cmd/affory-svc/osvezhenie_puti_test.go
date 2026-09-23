package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Путь с версией внутри: правило после обновления программы указывало в пустоту
// и молча переставало совпадать (найдено 23.09.2026 на живых машинах).

// polozhit создаёт файл вместе с его каталогами и ставит время изменения.
func polozhit(t *testing.T, put string, kogda time.Time) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(put), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(put, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(put, kogda, kogda); err != nil {
		t.Fatal(err)
	}
	return put
}

func TestPutDiscordChinitsyaPosleObnovleniya(t *testing.T) {
	koren := t.TempDir()
	staryy := filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe")
	novyy := polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9259", "Discord.exe"), time.Now())

	got, ok := osvezhitPut(staryy)
	if !ok {
		t.Fatal("путь не починен: правило осталось бы на несуществующей сборке")
	}
	if got != novyy {
		t.Fatalf("починили на %q, ждали %q", got, novyy)
	}
}

// MSIX кладёт программу глубже: версия в имени пакета, а exe лежит в app.
// Хвост пути обязан сохраниться целиком.
func TestPutMsixChinitsyaSHvostom(t *testing.T) {
	koren := t.TempDir()
	staryy := filepath.Join(koren, "WindowsApps", "Claude_2.2553.1.0_x64__pzs", "app", "Claude.exe")
	novyy := polozhit(t, filepath.Join(koren, "WindowsApps", "Claude_2.2554.0.0_x64__pzs", "app", "Claude.exe"), time.Now())
	// Сосед с ДРУГИМ префиксом лежит рядом намеренно: WindowsApps это общий
	// каталог всех пакетов машины, и уехать на чужой нельзя.
	polozhit(t, filepath.Join(koren, "WindowsApps", "OpenAI.Codex_26.9_x64__abc", "app", "Claude.exe"), time.Now().Add(time.Hour))

	got, ok := osvezhitPut(staryy)
	if !ok {
		t.Fatal("путь MSIX не починен")
	}
	if got != novyy {
		t.Fatalf("починили на %q, ждали %q: ушли из своей семьи пакетов", got, novyy)
	}
}

func TestZhivoyPutNeTrogayut(t *testing.T) {
	koren := t.TempDir()
	zhivoy := polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe"), time.Now())
	polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9259", "Discord.exe"), time.Now().Add(time.Hour))

	if _, ok := osvezhitPut(zhivoy); ok {
		t.Fatal("живой путь переписан на соседний: человек выбирал именно этот файл")
	}
}

// Программу снесли совсем - чинить нечем, и выдумывать замену нельзя.
func TestPutBezZhivoySmenyOstayotsyaKakEst(t *testing.T) {
	koren := t.TempDir()
	if _, ok := osvezhitPut(filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe")); ok {
		t.Fatal("путь починен там, где замены нет вовсе")
	}
}

// Путь без версии в каталоге (Steam, Telegram, Spotify) не чинится никогда:
// искать там нечего, а соседний каталог это соседняя программа.
func TestPutBezVersiiNeChinitsya(t *testing.T) {
	koren := t.TempDir()
	staryy := filepath.Join(koren, "Steam", "steam.exe")
	polozhit(t, filepath.Join(koren, "SteamLibrary", "steam.exe"), time.Now())

	if got, ok := osvezhitPut(staryy); ok {
		t.Fatalf("путь без версии уехал на %q: это чужая программа", got)
	}
}

// Берётся САМАЯ СВЕЖАЯ сборка: Discord оставляет прежний каталог до перезапуска,
// и починка на него вернула бы то же мёртвое правило.
func TestBeryotsyaSamayaSvezhayaSborka(t *testing.T) {
	koren := t.TempDir()
	staryy := filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe")
	polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9257", "Discord.exe"), time.Now().Add(-48*time.Hour))
	svezhiy := polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9259", "Discord.exe"), time.Now())

	got, ok := osvezhitPut(staryy)
	if !ok || got != svezhiy {
		t.Fatalf("взяли %q (ok=%v), ждали самую свежую %q", got, ok, svezhiy)
	}
}

// Набор целиком: правило приложения и программа сервиса чинятся одинаково, и
// каждая замена называется в отчёте.
func TestNaborChinitOboiVidaPravil(t *testing.T) {
	koren := t.TempDir()
	staryyDiscord := filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe")
	novyyDiscord := polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9259", "Discord.exe"), time.Now())
	staryyClaude := filepath.Join(koren, "WindowsApps", "Claude_2.1_x64__p", "app", "Claude.exe")
	novyyClaude := polozhit(t, filepath.Join(koren, "WindowsApps", "Claude_2.2_x64__p", "app", "Claude.exe"), time.Now())
	zhivoy := polozhit(t, filepath.Join(koren, "Steam", "steam.exe"), time.Now())

	n := &Nabor{Pravila: PravilaNabora{Trafik: &protokol.PravilaTrafika{
		Prilozheniya: []protokol.PraviloPrilozheniya{
			{Put: staryyDiscord, Imya: "Discord", Marshrut: protokol.TrafikVPN},
			{Put: zhivoy, Imya: "steam", Marshrut: protokol.TrafikVPN},
		},
		Servisy: []protokol.PraviloServisa{
			{Id: "claude", Marshrut: protokol.TrafikVPN, Programmy: []string{staryyClaude}},
		},
	}}}

	otchyot := n.osvezhitPutiProgramm()
	if len(otchyot) != 2 {
		t.Fatalf("замен %d, ждали 2: %v", len(otchyot), otchyot)
	}
	if n.Pravila.Trafik.Prilozheniya[0].Put != novyyDiscord {
		t.Fatalf("правило приложения осталось на %q", n.Pravila.Trafik.Prilozheniya[0].Put)
	}
	if n.Pravila.Trafik.Prilozheniya[1].Put != zhivoy {
		t.Fatal("живое правило переписано")
	}
	if n.Pravila.Trafik.Servisy[0].Programmy[0] != novyyClaude {
		t.Fatalf("программа сервиса осталась на %q", n.Pravila.Trafik.Servisy[0].Programmy[0])
	}
}

// Здоровый набор не даёт ни одной строки в журнал: «набор починен на чтении»
// на каждой команде читалось бы как непрерывная поломка.
func TestZdorovyyNaborMolchit(t *testing.T) {
	koren := t.TempDir()
	zhivoy := polozhit(t, filepath.Join(koren, "Discord", "app-1.0.9258", "Discord.exe"), time.Now())
	n := &Nabor{Pravila: PravilaNabora{Trafik: &protokol.PravilaTrafika{
		Prilozheniya: []protokol.PraviloPrilozheniya{{Put: zhivoy, Imya: "Discord", Marshrut: protokol.TrafikVPN}},
	}}}
	if otchyot := n.osvezhitPutiProgramm(); len(otchyot) != 0 {
		t.Fatalf("здоровый набор «починен»: %v", otchyot)
	}
}
