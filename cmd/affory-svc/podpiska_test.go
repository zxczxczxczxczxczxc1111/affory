package main

import (
	"testing"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/ssylki"
)

// Запись, которую подписка больше не отдаёт, удерживается ради живого ядра, но
// врать про её происхождение нельзя: на экране 12.09.2026 такой ключ стоял с
// пометкой «из подписки», а подписка про него уже не знала, и человек читал
// список как «восемь ключей из подписки» при семи настоящих.
func TestUderzhannyyKlyuchNeSchitaetsyaIzPodpiski(t *testing.T) {
	prezhnie := []protokol.Server{
		{Id: "a", Imya: "ostalsya", IzPodpiski: true},
		{Id: "b", Imya: "propal", IzPodpiski: true},
	}
	n := &Nabor{Servery: []protokol.Server{{Id: "a", Imya: "ostalsya", IzPodpiski: true}}}

	s := &Sluzhba{}
	s.mu.Lock()
	s.portClash, s.sekretClash = 9090, "sekret"
	s.mu.Unlock()

	imena := s.uderzhatZhivyh(prezhnie, n)
	if len(imena) != 1 {
		t.Fatalf("удержано %d записей, ждали одну: %v", len(imena), imena)
	}
	var uderzhan *protokol.Server
	for i := range n.Servery {
		if n.Servery[i].Id == "b" {
			uderzhan = &n.Servery[i]
		}
	}
	if uderzhan == nil {
		t.Fatal("пропавший ключ не удержан вовсе")
	}
	if uderzhan.IzPodpiski {
		t.Fatal("удержанный ключ помечен как из подписки, хотя подписка его не отдаёт")
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
