package sostoyanie

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Журнал соединений (задача 6.2): `soedineniya.jsonl` в каталоге данных.
//
// Это ровно тот журнал посещений, который на сервере выключен намеренно. На
// своей машине он законен и полезен для отладки, но заводить его молча нельзя:
// выключен по умолчанию, живёт не дольше суток, стирается кнопкой. Иначе
// получилось бы, что мы вычистили error.log на сервере и завели то же самое
// на клиенте.

const (
	ImyaZhurnalaSoedineniy = "soedineniya.jsonl"
	SrokZhurnala           = 24 * time.Hour
)

// Zapis это одна строка журнала: время, процесс, домен либо адрес, порт и
// выбранный выход. Ключи здесь не бывают по построению.
type Zapis struct {
	Vremya   time.Time `json:"vremya"`
	Protsess string    `json:"protsess,omitempty"`
	Host     string    `json:"host,omitempty"`
	Adres    string    `json:"adres,omitempty"`
	Port     int       `json:"port,omitempty"`
	Vyhod    string    `json:"vyhod"`
	Pravilo  string    `json:"pravilo,omitempty"`
}

type ZhurnalSoedineniy struct {
	put string
	mu  sync.Mutex
}

func NovyyZhurnalSoedineniy() *ZhurnalSoedineniy { return ZhurnalSoedineniyV(KatalogDannyh()) }

func ZhurnalSoedineniyV(katalog string) *ZhurnalSoedineniy {
	return &ZhurnalSoedineniy{put: filepath.Join(katalog, ImyaZhurnalaSoedineniy)}
}

// Dopisat кладёт записи в конец, по строке JSON на каждую.
func (z *ZhurnalSoedineniy) Dopisat(zapisi []Zapis) error {
	if len(zapisi) == 0 {
		return nil
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	f, err := os.OpenFile(z.put, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("журнал соединений: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, zp := range zapisi {
		b, err := json.Marshal(zp)
		if err != nil {
			return err
		}
		w.Write(b)
		w.WriteByte('\n')
	}
	return w.Flush()
}

// Ochistit стирает файл. Отсутствие файла не ошибка: кнопку жмут дважды.
func (z *ZhurnalSoedineniy) Ochistit() error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if err := os.Remove(z.put); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("журнал соединений не стёрт: %w", err)
	}
	return nil
}

// Podrezat оставляет записи не старше суток по времени ЗАПИСИ. Битая строка
// (обрыв при падении службы) выбрасывается, а не валит подрезку: иначе один
// такой хвост оставил бы журнал расти вечно.
func (z *ZhurnalSoedineniy) Podrezat(seychas time.Time) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	f, err := os.Open(z.put)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("журнал соединений: %w", err)
	}
	granica := seychas.Add(-SrokZhurnala)
	var ostavit [][]byte
	sk := bufio.NewScanner(f)
	sk.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sk.Scan() {
		stroka := sk.Bytes()
		var zp struct {
			Vremya time.Time `json:"vremya"`
		}
		if json.Unmarshal(stroka, &zp) != nil || zp.Vremya.Before(granica) {
			continue
		}
		ostavit = append(ostavit, append([]byte(nil), stroka...))
	}
	_ = f.Close()
	if err := sk.Err(); err != nil {
		return fmt.Errorf("журнал соединений не прочитан: %w", err)
	}
	// Через временный файл и переименование: обрыв на середине перезаписи
	// оставил бы половину журнала вместо целого.
	vrem := z.put + ".chast"
	out, err := os.OpenFile(vrem, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("журнал соединений: %w", err)
	}
	w := bufio.NewWriter(out)
	for _, s := range ostavit {
		w.Write(s)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(vrem, z.put)
}
