package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Проверка обновлений (план «шесть удобств» §5).
//
// Источник один: последний выпуск этого репозитория на GitHub. Служба читает
// описание выпуска через API, находит в нём архив affory-X.Y.Z.zip и файл
// .sha256 рядом, сравнивает версию из тега со своей. Запрос идёт НАПРЯМУЮ,
// как снимок состояния: обновление нужно и тогда, когда туннель лежит.
// Без токена GitHub даёт 60 запросов в час с адреса; раз в сутки это ничто.
const AdresObnovleniyPoUmolchaniyu = "https://api.github.com/repos/zxczxczxczxczxczxc1111/affory/releases/latest"

const (
	predelOpisaniya   = 256 << 10
	predelHesha       = 4 << 10
	predelArhivaSeti  = 64 << 20
	srokProverki      = 20 * time.Second
	srokZagruzki      = 3 * time.Minute
	pervayaProverka   = time.Minute
	periodProverki    = 24 * time.Hour
	otstupProverki    = time.Hour
	katalogObnovleniy = "obnovleniya"
)

// vypuskGitHub это та часть ответа releases/latest, которая здесь нужна.
type vypuskGitHub struct {
	Teg   string `json:"tag_name"`
	Fayly []struct {
		Imya   string `json:"name"`
		Razmer int64  `json:"size"`
		Adres  string `json:"browser_download_url"`
	} `json:"assets"`
}

// svedeniyaVypuska это то, что служба знает о последнем выпуске: версия,
// имя и адрес архива, его хеш из файла .sha256 рядом.
type svedeniyaVypuska struct {
	Versiya     string
	Arhiv       string
	AdresArhiva string
	Sha256      string
	Razmer      int64
}

var errNetVersii = errors.New("сборка без версии обновления не проверяет")

// versiyaDlyaEkrana: dev это не версия, экран покажет пустоту, а не «dev».
func versiyaDlyaEkrana() string {
	if versiyaProgrammy == "dev" {
		return ""
	}
	return versiyaProgrammy
}

// novee сравнивает X.Y.Z по числам. Нечисловое считается не новее: лучше
// промолчать, чем предложить «обновиться» на строку.
func novee(kandidat, tekushchaya string) bool {
	a, okA := razobratVersiyu(kandidat)
	b, okB := razobratVersiyu(tekushchaya)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func razobratVersiyu(v string) ([3]int, bool) {
	var itog [3]int
	ch := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(ch) != 3 {
		return itog, false
	}
	for i, c := range ch {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 {
			return itog, false
		}
		itog[i] = n
	}
	return itog, true
}

// skachatPoSeti это настоящая загрузка с потолком: адреса файлов приходят из
// описания выпуска, и страница-заглушка на их месте не должна занять диск.
func skachatPoSeti(ctx context.Context, adres string, predel int64) ([]byte, error) {
	z, err := http.NewRequestWithContext(ctx, http.MethodGet, adres, nil)
	if err != nil {
		return nil, err
	}
	z.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	z.Header.Set("User-Agent", "affory/"+versiyaProgrammy)
	o, err := (&http.Client{}).Do(z)
	if err != nil {
		return nil, err
	}
	defer o.Body.Close()
	if o.StatusCode == http.StatusNotFound {
		// 404 у этого адреса означает не поломку, а «смотреть нечего»:
		// репозиторий закрыт либо выпусков в нём пока нет. Голое «код ответа
		// 404» человек читает как отказ программы и идёт чинить не то.
		return nil, fmt.Errorf("код ответа 404: список выпусков не отдан, репозиторий закрыт или выпусков нет")
	}
	if o.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("код ответа %d", o.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(o.Body, predel+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > predel {
		return nil, fmt.Errorf("ответ больше потолка %d байт", predel)
	}
	return b, nil
}

// posledniyVypusk читает описание последнего выпуска и файл .sha256 рядом с
// архивом. Ошибка любого шага это ошибка проверки, не «обновлений нет».
func (s *Sluzhba) posledniyVypusk(ctx context.Context) (*svedeniyaVypuska, error) {
	b, err := s.skachatFayl(ctx, s.adresObnovleniy, predelOpisaniya)
	if err != nil {
		return nil, fmt.Errorf("описание выпуска не получено: %w", err)
	}
	var v vypuskGitHub
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("описание выпуска не разобрано: %w", err)
	}
	versiya := strings.TrimPrefix(strings.TrimSpace(v.Teg), "v")
	if _, ok := razobratVersiyu(versiya); !ok {
		return nil, fmt.Errorf("тег выпуска %q не вида X.Y.Z", v.Teg)
	}
	arhiv := "affory-" + versiya + ".zip"
	sv := &svedeniyaVypuska{Versiya: versiya, Arhiv: arhiv}
	var adresHesha string
	for _, f := range v.Fayly {
		switch f.Imya {
		case arhiv:
			sv.AdresArhiva, sv.Razmer = f.Adres, f.Razmer
		case arhiv + ".sha256":
			adresHesha = f.Adres
		}
	}
	// Адреса файлов пришли из описания, но качать по ним можно только https:
	// иначе подмена описания по дороге превратилась бы в загрузку с чужого узла.
	if !strings.HasPrefix(sv.AdresArhiva, "https://") || !strings.HasPrefix(adresHesha, "https://") {
		return nil, fmt.Errorf("в выпуске %s нет %s с .sha256 по https", versiya, arhiv)
	}
	h, err := s.skachatFayl(ctx, adresHesha, predelHesha)
	if err != nil {
		return nil, fmt.Errorf("%s.sha256 не получен: %w", arhiv, err)
	}
	polya := strings.Fields(string(h))
	if len(polya) == 0 || len(polya[0]) != 64 {
		return nil, fmt.Errorf("%s.sha256 не содержит sha256", arhiv)
	}
	if _, err := hex.DecodeString(polya[0]); err != nil {
		return nil, fmt.Errorf("%s.sha256 не шестнадцатеричный", arhiv)
	}
	sv.Sha256 = strings.ToLower(polya[0])
	return sv, nil
}

// proveritObnovlenie читает последний выпуск и запоминает находку. Ошибка сети
// это ошибка, а «новее нет» это nil без ошибки; отметка проверки ставится в
// обоих удачных случаях.
func (s *Sluzhba) proveritObnovlenie(ctx context.Context) (*protokol.ObnovlenieOtvet, error) {
	if versiyaProgrammy == "dev" {
		return nil, errNetVersii
	}
	do, otm := context.WithTimeout(ctx, srokProverki)
	defer otm()
	v, err := s.posledniyVypusk(do)
	if err != nil {
		return nil, err
	}
	seychas := s.seychas()
	var nahodka *protokol.ObnovlenieOtvet
	if novee(v.Versiya, versiyaProgrammy) {
		nahodka = &protokol.ObnovlenieOtvet{Versiya: v.Versiya, Razmer: v.Razmer, Provereno: seychas}
	}
	s.mu.Lock()
	bylo := s.obnovlenie
	s.obnovlenie, s.obnovlenieProvereno = nahodka, &seychas
	s.mu.Unlock()
	// Новая версия это событие состояния: трей скажет об этом один раз.
	if nahodka != nil && (bylo == nil || bylo.Versiya != nahodka.Versiya) {
		log.Printf("последний выпуск %s, у нас %s", nahodka.Versiya, versiyaProgrammy)
		s.izvestit("state", s.Status())
	}
	return nahodka, nil
}

// raspisanieObnovleniy: через минуту после старта, затем раз в сутки; при
// отказе сети через час. Сборка dev не проверяет ничего.
func (s *Sluzhba) raspisanieObnovleniy(ctx context.Context) {
	if versiyaProgrammy == "dev" {
		return
	}
	pauza := pervayaProverka
	for {
		if !s.zhdat(ctx, pauza) {
			return
		}
		if _, err := s.proveritObnovlenie(ctx); err != nil {
			log.Printf("проверка обновления не удалась: %v", err)
			pauza = otstupProverki
			continue
		}
		pauza = periodProverki
	}
}

func (s *Sluzhba) checkUpdate(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	if _, err := s.proveritObnovlenie(ctx); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeProvereno, err.Error())
	}
	return otvet(k.Id, k.Imya, s.Status())
}

// downloadUpdate качает архив последнего выпуска в каталог данных, кладёт
// рядом .sha256 из выпуска и ставит его тем же путём, что installUpdate.
func (s *Sluzhba) downloadUpdate(ctx context.Context, k protokol.Kadr) protokol.Kadr {
	do, otm := context.WithTimeout(ctx, srokZagruzki)
	defer otm()
	v, err := s.posledniyVypusk(do)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeProvereno, err.Error())
	}
	if !novee(v.Versiya, versiyaProgrammy) {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, fmt.Sprintf("последний выпуск %s, это не новее %s", v.Versiya, versiyaProgrammy))
	}
	arhiv, err := s.skachatFayl(do, v.AdresArhiva, predelArhivaSeti)
	if err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, "архив не скачан: "+err.Error())
	}
	if fakt := sha256.Sum256(arhiv); hex.EncodeToString(fakt[:]) != v.Sha256 {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, "sha256 скачанного архива не совпал с .sha256 выпуска")
	}
	kat := filepath.Join(s.dirDannyh, katalogObnovleniy)
	if err := os.MkdirAll(kat, 0o755); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, "каталог обновлений не создан: "+err.Error())
	}
	put := filepath.Join(kat, v.Arhiv)
	if err := os.WriteFile(put, arhiv, 0o644); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, "архив не записан: "+err.Error())
	}
	if err := os.WriteFile(put+".sha256", []byte(v.Sha256+"  "+v.Arhiv+"\n"), 0o644); err != nil {
		return otkaz(k.Id, k.Imya, protokol.KodObnovlenieNeSkachano, "хеш не записан: "+err.Error())
	}
	return s.ustanovitArhiv(k, put)
}
