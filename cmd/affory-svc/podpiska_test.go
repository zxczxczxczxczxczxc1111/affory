package main

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

func TestSnimokYadraOtdelenOtKataloga(t *testing.T) {
	s := &Sluzhba{portClash: 9090}
	old := []protokol.Server{{Id: "old", Host: "203.0.113.1", Uuid: "secret", IzPodpiski: true}}
	s.zapomnitServeryYadra(old)
	current := []protokol.Server{{Id: "new", Host: "203.0.113.2", IzPodpiski: true}}
	if err := s.spisokNeTeryaetZhivyh(Nabor{Servery: old}, Nabor{Servery: current}); err != nil {
		t.Fatal(err)
	}
	allowed := s.serveryDlyaRazresheniy(current)
	if len(allowed) != 2 || allowed[1].Uuid != "" {
		t.Fatal("снимок адресов потерян либо хранит секрет")
	}
	s.opustitYadro()
	if len(s.serveryDlyaRazresheniy(current)) != 1 || s.serveryYadra != nil {
		t.Fatal("снимок пережил отключение")
	}
}

// Удержанная запись не из подписки, но и не ручная. Разница видна только при
// следующем слиянии: ручную Slit тащит в новый набор вечно, а удержанную обязан
// выбросить, как только ядро её отпустит. Без отдельной пометки снятие флага
// «из подписки» превратило бы пропавший ключ в вечный.
func TestUderzhannyyKlyuchNeStanovitsyaRuchnym(t *testing.T) {
	bylo := []protokol.Server{
		{Id: "a", Imya: "ostalsya", IzPodpiski: true},
		{Id: "b", Imya: "uderzhan", Uderzhan: true},
		{Id: "c", Imya: "ruchnoy"},
	}
	stalo := []protokol.Server{{Id: "a", Imya: "ostalsya", IzPodpiski: true}}

	itog := ssylki.Slit(bylo, stalo)

	var ids []string
	for _, s := range itog {
		ids = append(ids, s.Id)
	}
	if len(itog) != 2 || ids[0] != "a" || ids[1] != "c" {
		t.Fatalf("после слияния %v, ждали [a c]: удержанный обязан уйти, ручной остаться", ids)
	}
}
