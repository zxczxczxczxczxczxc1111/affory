package reklama

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Начальный список едет внутри affory-svc.exe (+339 КБ): включение работает
// сразу и без сети, а на пути подключения нет ни сети, ни подпроцесса. Файлом
// рядом с программой он бы не обновлялся: архив самообновления везёт только
// exe. Исходник hagezi-light.txt лежит рядом для GPL и в exe не входит.

//go:embed vstroennyy-light.srs
var vstroennyySrs []byte

//go:embed vstroennyy.json
var vstroennyyPasport []byte

// pasport это vstroennyy.json, его пишет помощник obnovit.
type pasport struct {
	Uroven    Uroven     `json:"uroven"`
	Pravil    int        `json:"pravil"`
	Versiya   string     `json:"versiya"`
	Sobran    *time.Time `json:"sobran"`
	Sha256    string     `json:"sha256"`
	Istochnik string     `json:"istochnik"`
	Yadro     string     `json:"yadro"`
}

var metaVstroennogo = sync.OnceValue(func() meta {
	var p pasport
	if err := json.Unmarshal(vstroennyyPasport, &p); err != nil {
		// Паспорт вшит при сборке: битый это ошибка сборки, её ловит тест.
		panic(fmt.Sprintf("паспорт встроенного списка рекламы не разобран: %v", err))
	}
	return meta{Uroven: p.Uroven, Pravil: p.Pravil, Versiya: p.Versiya, Sobran: p.Sobran, Sha256: p.Sha256, Vstroennyy: true}
})

// vstroennyy отдаёт начальный список, едущий внутри программы, и его мету.
func vstroennyy() ([]byte, meta) {
	return vstroennyySrs, metaVstroennogo()
}
