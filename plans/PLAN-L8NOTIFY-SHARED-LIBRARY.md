# L8Notify — Shared Notification, Email & Webhook Library

## Purpose

Extract the notification/escalation/dispatch infrastructure from l8alarms into a shared library (`l8notify`) that both l8alarms and l8erp (and any future Layer 8 project) can consume. Then add the missing capabilities (SMTP email, webhook delivery logging, SMTP config persistence) that neither project has today.

---

## Current State

### l8alarms Has (to be extracted)
| Component | File | Lines | Status |
|-----------|------|-------|--------|
| Notification engine (policy matching, throttling, dispatch) | `notification/engine.go` | 202 | Working |
| Channel sender (webhook, slack stubs) | `notification/sender.go` | 48 | Webhook+Slack work; email/pagerduty are stubs |
| Template renderer (`{{field}}` placeholders) | `notification/engine.go:164-201` | 38 | Working but alarm-specific |
| Escalation scheduler (timer-based step chains) | `escalation/scheduler.go` | 181 | Working |
| NotificationPolicy proto (targets, throttling, filters) | `alm-policies.proto` | ~40 | Working |
| EscalationPolicy proto (steps, channels) | `alm-policies.proto` | ~30 | Working |
| NotificationChannel enum (EMAIL, WEBHOOK, SLACK, PAGERDUTY, CUSTOM) | `alm-common.proto` | 8 | Working |
| PolicyStatus enum (ACTIVE, DISABLED) | `alm-common.proto` | 4 | Working |

### Neither Project Has
| Component | Notes |
|-----------|-------|
| SMTP email sending | l8alarms stubs it (`sendLog`); l8erp has nothing |
| SMTP configuration persistence | No SmtpConfig type anywhere |
| Webhook delivery logging | Fire-and-forget with no audit trail |
| Webhook HMAC signing | No signature verification support |
| Webhook retry with backoff | Single attempt, no retry |
| Generic template variables | l8alarms hardcodes `{{alarm.*}}`; needs `{{key}}` generic map |

### l8notify Repo (starting state)
Empty — only LICENSE, README.md, .gitignore.

---

## Architecture

### What Goes Into l8notify

l8notify is a **library**, not a standalone service. It provides:

1. **Proto types** — shared notification/escalation/channel definitions
2. **Template engine** — generic `{{key}}` substitution from `map[string]string`
3. **Channel dispatcher** — webhook (with HMAC + retry), email (SMTP), slack, extensible
4. **Throttle engine** — cooldown + hourly rate limiting per policy
5. **Escalation scheduler** — generic time-based step progression
6. **Shared l8ui components** — reusable UI for notification admin (SMTP config, webhook management, delivery logs, notification target editor)

### What Stays in Consumer Projects

Each consumer (l8alarms, l8erp) keeps:
- **Policy matching logic** — alarm-specific filters (severity, definition IDs, node types) stay in l8alarms; ERP-specific filters (module, service, event type) stay in l8erp
- **Event emission** — each project decides WHEN to emit (After() hooks, business logic)
- **Runner/adapter code** — thin wrappers that call l8notify APIs
- **Service definitions** — each project manages its own NotificationPolicy/Rule service with its own proto types that embed l8notify's shared types
- **Policy-specific UI** — rule filter criteria are project-specific (alarm severity filters vs ERP module/event filters); the shared l8ui components handle the generic parts (target editing, SMTP config, webhook management, delivery logs)

### Dependency Direction

```
l8notify  (shared library — Go backend + l8ui components)
   ^             ^
   |             |
l8alarms      l8erp
```

**Go backend** depends only on: `l8types` (for `ifs` interfaces), Go stdlib (`net/smtp`, `net/http`, `crypto/hmac`), `google.golang.org/protobuf`.

**l8ui components** depend on: l8ui shared library (Layer8EnumFactory, Layer8FormFactory, Layer8DRenderers, `--layer8d-*` CSS tokens). They are copied into each consumer project's `l8ui/notification/` directory alongside the existing l8ui components.

l8notify does NOT depend on: `l8orm`, `l8services`, `l8bus`, `l8web`, `l8reflect`, or any consumer project.

---

## Directory Structure

```
l8notify/
├── proto/
│   ├── make-bindings.sh
│   └── l8notify.proto              # Shared types (channels, delivery status, targets)
├── go/
│   ├── go.mod
│   ├── types/
│   │   └── l8notify/
│   │       └── l8notify.pb.go      # Generated proto types
│   ├── template/
│   │   └── template.go             # Generic {{key}} template renderer
│   ├── channel/
│   │   ├── channel.go              # Dispatcher interface + factory
│   │   ├── webhook.go              # Webhook sender (HMAC, retry, backoff)
│   │   ├── email.go                # SMTP email sender
│   │   └── slack.go                # Slack webhook sender
│   ├── throttle/
│   │   └── throttle.go             # Cooldown + hourly rate limiter
│   └── escalation/
│       └── scheduler.go            # Generic time-based escalation scheduler
├── l8ui/
│   └── notification/
│       ├── l8notify-enums.js        # Shared enums (NotifyChannel, DeliveryStatus)
│       ├── l8notify-smtp-config.js  # SMTP config form component
│       ├── l8notify-webhook-mgmt.js # Webhook endpoint management (table + form)
│       ├── l8notify-delivery-log.js # Delivery log viewer (table + detail)
│       ├── l8notify-target-editor.js # Inline target editor (channel, endpoint, template)
│       └── l8notify-notification.css # Shared notification admin styles
├── plans/
│   └── PLAN-L8NOTIFY-SHARED-LIBRARY.md
├── LICENSE
├── README.md
└── .gitignore
```

---

## Protobuf Types

### `proto/l8notify.proto`

```protobuf
syntax = "proto3";
package l8notify;
option go_package = "./types/l8notify";

// ─── Enums ───

// Channel types for notification delivery.
// Consumers extend behavior via the CUSTOM channel + custom sender registration.
enum NotifyChannel {
  NOTIFY_CHANNEL_UNSPECIFIED = 0;
  NOTIFY_CHANNEL_EMAIL = 1;
  NOTIFY_CHANNEL_WEBHOOK = 2;
  NOTIFY_CHANNEL_SLACK = 3;
  NOTIFY_CHANNEL_PAGERDUTY = 4;
  NOTIFY_CHANNEL_CUSTOM = 5;
}

// Status of a delivery attempt.
enum DeliveryStatus {
  DELIVERY_STATUS_UNSPECIFIED = 0;
  DELIVERY_STATUS_PENDING = 1;
  DELIVERY_STATUS_SENT = 2;
  DELIVERY_STATUS_FAILED = 3;
  DELIVERY_STATUS_RETRYING = 4;
}

// ─── Shared Message Types ───

// A delivery target: where and how to send a notification.
// Embedded in consumer-specific policy types (not a Prime Object).
message NotifyTarget {
  string target_id = 1;
  NotifyChannel channel = 2;
  string endpoint = 3;           // email address, webhook URL, Slack webhook URL, etc.
  string template = 4;           // message template with {{key}} placeholders
}

// SMTP connection configuration.
// Consumers store this in their own service; l8notify reads it as a struct.
message SmtpConfig {
  string host = 1;
  int32 port = 2;
  string username = 3;
  string password = 4;
  bool use_tls = 5;
  string from_address = 6;
  string from_name = 7;
}

// Webhook endpoint configuration.
// Consumers store this in their own service; l8notify reads it as a struct.
message WebhookConfig {
  string url = 1;
  string secret = 2;            // HMAC-SHA256 signing secret
  int32 retry_count = 3;        // max retries (default 3)
  int32 timeout_ms = 4;         // HTTP timeout (default 5000)
}

// Result of a single delivery attempt.
message DeliveryResult {
  DeliveryStatus status = 1;
  int32 http_status = 2;        // for webhook/slack (0 for email)
  string error_message = 3;
  int32 attempt = 4;            // 1-based attempt number
  int64 sent_at = 5;            // Unix timestamp
}

// Configuration for the escalation scheduler.
// Consumers embed this in their own escalation policy types.
message EscalationStep {
  string step_id = 1;
  int32 step_order = 2;
  int32 delay_minutes = 3;
  NotifyChannel channel = 4;
  string endpoint = 5;
  string message_template = 6;
}
```

**Design choice**: These are shared building-block types, not Prime Objects. They have no List wrappers and no services. Consumer projects embed them in their own policy types and manage persistence themselves.

---

## Implementation Phases

### Phase 1: Proto Types & Project Scaffold

**1.1 Go module**
- Create `go/go.mod` with `module github.com/saichler/l8notify`
- Minimal dependencies: `google.golang.org/protobuf`

**1.2 Proto file**
- Create `proto/l8notify.proto` (as defined above)
- Create `proto/make-bindings.sh` (compile + move to `go/types/l8notify/`)
- Run `make-bindings.sh`

**1.3 Verify**
- `cd go && go build ./...` — zero errors

**Files**: 3 new (go.mod, l8notify.proto, make-bindings.sh) + 1 generated (l8notify.pb.go)

### Phase 2: Template Engine

**2.1 Create `go/template/template.go`**

Generic `{{key}}` template renderer. Unlike l8alarms' hardcoded `{{alarm.*}}` fields, this takes a `map[string]string` of variables.

```go
package template

// Render replaces all {{key}} placeholders in tmpl with values from vars.
// Unknown placeholders are left as-is. Returns the rendered string.
func Render(tmpl string, vars map[string]string) string

// RenderWithDefault replaces placeholders; unknown keys get defaultValue.
func RenderWithDefault(tmpl string, vars map[string]string, defaultValue string) string
```

Implementation: port l8alarms' `replaceAll`/`indexOf` approach but make it generic (iterate `vars` map, replace `{{key}}` with `value` for each).

**Files**: 1 new

### Phase 3: Channel Dispatcher

**3.1 Dispatcher interface — `go/channel/channel.go`**

```go
package channel

import ntf "github.com/saichler/l8notify/go/types/l8notify"

// Sender delivers a message to an endpoint.
type Sender interface {
    Send(endpoint, message string) (*ntf.DeliveryResult, error)
}

// Dispatch sends a message to a NotifyTarget using the appropriate channel.
// smtpCfg may be nil if no SMTP is configured (email sends will fail gracefully).
// webhookSecrets maps endpoint URLs to HMAC secrets (may be nil).
func Dispatch(target *ntf.NotifyTarget, message string, smtpCfg *ntf.SmtpConfig,
    webhookSecrets map[string]string) *ntf.DeliveryResult

// RegisterCustomSender registers a Sender for NOTIFY_CHANNEL_CUSTOM.
// Consumer projects call this at startup to extend dispatch.
func RegisterCustomSender(name string, sender Sender)
```

**3.2 Webhook sender — `go/channel/webhook.go`**

Port from l8alarms' `sendWebhook` and add:
- HMAC-SHA256 signature in `X-L8-Signature` header (using `WebhookConfig.Secret`)
- Configurable timeout (from `WebhookConfig.TimeoutMs`, default 5000ms)
- Retry with exponential backoff: 1s, 2s, 4s... up to `WebhookConfig.RetryCount` (default 3)
- Returns `DeliveryResult` with status, httpStatus, attempt count, error

```go
func SendWebhook(cfg *ntf.WebhookConfig, message string) *ntf.DeliveryResult
```

If `cfg` is nil (simple endpoint-only mode for backward compat):
```go
func SendWebhookSimple(endpoint, message string) *ntf.DeliveryResult
```

**3.3 Email sender — `go/channel/email.go`**

New — does not exist anywhere today.

```go
func SendEmail(cfg *ntf.SmtpConfig, to, subject, body string) *ntf.DeliveryResult
```

- Uses Go `net/smtp` with `smtp.PlainAuth`
- TLS support via `tls.Dial` when `cfg.UseTls` is true
- MIME headers for HTML content (`Content-Type: text/html; charset=UTF-8`)
- Returns `DeliveryResult` with status and error

**3.4 Slack sender — `go/channel/slack.go`**

Port from l8alarms' Slack path in `sendWebhook`:
```go
func SendSlack(webhookURL, message string) *ntf.DeliveryResult
```
Posts `{"text":"..."}` JSON payload to Slack incoming webhook URL.

**Files**: 4 new

### Phase 4: Throttle Engine

**4.1 Create `go/throttle/throttle.go`**

Extract from l8alarms' engine.go (throttle map + hourly counter):

```go
package throttle

// Throttler enforces per-key cooldown and hourly rate limits.
type Throttler struct { ... }

func New() *Throttler

// IsThrottled returns true if the key should be suppressed.
// cooldownSec: minimum seconds between sends for this key.
// maxPerHour: maximum sends per hour for this groupKey (0 = unlimited).
func (t *Throttler) IsThrottled(key string, groupKey string, cooldownSec, maxPerHour int32) bool

// Record marks a send for the given key and groupKey.
func (t *Throttler) Record(key string, groupKey string)

// Reset clears all state (useful for testing).
func (t *Throttler) Reset()
```

- `key` = per-entity throttle (e.g., "alarm-123+policy-456" or "order-789+rule-001")
- `groupKey` = per-policy hourly counter (e.g., "policy-456" or "rule-001")
- Thread-safe with `sync.Mutex`
- Hourly counter auto-resets when hour changes

**Files**: 1 new

### Phase 5: Escalation Scheduler

**5.1 Create `go/escalation/scheduler.go`**

Extract from l8alarms' scheduler.go and make it generic:

```go
package escalation

import ntf "github.com/saichler/l8notify/go/types/l8notify"

// StepHandler is called when an escalation step fires.
// Consumer provides this to customize delivery (e.g., look up SMTP config,
// create delivery log entries, update entity state).
type StepHandler func(entityID string, step *ntf.EscalationStep, message string) error

// Scheduler manages time-based escalation chains.
type Scheduler struct { ... }

func New(handler StepHandler) *Scheduler

// Schedule starts an escalation chain for the given entity.
// steps must be sorted by StepOrder. vars are template variables.
// If an escalation is already active for this entity, it is replaced.
func (s *Scheduler) Schedule(entityID string, steps []*ntf.EscalationStep, vars map[string]string)

// Cancel stops all pending escalation timers for the entity.
func (s *Scheduler) Cancel(entityID string)

// Active returns the number of entities with active escalation timers.
func (s *Scheduler) Active() int
```

- Each step spawns a goroutine with `time.After(step.DelayMinutes * time.Minute)`
- On fire: render template via `template.Render()`, call `StepHandler`
- On cancel: close cancel channel, goroutine exits
- Thread-safe with `sync.Mutex`

**Files**: 1 new

### Phase 6: Shared l8ui Notification Components

These components live in `l8notify/l8ui/notification/` and are copied into each consumer project's `l8ui/notification/` directory (same pattern as the existing toast notification component). They follow l8ui conventions: `--layer8d-*` CSS custom properties, no dark mode blocks, `Layer8` global namespace prefix.

**6.1 Shared enums — `l8notify-enums.js`**

Mirrors the proto enums for use in JS forms, columns, and renderers:

```javascript
window.L8NotifyEnums = {
    NOTIFY_CHANNEL: Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Email' },
        { value: 2, label: 'Webhook' },
        { value: 3, label: 'Slack' },
        { value: 4, label: 'PagerDuty' },
        { value: 5, label: 'Custom' }
    ]),
    DELIVERY_STATUS: Layer8EnumFactory.create([
        { value: 0, label: 'Unspecified' },
        { value: 1, label: 'Pending' },
        { value: 2, label: 'Sent' },
        { value: 3, label: 'Failed' },
        { value: 4, label: 'Retrying' }
    ])
};
```

Includes renderers:
- `L8NotifyEnums.render.channel` — plain enum renderer for NotifyChannel
- `L8NotifyEnums.render.deliveryStatus` — status badge renderer for DeliveryStatus (green=Sent, red=Failed, yellow=Pending/Retrying)

**6.2 SMTP config form — `l8notify-smtp-config.js`**

Reusable form component for SMTP configuration. Consumer projects call this from their admin pages.

```javascript
window.L8NotifySmtpConfig = {
    // Returns form definition for embedding in a consumer's admin page.
    // Uses Layer8FormFactory.
    getFormDefinition: function() { ... },

    // Renders SMTP config form inside a container element.
    // cfg: current SmtpConfig object (or null for new).
    // onSave: callback(smtpConfigData) called when user saves.
    render: function(container, cfg, onSave) { ... },

    // Sends a test email using the current config.
    // Uses consumer-provided endpoint to POST test request.
    sendTest: function(smtpConfig, testEmail, endpoint) { ... }
};
```

Form fields: Host, Port, Username, Password (masked), Use TLS (checkbox), From Address, From Name. Includes a "Send Test Email" button.

**6.3 Webhook management — `l8notify-webhook-mgmt.js`**

Table + form for managing webhook endpoints. Consumers embed this in their admin pages.

```javascript
window.L8NotifyWebhookMgmt = {
    // Renders webhook management table inside a container.
    // webhooks: array of WebhookConfig objects.
    // onSave: callback(webhookConfig) for add/edit.
    // onDelete: callback(webhookConfig) for delete.
    // onTest: callback(webhookConfig) for test ping.
    render: function(container, webhooks, onSave, onDelete, onTest) { ... }
};
```

Table columns: URL, Secret (masked), Retry Count, Timeout (ms), Actions (Edit, Delete, Test).
Form fields: URL, Secret, Retry Count (default 3), Timeout ms (default 5000).
Test button sends a ping payload and displays the DeliveryResult.

**6.4 Delivery log viewer — `l8notify-delivery-log.js`**

Read-only table for viewing delivery history. Consumers provide the data source.

```javascript
window.L8NotifyDeliveryLog = {
    // Renders delivery log table inside a container.
    // logs: array of DeliveryResult objects (augmented with target info by consumer).
    // options: { showTarget: bool, showChannel: bool, pageSize: int }
    render: function(container, logs, options) { ... }
};
```

Table columns: Timestamp (sentAt), Channel, Endpoint, Status (badge), HTTP Status, Attempt #, Error Message.
Detail popup on row click shows full error message and request/response details.

**6.5 Notification target editor — `l8notify-target-editor.js`**

Inline table editor for NotifyTarget arrays. Used inside policy/rule forms on any consumer project.

```javascript
window.L8NotifyTargetEditor = {
    // Returns inline table definition for use with f.inlineTable() in form definitions.
    // This is the standard pattern for editing repeated NotifyTarget fields.
    getInlineTableDef: function() {
        return {
            key: 'targets',
            label: 'Notification Targets',
            columns: [
                { key: 'targetId', label: 'ID', type: 'text', hidden: true },
                { key: 'channel', label: 'Channel', type: 'select',
                  options: L8NotifyEnums.NOTIFY_CHANNEL },
                { key: 'endpoint', label: 'Endpoint', type: 'text' },
                { key: 'template', label: 'Template', type: 'textarea' }
            ]
        };
    }
};
```

Consumer usage in forms:
```javascript
// In consumer's policy form definition:
f.section('Targets', [
    ...f.inlineTable(
        L8NotifyTargetEditor.getInlineTableDef().key,
        L8NotifyTargetEditor.getInlineTableDef().label,
        L8NotifyTargetEditor.getInlineTableDef().columns
    )
])
```

**6.6 CSS — `l8notify-notification.css`**

Shared styles for all notification admin components. Uses `--layer8d-*` theme tokens exclusively. No `[data-theme="dark"]` blocks. Covers:
- SMTP config form layout
- Webhook table action buttons
- Delivery status badge colors (maps to `--layer8d-success`, `--layer8d-error`, `--layer8d-warning`)
- Target editor inline table styling
- Test button states (loading, success, error)

**Files**: 6 new (5 JS + 1 CSS)

### Phase 7: Build Verification

**7.1 l8notify Go backend**
- `cd go && go build ./...` — library compiles
- `go vet ./...` — no issues

**7.2 l8ui components**
- Verify no JS syntax errors: `node -c` on all 5 JS files
- Verify `L8NotifyEnums`, `L8NotifySmtpConfig`, `L8NotifyWebhookMgmt`, `L8NotifyDeliveryLog`, `L8NotifyTargetEditor` are defined on `window`
- Verify CSS uses only `--layer8d-*` tokens, no hardcoded colors
- Verify no `[data-theme="dark"]` blocks in CSS

---

## Traceability Matrix

| # | Gap / Action Item | Phase |
|---|-------------------|-------|
| 1 | No shared proto types for channels/targets/delivery | Phase 1 |
| 2 | No generic template engine (l8alarms hardcodes alarm fields) | Phase 2 |
| 3 | No SMTP email implementation anywhere | Phase 3.3 |
| 4 | No webhook HMAC signing | Phase 3.2 |
| 5 | No webhook retry with backoff | Phase 3.2 |
| 6 | No configurable webhook sender (l8alarms is minimal) | Phase 3.2 |
| 7 | Slack sender not reusable (coupled to l8alarms) | Phase 3.4 |
| 8 | No shared throttle engine | Phase 4 |
| 9 | No shared escalation scheduler | Phase 5 |
| 10 | No shared JS enums for NotifyChannel/DeliveryStatus | Phase 6.1 |
| 11 | No reusable SMTP config UI component | Phase 6.2 |
| 12 | No reusable webhook management UI component | Phase 6.3 |
| 13 | No reusable delivery log viewer UI component | Phase 6.4 |
| 14 | No reusable notification target editor UI component | Phase 6.5 |
| 15 | Each consumer would duplicate notification admin UI | Phase 6 |
| 16 | No build verification | Phase 7 |

**Note**: Migration of l8alarms and l8erp to consume l8notify is covered in the separate `PLAN-L8ALARMS.md` and respective consumer project plans.

---

## File Summary

| Category | New Files | Modified Files |
|----------|-----------|----------------|
| l8notify proto | 2 (l8notify.proto, make-bindings.sh) | 0 |
| l8notify Go | 8 (go.mod, template.go, channel.go, webhook.go, email.go, slack.go, throttle.go, scheduler.go) | 0 |
| l8notify generated | 1 (l8notify.pb.go) | 0 |
| l8notify l8ui | 6 (l8notify-enums.js, l8notify-smtp-config.js, l8notify-webhook-mgmt.js, l8notify-delivery-log.js, l8notify-target-editor.js, l8notify-notification.css) | 0 |
| **Total** | **17 new** | **0 modified** |

**Note**: Consumer migration (l8alarms, l8erp) is covered in separate plans.

---

## API Summary (Consumer Cheat Sheet)

After l8notify is implemented, consumers use it like this:

```go
import (
    "github.com/saichler/l8notify/go/channel"
    "github.com/saichler/l8notify/go/template"
    "github.com/saichler/l8notify/go/throttle"
    "github.com/saichler/l8notify/go/escalation"
    ntf "github.com/saichler/l8notify/go/types/l8notify"
)

// Template rendering
msg := template.Render("Order {{orderNumber}} is now {{status}}", map[string]string{
    "orderNumber": "SO-001",
    "status":      "CONFIRMED",
})

// Channel dispatch
result := channel.Dispatch(target, msg, smtpCfg, webhookSecrets)

// Or direct channel calls
result := channel.SendEmail(smtpCfg, "user@example.com", "Subject", "<h1>Body</h1>")
result := channel.SendWebhook(webhookCfg, `{"event":"order_confirmed"}`)
result := channel.SendSlack("https://hooks.slack.com/...", "Order SO-001 confirmed")

// Throttling
t := throttle.New()
if !t.IsThrottled("order-123+rule-1", "rule-1", 60, 100) {
    channel.Dispatch(target, msg, smtpCfg, nil)
    t.Record("order-123+rule-1", "rule-1")
}

// Escalation
s := escalation.New(func(entityID string, step *ntf.EscalationStep, msg string) error {
    return channel.SendEmailSimple(smtpCfg, step.Endpoint, "Escalation", msg)
})
s.Schedule("alarm-123", steps, vars)
s.Cancel("alarm-123")  // on acknowledge/resolve
```

### l8ui Components (Consumer Cheat Sheet)

After copying `l8notify/l8ui/notification/` into your project's `l8ui/notification/`:

```html
<!-- Add to app.html (after l8ui shared scripts, before module scripts) -->
<link rel="stylesheet" href="l8ui/notification/l8notify-notification.css">
<script src="l8ui/notification/l8notify-enums.js"></script>
<script src="l8ui/notification/l8notify-smtp-config.js"></script>
<script src="l8ui/notification/l8notify-webhook-mgmt.js"></script>
<script src="l8ui/notification/l8notify-delivery-log.js"></script>
<script src="l8ui/notification/l8notify-target-editor.js"></script>
```

```javascript
// SMTP config in admin page
L8NotifySmtpConfig.render(container, currentConfig, (newConfig) => {
    // POST newConfig to your service endpoint
});

// Webhook management in admin page
L8NotifyWebhookMgmt.render(container, webhooks, onSave, onDelete, onTest);

// Delivery log viewer
L8NotifyDeliveryLog.render(container, deliveryResults, { pageSize: 25 });

// Target editor in policy/rule forms
f.section('Targets', [
    ...f.inlineTable(
        L8NotifyTargetEditor.getInlineTableDef().key,
        L8NotifyTargetEditor.getInlineTableDef().label,
        L8NotifyTargetEditor.getInlineTableDef().columns
    )
])

// Shared enums in column/form definitions
...col.enum('channel', 'Channel', null, L8NotifyEnums.render.channel)
...col.status('status', 'Status', null, L8NotifyEnums.render.deliveryStatus)
```
