package main

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/skorost"
)

type speedSnapshot struct {
	ID       int64           `json:"id"`
	Phase    string          `json:"phase"`
	Path     string          `json:"path"`
	Provider string          `json:"provider"`
	Name     string          `json:"name"`
	Attempt  int             `json:"attempt"`
	Started  time.Time       `json:"started,omitempty"`
	Finished *time.Time      `json:"finished,omitempty"`
	Result   *skorost.Result `json:"result,omitempty"`
	Reason   string          `json:"reason,omitempty"`
}
type speedJob struct {
	snapshot speedSnapshot
	cancel   context.CancelFunc
	done     bool
}
type speedRun func(context.Context, string, string, func(skorost.Progress)) skorost.Result

func runSpeed(ctx context.Context, proxy, preferred string, publish func(skorost.Progress)) skorost.Result {
	client, err := skorost.Client(proxy)
	if err != nil {
		return skorost.Result{Error: err.Error()}
	}
	defer client.CloseIdleConnections()
	return skorost.Default().Run(ctx, client, preferred, publish)
}

func (s *Sluzhba) startSpeedTest(k protokol.Kadr) protokol.Kadr {
	var input struct {
		Provider string `json:"provider"`
	}
	if len(k.Telo) > 0 {
		if err := json.Unmarshal(k.Telo, &input); err != nil {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "неверные параметры замера")
		}
	}
	if input.Provider != "" {
		valid := false
		for _, p := range skorost.Default().Providers {
			valid = valid || p.ID == input.Provider
		}
		if !valid {
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "сервис замера не найден")
		}
	}
	s.muVybor.Lock()
	defer s.muVybor.Unlock()
	s.mu.Lock()
	if s.ostanovlena || (s.speed != nil && !s.speed.done) {
		s.mu.Unlock()
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "замер уже выполняется или служба останавливается")
	}
	proxy, path := "", "system"
	switch s.sost {
	case protokol.SostVyklyuchen:
		if s.zaslonAktiven {
			s.mu.Unlock()
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "сначала восстановите сеть после отключения")
		}
	case protokol.SostPodnyat:
		if s.portProksiNash <= 0 {
			s.mu.Unlock()
			return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "локальный вход VPN недоступен")
		}
		proxy = net.JoinHostPort("127.0.0.1", strconv.Itoa(s.portProksiNash))
		path = "vpn"
	default:
		s.mu.Unlock()
		return otkaz(k.Id, k.Imya, protokol.KodTeloNegodno, "дождитесь завершения подключения")
	}
	ctx, cancel := context.WithTimeout(s.fonCtx, 60*time.Second)
	job := &speedJob{snapshot: speedSnapshot{ID: time.Now().UnixMilli(), Phase: "download", Path: path, Started: time.Now()}, cancel: cancel}
	s.speed = job
	run := s.speedRunner
	if run == nil {
		run = runSpeed
	}
	s.fon.Add(1)
	initial := job.snapshot
	s.mu.Unlock()
	go func() {
		defer s.fon.Done()
		defer cancel()
		result := run(ctx, proxy, input.Provider, func(p skorost.Progress) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if ctx.Err() == nil {
				job.snapshot.Phase = p.Phase
				job.snapshot.Provider = p.Provider
				job.snapshot.Name = p.Name
				job.snapshot.Attempt = p.Attempt
			}
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		finished := time.Now()
		job.done = true
		job.snapshot.Finished = &finished
		if ctx.Err() != nil {
			job.snapshot.Phase = "cancelled"
			if job.snapshot.Reason == "" {
				job.snapshot.Reason = "Замер отменён"
			}
			return
		}
		job.snapshot.Result = &result
		job.snapshot.Phase = "complete"
		if result.Error != "" {
			job.snapshot.Phase = "error"
		}
	}()
	return otvet(k.Id, k.Imya, initial)
}

func (s *Sluzhba) speedTestStatus(k protokol.Kadr) protokol.Kadr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.speed == nil {
		return otvet(k.Id, k.Imya, speedSnapshot{Phase: "idle"})
	}
	return otvet(k.Id, k.Imya, s.speed.snapshot)
}

// Caller holds s.mu. Cancellation closes live HTTP requests before paths can mix.
func (s *Sluzhba) cancelSpeedLocked(reason string) {
	if s.speed != nil && !s.speed.done {
		s.speed.snapshot.Reason = reason
		s.speed.snapshot.Phase = "cancelled"
		s.speed.cancel()
	}
}
func (s *Sluzhba) cancelSpeed(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelSpeedLocked(reason)
}
func (s *Sluzhba) cancelSpeedTest(k protokol.Kadr) protokol.Kadr {
	s.cancelSpeed("Замер остановлен")
	return s.speedTestStatus(k)
}
