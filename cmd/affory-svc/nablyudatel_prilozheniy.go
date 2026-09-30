package main

import (
	"errors"
	"fmt"
	"github.com/sagernet/sing-box/common/afforyprocess"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"log"
)

type nablyudatelPrilozheniy struct {
	watcher *afforyprocess.Watcher
	api     *afforyprocess.SharedServer
}

func novyyNablyudatelPrilozheniy() (*nablyudatelPrilozheniy, error) {
	w, err := afforyprocess.NewEventWatcher(func(err error) {
		log.Printf("наблюдение за запусками приложений: %v", err)
	})
	if err != nil {
		return nil, err
	}
	api, err := afforyprocess.NewSharedServer(w.FindIdentity)
	if err != nil {
		w.Close()
		return nil, err
	}
	return &nablyudatelPrilozheniy{watcher: w, api: api}, nil
}
func (n *nablyudatelPrilozheniy) Close() error {
	err := n.api.Close()
	if other := n.watcher.Close(); err == nil {
		err = other
	}
	return err
}

// Err отвечает, может ли трекер ещё отвечать ядру. Сбой событий Windows
// трекер переживает сам, переходя на снимки, поэтому смерть здесь это только
// остановленный наблюдатель или остановленный вход.
func (n *nablyudatelPrilozheniy) Err() error {
	if err := n.watcher.Err(); err != nil {
		return err
	}
	return n.api.Err()
}

func (s *Sluzhba) trackerDlyaKonfiga(trafik *protokol.PravilaTrafika) (genkonfig.ProcessTracker, error) {
	// Правило по дереву процессов дают и ручные правила с потомками, и
	// карточки сервисов с программами (L6 аудита 1.8.0). Прежде здесь
	// смотрелись только ручные, и Steam из сервисов уходил на опрос ядром.
	if !genkonfig.NuzhnaSemyaProtsessov(trafik) {
		return genkonfig.ProcessTracker{}, nil
	}
	n, err := s.zhivoyTracker()
	if err != nil {
		return genkonfig.ProcessTracker{}, fmt.Errorf("не удалось включить правила для запущенных программ: %w", err)
	}
	if n == nil {
		return genkonfig.ProcessTracker{}, nil
	} // Изолированные тесты ядра используют собственный трекер.
	return genkonfig.ProcessTracker{Endpoint: n.api.Address, Secret: n.api.Secret}, nil
}

// zhivoyTracker отдаёт работающий трекер, пересоздавая остановившийся или не
// поднявшийся при старте службы (В3 аудита 1.8.0). Прежде трекер создавался
// один раз за жизнь службы, и отказ на старте держался до её перезапуска.
//
// Ни трекера, ни ошибки означает изолированный тест: трекер ему не заводят.
func (s *Sluzhba) zhivoyTracker() (*nablyudatelPrilozheniy, error) {
	s.muTracker.Lock()
	defer s.muTracker.Unlock()
	s.mu.Lock()
	n, err, ostanovlena := s.processTracker, s.processTrackerErr, s.ostanovlena
	s.mu.Unlock()
	switch {
	case ostanovlena:
		return nil, errors.New("служба останавливается")
	case n == nil && err == nil:
		return nil, nil
	case n != nil:
		smert := n.Err()
		if smert == nil {
			return n, nil
		}
		log.Printf("наблюдение за приложениями остановилось, завожу заново: %v", smert)
		if err := n.Close(); err != nil {
			log.Printf("остановка прежнего наблюдения за приложениями: %v", err)
		}
	default:
		log.Printf("наблюдение за приложениями не было запущено (%v), пробую снова", err)
	}
	nov, err := s.novyyTracker()
	s.mu.Lock()
	s.processTracker, s.processTrackerErr = nov, err
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	log.Printf("наблюдение за приложениями запущено заново")
	return nov, nil
}
