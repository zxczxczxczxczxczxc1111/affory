package main

import (
	"context"
	"fmt"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/kanal"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// zapisatKomandu кладёт в журнал команд одну строку: имя, кто прислал, исход.
//
// Тела здесь нет и не будет. В addServer и setSubscription лежат ключи, и
// список «секретных» команд в protokol/tayny.go защищает от печати кадра
// целиком, но журналу команд тело не нужно вовсе: вопрос, ради которого он
// заведён, звучит «кто и когда послал setServer», а не «с чем».
//
// Формат нарочно однострочный и без JSON: журнал читают глазами через
// Get-Content -Tail, а не программой.
func (s *Sluzhba) zapisatKomandu(ctx context.Context, k, otv protokol.Kadr) {
	if s.zhurnalKomand == nil {
		return
	}
	d := kanal.DopuskIz(ctx)
	kto := "pid=?"
	if d.Pid != 0 {
		kto = fmt.Sprintf("pid=%d", d.Pid)
	}
	if d.Protsess != "" {
		kto += " " + d.Protsess
	}
	if d.Sid != "" {
		kto += " sid=" + d.Sid
	}
	ishod := "ok"
	if otv.Oshib != nil {
		ishod = "отказ " + otv.Oshib.Kod
	}
	s.zhurnalKomand.Printf("%s <- %s -> %s", k.Imya, kto, ishod)
}
