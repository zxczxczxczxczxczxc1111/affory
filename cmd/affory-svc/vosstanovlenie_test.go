package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/set"
)

// sluzhbaSMyortvymServerom восстанавливает туннель, который не встаёт никогда.
func sluzhbaSMyortvymServerom(t *testing.T) (*Sluzhba, *atomic.Int32) {
	t.Helper()
	s := podstavnaya(t, nil)
	var podnimali atomic.Int32
	s.podnyatTunnel = func(context.Context) (set.Adapter, error) {
		podnimali.Add(1)
		return set.Adapter{}, errors.New("тест: сервер лёг")
	}
	s.otstupy = []time.Duration{time.Millisecond}
	s.redkiyOtstup = time.Hour
	s.postavit(protokol.SostNeNeset, nil)
	return s, &podnimali
}

// С8 аудита 1.6.1. Сервер лёг, а сеть жива: после десятка неудач подряд
// стучаться к нему раз в полминуты незачем, пауза вырастает до пяти минут.
func TestMyortvyyServerPriZhivoySetiRedkiePopytki(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "десяток попыток", func() bool { return podnimali.Load() >= neudachDoRedkih })
	time.Sleep(100 * time.Millisecond)
	if n := podnimali.Load(); n != neudachDoRedkih {
		t.Fatalf("попыток %d при живой сети и мёртвом сервере, ждали %d и паузу", n, neudachDoRedkih)
	}
}

// Сеть мертва: пауза остаётся короткой, иначе вернувшуюся сеть человек ждал
// бы пять минут.
func TestMyortvayaSetNeZamedlyaetVosstanovlenie(t *testing.T) {
	s, podnimali := sluzhbaSMyortvymServerom(t)
	s.sprositVyhod = func(context.Context, string, int) (string, error) { return "", errors.New("тест: сети нет") }
	s.zapustitVosstanovlenie()
	dozhdatsya(t, "попытки после десятка при мёртвой сети", func() bool {
		return podnimali.Load() >= neudachDoRedkih+5
	})
}
