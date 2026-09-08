package hranenie

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// Профиль это единственный способ пережить смерть машины.
//
// Блоб DPAPI привязан к машине намеренно, и это же делает его бесполезным при
// переезде: новая машина не расшифрует старый блоб никогда. Значит, нужен
// формат, который защищён паролем человека, а не ключом системы.
var (
	ErrProfilNeNash    = errors.New("это не файл профиля Affory")
	ErrProfilIsporchen = errors.New("файл профиля испорчен или пароль неверен")
	ErrParametrySlaby  = errors.New("параметры защиты профиля ниже допустимых")
	ErrParametryDiki   = errors.New("параметры защиты профиля выше допустимых")
	ErrParolPust       = errors.New("пустой пароль")
)

var magiya = [12]byte{'A', 'F', 'F', 'O', 'R', 'Y', '-', 'P', 'R', 'F', 'L', '1'}

// Параметры по умолчанию: 64 МиБ, три прохода, четыре потока.
const (
	VremyaPoUmolchaniyu  uint32 = 3
	PamyatPoUmolchaniyu  uint32 = 64 * 1024
	PotokovPoUmolchaniyu uint8  = 4
)

// Нижний порог задаётся ЧИСЛОМ и проверяется.
//
// Параметры лежат в заголовке открытым текстом, потому что без них файл не
// расшифровать. Отсюда следует, что их можно переписать, и без порога это
// прямое приглашение понизить защиту чужого профиля до одного прохода и
// килобайта памяти, а потом перебирать пароль на ноутбуке.
const (
	VremyaMinimum uint32 = 2
	PamyatMinimum uint32 = 32 * 1024
)

// Верхний потолок нужен не меньше нижнего порога, и это НЕ симметрия ради
// красоты. Argon2 честно выделит столько памяти, сколько написано в заголовке:
// файл с `pamyat = 4 ГиБ` кладёт службу по памяти ещё до того, как выяснится,
// что пароль неверный. Отказ дешевле смерти.
const (
	VremyaMaksimum uint32 = 32
	PamyatMaksimum uint32 = 1024 * 1024 // 1 ГиБ
)

// DlinaZagolovka: магия 12 + версия 1 + время 4 + память 4 + потоки 1 + соль 16.
const (
	dlinaSoli  = 16
	dlinaNonce = 12
	// Публична ради теста: он обязан уметь испортить именно nonce, а не гадать,
	// где тот начинается.
	DlinaZagolovka = 12 + 1 + 4 + 4 + 1 + dlinaSoli
)

type parametry struct {
	Vremya  uint32
	Pamyat  uint32
	Potokov uint8
}

// Eksport шифрует телo профиля паролем.
func Eksport(parol string, telo []byte) ([]byte, error) {
	return eksportS(parol, telo, parametry{VremyaPoUmolchaniyu, PamyatPoUmolchaniyu, PotokovPoUmolchaniyu})
}

func eksportS(parol string, telo []byte, p parametry) ([]byte, error) {
	if parol == "" {
		return nil, ErrParolPust
	}
	sol := make([]byte, dlinaSoli)
	if _, err := rand.Read(sol); err != nil {
		return nil, fmt.Errorf("соль не получена: %w", err)
	}
	nonce := make([]byte, dlinaNonce)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("одноразовое число не получено: %w", err)
	}

	zagolovok := sobratZagolovok(p, sol)
	aead, err := aeadPoParolyu(parol, sol, p)
	if err != nil {
		return nil, err
	}
	// Заголовок идёт связанными данными. Подмена параметров и так ломает вывод
	// ключа, но с AAD отказ приходит из одного места и с одной формулировкой,
	// а не двумя разными путями в зависимости от того, что именно переписали.
	shifr := aead.Seal(nil, nonce, telo, zagolovok)

	var out bytes.Buffer
	out.Write(zagolovok)
	out.Write(nonce)
	out.Write(shifr)
	return out.Bytes(), nil
}

// Import расшифровывает профиль.
//
// Все режимы порчи сходятся в ОДНУ ошибку намеренно: обрубленный хвост,
// перевёрнутый бит в теге и неверный пароль неразличимы для того, кто подбирает
// пароль, и различимы для нас в журнале.
func Import(parol string, blob []byte) ([]byte, error) {
	if parol == "" {
		return nil, ErrParolPust
	}
	if len(blob) < DlinaZagolovka+dlinaNonce {
		return nil, fmt.Errorf("%w: короче заголовка", ErrProfilNeNash)
	}
	if !bytes.Equal(blob[:len(magiya)], magiya[:]) {
		return nil, ErrProfilNeNash
	}

	zagolovok := blob[:DlinaZagolovka]
	p, sol, err := razobratZagolovok(zagolovok)
	if err != nil {
		return nil, err
	}
	nonce := blob[DlinaZagolovka : DlinaZagolovka+dlinaNonce]
	shifr := blob[DlinaZagolovka+dlinaNonce:]

	aead, err := aeadPoParolyu(parol, sol, p)
	if err != nil {
		return nil, err
	}
	telo, err := aead.Open(nil, nonce, shifr, zagolovok)
	if err != nil {
		return nil, ErrProfilIsporchen
	}
	return telo, nil
}

func sobratZagolovok(p parametry, sol []byte) []byte {
	z := make([]byte, 0, DlinaZagolovka)
	z = append(z, magiya[:]...)
	z = append(z, 1) // версия формата
	z = binary.LittleEndian.AppendUint32(z, p.Vremya)
	z = binary.LittleEndian.AppendUint32(z, p.Pamyat)
	z = append(z, p.Potokov)
	z = append(z, sol...)
	return z
}

func razobratZagolovok(z []byte) (parametry, []byte, error) {
	if z[len(magiya)] != 1 {
		return parametry{}, nil, fmt.Errorf("%w: версия формата %d", ErrProfilNeNash, z[len(magiya)])
	}
	p := parametry{
		Vremya:  binary.LittleEndian.Uint32(z[13:17]),
		Pamyat:  binary.LittleEndian.Uint32(z[17:21]),
		Potokov: z[21],
	}
	if p.Vremya < VremyaMinimum || p.Pamyat < PamyatMinimum || p.Potokov < 1 {
		return parametry{}, nil, fmt.Errorf("%w: время %d, память %d КиБ, потоков %d",
			ErrParametrySlaby, p.Vremya, p.Pamyat, p.Potokov)
	}
	// Проверять потолок ОБЯЗАТЕЛЬНО до вызова argon2: он выделит ровно столько,
	// сколько написано, и упадёт по памяти раньше, чем скажет про пароль.
	if p.Vremya > VremyaMaksimum || p.Pamyat > PamyatMaksimum {
		return parametry{}, nil, fmt.Errorf("%w: время %d, память %d КиБ",
			ErrParametryDiki, p.Vremya, p.Pamyat)
	}
	return p, z[22 : 22+dlinaSoli], nil
}

func aeadPoParolyu(parol string, sol []byte, p parametry) (cipher.AEAD, error) {
	// Argon2id, а не PBKDF2: пароль профиля человек придумает сам, и он будет
	// слабым. Стойкость к перебору на видеокарте здесь единственная защита.
	klyuch := argon2.IDKey([]byte(parol), sol, p.Vremya, p.Pamyat, p.Potokov, 32)
	blok, err := aes.NewCipher(klyuch)
	if err != nil {
		return nil, fmt.Errorf("шифр не создан: %w", err)
	}
	aead, err := cipher.NewGCM(blok)
	if err != nil {
		return nil, fmt.Errorf("режим не создан: %w", err)
	}
	return aead, nil
}
