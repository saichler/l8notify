# L8Events — Shared Event, Alarm & Audit Library

## Purpose

Extract the generic event/alarm/archive/maintenance infrastructure from l8alarms into a shared library (`l8events`) that any Layer 8 project can consume. This provides every project with audit events, system events, alarm lifecycle management, archiving, and maintenance windows — without duplicating the l8alarms codebase.

---

## Current State

### l8alarms Has (to be extracted and generalized)
| Component | File | Generic? | Notes |
|-----------|------|----------|-------|
| Event model (immutable raw events) | `alm-events.proto` | Almost | Event types are alarm-centric (TRAP, SYSLOG); needs generic types |
| Alarm model (state machine lifecycle) | `alm-alarms.proto` | Partially | References topology (nodeId, nodeName); core state machine is generic |
| Archive engine (cascade archival) | `archiving/engine.go` | Pattern is generic | Types are alarm-specific |
| MaintenanceWindow model | `alm-maintenance.proto` | Pattern is generic | Scoping is alarm-specific (nodeId, nodeType) |
| AlarmFilter (saved views) | `alm-filters.proto` | Pattern is generic | Filter criteria are alarm-specific |
| Severity enum | `alm-common.proto` | Yes | INFO, WARNING, MINOR, MAJOR, CRITICAL — universal |
| State enum | `alm-common.proto` | Partially | ACTIVE, ACKNOWLEDGED, CLEARED, SUPPRESSED — generic lifecycle |

### Neither Project Has
| Component | Notes |
|-----------|-------|
| Generic audit event type | l8alarms Event is for monitoring events, not audit trails |
| Generic system event type | No shared "something happened in the system" event |
| Shared alarm UI components | Alarm table, alarm detail, state transition UI are in l8alarms only |
| Shared event viewer UI | Event table, event detail are in l8alarms only |
| Shared archive viewer UI | Archive browser is in l8alarms only |

---

## Architecture

### What Goes Into l8events

l8events is a **library**, not a standalone service. It provides:

1. **Proto types** — generic event, alarm, archive, maintenance window definitions
2. **Alarm state machine** — state transition validation and lifecycle management
3. **Archive engine** — generic move-to-archive pattern with cascade support
4. **Maintenance window engine** — suppression scheduling and evaluation
5. **Shared l8ui components** — reusable UI for event/alarm/archive viewing and management

### What Stays in Consumer Projects

Each consumer (l8alarms, l8erp) keeps:
- **Domain-specific event types** — l8alarms adds TRAP/SYSLOG/THRESHOLD; l8erp adds MODULE_EVENT/CRUD_EVENT/WORKFLOW_EVENT
- **Domain-specific alarm fields** — l8alarms adds topology fields (nodeId, correlation); l8erp adds module/service context
- **Domain-specific processing** — l8alarms adds correlation engine, RCA; l8erp adds business rule evaluation
- **Service definitions** — each project creates its own services using l8events' shared types as embedded building blocks
- **Policy matching** — alarm-specific filters stay in l8alarms; ERP-specific filters stay in l8erp

### Dependency Direction

```
l8events  (shared library — Go backend + l8ui components)
   ^             ^
   |             |
l8alarms      l8erp (and any future project)
```

l8events depends only on: Go stdlib (`sync`, `time`), `google.golang.org/protobuf`.

l8events does NOT depend on: `l8notify`, `l8orm`, `l8services`, `l8bus`, `l8web`, `l8reflect`, or any consumer project.

**Note**: l8events and l8notify are independent — neither depends on the other. Consumer projects may use both.

---

## Directory Structure

```
l8events/
├── proto/
│   ├── make-bindings.sh
│   └── l8events.proto                # Shared types (events, alarms, archive, maintenance)
├── go/
│   ├── go.mod
│   ├── types/
│   │   └── l8events/
│   │       └── l8events.pb.go        # Generated proto types
│   ├── state/
│   │   └── state.go                  # Alarm state machine (transition validation)
│   ├── archive/
│   │   └── archive.go                # Generic archive engine
│   └── maintenance/
│       └── maintenance.go            # Maintenance window evaluator
├── l8ui/
│   └── events/
│       ├── l8events-enums.js          # Shared enums (Severity, EventState, AlarmState, etc.)
│       ├── l8events-alarm-table.js    # Reusable alarm table with state badges
│       ├── l8events-alarm-detail.js   # Alarm detail popup (state history, notes, actions)
│       ├── l8events-event-viewer.js   # Event log viewer (filterable, read-only)
│       ├── l8events-archive-viewer.js # Archive browser (search, detail)
│       ├── l8events-maintenance.js    # Maintenance window form + calendar view
│       ├── l8events-state-actions.js  # Acknowledge/Clear/Suppress action buttons
│       └── l8events.css               # Shared event/alarm styles
├── plans/
├── LICENSE
├── README.md
└── .gitignore
```

---

## Protobuf Types

### `proto/l8events.proto`

```protobuf
syntax = "proto3";
package l8events;
option go_package = "./types/l8events";

// ─── Enums ───

// Severity levels — universal across all projects.
enum Severity {
  SEVERITY_UNSPECIFIED = 0;
  SEVERITY_INFO = 1;
  SEVERITY_WARNING = 2;
  SEVERITY_MINOR = 3;
  SEVERITY_MAJOR = 4;
  SEVERITY_CRITICAL = 5;
}

// Generic alarm lifecycle states.
enum AlarmState {
  ALARM_STATE_UNSPECIFIED = 0;
  ALARM_STATE_ACTIVE = 1;
  ALARM_STATE_ACKNOWLEDGED = 2;
  ALARM_STATE_CLEARED = 3;
  ALARM_STATE_SUPPRESSED = 4;
}

// Generic event processing states.
enum EventState {
  EVENT_STATE_UNSPECIFIED = 0;
  EVENT_STATE_NEW = 1;
  EVENT_STATE_PROCESSED = 2;
  EVENT_STATE_DISCARDED = 3;
  EVENT_STATE_ARCHIVED = 4;
}

// Event categories — generic, extensible via CUSTOM.
enum EventCategory {
  EVENT_CATEGORY_UNSPECIFIED = 0;
  EVENT_CATEGORY_AUDIT = 1;         // User action audit trail
  EVENT_CATEGORY_SYSTEM = 2;        // System/infrastructure event
  EVENT_CATEGORY_MONITORING = 3;    // Monitoring/telemetry event
  EVENT_CATEGORY_SECURITY = 4;      // Security event (login, access)
  EVENT_CATEGORY_INTEGRATION = 5;   // External system event
  EVENT_CATEGORY_CUSTOM = 6;        // Consumer-defined category
}

// Maintenance window status.
enum MaintenanceStatus {
  MAINTENANCE_STATUS_UNSPECIFIED = 0;
  MAINTENANCE_STATUS_SCHEDULED = 1;
  MAINTENANCE_STATUS_ACTIVE = 2;
  MAINTENANCE_STATUS_COMPLETED = 3;
  MAINTENANCE_STATUS_CANCELLED = 4;
}

// Maintenance recurrence type.
enum RecurrenceType {
  RECURRENCE_TYPE_UNSPECIFIED = 0;
  RECURRENCE_TYPE_NONE = 1;
  RECURRENCE_TYPE_DAILY = 2;
  RECURRENCE_TYPE_WEEKLY = 3;
  RECURRENCE_TYPE_MONTHLY = 4;
}

// ─── Shared Message Types ───

// A generic event occurrence.
// Consumers embed this in their own event types or use it directly.
// Immutable after creation — consumers should reject PUT on event services.
message EventRecord {
  string event_id = 1;
  EventCategory category = 2;
  string event_type = 3;            // Consumer-defined type string (e.g., "TRAP", "USER_LOGIN", "ORDER_CREATED")
  EventState state = 4;
  Severity severity = 5;
  string source_id = 6;             // ID of the entity that generated the event
  string source_name = 7;           // Human-readable source name
  string source_type = 8;           // Type of source (e.g., "Node", "User", "Service")
  string message = 9;
  map<string, string> attributes = 10;  // Extensible key-value pairs
  int64 occurred_at = 11;           // When the event happened (Unix timestamp)
  int64 received_at = 12;           // When the system received it
  int64 processed_at = 13;          // When processing completed
  string generated_alarm_id = 14;   // If this event generated an alarm
}

// A note/comment attached to an alarm.
message AlarmNote {
  string note_id = 1;
  string author = 2;
  string text = 3;
  int64 created_at = 4;
}

// A state transition record for alarm audit trail.
message AlarmStateChange {
  AlarmState from_state = 1;
  AlarmState to_state = 2;
  string changed_by = 3;
  string reason = 4;
  int64 changed_at = 5;
}

// A generic alarm instance — the core state machine.
// Consumers embed this in their own alarm types or extend it.
message AlarmRecord {
  string alarm_id = 1;
  string definition_id = 2;          // What rule/definition triggered this alarm
  string name = 3;
  string description = 4;
  AlarmState state = 5;
  Severity severity = 6;
  Severity original_severity = 7;
  string source_id = 8;              // Entity the alarm is about
  string source_name = 9;
  string source_type = 10;
  int64 first_occurrence = 11;
  int64 last_occurrence = 12;
  int32 occurrence_count = 13;
  string dedup_key = 14;             // Deduplication key
  string event_id = 15;              // Originating event
  string acknowledged_by = 16;
  int64 acknowledged_at = 17;
  string cleared_by = 18;
  int64 cleared_at = 19;
  bool is_suppressed = 20;
  string suppressed_by = 21;
  map<string, string> attributes = 22;
  repeated AlarmNote notes = 23;
  repeated AlarmStateChange state_history = 24;
}

// Archive metadata — added when an alarm or event is archived.
message ArchiveInfo {
  int64 archived_at = 1;
  string archived_by = 2;
  string archive_reason = 3;
}

// A maintenance window — period during which alarms/notifications are suppressed.
// Consumers store this in their own service; l8events provides the type and evaluator.
message MaintenanceWindow {
  string window_id = 1;
  string name = 2;
  string description = 3;
  MaintenanceStatus status = 4;
  int64 start_time = 5;
  int64 end_time = 6;
  string created_by = 7;
  int64 created_at = 8;
  RecurrenceType recurrence = 9;
  int32 recurrence_interval = 10;
  repeated string scope_ids = 11;         // Entity IDs this window applies to
  repeated string scope_types = 12;       // Entity types this window applies to
}
```

**Design choice**: `EventRecord` and `AlarmRecord` are shared building-block types. They are generic enough for any project but extensible via `attributes` map and consumer-defined `event_type`/`source_type` strings. Consumer projects can either use them directly or embed them in their own domain-specific types.

---

## Implementation Phases

### Phase 1: Proto Types & Project Scaffold

**1.1 Go module**
- Create `go/go.mod` with `module github.com/saichler/l8events`
- Minimal dependencies: `google.golang.org/protobuf`

**1.2 Proto file**
- Create `proto/l8events.proto` (as defined above)
- Create `proto/make-bindings.sh` (compile + move to `go/types/l8events/`)
- Run `make-bindings.sh`

**1.3 Verify**
- `cd go && go build ./...` — zero errors

**Files**: 3 new (go.mod, l8events.proto, make-bindings.sh) + 1 generated (l8events.pb.go)

### Phase 2: Alarm State Machine

**2.1 Create `go/state/state.go`**

Validates and enforces alarm state transitions. Consumer projects call this from their alarm service callbacks.

```go
package state

import evt "github.com/saichler/l8events/go/types/l8events"

// ValidTransition returns true if transitioning from → to is allowed.
// Valid transitions:
//   ACTIVE → ACKNOWLEDGED, CLEARED, SUPPRESSED
//   ACKNOWLEDGED → ACTIVE (re-activate), CLEARED, SUPPRESSED
//   SUPPRESSED → ACTIVE (unsuppress), ACKNOWLEDGED, CLEARED
//   CLEARED → (terminal, no transitions out)
func ValidTransition(from, to evt.AlarmState) bool

// Transition updates the alarm's state and appends a state history entry.
// Returns error if the transition is invalid.
func Transition(alarm *evt.AlarmRecord, newState evt.AlarmState, changedBy, reason string) error

// Acknowledge is a convenience for transitioning to ACKNOWLEDGED.
func Acknowledge(alarm *evt.AlarmRecord, acknowledgedBy string) error

// Clear is a convenience for transitioning to CLEARED.
func Clear(alarm *evt.AlarmRecord, clearedBy string) error

// Suppress is a convenience for transitioning to SUPPRESSED.
func Suppress(alarm *evt.AlarmRecord, suppressedBy string) error
```

**Files**: 1 new

### Phase 3: Archive Engine

**3.1 Create `go/archive/archive.go`**

Generic archive engine. Consumer projects provide callbacks for persistence operations.

```go
package archive

import evt "github.com/saichler/l8events/go/types/l8events"

// Store defines the persistence operations the consumer must provide.
type Store interface {
    // GetAlarm retrieves an alarm by ID.
    GetAlarm(alarmID string) (*evt.AlarmRecord, error)
    // SaveArchivedAlarm persists the archived alarm.
    SaveArchivedAlarm(alarm *evt.AlarmRecord, info *evt.ArchiveInfo) error
    // DeleteAlarm removes the active alarm after archiving.
    DeleteAlarm(alarmID string) error
    // GetEventsByAlarm retrieves events linked to an alarm.
    GetEventsByAlarm(alarmID string) ([]*evt.EventRecord, error)
    // SaveArchivedEvent persists the archived event.
    SaveArchivedEvent(event *evt.EventRecord, info *evt.ArchiveInfo) error
    // DeleteEvent removes the active event after archiving.
    DeleteEvent(eventID string) error
}

// Archiver manages alarm and event archival.
type Archiver struct { ... }

func New(store Store) *Archiver

// ArchiveAlarm archives an alarm and its associated events.
// Returns the ArchiveInfo used.
func (a *Archiver) ArchiveAlarm(alarmID, archivedBy, reason string) (*evt.ArchiveInfo, error)

// ArchiveEvent archives a single event.
func (a *Archiver) ArchiveEvent(eventID, archivedBy, reason string) (*evt.ArchiveInfo, error)
```

**Files**: 1 new

### Phase 4: Maintenance Window Evaluator

**4.1 Create `go/maintenance/maintenance.go`**

Evaluates whether an entity is currently under a maintenance window.

```go
package maintenance

import evt "github.com/saichler/l8events/go/types/l8events"

// Evaluator checks entities against active maintenance windows.
type Evaluator struct { ... }

func New() *Evaluator

// LoadWindows replaces the current set of active windows.
// Consumer calls this on startup and whenever windows change.
func (e *Evaluator) LoadWindows(windows []*evt.MaintenanceWindow)

// IsSuppressed returns true if the given entity (by ID or type) is covered
// by an active maintenance window at the current time.
func (e *Evaluator) IsSuppressed(entityID, entityType string) bool

// GetActiveWindow returns the active maintenance window covering the entity,
// or nil if none.
func (e *Evaluator) GetActiveWindow(entityID, entityType string) *evt.MaintenanceWindow
```

**Files**: 1 new

### Phase 5: Shared l8ui Event/Alarm Components

These components live in `l8events/l8ui/events/` and are copied into each consumer project's `l8ui/events/` directory. They follow l8ui conventions: `--layer8d-*` CSS tokens, `Layer8`/`L8Events` global namespace.

**5.1 Shared enums — `l8events-enums.js`**

Mirrors proto enums for JS:

```javascript
window.L8EventsEnums = {
    SEVERITY: Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Info' },
        { value: 2, label: 'Warning' },
        { value: 3, label: 'Minor' },
        { value: 4, label: 'Major' },
        { value: 5, label: 'Critical' }
    ]),
    ALARM_STATE: Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Active' },
        { value: 2, label: 'Acknowledged' },
        { value: 3, label: 'Cleared' },
        { value: 4, label: 'Suppressed' }
    ]),
    EVENT_STATE: Layer8EnumFactory.create([...]),
    EVENT_CATEGORY: Layer8EnumFactory.create([...]),
    MAINTENANCE_STATUS: Layer8EnumFactory.create([...]),
    RECURRENCE_TYPE: Layer8EnumFactory.create([...])
};
```

Includes renderers:
- `L8EventsEnums.render.severity` — color-coded severity badge (Critical=red, Major=orange, Minor=yellow, Warning=blue, Info=gray)
- `L8EventsEnums.render.alarmState` — state badge (Active=red, Acknowledged=blue, Cleared=green, Suppressed=gray)
- `L8EventsEnums.render.eventState` — state badge
- `L8EventsEnums.render.eventCategory` — plain enum label
- `L8EventsEnums.render.maintenanceStatus` — status badge

**5.2 Alarm table component — `l8events-alarm-table.js`**

Reusable alarm table with severity/state badges and row coloring. Consumer projects use this instead of building custom alarm tables.

```javascript
window.L8EventsAlarmTable = {
    // Returns column definitions for alarm tables.
    // Consumer can merge with domain-specific columns.
    getColumns: function() { ... },

    // Returns form definition for alarm detail popup.
    // Includes state history timeline and notes section.
    getFormDefinition: function() { ... }
};
```

Columns: Severity (badge), Name, Source, State (badge), First Occurrence, Last Occurrence, Occurrence Count, Acknowledged By.

**5.3 Alarm detail popup — `l8events-alarm-detail.js`**

Renders alarm detail with state history timeline and notes.

```javascript
window.L8EventsAlarmDetail = {
    // Renders alarm detail inside a popup body.
    // alarm: AlarmRecord object.
    // options: { showStateHistory: bool, showNotes: bool, onStateChange: callback }
    render: function(container, alarm, options) { ... }
};
```

Includes:
- Alarm fields display (severity badge, source, timestamps)
- State history timeline (visual timeline of state transitions)
- Notes section (list of AlarmNote entries)

**5.4 Event viewer — `l8events-event-viewer.js`**

Read-only event log viewer with category/severity filtering.

```javascript
window.L8EventsEventViewer = {
    // Returns column definitions for event tables.
    getColumns: function() { ... },

    // Returns form definition for event detail popup.
    getFormDefinition: function() { ... }
};
```

Columns: Timestamp, Category (badge), Type, Severity (badge), Source, Message, State (badge).

**5.5 Archive viewer — `l8events-archive-viewer.js`**

Browse archived alarms and events.

```javascript
window.L8EventsArchiveViewer = {
    // Returns column definitions for archived alarm table.
    getArchivedAlarmColumns: function() { ... },

    // Returns column definitions for archived event table.
    getArchivedEventColumns: function() { ... }
};
```

Adds archive-specific columns: Archived At, Archived By, Archive Reason.

**5.6 Maintenance window form — `l8events-maintenance.js`**

Form component for creating/editing maintenance windows.

```javascript
window.L8EventsMaintenance = {
    // Returns form definition for maintenance window create/edit.
    getFormDefinition: function() { ... },

    // Returns column definitions for maintenance window table.
    getColumns: function() { ... }
};
```

Form fields: Name, Description, Status, Start Time, End Time, Recurrence Type, Recurrence Interval, Scope IDs, Scope Types.

**5.7 State action buttons — `l8events-state-actions.js`**

Reusable Acknowledge/Clear/Suppress action buttons for alarm rows and detail popups.

```javascript
window.L8EventsStateActions = {
    // Renders action buttons appropriate for the alarm's current state.
    // onAction: callback(alarmId, newState, reason) called when user clicks.
    render: function(container, alarm, onAction) { ... },

    // Returns which actions are available for a given state.
    getAvailableActions: function(currentState) { ... }
};
```

Actions by state:
- ACTIVE → Acknowledge, Clear, Suppress
- ACKNOWLEDGED → Clear, Suppress
- SUPPRESSED → Acknowledge, Clear
- CLEARED → (none — terminal)

**5.8 CSS — `l8events.css`**

Shared styles for all event/alarm components. Uses `--layer8d-*` theme tokens exclusively. Covers:
- Severity badge colors (Critical=`--layer8d-error`, Major=orange, Minor=yellow, Warning=`--layer8d-warning`, Info=`--layer8d-text-muted`)
- Alarm state badge colors
- State history timeline styling
- Action button styling (Acknowledge=blue, Clear=green, Suppress=gray)
- Maintenance window calendar highlight

**Files**: 8 new (7 JS + 1 CSS)

### Phase 6: Build Verification

**6.1 l8events Go backend**
- `cd go && go build ./...` — library compiles
- `go vet ./...` — no issues

**6.2 l8ui components**
- Verify no JS syntax errors: `node -c` on all 7 JS files
- Verify all global objects are defined on `window`
- Verify CSS uses only `--layer8d-*` tokens, no hardcoded colors
- Verify no `[data-theme="dark"]` blocks in CSS

---

## Traceability Matrix

| # | Gap / Action Item | Phase |
|---|-------------------|-------|
| 1 | No shared proto types for events/alarms/archive | Phase 1 |
| 2 | No shared alarm state machine (transition validation) | Phase 2 |
| 3 | No shared archive engine (cascade archival pattern) | Phase 3 |
| 4 | No shared maintenance window evaluator | Phase 4 |
| 5 | No shared JS enums for Severity/AlarmState/EventState | Phase 5.1 |
| 6 | No reusable alarm table UI component | Phase 5.2 |
| 7 | No reusable alarm detail popup (state history, notes) | Phase 5.3 |
| 8 | No reusable event viewer UI component | Phase 5.4 |
| 9 | No reusable archive browser UI component | Phase 5.5 |
| 10 | No reusable maintenance window form UI component | Phase 5.6 |
| 11 | No reusable alarm state action buttons | Phase 5.7 |
| 12 | Each consumer would duplicate event/alarm admin UI | Phase 5 |
| 13 | No build verification | Phase 6 |

**Note**: Migration of l8alarms and l8erp to consume l8events is covered in the separate `PLAN-L8ALARMS.md` and respective consumer project plans.

---

## File Summary

| Category | New Files | Modified Files |
|----------|-----------|----------------|
| l8events proto | 2 (l8events.proto, make-bindings.sh) | 0 |
| l8events Go | 4 (go.mod, state.go, archive.go, maintenance.go) | 0 |
| l8events generated | 1 (l8events.pb.go) | 0 |
| l8events l8ui | 8 (l8events-enums.js, l8events-alarm-table.js, l8events-alarm-detail.js, l8events-event-viewer.js, l8events-archive-viewer.js, l8events-maintenance.js, l8events-state-actions.js, l8events.css) | 0 |
| **Total** | **15 new** | **0 modified** |

---

## API Summary (Consumer Cheat Sheet)

### Go Backend

```go
import (
    "github.com/saichler/l8events/go/state"
    "github.com/saichler/l8events/go/archive"
    "github.com/saichler/l8events/go/maintenance"
    evt "github.com/saichler/l8events/go/types/l8events"
)

// Alarm state transitions
err := state.Acknowledge(alarm, "admin")
err := state.Clear(alarm, "system")
err := state.Suppress(alarm, "maintenance-window-1")
valid := state.ValidTransition(evt.ALARM_STATE_ACTIVE, evt.ALARM_STATE_ACKNOWLEDGED) // true

// Archiving
archiver := archive.New(myStore)  // myStore implements archive.Store
info, err := archiver.ArchiveAlarm("alarm-123", "admin", "resolved")

// Maintenance windows
eval := maintenance.New()
eval.LoadWindows(activeWindows)
if eval.IsSuppressed("node-456", "Router") {
    // Skip alarm creation or suppress notification
}
```

### l8ui Components

```html
<!-- Add to app.html (after l8ui shared scripts, before module scripts) -->
<link rel="stylesheet" href="l8ui/events/l8events.css">
<script src="l8ui/events/l8events-enums.js"></script>
<script src="l8ui/events/l8events-alarm-table.js"></script>
<script src="l8ui/events/l8events-alarm-detail.js"></script>
<script src="l8ui/events/l8events-event-viewer.js"></script>
<script src="l8ui/events/l8events-archive-viewer.js"></script>
<script src="l8ui/events/l8events-maintenance.js"></script>
<script src="l8ui/events/l8events-state-actions.js"></script>
```

```javascript
// Use shared columns in module column definitions
const alarmColumns = L8EventsAlarmTable.getColumns();
const eventColumns = L8EventsEventViewer.getColumns();

// Use shared enums in column/form definitions
...col.status('severity', 'Severity', null, L8EventsEnums.render.severity)
...col.status('state', 'State', null, L8EventsEnums.render.alarmState)
...f.select('severity', 'Severity', L8EventsEnums.SEVERITY)

// Alarm detail in popup
L8EventsAlarmDetail.render(popupBody, alarm, {
    showStateHistory: true,
    showNotes: true,
    onStateChange: (alarmId, newState, reason) => { /* POST state change */ }
});

// State action buttons
L8EventsStateActions.render(buttonContainer, alarm, (alarmId, newState, reason) => {
    // POST to consumer's alarm service
});

// Maintenance window form
const maintForm = L8EventsMaintenance.getFormDefinition();
```
