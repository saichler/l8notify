# L8Alarms — Alarm Management System (Refactored)

## Purpose

Refactor l8alarms to consume the shared `l8notify` and `l8events` libraries, removing duplicated code and retaining only alarm-specific functionality: correlation engine, topology enrichment, alarm definitions, and alarm-specific policies/filters.

---

## Prerequisites

Both shared libraries must be implemented and published before this plan executes:
- **l8notify** — notification channels, templates, throttling, escalation, notification UI (see `PLAN-L8NOTIFY-SHARED-LIBRARY.md`)
- **l8events** — generic events, alarm state machine, archiving, maintenance windows, event/alarm UI (see `PLAN-L8EVENTS-SHARED-LIBRARY.md`)

---

## Current State

### l8alarms Today
| Component | Lines | Status |
|-----------|-------|--------|
| 10 services (Alarm, Event, AlarmDefinition, CorrelationRule, NotificationPolicy, EscalationPolicy, MaintenanceWindow, AlarmFilter, ArchivedAlarm, ArchivedEvent) | ~2000 | Working |
| Correlation engine (4 strategies: topological, temporal, pattern, composite) | ~400 | Working — alarm-specific |
| Notification engine (policy matching, throttling, dispatch) | ~250 | Working — **to be replaced by l8notify** |
| Escalation scheduler (timer-based step chains) | ~180 | Working — **to be replaced by l8notify** |
| Archiving engine (cascade archival) | ~100 | Working — **to be replaced by l8events** |
| 9 proto files + common enums | ~600 | Working — **partially replaced by l8events/l8notify** |
| UI (desktop + mobile) | ~2000 | Working |

### After Refactoring
| Component | Source | Notes |
|-----------|--------|-------|
| Generic event/alarm types | **l8events** | EventRecord, AlarmRecord, AlarmState, Severity, etc. |
| Alarm state machine | **l8events** | state.Transition(), state.Acknowledge(), etc. |
| Archive engine | **l8events** | archive.New(store) |
| Maintenance window evaluator | **l8events** | maintenance.New(), IsSuppressed() |
| Notification dispatch | **l8notify** | channel.Dispatch(), channel.SendWebhook(), etc. |
| Template rendering | **l8notify** | template.Render() |
| Throttling | **l8notify** | throttle.New(), IsThrottled() |
| Escalation scheduling | **l8notify** | escalation.New(handler) |
| Notification channel enums | **l8notify** | NotifyChannel, NotifyTarget |
| Shared alarm UI (enums, tables, detail) | **l8events l8ui** | L8EventsEnums, L8EventsAlarmTable, etc. |
| Shared notification UI (SMTP, webhook) | **l8notify l8ui** | L8NotifySmtpConfig, L8NotifyWebhookMgmt, etc. |
| Correlation engine | **stays in l8alarms** | Alarm-specific RCA logic |
| Topology enrichment | **stays in l8alarms** | l8topology integration |
| AlarmDefinition service | **stays in l8alarms** | Alarm-specific rule definitions |
| Alarm-specific filters | **stays in l8alarms** | AlarmFilter with alarm-specific criteria |
| Correlation UI | **stays in l8alarms** | Correlation tree visualization |

---

## Architecture After Refactoring

```
l8notify (channels, templates, throttle, escalation)
   ^
   |
l8events (event/alarm types, state machine, archive, maintenance)
   ^
   |
l8alarms (correlation, topology enrichment, alarm definitions, alarm-specific policies)
```

### What Stays in l8alarms

**Services** (alarm-specific):
- `AlarmDefinition` — alarm template definitions (event patterns, thresholds, auto-clear)
- `Alarm` — extends `l8events.AlarmRecord` with topology fields + correlation results
- `Event` — extends `l8events.EventRecord` with alarm-specific event types
- `CorrelationRule` — alarm correlation rules (topological, temporal, pattern)
- `NotificationPolicy` — alarm-specific policy (severity filters, definition ID filters, node type filters) + embeds `l8notify.NotifyTarget`
- `EscalationPolicy` — alarm-specific policy (severity/definition filters) + embeds `l8notify.EscalationStep`
- `AlarmFilter` — saved alarm filter views
- `ArchivedAlarm` — uses `l8events.ArchiveInfo`
- `ArchivedEvent` — uses `l8events.ArchiveInfo`
- `MaintenanceWindow` — uses `l8events.MaintenanceWindow` base type
- `EnrichmentService` — topology overlay (read-only)

**Engines** (alarm-specific):
- `correlation/` — 4 strategies (topological, temporal, pattern, composite) + condition evaluator

**Removed from l8alarms** (replaced by libraries):
- `notification/engine.go` → replaced by l8notify throttle + channel dispatch
- `notification/sender.go` → replaced by l8notify channel senders
- `escalation/scheduler.go` → replaced by l8notify escalation scheduler
- `archiving/engine.go` → replaced by l8events archive engine
- Duplicate enums (NotificationChannel, PolicyStatus) → replaced by l8notify/l8events enums

---

## Proto Changes

### `alm-common.proto` — Remove duplicated enums

**Remove** (now in l8events):
- `AlarmSeverity` → use `l8events.Severity`
- `AlarmState` → use `l8events.AlarmState`
- `EventType` → keep alarm-specific event types BUT based on `l8events.EventCategory`
- `EventProcessingState` → use `l8events.EventState`
- `MaintenanceWindowStatus` → use `l8events.MaintenanceStatus`
- `RecurrenceType` → use `l8events.RecurrenceType`

**Remove** (now in l8notify):
- `NotificationChannel` → use `l8notify.NotifyChannel`
- `PolicyStatus` → keep as `AlmPolicyStatus` (or use a generic one)

**Keep** (alarm-specific):
- `AlarmDefinitionStatus` (DRAFT, ACTIVE, DISABLED)
- `CorrelationRuleType` (TOPOLOGICAL, TEMPORAL, PATTERN, COMPOSITE)
- `CorrelationRuleStatus` (DRAFT, ACTIVE, DISABLED)
- `TraversalDirection` (UPSTREAM, DOWNSTREAM, BOTH)
- `ConditionOperator` (EQUALS, NOT_EQUALS, CONTAINS, REGEX, etc.)

### `alm-alarms.proto` — Use l8events base types

```protobuf
import "l8events.proto";

message Alarm {
  // Embed generic alarm fields from l8events
  l8events.AlarmRecord base = 1;

  // Alarm-specific topology fields
  string node_id = 2;
  string node_name = 3;
  string link_id = 4;
  string location = 5;

  // Alarm-specific correlation/RCA fields
  string root_cause_alarm_id = 6;
  string correlation_rule_id = 7;
  bool is_root_cause = 8;
  int32 symptom_count = 9;
}
```

### `alm-policies.proto` — Use l8notify types

```protobuf
import "l8notify.proto";

message NotificationPolicy {
  string policy_id = 1;
  string name = 2;
  string description = 3;
  AlmPolicyStatus status = 4;

  // Alarm-specific filter criteria (stays in l8alarms)
  l8events.Severity min_severity = 5;
  repeated string alarm_definition_ids = 6;
  repeated string node_type_filter = 7;
  bool notify_on_state_change = 8;

  // Throttling config (values passed to l8notify at runtime)
  int32 cooldown_seconds = 9;
  int32 max_notifications_per_hour = 10;

  // Shared notification targets (from l8notify)
  repeated l8notify.NotifyTarget targets = 11;

  int64 created_at = 12;
  int64 updated_at = 13;
}

message EscalationPolicy {
  string policy_id = 1;
  string name = 2;
  string description = 3;
  AlmPolicyStatus status = 4;

  // Alarm-specific filter criteria
  l8events.Severity min_severity = 5;
  repeated string alarm_definition_ids = 6;

  // Shared escalation steps (from l8notify)
  repeated l8notify.EscalationStep steps = 7;

  int64 created_at = 8;
  int64 updated_at = 9;
}
```

---

## Implementation Phases

### Phase 1: Update Dependencies

**1.1 Update go.mod**
- Add `require github.com/saichler/l8notify`
- Add `require github.com/saichler/l8events`
- `go mod tidy && go mod vendor`

**1.2 Update proto imports**
- Add `import "l8events.proto"` and `import "l8notify.proto"` to relevant proto files
- Download shared proto files for compilation
- Update `make-bindings.sh` to handle imports

**Files**: 2 modified (go.mod, make-bindings.sh)

### Phase 2: Replace Enums with Shared Types

**2.1 Update `alm-common.proto`**
- Remove `AlarmSeverity` → consumers use `l8events.Severity`
- Remove `AlarmState` → consumers use `l8events.AlarmState`
- Remove `EventProcessingState` → consumers use `l8events.EventState`
- Remove `NotificationChannel` → consumers use `l8notify.NotifyChannel`
- Remove `MaintenanceWindowStatus` → consumers use `l8events.MaintenanceStatus`
- Remove `RecurrenceType` → consumers use `l8events.RecurrenceType`
- Keep alarm-specific enums (AlarmDefinitionStatus, CorrelationRuleType, etc.)

**2.2 Update all proto files** that reference removed enums
- `alm-alarms.proto`: `AlarmSeverity` → `l8events.Severity`, `AlarmState` → `l8events.AlarmState`
- `alm-events.proto`: `EventProcessingState` → `l8events.EventState`
- `alm-policies.proto`: `NotificationChannel` → `l8notify.NotifyChannel`
- `alm-maintenance.proto`: `MaintenanceWindowStatus` → `l8events.MaintenanceStatus`

**2.3 Regenerate bindings**
- `cd proto && ./make-bindings.sh`

**2.4 Update Go code** to use new enum paths
- All service files, callbacks, engines, and common code

**Files**: ~15 modified (proto files + all Go files referencing enums)

### Phase 3: Replace Notification Engine with l8notify

**3.1 Replace `notification/sender.go`**
- Delete custom `sendWebhook`, `sendSlack`, `sendLog` functions
- Replace with calls to `l8notify/channel`:
  - `channel.SendWebhookSimple(endpoint, message)`
  - `channel.SendSlack(webhookURL, message)`
  - `channel.SendEmail(smtpCfg, to, subject, body)`

**3.2 Replace `notification/engine.go` template rendering**
- Replace `renderTemplate(tmpl, alarm)` with:
  ```go
  vars := map[string]string{
      "alarm.id": alarm.Base.AlarmId,
      "alarm.name": alarm.Base.Name,
      "alarm.severity": alarm.Base.Severity.String(),
      "alarm.state": alarm.Base.State.String(),
      "alarm.nodeId": alarm.NodeId,
      "alarm.nodeName": alarm.NodeName,
      "alarm.location": alarm.Location,
      "alarm.description": alarm.Base.Description,
  }
  msg := template.Render(target.Template, vars)
  ```
- Delete `replaceAll()` and `indexOf()` helper functions

**3.3 Replace throttle state**
- Replace `throttle map[string]int64` and `hourlyCount` with `throttle.New()`
- `isThrottled()` → `t.IsThrottled(key, groupKey, cooldownSec, maxPerHour)`
- `recordSend()` → `t.Record(key, groupKey)`

**3.4 Replace `notification/engine.go` policy evaluation**
- Keep alarm-specific filter matching logic (severity, definition IDs, node types)
- Replace dispatch calls with `channel.Dispatch(target, message, smtpCfg, webhookSecrets)`

**Files**: 2 modified (engine.go, sender.go — significantly simplified)

### Phase 4: Replace Escalation Scheduler with l8notify

**4.1 Replace `escalation/scheduler.go`**
- Replace with thin wrapper around `l8notify/escalation.New(handler)`:
  ```go
  handler := func(entityID string, step *ntf.EscalationStep, msg string) error {
      // Use l8notify channel dispatch
      return channel.Dispatch(targetFromStep(step), msg, smtpCfg, nil)
  }
  scheduler := escalation.New(handler)
  ```
- Keep alarm-specific policy matching (severity filter, definition ID filter)
- `Schedule()` and `Cancel()` delegate to l8notify scheduler

**Files**: 1 modified (scheduler.go — significantly simplified)

### Phase 5: Replace Archiving Engine with l8events

**5.1 Replace `archiving/engine.go`**
- Implement `l8events/archive.Store` interface backed by l8alarms' services
- Replace custom archive logic with `l8events/archive.New(store)`
- Keep alarm-specific cascade logic (root cause → symptoms) as a wrapper

**Files**: 1 modified (engine.go — simplified to adapter pattern)

### Phase 6: Replace Maintenance Window with l8events

**6.1 Update MaintenanceWindow service**
- Use `l8events.MaintenanceWindow` as base type
- Keep alarm-specific scoping (node_id, node_type mapping to l8events' scope_ids, scope_types)
- Replace custom `isUnderMaintenance()` with `l8events/maintenance.Evaluator.IsSuppressed()`

**Files**: 2 modified (service + callback)

### Phase 7: Update Alarm Service to Use l8events State Machine

**7.1 Update AlarmService callback**
- Use `l8events/state.Transition()` for all state changes
- Use `l8events/state.ValidTransition()` for validation in Before() hook
- Use `l8events/state.Acknowledge()`, `Clear()`, `Suppress()` convenience methods
- Keep alarm-specific Before/After hooks (correlation, notification, escalation)

**Files**: 1 modified (AlarmServiceCallback.go)

### Phase 8: Update UI to Use Shared l8ui Components

**8.1 Copy shared l8ui components**
- Copy `l8notify/l8ui/notification/` into `l8alarms/go/alm/ui/web/l8ui/notification/`
- Copy `l8events/l8ui/events/` into `l8alarms/go/alm/ui/web/l8ui/events/`

**8.2 Update `app.html`**
- Add CSS includes for shared components
- Add JS includes for shared components (after l8ui core, before alm module scripts)

**8.3 Update alarm enums** (`alm/alarms/alarms-enums.js`)
- Remove duplicated severity/state enum definitions
- Reference `L8EventsEnums.SEVERITY`, `L8EventsEnums.ALARM_STATE` instead
- Keep alarm-specific renderers that add l8alarms-specific styling

**8.4 Update alarm columns** (`alm/alarms/alarms-columns.js`)
- Use `L8EventsEnums.render.severity` and `L8EventsEnums.render.alarmState`
- Keep alarm-specific columns (nodeId, nodeName, correlationRuleId, etc.)

**8.5 Update alarm forms** (`alm/alarms/alarms-forms.js`)
- Use `L8EventsEnums.SEVERITY`, `L8EventsEnums.ALARM_STATE` for select fields
- Keep alarm-specific form sections (topology, correlation)

**8.6 Update policy forms** (`alm/policies/policies-forms.js`)
- Use `L8NotifyTargetEditor.getInlineTableDef()` for notification targets
- Use `L8NotifyEnums.NOTIFY_CHANNEL` for channel select fields
- Keep alarm-specific filter fields (severity, definition IDs, node types)

**8.7 Update mobile UI** (`m/`)
- Same changes as desktop: replace duplicated enums with shared ones
- Add shared l8ui component includes to `m/app.html`

**Files**: ~12 modified (app.html, m/app.html, enums, columns, forms for alarms + policies)

### Phase 9: Build Verification & Regression Test

**9.1 Build**
- `cd go && go build ./...` — zero errors
- `go vet ./...` — no issues

**9.2 Proto verification**
- All proto files compile without errors
- No duplicate enum definitions between l8alarms and shared libraries

**9.3 Functional verification**
- Alarm CRUD works (create, acknowledge, clear, suppress)
- Event creation works (immutable — PUT rejected)
- Correlation engine finds root causes (topological, temporal, pattern)
- Notification dispatch works (webhook, slack, email via l8notify)
- Escalation timers fire and cancel correctly (via l8notify)
- Archiving works (alarm + associated events, cascade for root cause)
- Maintenance window suppression works

**9.4 UI verification**
- Desktop: alarm table loads with severity/state badges
- Desktop: alarm detail shows state history and notes
- Desktop: notification policy form has target editor
- Desktop: event viewer loads
- Desktop: archive browser loads
- Mobile: same verifications

**9.5 No regressions**
- Webhook HTTP POST payloads identical to before
- Slack message format identical
- Template rendering produces same output
- Throttle behavior unchanged
- Escalation timing unchanged
- Correlation results unchanged

---

## Traceability Matrix

| # | Gap / Action Item | Phase |
|---|-------------------|-------|
| 1 | l8alarms depends on no shared libraries | Phase 1 |
| 2 | Duplicate AlarmSeverity/AlarmState enums | Phase 2 |
| 3 | Duplicate NotificationChannel enum | Phase 2 |
| 4 | Custom notification sender (should use l8notify) | Phase 3 |
| 5 | Custom template renderer (should use l8notify) | Phase 3 |
| 6 | Custom throttle implementation (should use l8notify) | Phase 3 |
| 7 | Custom escalation scheduler (should use l8notify) | Phase 4 |
| 8 | Custom archive engine (should use l8events) | Phase 5 |
| 9 | Custom maintenance window evaluator (should use l8events) | Phase 6 |
| 10 | Custom alarm state validation (should use l8events) | Phase 7 |
| 11 | Duplicated severity/state enums in UI JS | Phase 8 |
| 12 | No shared notification target editor in policy forms | Phase 8 |
| 13 | No build verification after refactoring | Phase 9 |
| 14 | No regression testing | Phase 9 |

---

## File Summary

| Category | New Files | Modified Files | Deleted Files |
|----------|-----------|----------------|---------------|
| Dependencies | 0 | 2 (go.mod, make-bindings.sh) | 0 |
| Proto | 0 | 5 (alm-common, alm-alarms, alm-events, alm-policies, alm-maintenance) | 0 |
| Notification engine | 0 | 2 (engine.go, sender.go) | 0 |
| Escalation | 0 | 1 (scheduler.go) | 0 |
| Archiving | 0 | 1 (engine.go) | 0 |
| Maintenance | 0 | 2 (service + callback) | 0 |
| Alarm service | 0 | 1 (AlarmServiceCallback.go) | 0 |
| UI shared components | ~14 (copied from l8notify + l8events) | ~12 (app.html, enums, columns, forms) | 0 |
| **Total** | **~14 copied** | **~26 modified** | **0** |

**Net effect**: Significant code reduction. The notification engine, escalation scheduler, archive engine, and maintenance evaluator become thin wrappers around shared library calls. Proto files shrink as duplicate enums are removed. UI JS files shrink as duplicate enum definitions are replaced by shared references.

---

## Dependency Order

Implementation must follow this order:

```
1. l8notify (PLAN-L8NOTIFY-SHARED-LIBRARY.md) — no dependencies
2. l8events (PLAN-L8EVENTS-SHARED-LIBRARY.md) — no dependencies
   (1 and 2 can be implemented in parallel)
3. l8alarms (this plan) — depends on both 1 and 2
```
