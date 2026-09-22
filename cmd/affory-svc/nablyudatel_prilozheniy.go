package main

import (
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

func (s *Sluzhba) trackerDlyaKonfiga(trafik *protokol.PravilaTrafika) (genkonfig.ProcessTracker, error) {
	if trafik == nil {
		return genkonfig.ProcessTracker{}, nil
	}
	usesFamily := false
	for _, a := range trafik.Prilozheniya {
		usesFamily = usesFamily || a.Potomki
	}
	if !usesFamily {
		return genkonfig.ProcessTracker{}, nil
	}
	s.mu.Lock()
	n, err := s.processTracker, s.processTrackerErr
	s.mu.Unlock()
	if err != nil {
		return genkonfig.ProcessTracker{}, fmt.Errorf("не удалось включить правила для запущенных программ: %w", err)
	}
	if n == nil {
		return genkonfig.ProcessTracker{}, nil
	} // Изолированные тесты ядра используют собственный трекер.
	return genkonfig.ProcessTracker{Endpoint: n.api.Address, Secret: n.api.Secret}, nil
}
