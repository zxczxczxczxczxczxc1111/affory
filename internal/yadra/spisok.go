package yadra

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/hranenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

// SobratSpisok превращает текст списка рекламы (только «||домен^») в набор
// .srs тем же ядром, что потом его читает, и проверяет результат им же:
// dolzhny обязаны совпасть, neDolzhny обязаны не совпасть (28.09.2026).
//
// convert пишет прямо в vyhod, поэтому у вызывающего vyhod всегда временный
// файл, а в рабочее имя набор попадает только переименованием. Срок задаёт
// вызывающий: зависший процесс не должен держать службу.
func SobratSpisok(ctx context.Context, imya, tekst, vyhod string, dolzhny, neDolzhny []string) error {
	put := filepath.Join(sostoyanie.KatalogProgrammy(), imya)
	if err := hranenie.Sverit(put); err != nil {
		return fmt.Errorf("список не собран, ядро %s не сошлось с отпечатком: %w", imya, err)
	}
	if v, err := podkomanda(ctx, put, "rule-set", "convert", "--type", "adguard", "--output", vyhod, tekst); err != nil {
		return fmt.Errorf("ядро не собрало список: %v: %s", err, v)
	}
	for _, d := range slices.Concat(dolzhny, neDolzhny) {
		v, err := podkomanda(ctx, put, "rule-set", "match", "--format", "binary", vyhod, d)
		if err != nil {
			return fmt.Errorf("собранный список ядро не прочло: %v: %s", err, v)
		}
		// Совпадение ядро печатает строкой «match rules.[N]: ...», код выхода 0
		// и без него (замер 28.09.2026).
		if sovpal, nado := strings.Contains(v, "match rules"), slices.Contains(dolzhny, d); sovpal != nado {
			if nado {
				return fmt.Errorf("собранный список не узнал %s", d)
			}
			return fmt.Errorf("собранный список режет %s", d)
		}
	}
	return nil
}

// predelVyvoda: convert печатает строку на каждое правило, которое не понял, и
// вывод без предела съел бы память службы на чужом списке.
const predelVyvoda = 64 << 10

func podkomanda(ctx context.Context, put string, arg ...string) (string, error) {
	cmd := exec.CommandContext(ctx, put, append([]string{"--disable-color"}, arg...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.WaitDelay = 5 * time.Second
	b := &ogranichennyy{predel: predelVyvoda}
	cmd.Stdout, cmd.Stderr = b, b
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("ядро не ответило за отведённый срок: %w", ctx.Err())
	}
	return bezTsveta(strings.TrimSpace(b.String())), err
}

// ogranichennyy копит вывод до предела и молча отбрасывает остальное: писатель,
// который отказывает, оборвал бы процесс посреди работы.
type ogranichennyy struct {
	bytes.Buffer
	predel int
}

func (o *ogranichennyy) Write(p []byte) (int, error) {
	if ostalos := o.predel - o.Len(); ostalos > 0 {
		o.Buffer.Write(p[:min(len(p), ostalos)])
	}
	return len(p), nil
}
