package throttle

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	th := New()
	if th == nil {
		t.Fatal("New() returned nil")
	}
}

func TestIsThrottled_NoCooldown_NoLimit(t *testing.T) {
	th := New()
	if th.IsThrottled("key1", "group1", 0, 0) {
		t.Error("should not be throttled with no cooldown and no limit")
	}
}

func TestIsThrottled_Cooldown(t *testing.T) {
	th := New()
	th.Record("key1", "group1")

	// Should be throttled within cooldown period
	if !th.IsThrottled("key1", "group1", 60, 0) {
		t.Error("should be throttled within cooldown period")
	}

	// Different key should not be throttled
	if th.IsThrottled("key2", "group1", 60, 0) {
		t.Error("different key should not be throttled")
	}
}

func TestIsThrottled_HourlyLimit(t *testing.T) {
	th := New()
	maxPerHour := int32(3)

	for i := int32(0); i < maxPerHour; i++ {
		if th.IsThrottled("key1", "group1", 0, maxPerHour) {
			t.Errorf("should not be throttled at count %d (limit %d)", i, maxPerHour)
		}
		th.Record("key1", "group1")
	}

	// Should be throttled after reaching limit
	if !th.IsThrottled("key1", "group1", 0, maxPerHour) {
		t.Error("should be throttled after reaching hourly limit")
	}

	// Different group should not be throttled
	if th.IsThrottled("key1", "group2", 0, maxPerHour) {
		t.Error("different group should not be throttled")
	}
}

func TestRecord(t *testing.T) {
	th := New()
	th.Record("key1", "group1")

	th.mtx.Lock()
	_, hasLastSent := th.lastSent["key1"]
	hc, hasHourly := th.hourlyCount["group1"]
	th.mtx.Unlock()

	if !hasLastSent {
		t.Error("Record should set lastSent for key")
	}
	if !hasHourly {
		t.Fatal("Record should set hourlyCount for group")
	}
	if hc.count != 1 {
		t.Errorf("expected hourly count 1, got %d", hc.count)
	}
}

func TestRecord_IncrementsCount(t *testing.T) {
	th := New()
	th.Record("key1", "group1")
	th.Record("key2", "group1")
	th.Record("key3", "group1")

	th.mtx.Lock()
	count := th.hourlyCount["group1"].count
	th.mtx.Unlock()

	if count != 3 {
		t.Errorf("expected hourly count 3, got %d", count)
	}
}

func TestReset(t *testing.T) {
	th := New()
	th.Record("key1", "group1")
	th.Record("key2", "group2")
	th.Reset()

	th.mtx.Lock()
	lastSentLen := len(th.lastSent)
	hourlyLen := len(th.hourlyCount)
	th.mtx.Unlock()

	if lastSentLen != 0 {
		t.Error("Reset should clear lastSent")
	}
	if hourlyLen != 0 {
		t.Error("Reset should clear hourlyCount")
	}
}

func TestIsThrottled_CooldownExpired(t *testing.T) {
	th := New()

	// Manually set lastSent to 2 seconds ago
	th.mtx.Lock()
	th.lastSent["key1"] = time.Now().Unix() - 2
	th.mtx.Unlock()

	// 1 second cooldown should have expired
	if th.IsThrottled("key1", "group1", 1, 0) {
		t.Error("should not be throttled after cooldown expired")
	}
}

func TestHourlyCounter_ResetsOnNewHour(t *testing.T) {
	th := New()

	// Manually set hourly counter with a different hour
	th.mtx.Lock()
	th.hourlyCount["group1"] = &hourCounter{
		hour:  (time.Now().Hour() + 23) % 24, // previous hour
		count: 999,
	}
	th.mtx.Unlock()

	// Should not be throttled because hour changed (counter resets)
	if th.IsThrottled("key1", "group1", 0, 5) {
		t.Error("should not be throttled after hour change")
	}
}
