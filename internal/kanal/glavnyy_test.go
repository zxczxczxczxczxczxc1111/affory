package kanal

import (
	"testing"

	"go.uber.org/goleak"
)

// Тот же механизм, что и в internal/yadra: горутина, пережившая набор, это
// брошенный пул соединений с другой стороны. Ловит прогон, а не бдительность.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Обработчик завершения ввода-вывода go-winio: заводится один раз на
		// процесс в initIO и живёт до его конца по устройству библиотеки.
		// Не наш, закрыть нечем, к пулу соединений отношения не имеет.
		goleak.IgnoreAnyFunction("github.com/Microsoft/go-winio.ioCompletionProcessor"),
	)
}
