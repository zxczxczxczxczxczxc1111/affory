package yadra

import (
	"net"
	"os"
	"testing"
)

// Pid ядра нужен диагностике: течь дескрипторов бывает и у него, а служба
// держит ядро внутри сторожа и наружу его не отдаёт.
//
// Искать по ИМЕНИ процесса нельзя категорически: на машине человека живут
// чужие sing-box от других клиентов, и спутать их значит мерить чужое, а
// однажды и трогать чужое. Порт clash_api наш по построению: мы его сами
// выдали и сами знаем секрет.

func TestPidPortaNahoditSvoegoSlushatelya(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель не поднялся: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	pid, err := PidPorta(port)
	if err != nil {
		t.Fatalf("pid порта %d: %v", port, err)
	}
	if pid != os.Getpid() {
		t.Fatalf("порт %d слушаем мы (%d), а найден %d", port, os.Getpid(), pid)
	}
}

// Никем не занятый порт отдаёт ноль без ошибки: ядра может не быть вовсе,
// когда туннель опущен, и это покой, а не сбой.
func TestSvobodnyyPortOtdayotNol(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("слушатель не поднялся: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	pid, err := PidPorta(port)
	if err != nil {
		t.Fatalf("свободный порт дал ошибку: %v", err)
	}
	if pid != 0 {
		t.Fatalf("порт свободен, а найден pid %d", pid)
	}
}
