package sostoyanie

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

const imyaFayla = "sostoyanie-klienta.json"

// This is a measuring instrument, not a debug dump. Four of the twelve
// acceptance thresholds are read off its timestamps, which is why it has a
// schema and why it is written atomically.
type SostoyanieFayla struct {
	Obnovlyon  time.Time           `json:"obnovlyon"`
	Sostoyanie protokol.Sostoyanie `json:"sostoyanie"`
	// Пара живёт здесь по той же причине, что и на проводе (protokol.StatusOtvet):
	// файл переживает процесс, и следующий старт обязан прочитать оба ответа, а
	// не один слипшийся. До Ш7-1 поле server_id тут не писал НИКТО.
	VybranId     string `json:"vybran_id,omitempty"`
	NesushchiyId string `json:"nesushchiy_id,omitempty"`

	// Timestamps the acceptance thresholds are measured from. Milliseconds are
	// not decoration: three of the four thresholds are single-digit seconds.
	ConnectNach   *time.Time `json:"connect_nachalo,omitempty"`
	ProbaPervaya  *time.Time `json:"proba_pervyy_otvet,omitempty"`
	SelectorNach  *time.Time `json:"selector_nachalo,omitempty"`
	SelectorGotov *time.Time `json:"selector_gotov,omitempty"`
	SetSmena      *time.Time `json:"set_smenilas,omitempty"`
	SetGotov      *time.Time `json:"set_vosstanovlena,omitempty"`
	SonVyhod      *time.Time `json:"son_vyhod,omitempty"`
	SonGotov      *time.Time `json:"son_vosstanovleno,omitempty"`

	PutSluzhby string `json:"put_sluzhby"`
	FaylEst    bool   `json:"fayl_est"`

	// Добавлено волной 2. Порт и секрет clash_api выдаются заново на каждый
	// старт, и без записи их сюда интерфейс волны 6 не найдёт, куда стучаться.
	// Файл лежит в каталоге с разорванным наследованием, куда пускают только
	// SYSTEM и админов, поэтому секрет тут не хуже, чем в памяти службы.
	PortClash   int    `json:"port_clash,omitempty"`
	SekretClash string `json:"sekret_clash,omitempty"`
	// Алиас TUN-адаптера появляется только после подъёма. На нём держится
	// порядок постановки правил брандмауэра в задаче 2.5.
	AdapterTun string `json:"adapter_tun,omitempty"`
	IndeksTun  uint32 `json:"indeks_tun,omitempty"`

	// Добавлено волной 3. Время ПОСЛЕДНЕГО УСПЕШНОГО обновления подписки.
	//
	// Без записи на диск расписание «раз в двенадцать часов» отсчитывается от
	// старта службы, то есть после каждой перезагрузки подписка тянется заново,
	// а токен светится в сети чаще, чем нужно. Возраст этого поля показывается
	// человеку при subscription-unreachable (§9.1), поэтому оно не отладочное.
	//
	// САМ АДРЕС ПОДПИСКИ СЮДА НЕ ПИШЕТСЯ. Он секрет того же класса, что ключ, и
	// живёт в DPAPI-хранилище задачи 3.3, а этот файл читают все админы машины.
	PodpiskaObnovlena *time.Time `json:"podpiska_obnovlena,omitempty"`

	// Добавлено волной 4 (задача 4.8). Настройка, а не состояние туннеля:
	// служба поднимает туннель при старте Windows сама, до всякого окна,
	// поэтому ответ обязан лежать там, где служба его найдёт без человека.
	// Переживает сброс файла при Disconnect так же, как отметка подписки.
	PodklyuchatPriStarte bool `json:"podklyuchat_pri_starte,omitempty"`
	KillSwitch           bool `json:"kill_switch,omitempty"`
	// Добавлено волной 6 (задача 6.2). Журнал соединений включён. Настройка,
	// переживает сброс при отключении по тому же доводу, что и флаг выше.
	Zhurnal bool `json:"zhurnal,omitempty"`
	// Эндпоинт checkExitIp (6.3). Пусто значит умолчание set.AdresProverkiPoUmolchaniyu.
	AdresProverki string `json:"adres_proverki,omitempty"`

	// Полоса канала ЧЕЛОВЕКА в мегабитах, добавлено 05.09.2026. Пара нулей это
	// «не измерена», и это умолчание.
	//
	// Настройка машины, а не свойство сервера: одна и та же ссылка у одного
	// человека несёт 440 Мбит, у другого 20. Нужна ровно одному протоколу,
	// hysteria2, у которого объявление полосы и ЕСТЬ переключатель Brutal.
	// Число обязано быть измеренным и с запасом вниз: завышенное наказывается
	// провалами, а не просто игнорируется.
	PolosaVverh int `json:"polosa_vverh,omitempty"`
	PolosaVniz  int `json:"polosa_vniz,omitempty"`
}

func Zapisat(s SostoyanieFayla) error     { return zapisatV(KatalogDannyh(), s) }
func Prochitat() (SostoyanieFayla, error) { return prochitatIz(KatalogDannyh()) }

func zapisatV(dir string, s SostoyanieFayla) error {
	s.Obnovlyon = time.Now()
	telo, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("состояние не сериализуется: %w", err)
	}
	vremen := filepath.Join(dir, imyaFayla+".tmp")
	if err := os.WriteFile(vremen, telo, 0o600); err != nil {
		return fmt.Errorf("временный файл не записан: %w", err)
	}
	// Rename over an existing file is atomic on NTFS. Write-in-place is not, and
	// the difference shows up exactly once, at the worst possible moment.
	if err := os.Rename(vremen, filepath.Join(dir, imyaFayla)); err != nil {
		// The temp file is ours and it is garbage now. Leaving it behind means
		// the next reader eventually finds two files and picks the wrong one.
		_ = os.Remove(vremen)
		return fmt.Errorf("переименование не удалось: %w", err)
	}
	return nil
}

func prochitatIz(dir string) (SostoyanieFayla, error) {
	var s SostoyanieFayla
	telo, err := os.ReadFile(filepath.Join(dir, imyaFayla))
	if err != nil {
		return s, fmt.Errorf("файл состояния не читается: %w", err)
	}
	if err := json.Unmarshal(telo, &s); err != nil {
		// A corrupt file is not a reason to lose the service. The caller starts
		// from a blank state; the error says why the history is gone.
		return s, fmt.Errorf("файл состояния не разбирается: %w", err)
	}
	return s, nil
}
