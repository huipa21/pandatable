package auth

import (
	"fmt"
	"net/netip"
	"testing"
	"time"
)

func TestRateLimitTiming(t *testing.T) {
	limiter := newLimits()
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	ip := netip.MustParseAddr("192.0.2.1")
	for range 10 {
		if !limiter.ip(ip, now) {
			t.Fatal("one of first ten attempts rejected")
		}
	}
	if limiter.ip(ip, now.Add(time.Minute-time.Nanosecond)) || !limiter.ip(ip, now.Add(time.Minute)) {
		t.Fatal("IP window boundary incorrect")
	}
	for range 5 {
		entry, ok := limiter.beginAccount("user-id", now)
		if !ok {
			t.Fatal("failure before fifth rejected")
		}
		if _, ok := limiter.beginAccount("user-id", now); ok {
			t.Fatal("parallel attempts for same account must serialize")
		}
		limiter.finishAccount("user-id", entry, now, true, false)
	}
	if _, ok := limiter.beginAccount("user-id", now.Add(time.Minute-time.Nanosecond)); ok {
		t.Fatal("locked account accepted before deadline")
	}
	entry, ok := limiter.beginAccount("user-id", now.Add(time.Minute))
	if !ok {
		t.Fatal("account not unlocked at exact deadline")
	}
	limiter.finishAccount("user-id", entry, now.Add(time.Minute), false, true)
	if len(limiter.accounts) != 0 {
		t.Fatal("success must reset failures")
	}
	entry, _ = limiter.beginAccount("user-id", now.Add(time.Minute))
	limiter.finishAccount("user-id", entry, now, false, false)
	if len(limiter.accounts) != 0 {
		t.Fatal("internal failure must release account reservation without counting a bad credential")
	}
}

func TestLimitCapacityFailsClosed(t *testing.T) {
	limiter := newLimits()
	now := time.Now()
	for index := range limitCapacity {
		ip := netip.AddrFrom4([4]byte{10, 1, byte(index / 256), byte(index % 256)})
		if !limiter.ip(ip, now) {
			t.Fatal("capacity reached early")
		}
		entry, ok := limiter.beginAccount(fmt.Sprint(index), now)
		if !ok {
			t.Fatal("account capacity reached early")
		}
		limiter.finishAccount(fmt.Sprint(index), entry, now, true, false)
	}
	if limiter.ip(netip.MustParseAddr("192.0.2.1"), now) {
		t.Fatal("capacity must not evict active IP limits")
	}
	if _, ok := limiter.beginAccount("overflow", now); ok {
		t.Fatal("capacity must not evict account failure counts")
	}
	if !limiter.ip(netip.MustParseAddr("192.0.2.1"), now.Add(time.Minute)) {
		t.Fatal("expired IP slots must be reclaimed")
	}
}
