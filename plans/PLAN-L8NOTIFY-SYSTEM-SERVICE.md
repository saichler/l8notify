# L8Notify — Library to System Service Transformation

## Purpose

Transform `l8notify` from a passive Go/JS library (functions consumers call, structs consumers persist themselves)
into an activatable **system service**, following the `l8events` pattern: a `go/services` package with an
`ActivateNotify(...)` entrypoint that any consumer project calls from its own `main.go`, backed by `l8orm` ORM
persistence for the notification records themselves, exposed over the web via L8Query, with generated UI type
registration — the same shape as `evtservices.ActivateEvents(dbcred, dbname, nic)`.

The existing Go packages (`channel`, `template`, `throttle`, `escalation`) and the existing `l8ui/notification/*`
components are **not thrown away** — they become the internals the new service calls, and the delivery-log UI
component gets wired to the new service's real endpoint.

**Scope note**: this transformation touches five repos, not one:
- `l8notify` — the service itself (unchanged goal)
- `l8common` — consolidated shared utilities (`ActivateService`, `GenerateID`, HMAC helper) so `l8events` and
  `l8notify` stop each hand-rolling the same boilerplate
- `l8events` — migrated onto the consolidated `l8common` helpers
- `l8types` — gains `INotify`/`IIntegration` interfaces on `IResources`, mirroring the existing `Events() IEvents`
  precedent, plus a new home for `l8notify`'s shared proto types
- `l8utils` — gains default concrete implementations of `INotify`/`IIntegration`, mirroring the existing
  `l8utils/go/utils/events` package

**Execution model**: this plan is executed **phase by phase, each phase in a separate session, no git operations
performed by the session** — the user reviews, pushes, and vendors between phases. Every phase below states its
**Repo**, its **Prerequisites** (what must already be pushed and vendored before that phase's session starts), and
its **Files**, specifically so a session with no memory of prior sessions can verify it's safe to begin from this
document alone. Phases 1 and 2 are no-op markers (retired/superseded pointers) — they need no session at all.
Phase 0.5 is split into 0.5a (`l8common`) and 0.5b (`l8events`) so every phase touches exactly one repo, matching
the "one phase → one push → one vendor" execution model.

It is a plan only; no code in any of these five repos is changed until each phase is individually executed.

---

## Reference Pattern: How l8events Does It

`l8events` is *not* a standalone binary — it has no `main.go`, no `Dockerfile`, no k8s manifests, no `proto/` or
`go/types/` directory of its own (its types were migrated into the central `l8types` repo). What it provides is:

- `go/services/EventService.go` — `ActivateEvents(creds, dbname string, vnic ifs.IVNic)`, which builds an
  ORM-backed persistence layer, a `ifs.ServiceLevelAgreement` for the Prime Object (`EventRecord`/`EventRecordList`,
  fixed `ServiceArea = 76`, `ServiceName = "Events"`), a `web.New(...)` web service with `POST`/`PATCH`/
  `GET(L8Query)` endpoints, and an `EventCallback` with `Before()`/`After()` — auto-generates the ID and timestamps
  on `POST`, rejects `PUT` (immutable), passes `PATCH`/`GET` through.

**Also part of the reference pattern (directly mirrored by this plan)**: `l8types` already has an `IEvents`
interface (`l8types/go/ifs/Events.go`) and an `Events() IEvents` accessor on `IResources`
(`l8types/go/ifs/Resources.go`). Its default concrete implementation lives in **`l8utils`**, not in `l8events` or
`l8types`: `l8utils/go/utils/events/events_api.go` defines `type Events struct{ vnic ifs.IVNic }` implementing
`ifs.IEvents`, and `PostXxxEvent(...)` methods build an `*l8events.EventRecord` and call
`vnic.Unicast("", "Events", 76, ifs.POST, record)`. `l8utils/go/utils/resources/Resources.go`'s `NewResources()`
wires up `r.events = &events.Events{}` as the default. Phase 0/0.25 below mirror this exactly for
`INotify`/`IIntegration`.

Every consumer (`l8erp`, `probler`, `fmc`, `l8learn`, `l8vendingmachine`, `l8rubi`) calls
`evtservices.ActivateEvents(dbcred, dbname, nic)` **once**, from its own backend `main.go`, right after
`services.ActivateAllServices(...)`, and registers the type in its UI `main.go`:
```go
common.RegisterType(resources, &l8events.EventRecord{}, &l8events.EventRecordList{}, "EventId")
```

**Note**: per Phase 0.5b, `l8events`' own `go/common/activate.go` gets deleted and `EventService.go` migrates onto
`l8common.ActivateService`/`l8common.GenerateID` — the description above of its current shape is accurate as of the
start of this review, not the end state once this plan is implemented.

---

## Design Decisions

### 1. The service actively dispatches on `POST`
POSTing a `NotifyRecord` triggers real delivery (email/webhook/Slack) inside the `Before(POST)` hook, then persists
the outcome. A notify service that only logs isn't doing the job of sending anything — this is the whole reason to
promote it from "library" to "service." (Contrast with `EventRecord`, which is a pure audit log with no side effect
— events don't need to "do" anything, notifications do.)

### 2. Delivery/integration config is an ORM-backed Prime Object again — history of this decision

This design has gone through three shapes across this review. Documenting the history because the reasoning at each
step still matters for judging edge cases later:

**v1 (original draft)**: `SmtpConfig`/`WebhookConfig` as two separate ORM-backed Prime Objects with their own CRUD
services (`NotifySmtp`, `NotifyHook`). Problem: two nearly-identical config services for what's conceptually one
concept ("a place to send a notification"), and no path to add a third integration type without a third service.

**v2 (SysConfig draft, superseded)**: moved connection config out of the database entirely, into `L8SysConfig` (a
shared `l8types` framework type), reasoning that connection/credential config is the same category of value as
database credentials — which can't be stored in the database they connect to (a bootstrap problem). **This
reasoning over-applied the DB-credential analogy.** DB credentials must exist *before* the database connection can
be established — that's the actual bootstrap problem. SMTP/webhook connection config is read at *dispatch time*,
well after `l8notify`'s own service (and its database connection) is already up. There's no bootstrap problem for
this data. Treating it like DB credentials came with a real cost too: no admin UI (SysConfig is deploy-time-only),
and a shared-framework-contract change for something that's `l8notify`-specific, not framework-wide.

**v3 (current)**: a single, generalized ORM-backed Prime Object, `IntegrationConfig` (Phase 3), replacing both
`SmtpConfig`-as-a-service and `WebhookConfig`-as-a-service with one CRUD service and one extensible message shape
(`IntegrationType` enum + flat fields + an `attributes` map for future integration-type-specific data — Phase 0.1).
This keeps what v2 got right — **secrets are still never stored in the ORM row**. `IntegrationConfig` holds only
non-secret routing/shape fields (host, port, URL, retry count, etc.) plus a `credential_key` string; the actual
SMTP password / webhook HMAC secret is still resolved at dispatch time via
`Security().Credential(credentialKey, ...)` against the consumer's own security config JSON `credentials` map —
exactly as v2 designed, just no longer bundled with the *non-secret* data that never needed to leave the database.

Lookups happen through a new `IIntegration` resource interface (Phase 0), not direct ORM calls from
`NotifyRecordService.go` — mirroring how `l8events` (indirectly, via `IEvents`) and now `l8notify` both expose their
capabilities through `IResources` rather than requiring callers to know service names/areas.

### 3. Escalation scheduling stays an in-memory library, unchanged
`escalation.Scheduler` (timer-based step chains) stays exactly as it is today — a Go package a consumer embeds in
its own process. This mirrors how `l8events` left `state`, `archive`, and `maintenance` as plain libraries rather
than services; only the core record type became a persisted Prime Object. Out of scope for this transformation.

### 4. `INotify`/`IIntegration` mirror `IEvents` exactly — including a naming collision that needs a decision

Per direction, `l8types/go/ifs/Resources.go` gains two new interfaces and accessors, matching the existing
`Events() IEvents` shape precisely:
- `Notify() INotify` — dispatches a notification (mirrors `IEvents.PostXxxEvent(...)`)
- `Integration() IIntegration` — looks up integration configuration (no direct `IEvents` equivalent, but the same
  "generic capability via `IResources`, default impl in `l8utils`, real impl talks to a service via `vnic`" shape)

**The blocker**: `IEvents`' types live in `l8types/go/types/l8events` — a package name free for `l8events` to claim
because nothing else used it. `l8notify`'s types do **not** have an equivalent free name: `l8types/go/types/l8notify`
is **already occupied** by an unrelated, pre-existing framework type (`L8Notification`/`L8NotificationSet` — the
internal service change-notification mechanism, confirmed present and in active use across the framework).

**Resolution used in this plan** (needs confirmation, not unilaterally final): migrate all of `l8notify`'s shared
types into a new `l8types/go/types/l8notifysvc` package (distinct spelling to avoid the collision — "svc" for
"service"). All of `l8notify`'s current proto content (`NotifyChannel`, `DeliveryStatus`, `NotifyTarget`,
`DeliveryResult`, `EscalationStep`, `NotifyRecord`, `NotifyRecordList`) moves there, plus the new
`IntegrationConfig`/`IntegrationConfigList`/`IntegrationType` (Phase 0.1). `l8notify`'s own `proto/l8notify.proto`
and `go/types/l8notify/` are retired entirely (Phase 1 is now just a pointer to Phase 0).

**Other names considered and rejected**: `l8notification` (too easily confused with the existing
`L8Notification`), unprefixed names (breaks the `l8`-prefix convention every other package in `l8types/go/types/`
follows). `l8notifysvc` is the working default in this plan; flagged in Open Items for explicit sign-off.

---

## Target Architecture

```
l8types (Phase 0)
    ├── go/types/l8notifysvc/            NEW package — migrated from l8notify's own proto
    │   ├── NotifyChannel, DeliveryStatus, IntegrationType (enums)
    │   ├── NotifyTarget, DeliveryResult, EscalationStep (embedded/child, unchanged)
    │   ├── NotifyRecord / NotifyRecordList (Prime Object)
    │   └── IntegrationConfig / IntegrationConfigList (Prime Object, NEW — generalizes former SmtpConfig+WebhookConfig)
    └── go/ifs/
        ├── Notify.go        NEW — INotify interface
        ├── Integration.go   NEW — IIntegration interface
        └── Resources.go     MODIFIED — IResources gains Notify() INotify, Integration() IIntegration

l8utils (Phase 0.25)
    └── go/utils/
        ├── notify/notify_api.go            NEW — default INotify impl, routes to "Notify" service via vnic
        ├── integration/integration_api.go  NEW — default IIntegration impl, routes to "IntegCfg" service via vnic
        └── resources/Resources.go          MODIFIED — wires defaults, Set()/Copy() cases, same as `events` today

l8common (Phase 0.5a)
    └── go/common/
        ├── service_factory.go   MODIFIED — additive ServiceConfig fields (Voter, NonUniqueKeys, Replication)
        └── hmac.go               NEW — ComputeHMACSHA256, VerifyHMACSHA256Hex

l8events (Phase 0.5b)
    └── go/ — migrated onto l8common.ActivateService / l8common.GenerateID

consumer project's own backend process (main.go)
    ├── services.ActivateAllServices(dbcred, dbname, nic)
    ├── evtservices.ActivateEvents(dbcred, dbname, nic)              (existing, unrelated)
    ├── notifyservices.ActivateNotify(dbcred, dbname, nic)           (Phase 4) — ServiceName "Notify",   ServiceArea 78
    └── notifyservices.ActivateIntegrationConfig(dbcred, dbname, nic)(Phase 3) — ServiceName "IntegCfg", ServiceArea 78

    NotifyRecordService.Before(POST):
        cfg, _  := vnic.Resources().Integration().GetIntegrationConfig("smtp")      // non-secret routing fields
        secret, _ := vnic.Resources().Security().Credential(cfg.CredentialKey, ...) // secret, from consumer's credentials map
        result := channel.Dispatch(target, message, adaptedSmtpConfig, webhookSecrets)

consumer's UI main.go
    └── RegisterType(&l8notifysvc.NotifyRecord{}, &l8notifysvc.NotifyRecordList{}, "NotifyId")
        RegisterType(&l8notifysvc.IntegrationConfig{}, &l8notifysvc.IntegrationConfigList{}, "IntegrationId")

consumer's app.html / l8ui wiring (Phase 5)
    ├── L8NotifyDeliveryLog       → Layer8DTable at /<prefix>/78/Notify    (read-only — NotifyRecord immutable)
    └── L8NotifyIntegrationMgmt   → Layer8DTable/Forms at /<prefix>/78/IntegCfg (editable CRUD)

any other service in the ecosystem (not just l8notify's own consumer)
    └── vnic.Resources().Notify().Send(channel, endpoint, subject, message, attrs)
        — generic notification dispatch, same ergonomics as vnic.Resources().Events().PostSystemEvent(...)
```

`ServiceArea = 78` is fixed inside `l8notify` itself (same as `l8events` fixing `76`). Verified unused across the
fleet — the highest allocated global/shared area currently in use is `77` (`l8secure`'s `PortalService`).

---

## Directory Structure (after)

```
l8notify/
├── go/
│   ├── go.mod                      # MODIFIED — depends on l8common (Phase 0.5a), l8types (bumped, Phase 0)
│   ├── channel/
│   │   ├── webhook.go              # MODIFIED — uses common.ComputeHMACSHA256 (l8common) instead of inline hmac
│   │   ├── email.go, slack.go, channel.go  # unchanged — still take *l8notifysvc.SmtpConfig / WebhookConfig
│   ├── template/, throttle/, escalation/   # unchanged
│   └── services/
│       ├── IntegrationConfigService.go  # NEW (Phase 3) — ActivateIntegrationConfig(), editable CRUD callback
│       └── NotifyRecordService.go       # NEW (Phase 4) — ActivateNotify() via l8common.ActivateService, NotifyCallback
├── l8ui/notification/
│   ├── l8notify-enums.js                # MODIFIED — adds IntegrationType enum/renderer
│   ├── l8notify-delivery-log.js         # MODIFIED — getColumns()/getFormDefinition() pattern (data-only, read-only)
│   ├── l8notify-integration-mgmt.js     # NEW — replaces l8notify-smtp-config.js + l8notify-webhook-mgmt.js;
│   │                                     #      editable CRUD table+form for IntegrationConfig
│   ├── l8notify-target-editor.js        # unchanged (still embedded-type inline table editor)
│   └── l8notify-notification.css        # unchanged
├── plans/PLAN-L8NOTIFY-SYSTEM-SERVICE.md   # this file
└── README.md                        # MODIFIED (Phase 6)

# RETIRED entirely (Phase 1 — types migrate to l8types in Phase 0):
# ├── proto/l8notify.proto
# ├── proto/make-bindings.sh
# └── go/types/l8notify/l8notify.pb.go
```

**Outside this repo**:
```
l8types/ (Phase 0)
├── proto/l8notifysvc.proto         # NEW — migrated content of l8notify's former proto, plus IntegrationConfig
├── go/types/l8notifysvc/*.pb.go    # regenerated
├── go/ifs/Notify.go                # NEW — INotify
├── go/ifs/Integration.go           # NEW — IIntegration
└── go/ifs/Resources.go             # MODIFIED — IResources gains Notify(), Integration()

l8utils/ (Phase 0.25)
├── go/utils/notify/notify_api.go            # NEW
├── go/utils/integration/integration_api.go  # NEW
└── go/utils/resources/Resources.go          # MODIFIED — wire defaults, Set()/Copy() cases

l8common/ (Phase 0.5a)
└── go/common/service_factory.go, hmac.go

l8events/ (Phase 0.5b)
└── go/common/activate.go (deleted), go/services/EventService.go, go.mod
```

---

## Phase 0 [DONE] — `l8types`: migrate `l8notify`'s types, add `INotify`/`IIntegration`

**Repo**: `l8types`
**Prerequisites**: none — this is the first phase, and it's self-contained within `l8types`.
**Sign-off**: requires framework-owner sign-off (`framework-interface-boundaries.md`) — this is the phase that
changes `l8types/go/ifs`, the actual interface-contract layer. Justified because `l8notify`, like `l8events`, is
meant to be usable by *any* service in the ecosystem via `IResources`, not just its own consumer's code.

### 0.1 New proto package — `l8types/proto/l8notifysvc.proto`

Migrated content (unchanged from `l8notify`'s current proto) plus the new generalized `IntegrationConfig`:

```protobuf
syntax = "proto3";
package l8notifysvc;
option go_package = "./types/l8notifysvc";
import "api.proto";

// ─── Enums (unchanged from l8notify's current proto) ───
enum NotifyChannel {
  NOTIFY_CHANNEL_UNSPECIFIED = 0;
  NOTIFY_CHANNEL_EMAIL = 1;
  NOTIFY_CHANNEL_WEBHOOK = 2;
  NOTIFY_CHANNEL_SLACK = 3;
  NOTIFY_CHANNEL_PAGERDUTY = 4;
  NOTIFY_CHANNEL_CUSTOM = 5;
}
enum DeliveryStatus {
  DELIVERY_STATUS_UNSPECIFIED = 0;
  DELIVERY_STATUS_PENDING = 1;
  DELIVERY_STATUS_SENT = 2;
  DELIVERY_STATUS_FAILED = 3;
  DELIVERY_STATUS_RETRYING = 4;
}

// NEW enum — replaces the separate "is this an SMTP config vs a webhook config" distinction
// that used to be implicit in having two separate message types.
enum IntegrationType {
  INTEGRATION_TYPE_UNSPECIFIED = 0;
  INTEGRATION_TYPE_SMTP = 1;
  INTEGRATION_TYPE_WEBHOOK = 2;
  INTEGRATION_TYPE_SLACK = 3;
  INTEGRATION_TYPE_PAGERDUTY = 4;
  INTEGRATION_TYPE_CUSTOM = 5;
}

// ─── Embedded/child types (unchanged from l8notify's current proto — NOT Prime Objects) ───
message NotifyTarget {
  string target_id = 1;
  NotifyChannel channel = 2;
  string endpoint = 3;
  string template = 4;
}
message SmtpConfig {
  string host = 1;
  int32 port = 2;
  string username = 3;
  string password = 4;
  bool use_tls = 5;
  string from_address = 6;
  string from_name = 7;
}
message WebhookConfig {
  string url = 1;
  string secret = 2;
  int32 retry_count = 3;
  int32 timeout_ms = 4;
}
message DeliveryResult {
  DeliveryStatus status = 1;
  int32 http_status = 2;
  string error_message = 3;
  int32 attempt = 4;
  int64 sent_at = 5;
}
message EscalationStep {
  string step_id = 1;
  int32 step_order = 2;
  int32 delay_minutes = 3;
  NotifyChannel channel = 4;
  string endpoint = 5;
  string message_template = 6;
}

// ─── Prime Objects ───

// Immutable delivery-log record. POST triggers real dispatch (Design Decision 1).
message NotifyRecord {
  string notify_id = 1;
  NotifyChannel channel = 2;
  string endpoint = 3;
  string subject = 4;
  string template = 5;
  map<string, string> vars = 6;
  string message = 7;
  string source_id = 8;
  string source_type = 9;
  DeliveryStatus status = 10;
  int32 http_status = 11;
  string error_message = 12;
  int32 attempt = 13;
  int64 requested_at = 14;
  int64 sent_at = 15;
  map<string, string> attributes = 16;
}

// A single configured integration endpoint (SMTP server, webhook, Slack incoming webhook,
// or a future type). Generalizes what used to be two separate config concepts. Editable —
// unlike NotifyRecord, this is meant to be created/updated/deleted by an admin.
// Non-secret fields only: the actual secret (SMTP password, webhook HMAC secret, API key)
// is NEVER stored here — credential_key is a lookup key into the consumer's own security
// config JSON `credentials` map, resolved at dispatch time via
// ISecurityProvider.Credential(credential_key, ...).
message IntegrationConfig {
  string integration_id = 1;         // primary key, auto-generated
  string name = 2;                   // logical/unique name, e.g. "smtp", "ops-alerts"
  IntegrationType type = 3;
  string host = 4;                   // SMTP host; empty for webhook/slack
  int32 port = 5;                    // SMTP port
  bool use_tls = 6;                  // SMTP TLS
  string from_address = 7;           // SMTP from address
  string from_name = 8;              // SMTP from display name
  string url = 9;                    // webhook/slack URL
  int32 retry_count = 10;            // webhook retry count
  int32 timeout_ms = 11;             // webhook timeout
  string credential_key = 12;        // key into the consumer's `credentials` map for the secret
  map<string, string> attributes = 13;  // extensible bag for future integration-type-specific fields
}

// ─── List types ───
message NotifyRecordList {
  repeated NotifyRecord list = 1;
  l8api.L8MetaData metadata = 2;
}
message IntegrationConfigList {
  repeated IntegrationConfig list = 1;
  l8api.L8MetaData metadata = 2;
}
```

**Design note on `IntegrationConfig`'s shape**: flat fields (most unset for a given `type`) plus a generic
`attributes` map, rather than a `oneof`. Considered and rejected the `oneof` alternative for consistency with how
`NotifyTarget` (and most other shared building-block types in this codebase) already use a flat, mostly-optional
shape — simpler to work with from JS forms and L8Query filters, at the cost of some unused fields per row. Flag if
a stricter shape is preferred; revisit once a third/fourth integration type is added and the sparseness grows.

### 0.2 `l8types/go/ifs/Notify.go` (NEW)

```go
package ifs

import "github.com/saichler/l8types/go/types/l8notifysvc"

// INotify provides the API for dispatching notifications across the Layer 8 system.
// Implementations route requests to the Notify service via the VNic — mirrors IEvents exactly.
type INotify interface {
    // Send posts a notification for dispatch (email/webhook/Slack/etc.) and returns the
    // delivery outcome. attributes is forwarded onto the persisted NotifyRecord.
    Send(channel l8notifysvc.NotifyChannel, endpoint, subject, message string,
        attributes map[string]string) *l8notifysvc.DeliveryResult
    SetVNic(IVNic)
}
```

### 0.3 `l8types/go/ifs/Integration.go` (NEW)

```go
package ifs

import "github.com/saichler/l8types/go/types/l8notifysvc"

// IIntegration provides lookup of configured integration endpoints (SMTP, webhook, etc.).
type IIntegration interface {
    // GetIntegrationConfig retrieves a named integration configuration.
    GetIntegrationConfig(name string) (*l8notifysvc.IntegrationConfig, error)
    // ListIntegrationConfigs retrieves all integrations of the given type
    // (INTEGRATION_TYPE_UNSPECIFIED returns all types).
    ListIntegrationConfigs(integrationType l8notifysvc.IntegrationType) ([]*l8notifysvc.IntegrationConfig, error)
    SetVNic(IVNic)
}
```

### 0.4 `l8types/go/ifs/Resources.go` (MODIFIED — additive)

```go
type IResources interface {
    // ... existing methods unchanged ...
    // Events Service
    Events() IEvents
    // Notify Service — NEW
    Notify() INotify
    // Integration Config lookup — NEW
    Integration() IIntegration
}
```

**Verification note (concern #3 from review)**: adding methods to a Go *interface* is not purely additive the way
adding fields to a struct is — every other implementer of `IResources` must add these methods too, or the ecosystem
fails to compile. `IEvents` proves this cost has been paid successfully before, but before this phase is executed,
grep for any other `IResources` implementation (e.g. test mocks under `l8types/go/tests/` or elsewhere) so none are
missed.

### 0.5 Regenerate & bump
```bash
cd l8types/proto && ./make-bindings.sh
```
Consumer projects (and `l8notify`, `l8utils` themselves) pick up the new `l8types` version through their normal
vendor refresh — **not run by this plan**, per `vendor-and-git.md`.

**Files**: 1 new proto + regenerated package (`l8notifysvc`), 2 new Go files (`Notify.go`, `Integration.go`), 1
modified (`Resources.go`)

---

## Phase 0.25 [DONE] — `l8utils`: default `INotify`/`IIntegration` implementations

**Repo**: `l8utils`
**Prerequisites**: Phase 0 pushed; `l8utils/go.mod` bumped to require the `l8types` version containing
`l8notifysvc` + `INotify`/`IIntegration`.

Mirrors `l8utils/go/utils/events/events_api.go` exactly.

### 0.25.1 `l8utils/go/utils/notify/notify_api.go` (NEW)

```go
package notify

import (
    "github.com/saichler/l8types/go/ifs"
    ntf "github.com/saichler/l8types/go/types/l8notifysvc"
)

const (
    NotifyServiceName = "Notify"
    NotifyServiceArea = byte(78)
)

// Notify implements ifs.INotify, routing notification requests to the Notify service via VNic.
type Notify struct {
    vnic ifs.IVNic
}

func (this *Notify) SetVNic(vnic ifs.IVNic) { this.vnic = vnic }

func (this *Notify) Send(channel ntf.NotifyChannel, endpoint, subject, message string,
    attributes map[string]string) *ntf.DeliveryResult {
    if this.vnic == nil {
        return &ntf.DeliveryResult{Status: ntf.DeliveryStatus_DELIVERY_STATUS_FAILED, ErrorMessage: "no VNic"}
    }
    record := &ntf.NotifyRecord{
        Channel: channel, Endpoint: endpoint, Subject: subject, Message: message, Attributes: attributes,
    }
    // Unlike Events' fire-and-forget Unicast, Send needs the dispatch OUTCOME back —
    // use a synchronous request, same pattern l8common.GetEntity/PostEntity use.
    resp := this.vnic.Request("", NotifyServiceName, NotifyServiceArea, ifs.POST, record, 30)
    if resp.Error() != nil {
        return &ntf.DeliveryResult{Status: ntf.DeliveryStatus_DELIVERY_STATUS_FAILED, ErrorMessage: resp.Error().Error()}
    }
    posted, ok := resp.Element().(*ntf.NotifyRecord)
    if !ok {
        return &ntf.DeliveryResult{Status: ntf.DeliveryStatus_DELIVERY_STATUS_FAILED, ErrorMessage: "unexpected response type"}
    }
    return &ntf.DeliveryResult{
        Status: posted.Status, HttpStatus: posted.HttpStatus,
        ErrorMessage: posted.ErrorMessage, Attempt: posted.Attempt, SentAt: posted.SentAt,
    }
}
```

### 0.25.2 `l8utils/go/utils/integration/integration_api.go` (NEW)

Uses the local-service-handler-first pattern `l8common.GetEntity` already establishes (check `ServiceHandler`
before falling back to a full `vnic.Request` round-trip) — since `Notify`/`IntegCfg` almost always run in the same
process (`single-owner-database-table.md`), this avoids paying request/serialization overhead on every dispatch:

```go
package integration

import (
    "fmt"
    "github.com/saichler/l8types/go/ifs"
    "github.com/saichler/l8types/go/types/l8api"
    ntf "github.com/saichler/l8types/go/types/l8notifysvc"
)

const (
    IntegrationServiceName = "IntegCfg"
    IntegrationServiceArea = byte(78)
)

type Integration struct {
    vnic ifs.IVNic
}

func (this *Integration) SetVNic(vnic ifs.IVNic) { this.vnic = vnic }

func (this *Integration) GetIntegrationConfig(name string) (*ntf.IntegrationConfig, error) {
    filter := &ntf.IntegrationConfig{Name: name}
    if handler, ok := this.vnic.Resources().Services().ServiceHandler(IntegrationServiceName, IntegrationServiceArea); ok {
        resp := handler.Get(object.New(nil, filter), this.vnic)
        if resp.Error() != nil {
            return nil, resp.Error()
        }
        cfg, ok := resp.Element().(*ntf.IntegrationConfig)
        if !ok {
            return nil, fmt.Errorf("integration config %q not found", name)
        }
        return cfg, nil
    }
    resp := this.vnic.Request("", IntegrationServiceName, IntegrationServiceArea, ifs.GET, filter, 30)
    if resp.Error() != nil {
        return nil, resp.Error()
    }
    cfg, ok := resp.Element().(*ntf.IntegrationConfig)
    if !ok {
        return nil, fmt.Errorf("integration config %q not found", name)
    }
    return cfg, nil
}

func (this *Integration) ListIntegrationConfigs(integrationType ntf.IntegrationType) ([]*ntf.IntegrationConfig, error) {
    query := "select * from IntegrationConfig"
    if integrationType != ntf.IntegrationType_INTEGRATION_TYPE_UNSPECIFIED {
        query = fmt.Sprintf("select * from IntegrationConfig where type=%d", int32(integrationType))
    }
    var elements []interface{}
    if handler, ok := this.vnic.Resources().Services().ServiceHandler(IntegrationServiceName, IntegrationServiceArea); ok {
        elems, err := object.NewQuery(query, this.vnic.Resources())
        if err != nil {
            return nil, err
        }
        resp := handler.Get(elems, this.vnic)
        if resp.Error() != nil {
            return nil, resp.Error()
        }
        elements = resp.Elements()
    } else {
        resp := this.vnic.Request("", IntegrationServiceName, IntegrationServiceArea, ifs.GET,
            &l8api.L8Query{Text: query}, 30)
        if resp.Error() != nil {
            return nil, resp.Error()
        }
        elements = resp.Elements()
    }
    result := make([]*ntf.IntegrationConfig, 0, len(elements))
    for _, e := range elements {
        if cfg, ok := e.(*ntf.IntegrationConfig); ok {
            result = append(result, cfg)
        }
    }
    return result, nil
}
```

(`object.New`/`object.NewQuery` are the same `l8srlz/go/serialize/object` helpers `l8common.GetEntity`/`GetEntities`
already use — verify the exact import path against `l8common/go/common/service_factory.go` at implementation time.)

### 0.25.3 `l8utils/go/utils/resources/Resources.go` (MODIFIED — additive, same shape as the existing `events` wiring)

- Add `notify ifs.INotify` and `integration ifs.IIntegration` fields to the `Resources` struct.
- `NewResources()`: `r.notify = &notify.Notify{}`, `r.integration = &integration.Integration{}`.
- `Set(interface{})`: add `ifs.INotify`/`ifs.IIntegration` type-switch cases (same shape as the existing
  `events, ok := any.(ifs.IEvents)` case).
- `Copy(other)`: add `this.notify = other.Notify()`, `this.integration = other.Integration()`.
- Add `Notify() ifs.INotify` / `Integration() ifs.IIntegration` getter methods (same shape as `Events()`).

**Files**: 2 new (`notify/notify_api.go`, `integration/integration_api.go`), 1 modified
(`resources/Resources.go`)

---

## Phase 0.5a [DONE] — `l8common`: consolidate shared activation/ID/HMAC helpers

**Repo**: `l8common`
**Prerequisites**: none — independent of Phase 0/0.25 (doesn't reference any `l8notify`/`l8notifysvc` type), can
run in any order relative to them.

While designing the service-activation code, review surfaced that the helper code `l8notify` was about to write a
second (really third) time already exists: `github.com/saichler/l8common/go/common` implements `ActivateService`,
`GenerateID`, `GetEntity`, `GetEntities`, `RegisterType`, `CreateResources`, `OpenDBConection`. Meanwhile `l8events`
reinvented its own smaller version in its own `go/common/activate.go`, rather than depending on `l8common`. This
plan consolidates all of it into `l8common` instead of letting `l8notify` add a third copy.

### 0.5a.1 Audit — what's duplicated where

| Concern | `l8common` (existing) | `l8events` (existing) | `l8notify` (this plan, before this fix) |
|---|---|---|---|
| DB credential resolution + Postgres connection | `OpenDBConection` + `ActivateService`'s inline `Security().Credential(...)` call | own `CreatePersistency` (`go/common/activate.go`) | own `CreatePersistency` (removed) |
| SLA + web-service boilerplate | `ActivateService(ServiceConfig{...}, item, list, creds, dbname, vnic)` | inlined directly in `EventService.go` | inlined directly (removed) |
| Primary-key auto-generation | `GenerateID(id *string)` | inline `event.EventId = ifs.NewUuid()` | own `GenerateID` (removed) |

### 0.5a.2 Constraint: the dependency direction is not symmetric

`l8common`'s own `go.mod` already requires `l8web` (used by `defaults.go`'s `CreateWebServer`/`CreateVnic`
bootstrap helpers). `l8web`'s own `go.mod` does **not** require `l8common`. This means `l8events`/`l8notify`
depending on `l8common` is safe (one-directional, same relationship `l8common`'s 10+ existing dependents already
have), but `l8web` depending on `l8common` (to reuse a consolidated HMAC helper) is **not** safe — it would create
a module-level mutual dependency between the two repos. So: the webhook-signing HMAC helper gets **added** to
`l8common`, but `l8web`'s own existing `web/webhook/signature.go` is **left as-is** — a small, accepted, pre-existing
duplication rather than a riskier dependency restructure.

### 0.5a.3 Changes

**Extend `ServiceConfig`/`ActivateService` (`go/common/service_factory.go`) — additive only** (10+ existing
dependents, e.g. `l8erp`'s MFG services call `common.ActivateService` directly):

```go
type ServiceConfig struct {
    ServiceName      string
    ServiceArea      byte
    PrimaryKey       string
    Callback         ifs.IServiceCallback
    ServiceGroup     string
    Voter            bool     // NEW — default false, preserves today's implicit behavior
    NonUniqueKeys    []string // NEW — default nil (SetNonUniqueKeys skipped if empty)
    Replication      *bool    // NEW — nil means "true" (today's hardcoded default); explicit false opts out
    ReplicationCount int      // NEW — 0 means "3" (today's hardcoded default) when replication is true/nil
}
```

`ActivateService`'s body gains conditional handling for these four new fields, falling back to today's hardcoded
behavior when unset — existing callers get byte-for-byte identical behavior.

**Add a new HMAC helper** — `go/common/hmac.go` (new file, stdlib-only):
```go
package common
func ComputeHMACSHA256(payload []byte, secret string) string
func VerifyHMACSHA256Hex(payload []byte, signatureHex, secret string) bool
```

### 0.5a.4 Open items surfaced by this consolidation

- `l8common.ActivateService` has no configurable endpoint subset — acceptable since callbacks already reject
  unwanted actions, but a real gap if a future consumer needs actual route-level omission.
- `l8web`'s existing HMAC code is *not* touched by this consolidation — accepted, pre-existing duplication.
- Build-verify `l8common` itself and spot-check an existing consumer (e.g. `l8erp`) still compiles against the
  additive `ServiceConfig` change, before moving on to Phase 0.5b or any `l8notify` phase.

**Files**: 1 modified (`service_factory.go`), 1 new (`hmac.go`)

---

## Phase 0.5b [DONE] — `l8events`: migrate onto the consolidated `l8common` helpers

**Repo**: `l8events`
**Prerequisites**: Phase 0.5a pushed; `l8events/go.mod` bumped to require the new `l8common` version.

- Delete `l8events/go/common/activate.go` entirely; `go.mod` adds `github.com/saichler/l8common/go`.
- `EventService.go`'s `ActivateEvents` becomes:
```go
func ActivateEvents(creds, dbname string, vnic ifs.IVNic) {
    common.ActivateService(common.ServiceConfig{
        ServiceName: EventsServiceName, ServiceArea: EventsServiceArea,
        PrimaryKey: "EventId", Voter: true, NonUniqueKeys: []string{"OccurredAt"},
        Replication: boolPtr(false), Callback: &EventCallback{},
    }, &evt.EventRecord{}, &evt.EventRecordList{}, creds, dbname, vnic)
}
```
  (`common` now refers to `github.com/saichler/l8common/go/common`, replacing the deleted local package — same
  import alias, different path, minimal call-site churn.)
- `EventCallback.Before()`'s `event.EventId = ifs.NewUuid()` becomes `common.GenerateID(&event.EventId)`.
- `l8common.ActivateService` always registers `PUT`/`DELETE`/bulk-`POST(list)` endpoints (no configurable endpoint
  subset today); `EventCallback` already rejects `PUT` unconditionally, so the extra registered-but-rejected routes
  are harmless — no behavior change for callers.
- Also flags `l8events`' own pre-existing misplaced `_test.go` files (`archive_test.go`, `state_test.go`,
  `convert_test.go`, sitting next to their source packages) as an out-of-scope, pre-existing
  `test-location-and-approach.md` violation — noted here since this phase touches the repo anyway, not fixed by it.

**Files**: 1 deleted (`go/common/activate.go`), 1 modified (`EventService.go`), 1 modified (`go.mod`)

---

## Phase 1 — `l8notify` proto: RETIRED, SKIP (no session needed)

Earlier drafts of this plan had `l8notify` own its proto (`proto/l8notify.proto`, `go/types/l8notify/`). Per Design
Decision 4 / Phase 0, all of that content — plus the new `IntegrationConfig` — now lives in
`l8types/go/types/l8notifysvc` instead. `l8notify`'s own `proto/` and `go/types/l8notify/` directories are simply
deleted as part of Phase 4's session (whichever `l8notify` phase first needs the new import path) — **this phase
produces no independent deliverable and needs no session of its own.**

---

## Phase 2 — `go/common/activate.go`: SUPERSEDED, SKIP (no session needed)

`l8notify` depends on `github.com/saichler/l8common/go/common` directly and calls `common.ActivateService(...)`
and `common.GenerateID(...)` (Phase 0.5a). No `l8notify`-local `go/common` package is ever created — **this phase
produces no independent deliverable and needs no session of its own.**

---

## Phase 3 [DONE] — `l8notify`: `go/services/IntegrationConfigService.go`

**Repo**: `l8notify`
**Prerequisites**: Phase 0 pushed + `l8notify/go.mod` bumped to require the new `l8types` (for `l8notifysvc` types);
Phase 0.5a pushed + `l8notify/go.mod` bumped to require the new `l8common` (for `ActivateService`/`GenerateID`).
Phase 0.25/0.5b are **not** required to compile this phase — only required later, for end-to-end runtime testing
(Phase 8), since they live in the consumer's dependency chain, not `l8notify`'s own.

The service that actually owns `IntegrationConfig` — fully editable CRUD, unlike `NotifyRecord`'s immutability,
since these rows are meant to be created/edited/deleted by an admin.

```go
const IntegrationServiceName = "IntegCfg"   // shares NotifyServiceArea = byte(78), declared in Phase 4's file

func ActivateIntegrationConfig(creds, dbname string, vnic ifs.IVNic) {
    common.ActivateService(common.ServiceConfig{
        ServiceName: IntegrationServiceName, ServiceArea: NotifyServiceArea,
        PrimaryKey: "IntegrationId", Callback: &IntegrationConfigCallback{},
    }, &ntf.IntegrationConfig{}, &ntf.IntegrationConfigList{}, creds, dbname, vnic)
}

type IntegrationConfigCallback struct{}

func (this *IntegrationConfigCallback) Before(elem interface{}, action ifs.Action, isNotification bool, vnic ifs.IVNic) (interface{}, bool, error) {
    if action == ifs.GET {
        return nil, true, nil
    }
    cfg, ok := elem.(*ntf.IntegrationConfig)
    if !ok {
        return nil, true, errors.New("invalid integration config type")
    }
    switch action {
    case ifs.POST:
        common.GenerateID(&cfg.IntegrationId)
        return cfg, true, nil
    case ifs.PUT, ifs.PATCH:
        return cfg, true, nil   // editable — no immutability constraint, unlike NotifyRecord
    }
    return nil, true, nil
}

func (this *IntegrationConfigCallback) After(elem interface{}, action ifs.Action, notify bool, vnic ifs.IVNic) (interface{}, bool, error) {
    return nil, true, nil
}
```

If this phase's session runs before Phase 1's cleanup has happened, delete `l8notify/proto/` and
`l8notify/go/types/l8notify/` as part of this session too (whichever `l8notify` phase runs first absorbs Phase 1's
no-op cleanup — see Phase 1 above).

**Files**: 1 new (`IntegrationConfigService.go`); also deletes `proto/l8notify.proto`, `proto/make-bindings.sh`,
`go/types/l8notify/` if not already removed by an earlier `l8notify` session

---

## Phase 4 [DONE] — `l8notify`: `go/services/NotifyRecordService.go`

**Repo**: `l8notify`
**Prerequisites**: same as Phase 3 — Phase 0 and Phase 0.5a pushed and vendored into `l8notify/go.mod`. No ordering
dependency on Phase 3 itself (different files, no shared code beyond both reading `NotifyServiceArea` — declare
that constant once, in whichever of Phase 3/4 lands first, and reference it from the other). Phase 0.25/0.5b are
not required to compile this phase, only for end-to-end runtime testing (Phase 8).

**Phase 3 already landed and already declared `NotifyServiceArea = byte(78)`** in
`go/services/IntegrationConfigService.go` (same `services` package). **Do NOT redeclare it here** — a second
`const NotifyServiceArea = ...` in the same package is a compile error. Reference the existing constant instead;
only declare `NotifyServiceName = "Notify"` in this file.

The core service — persists `NotifyRecord` **and** dispatches on `POST`, via `l8common.ActivateService`. Includes
what earlier drafts of this plan called "Phase 3" (config resolution) as subsections 4.1/4.2 below — they only ever
had one real deliverable (this file), so they're one phase/session now, not two.

```go
import (
    common "github.com/saichler/l8common/go/common"
    ntf "github.com/saichler/l8types/go/types/l8notifysvc"
)

const (
    NotifyServiceName = "Notify"
    // NotifyServiceArea is declared in IntegrationConfigService.go — do not redeclare it here.
)
// L8Query `from` clause uses the protobuf type name "NotifyRecord", NOT the ServiceName "Notify".

func ActivateNotify(creds, dbname string, vnic ifs.IVNic) {
    common.ActivateService(common.ServiceConfig{
        ServiceName: NotifyServiceName, ServiceArea: NotifyServiceArea,
        PrimaryKey: "NotifyId", Voter: true, NonUniqueKeys: []string{"RequestedAt"},
        Replication: boolPtr(false), Callback: &NotifyCallback{},
    }, &ntf.NotifyRecord{}, &ntf.NotifyRecordList{}, creds, dbname, vnic)
}

func boolPtr(b bool) *bool { return &b }

type NotifyCallback struct{}

func (this *NotifyCallback) Before(elem interface{}, action ifs.Action, isNotification bool, vnic ifs.IVNic) (interface{}, bool, error) {
    if action == ifs.GET {
        return nil, true, nil
    }
    record, ok := elem.(*ntf.NotifyRecord)
    if !ok {
        return nil, true, errors.New("invalid notify record type")
    }
    switch action {
    case ifs.POST:
        common.GenerateID(&record.NotifyId)
        record.RequestedAt = time.Now().Unix()
        if record.Status == ntf.DeliveryStatus_DELIVERY_STATUS_UNSPECIFIED {
            record.Status = ntf.DeliveryStatus_DELIVERY_STATUS_PENDING
        }
        if record.Template != "" {
            record.Message = template.Render(record.Template, record.Vars)
        }

        target := &ntf.NotifyTarget{Channel: record.Channel, Endpoint: record.Endpoint, Template: record.Template}
        smtpCfg := resolveSmtpConfig(vnic)            // 4.1
        webhookSecrets := resolveWebhookSecrets(vnic)  // 4.2
        result := channel.Dispatch(target, record.Message, smtpCfg, webhookSecrets)

        record.Status = result.Status
        record.HttpStatus = result.HttpStatus
        record.ErrorMessage = result.ErrorMessage
        record.Attempt = result.Attempt
        record.SentAt = result.SentAt
        return record, true, nil
    case ifs.PUT:
        return nil, true, errors.New("notify records are immutable, PUT is not allowed")
    case ifs.PATCH:
        return record, true, nil
    }
    return nil, true, nil
}

func (this *NotifyCallback) After(elem interface{}, action ifs.Action, notify bool, vnic ifs.IVNic) (interface{}, bool, error) {
    return nil, true, nil
}
```

### 4.1 `resolveSmtpConfig`
```go
func resolveSmtpConfig(vnic ifs.IVNic) *ntf.SmtpConfig {
    cfg, err := vnic.Resources().Integration().GetIntegrationConfig("smtp")
    if err != nil || cfg == nil || cfg.Host == "" {
        return nil   // not configured — channel.Dispatch already handles a nil SmtpConfig gracefully
    }
    user, pass := "", ""
    if cfg.CredentialKey != "" {
        _, zside, aside, _, cerr := vnic.Resources().Security().Credential(cfg.CredentialKey, "smtp", vnic.Resources())
        if cerr == nil {
            user, pass = aside, zside
        }
    }
    return &ntf.SmtpConfig{
        Host: cfg.Host, Port: cfg.Port, UseTls: cfg.UseTls,
        FromAddress: cfg.FromAddress, FromName: cfg.FromName,
        Username: user, Password: pass,
    }
}
```

### 4.2 `resolveWebhookSecrets`
```go
func resolveWebhookSecrets(vnic ifs.IVNic) map[string]string {
    configs, err := vnic.Resources().Integration().ListIntegrationConfigs(ntf.IntegrationType_INTEGRATION_TYPE_WEBHOOK)
    if err != nil {
        return nil
    }
    secrets := make(map[string]string, len(configs))
    for _, cfg := range configs {
        if cfg.CredentialKey == "" {
            continue
        }
        _, zside, _, _, cerr := vnic.Resources().Security().Credential(cfg.CredentialKey, "webhook", vnic.Resources())
        if cerr == nil {
            secrets[cfg.Url] = zside
        }
    }
    return secrets
}
```

**Note on `Security().Credential()`'s exact return order**: `l8types/go/ifs/Security.go` documents it as
`Credential(name, type string, resources IResources) (aside, zside, yside, name string, err error)`; existing call
sites (`l8common.ActivateService`) name locals as `(_, user, pass, port, err)`. Verify the real order at
implementation time — getting this backwards would silently swap username and password.

**No `WebhookConfig` struct assembly needed beyond this** — `channel.Dispatch`'s existing signature already accepts
a plain `map[string]string` of `endpoint → secret`. Retry count / timeout per webhook, if enforced, are read from
`IntegrationConfig.RetryCount`/`TimeoutMs` and adapted into a `*ntf.WebhookConfig` at the `channel.SendWebhook` call
site inside `channel.Dispatch` — no change needed to `channel/`.

**Files**: 1 new (`NotifyRecordService.go`). Per `maintainability.md`'s 500-line rule, split `resolveSmtpConfig`/
`resolveWebhookSecrets` into a small `NotifyLookup.go` if this file approaches 450 lines.

---

## Phase 5 [DONE] — `l8notify`: l8ui Component Rework

**Repo**: `l8notify`
**Prerequisites**: none to *author* — pure JS, no Go compile dependency on any earlier phase. Functional
verification (actually clicking through the UI against live endpoints) requires Phase 3 and Phase 4 already
deployed in a running consumer. Can be done in parallel with Phase 0–0.5b if desired, since nothing here imports Go
code from any other repo.

### 5.1 `l8notify-delivery-log.js`
Replace `render(container, logs, options)` with `getColumns()` (already exists) + `getFormDefinition()` (detail
popup: channel, endpoint, subject, message, status, http status, attempt, error, requested/sent timestamps) — same
**data-only** pattern `l8events`' `L8EventsEventViewer` uses.

Per `immutability-ui-alignment.md`: `NotifyRecord` rejects `PUT` (Phase 4), so the consumer MUST open this form via
`Layer8DForms.openViewForm(...)`, never `openEditForm(...)` — display-only fields, no edit/delete controls.

### 5.2 `l8notify-integration-mgmt.js` (NEW — replaces `l8notify-smtp-config.js` + `l8notify-webhook-mgmt.js`)

Since `IntegrationConfig` is now a real, editable Prime Object (Phase 3), this is a normal CRUD component:

```javascript
window.L8NotifyIntegrationMgmt = {
    getColumns: function() {
        // name, type (enum badge), host/url (whichever applies), credentialKey
    },
    getFormDefinition: function() {
        // Name, Type (select: enums.INTEGRATION_TYPE), Host, Port, UseTls, FromAddress, FromName,
        // Url, RetryCount, TimeoutMs, CredentialKey (text — NOT a password field; this is a lookup
        // key into the consumer's OWN credentials map, not the secret itself)
    }
};
```
Consumer wires this into a standard `Layer8DTable` + `Layer8DForms` CRUD flow against `/<prefix>/78/IntegCfg`,
`modelName: 'IntegrationConfig'`. `l8notify-enums.js` gains `INTEGRATION_TYPE` alongside the existing
`NOTIFY_CHANNEL`/`DELIVERY_STATUS`.

**Note on `credential_key`**: the form field is a plain text input, not a masked/password field — it's a *reference*
to an entry in the consumer's security config JSON `credentials` map, not a secret value itself.

### 5.3 `l8notify-target-editor.js`
Unchanged — `NotifyTarget` stays an embedded/child type, used the same way in consumer-defined policy forms.

### 5.4 Platform Completeness Audit (desktop × mobile) — required by `plan-requirements.md`

| Component | Desktop file | Desktop status | Mobile file | Mobile status |
|---|---|---|---|---|
| Delivery log viewer | `l8notify-delivery-log.js` | Reworked, 5.1 | *none exists* | **Deferred** |
| Integration config mgmt | `l8notify-integration-mgmt.js` | New, 5.2 | *none exists* | **Deferred** |
| Target editor | `l8notify-target-editor.js` | Unchanged | *none exists* | **Deferred** |

**Reason for deferral**: `l8notify` has never shipped `Layer8M*` equivalents of any of its components. No consumer
currently exposes these on mobile. This transformation's scope is the backend service; a mobile build-out is a
separable follow-up using the same `Layer8MTable`/`Layer8MForms` data-only pattern established here.

**Files**: 2 modified (`l8notify-delivery-log.js`, `l8notify-enums.js`), 1 new (`l8notify-integration-mgmt.js`),
2 deleted (`l8notify-smtp-config.js`, `l8notify-webhook-mgmt.js`), 1 unchanged (`l8notify-target-editor.js`)

---

## Phase 6 [DONE] — `l8notify`: Consumer Integration Reference (`README.md`)

**Repo**: `l8notify`
**Prerequisites**: none to write, but most accurate once Phase 3/4/5 have landed (documents their final shape).
Can run before implementation as a design-freeze checkpoint, or after as final documentation — either is fine.

### 6.1 Go activation
```go
import notifyservices "github.com/saichler/l8notify/go/services"

notifyservices.ActivateNotify(dbcred, dbname, nic)
notifyservices.ActivateIntegrationConfig(dbcred, dbname, nic)
```

Per `single-owner-database-table.md`: both calls must run in **exactly one process** within a given consumer
project — same constraint `evtservices.ActivateEvents` already relies on. State as a **bolded warning**.

### 6.2 UI type registration
```go
common.RegisterType(resources, &l8notifysvc.NotifyRecord{}, &l8notifysvc.NotifyRecordList{}, "NotifyId")
common.RegisterType(resources, &l8notifysvc.IntegrationConfig{}, &l8notifysvc.IntegrationConfigList{}, "IntegrationId")
```

### 6.3 Credentials setup (deploy-time, not code)
The consumer's own security config JSON `credentials` map gets one entry per integration's secret — the
non-secret routing data (host, port, URL, etc.) is entered through the admin UI (Phase 5.2) as regular
`IntegrationConfig` rows, not deploy-time config:

```json
"credentials": {
  "smtp": { "aside": "smtp-username", "zside": "smtp-password" },
  "ops-alerts": { "zside": "hmac-secret-value" }
}
```

**Discrepancy found during Phase 4 — verify before finalizing this section.** `NotifyRecordService.go`'s
`resolveSmtpConfig` was implemented against `l8common.ActivateService`'s own DB-credential resolution, the only
*verified, working* `Security().Credential()` call site available (`OpenDBConection` confirmed it builds
`user=%s password=%s` from the 2nd/3rd return values). That pattern is `_, user, pass, _, err :=
Credential(key, type, resources)` — i.e. **2nd return (`zside`) = username, 3rd return (`yside`) = password**. The
JSON example above (`aside` = username, `zside` = password) uses the *opposite* mapping for the `smtp` entry and was
never independently verified — it's this plan's own illustrative guess, not tested code. The `ops-alerts` entry
(`zside` = the one-and-only secret) doesn't conflict either way. Before writing this section for real, confirm
which mapping the consumer's actual `ISecurityProvider` implementation uses for JSON key → return-position, and fix
either the JSON example or `resolveSmtpConfig` — do not assume they already agree.

### 6.4 Any-service dispatch via `INotify`
```go
result := vnic.Resources().Notify().Send(l8notifysvc.NotifyChannel_NOTIFY_CHANNEL_EMAIL,
    "user@example.com", "Order Confirmed", "Your order SO-001 has shipped.", nil)
```

**Files**: 1 modified (`README.md`)

---

## Phase 7 — `l8notify`: Tests (documentation only, no code)

**Repo**: `l8notify`
**Prerequisites**: same as Phase 6 — most accurate once Phase 3/4 exist, but doesn't block on them to write.

Per `test-location-and-approach.md`: tests for the new services live under `go/tests/` in the *consumer* project
(l8notify itself has no `go/tests/` directory — same exemption `l8events` takes). This phase produces no code in
`l8notify` — it's a recipe documented in `README.md` (can be folded into the Phase 6 session):
1. Test file lives in the consumer's `go/tests/integration/notify_test.go`, not inside `l8notify`.
2. Seed the consumer's `credentials` map with a `smtp` entry pointed at a local SMTP catcher (e.g. `smtp4dev`).
3. Stand up the consumer's `IVNic`, call `ActivateNotify`/`ActivateIntegrationConfig`.
4. `POST /<prefix>/78/IntegCfg` an SMTP `IntegrationConfig` row (`type: SMTP`, `credentialKey: "smtp"`, host/port
   pointed at the catcher).
5. `POST /<prefix>/78/Notify` with `channel: EMAIL`, assert `status: DELIVERY_STATUS_SENT`; `GET
   /<prefix>/78/Notify?body=...` returns the delivery log; `PUT /<prefix>/78/Notify` is rejected. Assert on HTTP
   responses only, never by calling `NotifyCallback.Before()` or `IntegrationConfigCallback.Before()` directly.

**Files**: 0 new (recipe documented in `README.md`, Phase 6)

---

## Phase 8 — Build & Runtime Verification (all five repos)

**Repo**: all — `l8types`, `l8utils`, `l8common`, `l8events`, `l8notify`
**Prerequisites**: every prior phase pushed and vendored, in dependency order:
`l8types` (Phase 0) → `l8utils` (0.25) and `l8common` (0.5a) → `l8events` (0.5b) and `l8notify` (3, 4, 5, 6) → this
phase.

- `go build ./...` and `go vet ./...` in each of the five repos
- `node -c` on the modified/new JS files
- Manual: activate both `l8notify` services in a scratch consumer, seed a `credentials.smtp` entry + an SMTP
  `IntegrationConfig` row, and confirm:
  - `POST /<prefix>/78/Notify` with `channel: EMAIL` actually sends; persisted record shows `DELIVERY_STATUS_SENT`
  - `GET /<prefix>/78/Notify?body=...` (L8Query `select * from NotifyRecord`) returns the delivery log
  - `PUT /<prefix>/78/Notify` is rejected (immutable)
  - `PUT /<prefix>/78/IntegCfg` on an existing `IntegrationConfig` row succeeds (editable)
  - With no `IntegrationConfig` row named `"smtp"`, `POST .../Notify` with `channel: EMAIL` returns
    `DELIVERY_STATUS_FAILED` rather than a panic
  - `vnic.Resources().Notify().Send(...)` from a throwaway caller in a different service produces the same result
    as POSTing directly to `/Notify`

**User runs `go mod tidy`/`go mod vendor` in each repo** — per `vendor-and-git.md`, no phase runs those commands or
any git command; all pushing and vendoring between phases is the user's own action.

---

## Rule Compliance Notes

- **`prd-compliance.md` / `deployment-artifacts.md` / `k8s-*` rules**: not applicable — `l8notify` produces no new
  deployable binary, same exemption `l8events` already takes.
- **`protobuf-rules.md`**: `NotifyRecordList`/`IntegrationConfigList` follow the `repeated X list = 1;
  l8api.L8MetaData metadata = 2;` convention; all enums have `_UNSPECIFIED = 0`. ServiceName (`Notify`, `IntegCfg`)
  vs. protobuf type name (`NotifyRecord`, `IntegrationConfig`) distinction called out explicitly in Phases 3/4/6.
- **`prime-object-references.md`**: `NotifyRecord` and `IntegrationConfig` both pass the Prime Object test.
  `SmtpConfig`/`WebhookConfig`/`NotifyTarget`/`EscalationStep` all remain embedded/pass-through structs.
- **`maintainability.md`**: `ServiceName` values (`Notify`, `IntegCfg`) are ≤10 chars; `ServiceArea` (78) identical
  across both services; primary key auto-generation via `common.GenerateID`, sourced from `l8common`. Second
  Instance Rule / Copy-Paste Detection satisfied at the ecosystem level via `l8common.ActivateService` (Phase 0.5a)
  and `IIntegration`/`INotify` replacing what would otherwise have been two near-identical config-lookup paths.
- **`framework-interface-boundaries.md`**: three changes reach outside `l8notify`'s own repo, each its own phase
  requiring explicit sign-off: Phase 0.5a's `l8common.ServiceConfig` extension (additive, implementation layer, not
  `ifs`); Phase 0's `l8notifysvc` types + `INotify`/`IIntegration` on `IResources` (this one **does** touch
  `l8types/go/ifs` — justified because `l8notify`, like `l8events`, is meant to be usable via `IResources` by any
  service in the ecosystem, mirroring the existing `IEvents` precedent exactly).
- **`security-config-structure.md`**: secrets are resolved exclusively via the `credentials` map +
  `ISecurityProvider.Credential(...)`, never stored in `IntegrationConfig`'s ORM row.
- **`vendor-and-git.md`**: no phase runs `go mod tidy`/`vendor`/`init` or any git command, in any of the five
  repos — every phase's Prerequisites section states what the *user* must push/vendor before that phase begins.
- **`l8ui-no-project-specific-code.md`**: `l8ui/notification/*` stays project-agnostic.
- **`l8query-rules.md`**: `ListIntegrationConfigs` (Phase 0.25.2) builds an explicit L8Query string rather than
  relying on an empty-filter lookup — avoids the Rule 3 trap from the start.
- **`immutability-ui-alignment.md`**: `NotifyRecord`'s read-only requirement is explicit in Phase 5.1
  (`openViewForm`, not `openEditForm`). `IntegrationConfig` is correctly editable — Phase 5.2 doesn't apply this
  rule to it.
- **`test-location-and-approach.md`**: Phase 7 places new tests in the consumer's `go/tests/`, HTTP-API-only, and
  Phase 0.5b flags pre-existing misplaced tests in `l8events` (touched by that phase) alongside `l8notify`'s own.
- **`single-owner-database-table.md`**: Phase 6.1 states explicitly, as a bolded `README.md` warning, that both
  `ActivateNotify` and `ActivateIntegrationConfig` must run in exactly one process per consumer project.
- **`plan-requirements.md` Platform Completeness / `mobile-rules.md`**: Phase 5.4 is the required desktop×mobile
  audit, explicitly deferred with reason.

---

## Traceability Matrix

| # | Gap / Action Item | Phase |
|---|-------------------|-------|
| 1 | `L8SysConfig` was the wrong home for connection config — over-applied the DB-credential bootstrap analogy | Design Decision 2 |
| 2 | No generalized Prime Object for integration endpoints (was two separate config concepts) | Phase 0.1 |
| 3 | `l8notify` has no free package name in `l8types/go/types/` (collision with existing `l8notify` change-notification package) | Design Decision 4 |
| 4 | No `INotify`/`IIntegration` resource-interface access, unlike the existing `IEvents` precedent | Phase 0.2, 0.3, 0.4 |
| 5 | No default concrete implementations of `INotify`/`IIntegration` | Phase 0.25 |
| 6 | `Integration` lookup should use the local-service-handler-first pattern, not always a full request round-trip | Phase 0.25.2 |
| 7 | `l8events` and `l8notify` would each hand-roll SLA/web-service + ID-generation boilerplate `l8common` already implements | Phase 0.5a |
| 8 | `l8notify`'s outbound webhook HMAC signing would duplicate a primitive instead of sharing one | Phase 0.5a, 4 |
| 9 | `l8events` still on its own hand-rolled activation code | Phase 0.5b |
| 10 | No ORM persistence / CRUD service for `IntegrationConfig` | Phase 3 |
| 11 | No ORM persistence layer for `NotifyRecord`, no activatable service with real dispatch-on-POST | Phase 4 |
| 12 | l8ui SMTP/webhook components assumed a live CRUD backend that (in a prior revision) didn't exist | Phase 5.2 |
| 13 | l8ui delivery-log component used ad hoc render() instead of standard data-only pattern | Phase 5.1 |
| 14 | `NotifyRecord` immutability not reflected as an explicit UI requirement | Phase 5.1 |
| 15 | No mobile platform audit for the reworked/new l8ui components | Phase 5.4 |
| 16 | No documented consumer integration steps (mirrors `events-service-required.md`) | Phase 6 |
| 17 | No documented credentials setup for consumers | Phase 6.3 |
| 18 | Single-owner-per-table constraint not stated for either service | Phase 6.1 |
| 19 | No test plan / location guidance for the new services | Phase 7 |
| 20 | No build/runtime verification pass across all five affected repos | Phase 8 |
| 21 | Phases weren't independently executable across separate, memoryless sessions (no prerequisites stated, Phase 3 had no standalone deliverable, Phase 0.5 spanned two repos) | This revision — Prerequisites added throughout, Phase 3 merged into 4, Phase 0.5 split into 0.5a/0.5b |

---

## File Summary

| Category | New | Modified | Deleted |
|----------|-----|----------|---------|
| `l8types` proto + generated (Phase 0) | 1 (`l8notifysvc` package) | 0 | 0 |
| `l8types` go/ifs (Phase 0) | 2 (`Notify.go`, `Integration.go`) | 1 (`Resources.go`) | 0 |
| `l8utils` go/utils (Phase 0.25) | 2 (`notify/`, `integration/`) | 1 (`resources/Resources.go`) | 0 |
| `l8common` go/common (Phase 0.5a) | 1 (`hmac.go`) | 1 (`service_factory.go`) | 0 |
| `l8events` go/services, go.mod (Phase 0.5b) | 0 | 2 | 1 (`go/common/activate.go`) |
| `l8notify` proto/types (Phase 1, folded into 3/4) | 0 | 0 | 3 (`proto/l8notify.proto`, `proto/make-bindings.sh`, `go/types/l8notify/`) |
| `l8notify` go/services (Phase 3, 4) | 2 (`IntegrationConfigService.go`, `NotifyRecordService.go`) | 0 | 0 |
| `l8notify` go/channel (Phase 4) | 0 | 1 (`webhook.go`) | 0 |
| `l8notify` go.mod (Phase 3/4) | 0 | 1 | 0 |
| `l8notify` l8ui/notification (Phase 5) | 1 (`l8notify-integration-mgmt.js`) | 2 (`l8notify-delivery-log.js`, `l8notify-enums.js`) | 2 (`l8notify-smtp-config.js`, `l8notify-webhook-mgmt.js`) |
| `l8notify` docs (Phase 6/7) | 0 | 1 (`README.md`) | 0 |
| **Total** | **9 new** | **9 modified** | **6 deleted** |

---

## Open Items for Peer Review

1. **`l8types/go/types/l8notifysvc` naming** (Design Decision 4) — the working default; needs explicit confirmation
   since it's a framework-owner naming call.
2. **`ServiceArea = 78`** — confirmed free against every sibling project checked; worth a final grep before Phase 3.
3. **`l8types`/`l8utils` sign-off** (Phase 0, 0.25) — these touch actual interface contracts (`IResources`); needs
   explicit agreement before Phase 0's session starts.
4. **`l8common`/`l8events` sign-off** (Phase 0.5a/0.5b) — `l8common` has 10+ existing dependents; additive-only
   change, but still needs a build-verify pass against an existing consumer before Phase 0.5b begins.
5. **`Security().Credential()` return order** (Phase 4.1/4.2) — verify the real order against a working call site
   before implementation.
6. **Dispatch-on-POST vs. log-only** (Design Decision 1).
7. **Pre-existing misplaced tests** in `l8notify` and `l8events` — flagged in Phase 0.5b/7, out of scope here.
8. **`l8common.ActivateService`'s fixed endpoint set** (Phase 0.5a.4).
9. **`l8web`'s HMAC code stays un-consolidated** (Phase 0.5a.2) — accepted duplication.
10. **`IntegrationConfig`'s flat-fields-vs-`oneof` shape** (Phase 0.1) — revisit once a third/fourth integration
    type is added.
11. **Reentrancy/deadlock risk** — `NotifyCallback.Before()` (Phase 4) synchronously calls out to the `IntegCfg`
    service (Phase 0.25.2) and then to an external system (`channel.Dispatch`), all within one incoming request's
    handling. Not yet verified against the framework's actual threading/locking model — do this check before
    Phase 4's session, since the whole design assumes it's safe.
12. **Notification-storm risk** — `vnic.Resources().Notify().Send(...)` (Phase 0.25.1) is a trivially-callable,
    ecosystem-wide primitive with no built-in rate limiting (`throttle` stays opt-in, Design Decision 3). Any
    service can now spam SMTP/webhook endpoints with no guardrail.
13. **Naming overlap** — `l8events` already has `PostIntegrationEvent(*l8events.IntegrationEvent)` ("external
    system integration event"). `IIntegration`/`vnic.Resources().Integration()` (this plan) is a different concept
    (endpoint config lookup) that happens to share the word "Integration" — a readability risk, not a compile
    conflict.
