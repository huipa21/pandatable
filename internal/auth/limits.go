package auth

import (
	"net/netip"
	"sync"
	"time"
)

const limitCapacity = 4096

type ipWindow struct {
	start time.Time
	count int
}

type accountWindow struct {
	failures int
	busy     bool
	lockedTo time.Time
}

type limits struct {
	mu       sync.Mutex
	ips      map[netip.Addr]ipWindow
	accounts map[string]*accountWindow
}

func newLimits() *limits {
	return &limits{ips: make(map[netip.Addr]ipWindow), accounts: make(map[string]*accountWindow)}
}

func (limits *limits) ip(ip netip.Addr, now time.Time) bool {
	limits.mu.Lock()
	defer limits.mu.Unlock()
	window, exists := limits.ips[ip]
	if exists && now.Before(window.start.Add(time.Minute)) {
		if window.count >= 10 {
			return false
		}
		window.count++
		limits.ips[ip] = window
		return true
	}
	if len(limits.ips) >= limitCapacity {
		for key, entry := range limits.ips {
			if !now.Before(entry.start.Add(time.Minute)) {
				delete(limits.ips, key)
			}
		}
		if len(limits.ips) >= limitCapacity {
			return false
		}
	}
	limits.ips[ip] = ipWindow{start: now, count: 1}
	return true
}

func (limits *limits) beginAccount(id string, now time.Time) (*accountWindow, bool) {
	limits.mu.Lock()
	defer limits.mu.Unlock()
	entry := limits.accounts[id]
	if entry == nil {
		if len(limits.accounts) >= limitCapacity {
			for key, value := range limits.accounts {
				if !value.busy && !value.lockedTo.IsZero() && !now.Before(value.lockedTo) {
					delete(limits.accounts, key)
				}
			}
			if len(limits.accounts) >= limitCapacity {
				return nil, false
			}
		}
		entry = &accountWindow{}
		limits.accounts[id] = entry
	}
	if entry.busy || now.Before(entry.lockedTo) {
		return nil, false
	}
	if !entry.lockedTo.IsZero() {
		entry.failures, entry.lockedTo = 0, time.Time{}
	}
	entry.busy = true
	return entry, true
}

func (limits *limits) finishAccount(id string, entry *accountWindow, now time.Time, failed, success bool) {
	limits.mu.Lock()
	defer limits.mu.Unlock()
	entry.busy = false
	if success || (!failed && entry.failures == 0) {
		delete(limits.accounts, id)
		return
	}
	if failed {
		entry.failures++
		if entry.failures >= 5 {
			entry.lockedTo = now.Add(time.Minute)
		}
	}
}
