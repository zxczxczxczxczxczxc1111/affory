// Package hranenie keeps the things the service must be able to trust between
// runs. Right now that is one map of file hashes.
package hranenie

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/sostoyanie"
)

const imyaFayla = "otpechatki.json"

// С 0.7.0 выпуск подписывается самоподписанным сертификатом, но эти отпечатки
// он не заменяет и заменить не может: подпись говорит «байты те же, что вышли
// со сборки», а карта здесь говорит «файлы те же, что легли при установке».
// Второе про эту машину, и посторонний по нему ничего не докажет.
//
// Порядок важен и он такой: подпись ложится на сборке, карта снимается из
// install на машине человека, по уже подписанным байтам. Подписывать что-либо
// в каталоге программы ПОСЛЕ установки нельзя, это ровно то, что карта ловит.
func SnyatOtpechatki(dirProgrammy string) error {
	return snyatV(dirProgrammy, sostoyanie.KatalogDannyh())
}

func Sverit(put string) error { return sveritV(sostoyanie.KatalogDannyh(), put) }

func snyatV(dirProgrammy, dirDannyh string) error {
	zapisi, err := os.ReadDir(dirProgrammy)
	if err != nil {
		return fmt.Errorf("каталог программы не читается: %w", err)
	}
	karta := map[string]string{}
	for _, z := range zapisi {
		// Top level only. Subdirectories in the program folder are not ours, and
		// walking into them turns install into an unbounded hashing job.
		if z.IsDir() {
			continue
		}
		s, err := hashFayla(filepath.Join(dirProgrammy, z.Name()))
		if err != nil {
			return err
		}
		karta[strings.ToLower(z.Name())] = s
	}
	telo, err := json.MarshalIndent(karta, "", "  ")
	if err != nil {
		return fmt.Errorf("отпечатки не сериализуются: %w", err)
	}
	if err := os.MkdirAll(dirDannyh, 0o700); err != nil {
		return fmt.Errorf("каталог данных недоступен: %w", err)
	}
	// Same atomic dance as the state file, for the same reason: a half-written
	// fingerprint map fails every later check and looks exactly like tampering.
	vremen := filepath.Join(dirDannyh, imyaFayla+".tmp")
	if err := os.WriteFile(vremen, telo, 0o600); err != nil {
		return fmt.Errorf("отпечатки не записаны: %w", err)
	}
	if err := os.Rename(vremen, filepath.Join(dirDannyh, imyaFayla)); err != nil {
		_ = os.Remove(vremen)
		return fmt.Errorf("отпечатки не переименованы: %w", err)
	}
	return nil
}

func sveritV(dirDannyh, put string) error {
	telo, err := os.ReadFile(filepath.Join(dirDannyh, imyaFayla))
	if err != nil {
		// Not a reason to shrug. No fingerprint file means either the install
		// never ran or somebody removed it, and both deserve a loud refusal.
		return fmt.Errorf("файл отпечатков недоступен: %w", err)
	}
	var karta map[string]string
	if err := json.Unmarshal(telo, &karta); err != nil {
		return fmt.Errorf("файл отпечатков не разбирается: %w", err)
	}
	imya := strings.ToLower(filepath.Base(put))
	hotim, est := karta[imya]
	if !est {
		return fmt.Errorf("файла %s нет в отпечатках", imya)
	}
	est2, err := hashFayla(put)
	if err != nil {
		return err
	}
	if est2 != hotim {
		return fmt.Errorf("отпечаток %s не совпал: ожидался %s, получен %s", imya, hotim, est2)
	}
	return nil
}

func hashFayla(put string) (string, error) {
	f, err := os.Open(put)
	if err != nil {
		return "", fmt.Errorf("файл %s не открывается: %w", filepath.Base(put), err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("файл %s не читается: %w", filepath.Base(put), err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
