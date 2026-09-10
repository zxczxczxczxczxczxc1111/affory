package set

import (
	"testing"

	"go.uber.org/goleak"
)

// Тот же механизм, что и в internal/yadra: горутина, пережившая набор, это
// брошенный пул соединений с другой стороны. Ловит прогон, а не бдительность.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Резолвер Windows, а не наша утечка. TestNastoyashchiyRezolverNeRazreshaetInvalid
		// намеренно спрашивает живой резолвер про имя в .invalid, а net.Resolver
		// под Windows зовёт БЛОКИРУЮЩУЮ GetAddrInfoW в отдельной горутине, и
		// отменить её нечем: контекст возвращает управление, системный вызов
		// продолжает висеть. Исключение адресное, по кадру стека, а не по
		// «всё, что сидит в syscall»: иначе оно накрыло бы и настоящие утечки.
		goleak.IgnoreAnyFunction("net.(*Resolver).lookupIP.func1"),
	)
}
