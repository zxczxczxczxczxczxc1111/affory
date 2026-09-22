package main

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

func otpechatokServera(srv protokol.Server) [32]byte {
	// В Server только сериализуемые поля. Имя и источник не меняют соединение.
	srv.Imya = ""
	srv.IzPodpiski = false
	srv.Uderzhan = false
	telo, _ := json.Marshal(srv)
	return sha256.Sum256(telo)
}

// Старые версии сохраняли снимок ядра в каталоге. Эти записи больше не
// участвуют в следующем подключении и не возвращаются в кэш подписок.
func aktualnyeServery(v []protokol.Server) []protokol.Server {
	out := make([]protokol.Server, 0, len(v))
	seen := make(map[string]bool, len(v))
	for _, srv := range v {
		if !srv.Uderzhan && !seen[srv.Id] {
			out = append(out, srv)
			seen[srv.Id] = true
		}
	}
	return out
}

// Снимок содержит только адреса и подписи, без ключей. Он нужен правилам
// сети до остановки ядра, но не является каталогом доступных серверов.
func (s *Sluzhba) zapomnitServeryYadra(v []protokol.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serveryYadra = make([]protokol.Server, 0, len(v))
	s.otpechatkiYadra = make(map[string][32]byte, len(v))
	for _, srv := range v {
		s.serveryYadra = append(s.serveryYadra, dlyaEkrana(srv))
		s.otpechatkiYadra[srv.Id] = otpechatokServera(srv)
	}
}

// Один адресный ID может иметь другие ключи после обновления или смены
// подписки. PUT старого тега не применяет новые параметры: нужен подъём.
func (s *Sluzhba) serverTrebuetPodyoma(id string) (bool, error) {
	srv, err := s.serverPoId(id)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.portClash == 0 || s.otpechatkiYadra == nil {
		return false, nil
	}
	hash, est := s.otpechatkiYadra[id]
	return !est || hash != otpechatokServera(srv), nil
}

func (s *Sluzhba) serveryDlyaRazresheniy(v []protokol.Server) []protokol.Server {
	out := append([]protokol.Server(nil), v...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.portClash != 0 {
		// Совпадение ID не означает совпадение адресов после изменения профиля.
		out = append(out, s.serveryYadra...)
	}
	return out
}
