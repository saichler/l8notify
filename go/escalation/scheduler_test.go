package escalation

import (
	ntf "github.com/saichler/l8types/go/types/l8notify"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		return nil
	})
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.Active() != 0 {
		t.Errorf("expected 0 active, got %d", s.Active())
	}
}

func TestSchedule_EmptySteps(t *testing.T) {
	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		return nil
	})
	s.Schedule("entity-1", nil, nil)
	if s.Active() != 0 {
		t.Errorf("expected 0 active for empty steps, got %d", s.Active())
	}
}

func TestSchedule_SingleStep(t *testing.T) {
	var mu sync.Mutex
	var firedEntity string
	var firedStep *ntf.EscalationStep
	var firedMsg string
	done := make(chan struct{})

	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		mu.Lock()
		firedEntity = entityID
		firedStep = step
		firedMsg = message
		mu.Unlock()
		close(done)
		return nil
	})

	steps := []*ntf.EscalationStep{
		{
			StepId:          "step-1",
			StepOrder:       1,
			DelayMinutes:    0, // fires immediately (0 minute delay)
			Channel:         ntf.NotifyChannel_NOTIFY_CHANNEL_EMAIL,
			Endpoint:        "admin@test.com",
			MessageTemplate: "Alert for {{entity}}",
		},
	}

	s.Schedule("alarm-123", steps, map[string]string{"entity": "server-1"})

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("step did not fire within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if firedEntity != "alarm-123" {
		t.Errorf("expected entity 'alarm-123', got %q", firedEntity)
	}
	if firedStep.StepId != "step-1" {
		t.Errorf("expected step 'step-1', got %q", firedStep.StepId)
	}
	if firedMsg != "Alert for server-1" {
		t.Errorf("expected rendered message, got %q", firedMsg)
	}
}

func TestSchedule_MultipleSteps_OrderedByStepOrder(t *testing.T) {
	var mu sync.Mutex
	var firedOrder []int32
	done := make(chan struct{}, 2)

	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		mu.Lock()
		firedOrder = append(firedOrder, step.StepOrder)
		mu.Unlock()
		done <- struct{}{}
		return nil
	})

	// Steps given out of order — scheduler should sort them
	steps := []*ntf.EscalationStep{
		{StepId: "step-2", StepOrder: 2, DelayMinutes: 0},
		{StepId: "step-1", StepOrder: 1, DelayMinutes: 0},
	}

	s.Schedule("entity-1", steps, nil)

	// Wait for both steps
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("steps did not fire within timeout")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(firedOrder) != 2 {
		t.Fatalf("expected 2 steps fired, got %d", len(firedOrder))
	}
	if firedOrder[0] != 1 || firedOrder[1] != 2 {
		t.Errorf("expected order [1, 2], got %v", firedOrder)
	}
}

func TestCancel_StopsEscalation(t *testing.T) {
	fired := false
	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		fired = true
		return nil
	})

	steps := []*ntf.EscalationStep{
		{StepId: "step-1", StepOrder: 1, DelayMinutes: 60}, // 60 min delay — should never fire
	}

	s.Schedule("entity-1", steps, nil)
	if s.Active() != 1 {
		t.Errorf("expected 1 active, got %d", s.Active())
	}

	s.Cancel("entity-1")
	if s.Active() != 0 {
		t.Errorf("expected 0 active after cancel, got %d", s.Active())
	}

	// Give some time to ensure step doesn't fire
	time.Sleep(100 * time.Millisecond)
	if fired {
		t.Error("handler should not have fired after cancel")
	}
}

func TestCancel_NonexistentEntity(t *testing.T) {
	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		return nil
	})
	// Should not panic
	s.Cancel("nonexistent")
}

func TestSchedule_ReplacesExisting(t *testing.T) {
	var mu sync.Mutex
	var firedSteps []string
	done := make(chan struct{}, 1)

	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		mu.Lock()
		firedSteps = append(firedSteps, step.StepId)
		mu.Unlock()
		done <- struct{}{}
		return nil
	})

	// Schedule first escalation with long delay
	steps1 := []*ntf.EscalationStep{
		{StepId: "old-step", StepOrder: 1, DelayMinutes: 60},
	}
	s.Schedule("entity-1", steps1, nil)

	// Replace with immediate escalation
	steps2 := []*ntf.EscalationStep{
		{StepId: "new-step", StepOrder: 1, DelayMinutes: 0},
	}
	s.Schedule("entity-1", steps2, nil)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement step did not fire")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(firedSteps) != 1 || firedSteps[0] != "new-step" {
		t.Errorf("expected only 'new-step' to fire, got %v", firedSteps)
	}
}

func TestSchedule_DefaultMessage(t *testing.T) {
	var firedMsg string
	done := make(chan struct{})

	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		firedMsg = message
		close(done)
		return nil
	})

	// Step with empty template — should get default message
	steps := []*ntf.EscalationStep{
		{StepId: "step-1", StepOrder: 1, DelayMinutes: 0, MessageTemplate: ""},
	}
	s.Schedule("entity-42", steps, nil)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("step did not fire")
	}

	if firedMsg == "" {
		t.Error("expected non-empty default message")
	}
}

func TestActive_MultipleEntities(t *testing.T) {
	s := New(func(entityID string, step *ntf.EscalationStep, message string) error {
		return nil
	})

	longDelay := []*ntf.EscalationStep{
		{StepId: "s1", StepOrder: 1, DelayMinutes: 60},
	}

	s.Schedule("entity-1", longDelay, nil)
	s.Schedule("entity-2", longDelay, nil)
	s.Schedule("entity-3", longDelay, nil)

	if s.Active() != 3 {
		t.Errorf("expected 3 active, got %d", s.Active())
	}

	s.Cancel("entity-2")
	if s.Active() != 2 {
		t.Errorf("expected 2 active after cancel, got %d", s.Active())
	}

	s.Cancel("entity-1")
	s.Cancel("entity-3")
	if s.Active() != 0 {
		t.Errorf("expected 0 active, got %d", s.Active())
	}
}
