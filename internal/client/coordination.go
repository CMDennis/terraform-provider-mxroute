package client

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"
	"time"
)

// A rolling minute with headroom below the documented 200 reads / 60 writes.
const readBudget, writeBudget = 180, 48

type budget struct {
	sent         []time.Time
	blockedUntil time.Time
}
type accountLimits struct {
	mu            sync.Mutex
	reads, writes budget
}
type settingLock struct{ token chan struct{} }

var processCoordination = struct {
	sync.Mutex
	limits   map[[32]byte]*accountLimits
	settings map[[32]byte]*settingLock
}{limits: make(map[[32]byte]*accountLimits), settings: make(map[[32]byte]*settingLock)}

// Keys contain no plaintext account identities or credentials. Different keys
// owned by one account deliberately share a budget. Mock origins are isolated.
func coordination(origin, server, owner, effective string) (*accountLimits, *settingLock) {
	key := func(name string) [32]byte {
		return sha256.Sum256([]byte(origin + "\x00" + strings.ToLower(server) + "\x00" + strings.ToLower(name)))
	}
	processCoordination.Lock()
	defer processCoordination.Unlock()
	l := processCoordination.limits[key(owner)]
	if l == nil {
		l = &accountLimits{}
		processCoordination.limits[key(owner)] = l
	}
	s := processCoordination.settings[key(effective)]
	if s == nil {
		s = &settingLock{token: make(chan struct{}, 1)}
		processCoordination.settings[key(effective)] = s
	}
	return l, s
}
func pause(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func (s *settingLock) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.token <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *settingLock) release() { <-s.token }
func (l *accountLimits) reserve(ctx context.Context, read bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		b, maximum := &l.writes, writeBudget
		if read {
			b, maximum = &l.reads, readBudget
		}
		now := time.Now()
		i := 0
		for i < len(b.sent) && !b.sent[i].Add(time.Minute).After(now) {
			i++
		}
		b.sent = b.sent[i:]
		until := b.blockedUntil
		if len(b.sent) >= maximum && b.sent[0].Add(time.Minute).After(until) {
			until = b.sent[0].Add(time.Minute)
		}
		if !until.After(now) {
			b.sent = append(b.sent, now)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		if err := pause(ctx, time.Until(until)); err != nil {
			return err
		}
	}
}
func (l *accountLimits) block(read bool, until time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := &l.writes
	if read {
		b = &l.reads
	}
	if until.After(b.blockedUntil) {
		b.blockedUntil = until
	}
}
