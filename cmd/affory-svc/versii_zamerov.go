package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Непрозрачная версия связывает замер с параметрами сервера. Обычный хеш
// ключей нельзя отдавать интерфейсу: он позволил бы проверять догадки о пароле.
func (s *Sluzhba) versiiServerov(servery []protokol.Server) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.klyuchVersiy == nil {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		s.klyuchVersiy = key
	}
	versii := make(map[string]string, len(servery))
	for _, srv := range servery {
		hash := otpechatokServera(srv)
		mac := hmac.New(sha256.New, s.klyuchVersiy)
		mac.Write(hash[:])
		versii[srv.Id] = hex.EncodeToString(mac.Sum(nil))
	}
	return versii, nil
}
