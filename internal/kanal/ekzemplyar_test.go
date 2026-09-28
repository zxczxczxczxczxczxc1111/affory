package kanal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// zavestiEkzemplyar пробует завести ВТОРОЙ экземпляр канала под занятым
// именем, как это сделала бы чужая программа, чтобы принимать наших клиентов.
func zavestiEkzemplyar(imya string) error {
	u, err := windows.UTF16PtrFromString(imya)
	if err != nil {
		return err
	}
	h, err := windows.CreateNamedPipe(u, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE, windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, nil)
	if err != nil {
		return err
	}
	return windows.CloseHandle(h)
}

// Н7 аудита 1.6.1. GRGW у INTERACTIVE включал FILE_APPEND_DATA, а у канала
// это FILE_CREATE_PIPE_INSTANCE: любой вошедший пользователь заводил свой
// экземпляр под нашим именем. Проверка владельца по имени этого не видит,
// владелец у имени по-прежнему SYSTEM.
//
// Дескриптор в тесте без SY и BA: процесс теста может идти с правами
// администратора, и тогда запись BA дала бы ему всё, закрыв предмет проверки.
func TestPravaKanalaNeDayutZavestiEkzemplyar(t *testing.T) {
	for _, sluchay := range []struct {
		imya       string
		prava      string
		zavoditsya bool
	}{
		// Контроль прибора: с прежними правами экземпляр заводится, значит
		// отказ ниже говорит о правах, а не о сломанном тесте.
		{"прежние права GRGW", "GRGW", true},
		{"права канала", fmt.Sprintf("%#x", PravaKanala), false},
	} {
		t.Run(sluchay.imya, func(t *testing.T) {
			imya := fmt.Sprintf(`\\.\pipe\affory-ekz-%d-%d`, os.Getpid(), time.Now().UnixNano())
			l, err := slushatImenem(imya, "D:P(A;;"+sluchay.prava+";;;WD)")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			err = zavestiEkzemplyar(imya)
			if sluchay.zavoditsya && err != nil {
				t.Fatalf("контроль не сработал, экземпляр не заводится и с прежними правами: %v", err)
			}
			if !sluchay.zavoditsya && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
				t.Fatalf("чужой экземпляр под нашим именем: %v", err)
			}
		})
	}
}

// Дескриптор службы даёт INTERACTIVE ровно PravaKanala.
//
// Что обычный пользователь с этими правами подключается, проверяется в
// госте, а не здесь: go-winio заводит каждый слушающий экземпляр заново, и
// тест, которому не дали права на экземпляр, не смог бы принять и себя.
func TestDeskriptorSluzhbyDayotInteraktivnymPravaKanala(t *testing.T) {
	if PravaKanala&windows.FILE_APPEND_DATA != 0 {
		t.Fatal("в правах канала FILE_APPEND_DATA, то есть создание экземпляра")
	}
	for _, nuzhno := range []uint32{windows.FILE_READ_DATA, windows.FILE_WRITE_DATA, windows.SYNCHRONIZE} {
		if PravaKanala&nuzhno == 0 {
			t.Fatalf("в правах канала нет %#x: окно не прочтёт или не запишет кадр", nuzhno)
		}
	}
	if !strings.Contains(strings.ToLower(sddl), fmt.Sprintf("(a;;%#x;;;iu)", PravaKanala)) {
		t.Fatalf("дескриптор службы %q не даёт INTERACTIVE права канала", sddl)
	}
}

// Клиент сверяет процесс за каналом с процессом службы из SCM: экземпляр,
// заведённый кем-то ещё, отвечает другим pid.
func TestKlientSveryaetProtsessZaKanalom(t *testing.T) {
	imya := fmt.Sprintf(`\\.\pipe\affory-pid-%d`, os.Getpid())
	l, err := slushatImenem(imya, "D:P(A;;GA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	prinyato := make(chan struct{})
	go func() {
		defer close(prinyato)
		if c, err := l.Accept(); err == nil {
			<-time.After(2 * time.Second)
			_ = c.Close()
		}
	}()
	defer func() { <-prinyato }()
	ctx, otmena := context.WithTimeout(context.Background(), 5*time.Second)
	defer otmena()
	c, err := winio.DialPipeAccessImpLevel(ctx, imya, PravaKanala, winio.PipeImpLevelImpersonation)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	prezhniy := pidSluzhby
	t.Cleanup(func() { pidSluzhby = prezhniy })

	// Канал держит сам тестовый процесс.
	pidSluzhby = func() (uint32, error) { return uint32(os.Getpid()), nil }
	if err := sveritServerKanala(c); err != nil {
		t.Fatalf("свой процесс за каналом отвергнут: %v", err)
	}
	pidSluzhby = func() (uint32, error) { return uint32(os.Getpid()) + 4, nil }
	if err := sveritServerKanala(c); err == nil || !strings.Contains(err.Error(), "pipe-squatted") {
		t.Fatalf("чужой процесс за каналом принят: %v", err)
	}
}
