package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// sprositParol читает пароль со стандартного ввода без эха.
//
// Пароль НЕ приходит аргументом командной строки и не приходит переменной
// окружения. Аргументы на Windows видны любому процессу через
// Win32_Process.CommandLine, а окружение процесса читается той же учёткой без
// каких-либо прав. Терминал не идеален, но он хотя бы не хранит.
//
// Если ввод НЕ консоль (запуск из скрипта через конвейер), гашение эха просто
// не применяется, и строка читается как есть. Это не запасной путь на случай
// ошибки, а ЗАЯВЛЕННЫЙ способ прогонять живые проверки: скрипт подаёт пароль
// сам, и он не проходит ни через аргументы, ни через глаза.
func sprositParol(zapros string) (string, error) {
	// Приглашение уходит в stderr: stdout у этой команды может быть перенаправлен
	// в файл, и приглашение оказалось бы внутри результата.
	fmt.Fprint(os.Stderr, zapros)

	h := windows.Handle(os.Stdin.Fd())
	var rezhim uint32
	if err := windows.GetConsoleMode(h, &rezhim); err == nil {
		// ENABLE_LINE_INPUT остаётся: без него ReadString не дождётся перевода
		// строки и человек будет смотреть на замерший терминал.
		if err := windows.SetConsoleMode(h, rezhim&^windows.ENABLE_ECHO_INPUT); err == nil {
			defer func() {
				_ = windows.SetConsoleMode(h, rezhim)
				fmt.Fprintln(os.Stderr)
			}()
		}
	}

	stroka, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("пароль не прочитан: %w", err)
	}
	// TrimSpace снимает и \r, который приходит с CRLF из скрипта. Пароль с
	// хвостовым переводом строки не совпал бы с тем же паролем, введённым
	// руками, и разбираться в этом пришлось бы по расшифровке, которая молча
	// не сходится.
	parol := strings.TrimSpace(stroka)
	if parol == "" {
		return "", errParolPust
	}
	return parol, nil
}
