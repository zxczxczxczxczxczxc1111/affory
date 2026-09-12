package main

import (
	"encoding/json"
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/obnovlenie"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Жалоба на неудачное обновление обязана умолкнуть, когда её уже исправили.
//
// 13.09.2026 на живой машине подмена 1.0.3 на 1.1.0 не удалась, человек поставил
// 1.1.0 установщиком руками, и первое, что показало окно новой версии, это
// «обновление откатилось, но служба не отвечает: переустановите из установщика».
// Совет уже выполнен, а исход об этом не знал: в файле не было версии, при
// которой он случился.
func TestIshodOtProshloyVersiiNePokazyvaetsya(t *testing.T) {
	s := sVersiey(t, "1.1.0")
	s.dirDannyh = t.TempDir()
	if err := obnovlenie.ZapisatItog(s.dirDannyh, obnovlenie.Itog{
		Kod:     obnovlenie.KodOtkatNeUdalsya,
		Tekst:   "откат не удался",
		Versiya: "1.0.3",
	}); err != nil {
		t.Fatal(err)
	}

	st := statusSluzhby(t, s)
	if st.Oshib != nil {
		t.Fatalf("показана жалоба прошлой версии: %+v", st.Oshib)
	}
	if _, est, _ := obnovlenie.ProchitatItog(s.dirDannyh); est {
		t.Fatal("устаревший исход остался лежать и всплывёт снова")
	}
}

// Обратная сторона: исход СВОЕЙ версии показывается как прежде. Иначе починка
// выше молча выключила бы весь показ отката.
func TestIshodSvoeyVersiiPokazyvaetsya(t *testing.T) {
	s := sVersiey(t, "1.0.3")
	s.dirDannyh = t.TempDir()
	if err := obnovlenie.ZapisatItog(s.dirDannyh, obnovlenie.Itog{
		Kod:     obnovlenie.KodOtkat,
		Tekst:   "новая служба молчит",
		Versiya: "1.0.3",
	}); err != nil {
		t.Fatal(err)
	}

	st := statusSluzhby(t, s)
	if st.Oshib == nil || st.Oshib.Kod != obnovlenie.KodOtkat {
		t.Fatalf("статус не показал откат: %+v", st.Oshib)
	}
}

func statusSluzhby(t *testing.T, s *Sluzhba) protokol.StatusOtvet {
	t.Helper()
	o := s.Obrabotat(t.Context(), protokol.Kadr{Tip: "cmd", Id: 1, Imya: "status"})
	var st protokol.StatusOtvet
	if err := json.Unmarshal(o.Telo, &st); err != nil {
		t.Fatal(err)
	}
	return st
}
