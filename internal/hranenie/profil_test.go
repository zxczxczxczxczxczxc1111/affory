package hranenie_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
)

const teloProfilya = `{"servery":[{"id":"aaa","host":"203.0.113.10"}]}`

func eksport(t *testing.T, parol string) []byte {
	t.Helper()
	b, err := hranenie.Eksport(parol, []byte(teloProfilya))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func slepok(b []byte) []byte { return append([]byte(nil), b...) }

func TestProfilKrugovoyReys(t *testing.T) {
	nazad, err := hranenie.Import("parol", eksport(t, "parol"))
	if err != nil {
		t.Fatal(err)
	}
	if string(nazad) != teloProfilya {
		t.Fatalf("вернулось %q", nazad)
	}
}

func TestProfilNeSoderzhitOtkrytogoTeksta(t *testing.T) {
	// The whole point of the file is that it travels between machines. If the
	// servers are readable in it, the password is decoration.
	blob := eksport(t, "parol")
	if bytes.Contains(blob, []byte("203.0.113.10")) {
		t.Fatal("адрес сервера лежит в профиле открытым текстом")
	}
}

func TestDvaEksportaRazlichayutsya(t *testing.T) {
	// Equal outputs would mean a fixed salt or a fixed nonce, and a fixed nonce
	// under GCM is not a weakness, it is a full break.
	a, b := eksport(t, "parol"), eksport(t, "parol")
	if bytes.Equal(a, b) {
		t.Fatal("два экспорта совпали: соль или одноразовое число не случайны")
	}
}

func podmenitArgon(vremya, pamyat uint32) func([]byte) []byte {
	return func(b []byte) []byte {
		binary.LittleEndian.PutUint32(b[13:17], vremya)
		binary.LittleEndian.PutUint32(b[17:21], pamyat)
		return b
	}
}

func TestImportOtvergaetIsporchennoe(t *testing.T) {
	// "Wrong password is rejected loudly" is nearly tautological under AEAD. The
	// real failure modes are these.
	horoshiy := eksport(t, "parol")
	sluchai := map[string]func([]byte) []byte{
		"обрубленный хвост":  func(b []byte) []byte { return b[:len(b)-8] },
		"испорченный тег":    func(b []byte) []byte { b[len(b)-1] ^= 1; return b },
		"испорченная соль":   func(b []byte) []byte { b[24] ^= 1; return b },
		"испорченный nonce":  func(b []byte) []byte { b[hranenie.DlinaZagolovka] ^= 1; return b },
		"низкие параметры":   podmenitArgon(1, 1),
		"дикие параметры":    podmenitArgon(3, 4*1024*1024),
		"чужая магия":        func(b []byte) []byte { b[0] = 'X'; return b },
		"неизвестная версия": func(b []byte) []byte { b[12] = 9; return b },
	}
	for imya, portit := range sluchai {
		t.Run(imya, func(t *testing.T) {
			if _, err := hranenie.Import("parol", portit(slepok(horoshiy))); err == nil {
				t.Fatal("испорченный профиль принят")
			}
		})
	}
}

func TestNizkieParametryOtvergayutsyaImenno(t *testing.T) {
	// Not just «rejected»: the reason must be the floor, otherwise a downgraded
	// file would look like a corrupted one and nobody would ever learn that
	// somebody rewrote the header.
	blob := podmenitArgon(1, 1)(slepok(eksport(t, "parol")))
	_, err := hranenie.Import("parol", blob)
	if !errors.Is(err, hranenie.ErrParametrySlaby) {
		t.Fatalf("понижение параметров не опознано: %v", err)
	}
}

func TestDikieParametryOtvergayutsyaDoVydeleniyaPamyati(t *testing.T) {
	// Argon2 allocates exactly what the header says. A profile claiming 4 GiB
	// kills the service on memory BEFORE it can say the password is wrong, and
	// the file for that costs nothing to craft.
	blob := podmenitArgon(3, 4*1024*1024)(slepok(eksport(t, "parol")))
	_, err := hranenie.Import("parol", blob)
	if !errors.Is(err, hranenie.ErrParametryDiki) {
		t.Fatalf("дикие параметры не опознаны: %v", err)
	}
}

func TestNevernyyParolOtvergaetsya(t *testing.T) {
	_, err := hranenie.Import("ne-tot", eksport(t, "parol"))
	if !errors.Is(err, hranenie.ErrProfilIsporchen) {
		t.Fatalf("неверный пароль: %v", err)
	}
}

func TestPustoyParolOtvergaetsyaSObeihStoron(t *testing.T) {
	// An empty password on export produces a file that opens for anybody who
	// presses Enter, and the человек believes it is protected.
	if _, err := hranenie.Eksport("", []byte(teloProfilya)); !errors.Is(err, hranenie.ErrParolPust) {
		t.Fatalf("экспорт с пустым паролем: %v", err)
	}
	if _, err := hranenie.Import("", eksport(t, "parol")); !errors.Is(err, hranenie.ErrParolPust) {
		t.Fatalf("импорт с пустым паролем: %v", err)
	}
}

func TestChuzhayaMagiyaEtoChuzhoyFaylANeNevernyyParol(t *testing.T) {
	// Мутационный прогон 01.09.2026: удаление проверки магии НЕ ЗАМЕЧАЛОСЬ,
	// потому что подтест «чужая магия» требовал лишь «какой-нибудь ошибки», а
	// байт версии всё равно ловил подмену первого байта. Проверка магии на самом
	// деле нужна для другого: отличить чужой файл от неверного пароля. Человек,
	// открывший не тот файл, иначе пойдёт вспоминать пароль, которого тут и не
	// было.
	blob := slepok(eksport(t, "parol"))
	blob[0] = 'X'
	if _, err := hranenie.Import("parol", blob); !errors.Is(err, hranenie.ErrProfilNeNash) {
		t.Fatalf("подмена магии выдана не за чужой файл: %v", err)
	}
}

func TestChuzhoyFaylOtlichaetsyaOtIsporchennogo(t *testing.T) {
	// Picking the wrong file must not read as «your password is wrong»: the
	// человек would then go hunting a password that was never involved.
	_, err := hranenie.Import("parol", []byte(strings.Repeat("не наш файл ", 20)))
	if !errors.Is(err, hranenie.ErrProfilNeNash) {
		t.Fatalf("чужой файл принят за испорченный: %v", err)
	}
}
