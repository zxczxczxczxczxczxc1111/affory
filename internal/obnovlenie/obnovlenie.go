// Пакет obnovlenie это подмена файлов программы с откатом (задача 6.5).
//
// Хозяин обновления служба, а не интерфейс: каталог программы пишется только
// админом, а служба под LocalSystem пишет туда без UAC. Файлы выпуска с 0.7.0
// подписаны самоподписанным сертификатом, но эта подпись здесь НЕ проверяется
// и проверять её нечем: доверие к самоподписанному корню есть только на своих
// машинах. Проверка тут одна, sha256 из соседнего файла, и она про целостность
// доставки, а не про подлинность источника.
package obnovlenie

import (
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

const (
	// Коды исхода из словаря §9.1. Дубликат строкой здесь ослеплял ворота:
	// код слался, а по словарю числился «не шлёт никто».
	// Second constant: rollback after which the old service is ALSO silent.
	// Different news, and worse.
	KodOtkat          = protokol.KodUpdateRollback
	KodOtkatNeUdalsya = protokol.KodOtkatNeUdalsya

	KatalogPredydushchey = "predydushchaya"
	KatalogNovoy         = "novaya"
	imyaItoga            = "obnovlenie.json"
	SrokPodyoma          = 20 * time.Second
	predelArhiva         = 256 << 20
)

// Sverit сравнивает sha256 архива с первым словом файла <архив>.sha256.
// Формат sha256sum: хеш, два пробела, имя.
func Sverit(putArhiva string) error {
	b, err := os.ReadFile(putArhiva + ".sha256")
	if err != nil {
		return fmt.Errorf("рядом с архивом нет .sha256, сверять не с чем: %w", err)
	}
	// Ровно та же проверка, что в proverka_obnovleniy.go:174-181, и она там уже
	// стояла: путь через downloadUpdate файл суммы читает и проверяет, а путь
	// через installUpdate брал первое поле не глядя. Пустой файл получается от
	// оборванной записи, без всякого злого умысла, и стоил бы службы вместе с
	// туннелем.
	//
	// Длина и шестнадцатеричность проверяются ОТДЕЛЬНО от совпадения: иначе
	// битый файл суммы отвечает «хеш не совпал», и человек идёт перекачивать
	// исправный архив.
	polya := strings.Fields(string(b))
	if len(polya) == 0 {
		return fmt.Errorf("файл .sha256 пуст: контрольной суммы в нём нет")
	}
	ozhid := strings.ToLower(polya[0])
	if len(ozhid) != 64 {
		return fmt.Errorf("в .sha256 не контрольная сумма: %d знаков вместо 64", len(ozhid))
	}
	if _, err := hex.DecodeString(ozhid); err != nil {
		return fmt.Errorf("контрольная сумма в .sha256 не шестнадцатеричная")
	}
	f, err := os.Open(putArhiva)
	if err != nil {
		return fmt.Errorf("архив не открыт: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, predelArhiva)); err != nil {
		return fmt.Errorf("архив не прочитан: %w", err)
	}
	if fakt := hex.EncodeToString(h.Sum(nil)); fakt != ozhid {
		return fmt.Errorf("sha256 архива не совпал с .sha256")
	}
	return nil
}

// Raspakovat кладёт файлы верхнего уровня архива в kuda. Каталоги и пути с
// разделителями отвергаются целиком: запись «..\x.exe» это zip slip, и
// молча пропустить её значило бы принять архив, которому нельзя верить.
func Raspakovat(putArhiva, kuda string) error {
	r, err := zip.OpenReader(putArhiva)
	if err != nil {
		return fmt.Errorf("архив не читается: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.ContainsAny(f.Name, `/\`) || f.Name == "" || f.Name == "." || f.Name == ".." {
			return fmt.Errorf("в архиве запись с путём %q: такой архив не принимается", f.Name)
		}
	}
	if err := os.RemoveAll(kuda); err != nil {
		return err
	}
	if err := os.MkdirAll(kuda, 0o700); err != nil {
		return err
	}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := izvlech(f, filepath.Join(kuda, f.Name)); err != nil {
			return err
		}
	}
	return nil
}

func izvlech(f *zip.File, kuda string) error {
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(kuda, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	defer dst.Close()
	_, err = io.Copy(dst, io.LimitReader(src, predelArhiva))
	return err
}

// Podmena это шаги, которые нельзя делать из живой службы: она сама один из
// подменяемых файлов. Выполняет их подменщик, отдельный процесс из копии
// нового affory-svc.exe во временном каталоге.
type Podmena struct {
	KatalogProgrammy string
	Novaya           string
	// Остановить службу, поставить (install: заново регистрирует, снимает
	// отпечатки, запускает), дождаться ответа по каналу.
	Ostanovit   func() error
	Ustanovit   func() error
	ZhdatOtveta func(srok time.Duration) error
	Srok        time.Duration
}

type Itog struct {
	Ok     bool      `json:"ok"`
	Kod    string    `json:"kod,omitempty"`
	Tekst  string    `json:"tekst,omitempty"`
	Vremya time.Time `json:"vremya"`
}

// Vypolnit подменяет файлы и откатывает, если новая служба не ответила в срок.
// Прежняя версия остаётся в predydushchaya и после успеха: следующий откат
// руками стоит одного копирования.
func (p Podmena) Vypolnit() Itog {
	pred := filepath.Join(p.KatalogProgrammy, KatalogPredydushchey)
	novye, err := imenaFaylov(p.Novaya)
	if err != nil || len(novye) == 0 {
		return Itog{Kod: KodOtkat, Tekst: fmt.Sprintf("новая сборка пуста или не читается: %v", err), Vremya: time.Now()}
	}
	if err := p.Ostanovit(); err != nil {
		return Itog{Kod: KodOtkat, Tekst: "служба не остановлена: " + err.Error(), Vremya: time.Now()}
	}
	// Копируется только то, что будет подменено: ядро и базы едут отдельно.
	if err := skopirovat(p.KatalogProgrammy, pred, novye, true); err != nil {
		return Itog{Kod: KodOtkat, Tekst: "прежняя версия не сохранена: " + err.Error(), Vremya: time.Now()}
	}
	if err := skopirovat(p.Novaya, p.KatalogProgrammy, novye, false); err != nil {
		return p.otkatit(pred, novye, "новые файлы не легли: "+err.Error())
	}
	if err := p.Ustanovit(); err != nil {
		return p.otkatit(pred, novye, "новая служба не установилась: "+err.Error())
	}
	if err := p.ZhdatOtveta(p.Srok); err != nil {
		return p.otkatit(pred, novye, "новая служба не ответила за "+p.Srok.String()+": "+err.Error())
	}
	return Itog{Ok: true, Vremya: time.Now()}
}

func (p Podmena) otkatit(pred string, imena []string, prichina string) Itog {
	_ = p.Ostanovit()
	if err := skopirovat(pred, p.KatalogProgrammy, imena, false); err != nil {
		return Itog{Kod: KodOtkatNeUdalsya, Tekst: prichina + "; откат не удался: " + err.Error(), Vremya: time.Now()}
	}
	if err := p.Ustanovit(); err != nil {
		return Itog{Kod: KodOtkatNeUdalsya, Tekst: prichina + "; прежняя служба не установилась: " + err.Error(), Vremya: time.Now()}
	}
	if err := p.ZhdatOtveta(p.Srok); err != nil {
		return Itog{Kod: KodOtkatNeUdalsya, Tekst: prichina + "; прежняя служба тоже молчит: " + err.Error(), Vremya: time.Now()}
	}
	return Itog{Kod: KodOtkat, Tekst: prichina, Vremya: time.Now()}
}

func imenaFaylov(dir string) ([]string, error) {
	z, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var imena []string
	for _, e := range z {
		if !e.IsDir() {
			imena = append(imena, e.Name())
		}
	}
	return imena, nil
}

// skopirovat переносит названные файлы из otkuda в kuda. propuskatNet: файла
// может не быть в источнике (новая сборка добавила файл, прежней версии его
// нет), и это не ошибка сохранения.
func skopirovat(otkuda, kuda string, imena []string, propuskatNet bool) error {
	if err := os.MkdirAll(kuda, 0o700); err != nil {
		return err
	}
	for _, imya := range imena {
		src := filepath.Join(otkuda, imya)
		b, err := os.ReadFile(src)
		if err != nil {
			if propuskatNet && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		// Через временный файл: оборванная запись exe это служба, которая не
		// запустится ни новой, ни старой.
		vrem := filepath.Join(kuda, imya+".chast")
		if err := os.WriteFile(vrem, b, 0o700); err != nil {
			return err
		}
		dst := filepath.Join(kuda, imya)
		// Работающий exe (окно, которое и прислало installUpdate) поверх себя
		// переименовать не даст, а отодвинуться в сторону даст: Windows держит
		// не имя, а открытый файл. Хвост .ubrat подчищается на следующем заходе.
		ubrat := dst + ".ubrat"
		_ = os.Remove(ubrat)
		if _, err := os.Stat(dst); err == nil {
			if err := os.Rename(dst, ubrat); err != nil {
				return err
			}
		}
		if err := os.Rename(vrem, dst); err != nil {
			_ = os.Rename(ubrat, dst)
			return err
		}
		_ = os.Remove(ubrat)
	}
	return nil
}

// ZapisatItog кладёт исход в каталог данных: подменщик умирает, а служба на
// следующем старте обязана показать человеку, чем кончилось.
func ZapisatItog(dirDannyh string, i Itog) error {
	b, err := json.Marshal(i)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dirDannyh, imyaItoga), b, 0o600)
}

// ProchitatItog забирает исход: показывается один раз, а не на каждом старте.
func ProchitatItog(dirDannyh string) (Itog, bool, error) {
	put := filepath.Join(dirDannyh, imyaItoga)
	f, err := os.Open(put)
	if errors.Is(err, os.ErrNotExist) {
		return Itog{}, false, nil
	}
	if err != nil {
		return Itog{}, false, err
	}
	var i Itog
	err = json.NewDecoder(bufio.NewReader(f)).Decode(&i)
	_ = f.Close()
	_ = os.Remove(put)
	if err != nil {
		return Itog{}, false, fmt.Errorf("исход обновления не разбирается: %w", err)
	}
	return i, true, nil
}
