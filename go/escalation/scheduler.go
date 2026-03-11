package escalation

import (
	"fmt"
	"github.com/saichler/l8notify/go/template"
	ntf "github.com/saichler/l8notify/go/types/l8notify"
	"sort"
	"sync"
	"time"
)

// StepHandler is called when an escalation step fires.
// Consumer provides this to customize delivery (e.g., look up SMTP config,
// create delivery log entries, update entity state).
type StepHandler func(entityID string, step *ntf.EscalationStep, message string) error

// Scheduler manages time-based escalation chains.
type Scheduler struct {
	handler StepHandler
	active  map[string]*escalationState
	mtx     sync.Mutex
}

type escalationState struct {
	entityID string
	timer    *time.Timer
	cancel   chan struct{}
}

// New creates a new escalation scheduler with the given step handler.
func New(handler StepHandler) *Scheduler {
	return &Scheduler{
		handler: handler,
		active:  make(map[string]*escalationState),
	}
}

// Schedule starts an escalation chain for the given entity.
// steps must be sorted by StepOrder. vars are template variables.
// If an escalation is already active for this entity, it is replaced.
func (s *Scheduler) Schedule(entityID string, steps []*ntf.EscalationStep, vars map[string]string) {
	if len(steps) == 0 {
		return
	}

	// Sort steps by order
	sorted := make([]*ntf.EscalationStep, len(steps))
	copy(sorted, steps)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StepOrder < sorted[j].StepOrder
	})

	s.Cancel(entityID)
	s.startStep(entityID, sorted, 0, vars)
}

// Cancel stops all pending escalation timers for the entity.
func (s *Scheduler) Cancel(entityID string) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if state, ok := s.active[entityID]; ok {
		close(state.cancel)
		state.timer.Stop()
		delete(s.active, entityID)
	}
}

// Active returns the number of entities with active escalation timers.
func (s *Scheduler) Active() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return len(s.active)
}

func (s *Scheduler) startStep(entityID string, steps []*ntf.EscalationStep, stepIdx int, vars map[string]string) {
	if stepIdx >= len(steps) {
		return
	}

	step := steps[stepIdx]
	delay := time.Duration(step.DelayMinutes) * time.Minute

	cancel := make(chan struct{})
	timer := time.NewTimer(delay)

	s.mtx.Lock()
	s.active[entityID] = &escalationState{
		entityID: entityID,
		timer:    timer,
		cancel:   cancel,
	}
	s.mtx.Unlock()

	go func() {
		select {
		case <-timer.C:
			s.fireStep(entityID, steps, stepIdx, vars)
		case <-cancel:
			timer.Stop()
		}
	}()
}

func (s *Scheduler) fireStep(entityID string, steps []*ntf.EscalationStep, stepIdx int, vars map[string]string) {
	step := steps[stepIdx]

	msg := step.MessageTemplate
	if msg != "" && len(vars) > 0 {
		msg = template.Render(msg, vars)
	}
	if msg == "" {
		msg = fmt.Sprintf("[ESCALATION] Entity %s - unresolved for %d minutes",
			entityID, step.DelayMinutes)
	}

	if err := s.handler(entityID, step, msg); err != nil {
		fmt.Printf("[escalation] step %d failed for entity %s: %v\n",
			step.StepOrder, entityID, err)
	}

	// Clean up current state
	s.mtx.Lock()
	delete(s.active, entityID)
	s.mtx.Unlock()

	// Schedule next step if available
	if stepIdx+1 < len(steps) {
		s.startStep(entityID, steps, stepIdx+1, vars)
	}
}
