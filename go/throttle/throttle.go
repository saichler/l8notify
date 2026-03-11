package throttle

import (
	"sync"
	"time"
)

// Throttler enforces per-key cooldown and hourly rate limits.
type Throttler struct {
	lastSent    map[string]int64
	hourlyCount map[string]*hourCounter
	mtx         sync.Mutex
}

type hourCounter struct {
	hour  int
	count int32
}

// New creates a new Throttler.
func New() *Throttler {
	return &Throttler{
		lastSent:    make(map[string]int64),
		hourlyCount: make(map[string]*hourCounter),
	}
}

// IsThrottled returns true if the key should be suppressed.
// cooldownSec: minimum seconds between sends for this key.
// maxPerHour: maximum sends per hour for this groupKey (0 = unlimited).
func (t *Throttler) IsThrottled(key string, groupKey string, cooldownSec, maxPerHour int32) bool {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	now := time.Now()

	// Check cooldown
	if cooldownSec > 0 {
		if lastSent, ok := t.lastSent[key]; ok {
			if now.Unix()-lastSent < int64(cooldownSec) {
				return true
			}
		}
	}

	// Check hourly limit
	if maxPerHour > 0 {
		currentHour := now.Hour()
		hc, ok := t.hourlyCount[groupKey]
		if !ok {
			hc = &hourCounter{hour: currentHour}
			t.hourlyCount[groupKey] = hc
		}
		if hc.hour != currentHour {
			hc.hour = currentHour
			hc.count = 0
		}
		if hc.count >= maxPerHour {
			return true
		}
	}

	return false
}

// Record marks a send for the given key and groupKey.
func (t *Throttler) Record(key string, groupKey string) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	now := time.Now()
	t.lastSent[key] = now.Unix()

	currentHour := now.Hour()
	hc, ok := t.hourlyCount[groupKey]
	if !ok {
		hc = &hourCounter{hour: currentHour}
		t.hourlyCount[groupKey] = hc
	}
	if hc.hour != currentHour {
		hc.hour = currentHour
		hc.count = 0
	}
	hc.count++
}

// Reset clears all state (useful for testing).
func (t *Throttler) Reset() {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.lastSent = make(map[string]int64)
	t.hourlyCount = make(map[string]*hourCounter)
}
