package kanal

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Срок ЗАПИСИ кадра в канал, а не ожидания ответа.
//
// Это две разные вещи, и раньше они были одним числом. Запись нескольких сотен
// байт в локальный именованный канал за пять секунд не укладывается только если
// канал мёртв. Сколько ждать ОТВЕТА, решает protokol.SrokOtveta по имени
// команды: подъём туннеля и чтение поля под мьютексом это работа разного
// порядка. Сроки ставятся на вызов, а не на соединение: долгоживущая подписка
// не должна умирать оттого, что первый статус подзадержался.
const TaymautOtveta = 5 * time.Second

// 1 MiB. A status answer is a few hundred bytes; anything near this cap is
// either a bug or somebody being creative on the other end.
const maksKadr = 1 << 20

func PisatKadr(w io.Writer, k protokol.Kadr) error {
	telo, err := json.Marshal(k)
	if err != nil {
		return fmt.Errorf("кадр не сериализуется: %w", err)
	}
	if len(telo) > maksKadr {
		return fmt.Errorf("кадр длиннее допустимого: %d", len(telo))
	}
	var dlina [4]byte
	binary.LittleEndian.PutUint32(dlina[:], uint32(len(telo)))
	if _, err := w.Write(dlina[:]); err != nil {
		return fmt.Errorf("длина не записалась: %w", err)
	}
	if _, err := w.Write(telo); err != nil {
		return fmt.Errorf("тело не записалось: %w", err)
	}
	return nil
}

func ChitatKadr(r io.Reader) (protokol.Kadr, error) {
	var k protokol.Kadr
	var dlina [4]byte
	if _, err := io.ReadFull(r, dlina[:]); err != nil {
		return k, fmt.Errorf("длина не читается: %w", err)
	}
	n := binary.LittleEndian.Uint32(dlina[:])
	if n > maksKadr {
		return k, fmt.Errorf("заявлена длина %d, предел %d", n, maksKadr)
	}
	telo := make([]byte, n)
	if _, err := io.ReadFull(r, telo); err != nil {
		return k, fmt.Errorf("тело не читается: %w", err)
	}
	if err := json.Unmarshal(telo, &k); err != nil {
		return k, fmt.Errorf("тело не разбирается: %w", err)
	}
	return k, nil
}
