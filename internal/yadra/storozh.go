package yadra

import (
	"context"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
)

// Clamp the COUNTER, not the result. time.Duration(1<<uint(popytka)) with a
// counter of 63 overflows into a negative duration, and a negative sleep returns
// instantly: the ceiling that was supposed to protect the CPU becomes the thing
// that sets it on fire. The bug only shows up after an hour of a core that
// refuses to start, which is exactly when nobody is watching.
const maksPopytok = 5 // 1, 2, 4, 8, 16, then the ceiling

func pauzaStorozha(popytka int) time.Duration {
	if popytka < 0 {
		popytka = 0
	}
	if popytka > maksPopytok {
		return 30 * time.Second
	}
	p := time.Duration(1<<uint(popytka)) * time.Second
	if p > 30*time.Second {
		return 30 * time.Second
	}
	return p
}

// Long enough that a core which crashes on a bad config never counts as healthy,
// short enough that a real day's uptime resets the counter.
const minZhivoy = 60 * time.Second

// Storozhit restarts the core until the context is cancelled. Cancellation is
// how disconnect stops it: without that, disconnect kills the process and the
// watchdog cheerfully brings it back, and the user watches a tunnel that refuses
// to die.
// A seam, and not for decoration: without it the only way to check that a killed
// core comes back is to have a guest, a hypervisor and a human with a password.
// The restart logic is ours, the process is the OS's, and only the first one is
// worth a unit test.
var zapuskatel = Zapustit

func Storozhit(ctx context.Context, imya, konfig string, sobytie func(protokol.Sostoyanie)) error {
	popytka := 0
	for {
		y, err := zapuskatel(imya, konfig)
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			_ = y.Ostanovit()
			return ctx.Err()
		case <-y.smert:
			// A core that ran for a while and then died is a different animal
			// from one that cannot start at all. Resetting the counter after a
			// healthy interval means a nightly hiccup does not leave us waiting
			// thirty seconds for the next restart forever.
			if time.Since(y.podnyt) > minZhivoy {
				popytka = 0
			}
			if sobytie != nil {
				sobytie(protokol.SostVosstanavl)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pauzaStorozha(popytka)):
		}
		popytka++
	}
}
