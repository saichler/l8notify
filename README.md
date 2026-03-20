# l8notify

Shared notification library for Layer 8 applications. Provides reusable Go packages and l8ui components for notification delivery, template rendering, throttling, and escalation scheduling.

l8notify is a **library**, not a standalone service. Consumer projects (l8alarms, l8erp, etc.) import the Go packages and copy the l8ui components into their own web directories.

---

## Directory Structure

```
l8notify/
├── proto/
│   ├── l8notify.proto              # Shared protobuf types
│   └── make-bindings.sh            # Generates go/types/l8notify/l8notify.pb.go
├── go/
│   ├── go.mod                      # Module: github.com/saichler/l8notify/go
│   ├── test.sh                     # Runs all tests with coverage report
│   ├── types/l8notify/
│   │   └── l8notify.pb.go          # Generated proto types
│   ├── channel/
│   │   ├── channel.go              # Dispatch router + Sender interface + custom sender registry
│   │   ├── email.go                # SMTP email sender (plain + TLS)
│   │   ├── webhook.go              # Webhook sender (HMAC-SHA256 signing + retry with backoff)
│   │   ├── slack.go                # Slack incoming webhook sender
│   │   └── channel_test.go         # Tests for dispatch routing and custom senders
│   ├── template/
│   │   ├── template.go             # Generic {{key}} placeholder renderer
│   │   └── template_test.go        # Tests for template rendering edge cases
│   ├── throttle/
│   │   ├── throttle.go             # Per-key cooldown + hourly rate limiter
│   │   └── throttle_test.go        # Tests for cooldown and hourly limits
│   └── escalation/
│       ├── scheduler.go            # Time-based escalation chain scheduler
│       └── scheduler_test.go       # Tests for escalation scheduling and cancellation
├── l8ui/notification/
│   ├── l8notify-enums.js           # NotifyChannel + DeliveryStatus enums with renderers
│   ├── l8notify-smtp-config.js     # SMTP configuration form component
│   ├── l8notify-webhook-mgmt.js    # Webhook endpoint CRUD (table + form)
│   ├── l8notify-delivery-log.js    # Delivery log viewer (table + detail popup)
│   ├── l8notify-target-editor.js   # Inline table definition for NotifyTarget arrays
│   └── l8notify-notification.css   # Shared styles using --layer8d-* theme tokens
└── plans/                          # Implementation plans
```

---

## Protobuf Types

All types are shared building blocks embedded in consumer-specific types. None are Prime Objects — they have no services or List wrappers.

| Type | Purpose |
|------|---------|
| `NotifyChannel` | Enum: EMAIL, WEBHOOK, SLACK, PAGERDUTY, CUSTOM |
| `DeliveryStatus` | Enum: PENDING, SENT, FAILED, RETRYING |
| `NotifyTarget` | Delivery target (channel + endpoint + template) |
| `SmtpConfig` | SMTP connection settings |
| `WebhookConfig` | Webhook endpoint with HMAC secret, retry count, timeout |
| `DeliveryResult` | Result of a delivery attempt (status, HTTP code, error, attempt #) |
| `EscalationStep` | Single step in an escalation chain (delay, channel, endpoint, template) |

### Embedding l8notify Types in Consumer Protos

Consumer projects import l8notify types into their own proto files and embed them in their own policy/rule types. The consumer proto owns the policy structure; l8notify provides the building blocks.

```protobuf
syntax = "proto3";
package myproject;
import "l8notify.proto";  // import the shared types

// Consumer-specific notification rule with project-specific filter criteria
message NotificationRule {
  string rule_id = 1;
  string name = 2;
  bool enabled = 3;

  // Project-specific filter criteria (what triggers the notification)
  string module_filter = 10;          // e.g., "sales", "inventory"
  string event_type_filter = 11;      // e.g., "order_created", "stock_low"
  int32 severity_filter = 12;         // consumer decides what this means

  // Embedded l8notify types (shared building blocks)
  repeated l8notify.NotifyTarget targets = 20;   // where to send
  int32 cooldown_seconds = 21;                    // throttle config
  int32 max_per_hour = 22;                        // throttle config

  // Consumer manages persistence — l8notify does not store anything
}

// Consumer-specific escalation policy
message EscalationPolicy {
  string policy_id = 1;
  string name = 2;
  string entity_type = 3;            // what entity this applies to
  repeated l8notify.EscalationStep steps = 10;  // shared escalation steps
}

// Consumer stores SMTP config in their own service
message ProjectSettings {
  string settings_id = 1;
  l8notify.SmtpConfig smtp_config = 10;
  repeated l8notify.WebhookConfig webhook_configs = 11;
}
```

**Key point**: l8notify never persists anything. The consumer project stores policies, rules, SMTP configs, webhook configs, and delivery logs in its own services. l8notify only provides the types and the runtime behavior (dispatch, throttle, escalate).

---

## Go Integration

### Step 1: Add the Dependency

```bash
cd go
# Add l8notify to go.mod
GOPROXY=direct GOPRIVATE=github.com go get github.com/saichler/l8notify/go@latest
go mod vendor
```

After vendoring, l8notify code is at `go/vendor/github.com/saichler/l8notify/go/`.

### Step 2: Import Packages

```go
import (
    "github.com/saichler/l8notify/go/channel"
    "github.com/saichler/l8notify/go/template"
    "github.com/saichler/l8notify/go/throttle"
    "github.com/saichler/l8notify/go/escalation"
    ntf "github.com/saichler/l8notify/go/types/l8notify"
)
```

### Step 3: Wire Up in ServiceCallback

The typical integration point is a ServiceCallback `After()` hook. When an entity is created/updated/deleted, the callback evaluates notification rules and dispatches via l8notify.

```go
// In your ServiceCallback After() method:
func (cb *MyServiceCallback) After(action byte, params ...interface{}) {
    if action == ifs.POST || action == ifs.PUT {
        entity := params[0].(*myproject.MyEntity)
        cb.evaluateNotificationRules(entity, action)
    }
}

func (cb *MyServiceCallback) evaluateNotificationRules(entity *myproject.MyEntity, action byte) {
    // 1. Load notification rules from your service (consumer responsibility)
    rules := cb.loadMatchingRules(entity)

    for _, rule := range rules {
        // 2. Build template variables from your entity (consumer responsibility)
        vars := map[string]string{
            "entityId":   entity.Id,
            "name":       entity.Name,
            "status":     entity.Status.String(),
            "actionType": actionLabel(action),
        }

        // 3. For each target on the rule, render + throttle + dispatch
        for _, target := range rule.Targets {
            throttleKey := entity.Id + "+" + rule.RuleId
            groupKey := rule.RuleId

            if cb.throttler.IsThrottled(throttleKey, groupKey, rule.CooldownSeconds, rule.MaxPerHour) {
                continue
            }

            // Render the target's template with entity variables
            msg := template.Render(target.Template, vars)

            // Dispatch via the appropriate channel
            result := channel.Dispatch(target, msg, cb.smtpCfg, cb.webhookSecrets)

            // Record the send for throttling
            if result.Status == ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
                cb.throttler.Record(throttleKey, groupKey)
            }

            // Log the delivery result (consumer responsibility — store in your service)
            cb.logDeliveryResult(rule.RuleId, target, result)
        }
    }
}
```

---

## Go Package API Reference

### channel — Notification Dispatch

```go
import "github.com/saichler/l8notify/go/channel"
```

#### Dispatch

Routes a message to the appropriate sender based on `target.Channel`.

```go
func Dispatch(
    target *ntf.NotifyTarget,       // where to send (channel + endpoint + template)
    message string,                  // rendered message body
    smtpCfg *ntf.SmtpConfig,       // SMTP config (nil if no email configured)
    webhookSecrets map[string]string, // endpoint URL → HMAC secret (nil if no signing)
) *ntf.DeliveryResult
```

Channel routing:
- `NOTIFY_CHANNEL_EMAIL` → `SendEmail(smtpCfg, target.Endpoint, "Notification", message)`
- `NOTIFY_CHANNEL_WEBHOOK` → `SendWebhook` (with HMAC if secret found) or `SendWebhookSimple`
- `NOTIFY_CHANNEL_SLACK` → `SendSlack(target.Endpoint, message)`
- `NOTIFY_CHANNEL_PAGERDUTY` → returns FAILED (not yet implemented)
- `NOTIFY_CHANNEL_CUSTOM` → iterates registered custom senders

#### SendEmail

Sends an email via SMTP. Supports plain and TLS connections.

```go
func SendEmail(
    cfg *ntf.SmtpConfig,  // host, port, username, password, useTls, fromAddress, fromName
    to string,             // recipient email address
    subject string,        // email subject line
    body string,           // email body (HTML supported, Content-Type: text/html)
) *ntf.DeliveryResult
```

#### SendWebhook

Posts JSON to a webhook endpoint with HMAC-SHA256 signing and retry with exponential backoff.

```go
func SendWebhook(
    cfg *ntf.WebhookConfig,  // url, secret, retryCount (default 3), timeoutMs (default 5000)
    message string,           // message content (wrapped in {"message":"..."} JSON)
) *ntf.DeliveryResult
```

- Signs request body with HMAC-SHA256 using `cfg.Secret`, sent in `X-L8-Signature` header
- Retries on failure with exponential backoff: 1s, 2s, 4s...
- Returns result of last attempt if all fail

#### SendWebhookSimple

Posts JSON without HMAC signing or retry. Single attempt, 5s timeout.

```go
func SendWebhookSimple(endpoint, message string) *ntf.DeliveryResult
```

#### SendSlack

Posts `{"text":"..."}` to a Slack incoming webhook URL.

```go
func SendSlack(webhookURL, message string) *ntf.DeliveryResult
```

#### RegisterCustomSender / Sender Interface

Extend dispatch with custom channels. Consumer registers at startup; Dispatch tries all registered senders for `NOTIFY_CHANNEL_CUSTOM`.

```go
type Sender interface {
    Send(endpoint, message string) (*ntf.DeliveryResult, error)
}

func RegisterCustomSender(name string, sender Sender)
```

### template — Message Rendering

```go
import "github.com/saichler/l8notify/go/template"
```

#### Render

Replaces `{{key}}` placeholders with values from the vars map. Unknown placeholders are left as-is.

```go
func Render(tmpl string, vars map[string]string) string
```

#### RenderWithDefault

Same as Render, but replaces any remaining `{{...}}` placeholders with `defaultValue`.

```go
func RenderWithDefault(tmpl string, vars map[string]string, defaultValue string) string
```

### throttle — Rate Limiting

```go
import "github.com/saichler/l8notify/go/throttle"
```

#### New

Creates a new Throttler. Thread-safe — safe to share across goroutines.

```go
func New() *Throttler
```

#### IsThrottled

Returns true if the key should be suppressed. Check this BEFORE dispatching.

```go
func (t *Throttler) IsThrottled(
    key string,        // per-entity throttle key (e.g., "entity-123+rule-456")
    groupKey string,   // per-policy hourly counter key (e.g., "rule-456")
    cooldownSec int32, // minimum seconds between sends for this key (0 = no cooldown)
    maxPerHour int32,  // max sends per hour for this groupKey (0 = unlimited)
) bool
```

#### Record

Marks a successful send. Call this AFTER a successful dispatch.

```go
func (t *Throttler) Record(key string, groupKey string)
```

#### Reset

Clears all throttle state. Useful for testing.

```go
func (t *Throttler) Reset()
```

### escalation — Timed Step Chains

```go
import "github.com/saichler/l8notify/go/escalation"
```

#### StepHandler

Callback type invoked when an escalation step fires. The consumer implements this to perform the actual delivery (look up SMTP config, call `channel.Dispatch`, log the result, etc.).

```go
type StepHandler func(entityID string, step *ntf.EscalationStep, message string) error
```

#### New

Creates a new Scheduler with the given step handler.

```go
func New(handler StepHandler) *Scheduler
```

#### Schedule

Starts an escalation chain for the given entity. Steps are sorted by `StepOrder` automatically. Each step fires after its `DelayMinutes` via a goroutine timer. The step's `MessageTemplate` is rendered using `template.Render(tmpl, vars)` before calling the handler.

If an escalation is already active for this entity, it is cancelled and replaced.

```go
func (s *Scheduler) Schedule(
    entityID string,                  // entity being escalated
    steps []*ntf.EscalationStep,     // escalation steps (sorted by StepOrder)
    vars map[string]string,           // template variables for message rendering
)
```

#### Cancel

Stops all pending escalation timers for the entity. Call this when the entity is acknowledged, resolved, or deleted.

```go
func (s *Scheduler) Cancel(entityID string)
```

#### Active

Returns the number of entities with active escalation timers.

```go
func (s *Scheduler) Active() int
```

---

## Consumer Responsibilities

l8notify provides the delivery infrastructure. The consumer project is responsible for everything else:

| Responsibility | l8notify | Consumer |
|----------------|----------|----------|
| Protobuf types for targets, channels, delivery results | Provides | Embeds in own types |
| Policy/rule types with filter criteria | -- | Defines and persists |
| SMTP config storage | Provides `SmtpConfig` struct | Stores in own service |
| Webhook config storage | Provides `WebhookConfig` struct | Stores in own service |
| Delivery result logging | Returns `DeliveryResult` | Stores in own service |
| Template rendering | `template.Render()` | Builds vars map from entity fields |
| Channel dispatch | `channel.Dispatch()` | Calls it with target + message |
| Throttling | `throttle.IsThrottled/Record()` | Holds `*Throttler` instance, defines keys |
| Escalation scheduling | `escalation.Schedule/Cancel()` | Holds `*Scheduler` instance, provides `StepHandler` |
| Event emission (when to notify) | -- | ServiceCallback After() hooks |
| Policy matching (which rules apply) | -- | Filter logic in callback |
| Custom channels | `RegisterCustomSender()` | Implements `Sender` interface |
| Admin UI for SMTP/webhooks | Provides l8ui components | Embeds in admin pages |
| Notification rule UI | Provides target editor | Builds rule form with project-specific filters |

---

## End-to-End Flow

This is the typical notification flow in a consumer project:

```
1. Entity event (POST/PUT/DELETE)
   └─ ServiceCallback.After() fires

2. Load matching rules
   └─ Consumer queries own NotificationRule service
   └─ Filter by module, event type, severity, etc. (consumer-specific)

3. For each matching rule, for each target:
   ├─ Build template vars from entity fields       → consumer
   ├─ Check throttle                                → throttle.IsThrottled()
   ├─ Render message template                       → template.Render(target.Template, vars)
   ├─ Dispatch to channel                           → channel.Dispatch(target, msg, smtp, secrets)
   ├─ Record throttle on success                    → throttle.Record()
   └─ Log delivery result                           → consumer stores DeliveryResult

4. For escalation policies:
   ├─ On entity create/update (unresolved state)    → scheduler.Schedule(entityID, steps, vars)
   └─ On entity resolve/acknowledge                 → scheduler.Cancel(entityID)
```

---

## l8ui Integration

### Step 1: Copy Files

Copy `l8notify/l8ui/notification/` into the consumer project's web directory:

```bash
cp -r <path-to-l8notify>/l8ui/notification/ <consumer>/go/<project>/ui/web/l8ui/notification/
```

### Step 2: Add to app.html

Add after l8ui shared scripts, before module scripts. Order matters — enums must load first.

```html
<!-- L8Notify shared components -->
<link rel="stylesheet" href="l8ui/notification/l8notify-notification.css">
<script src="l8ui/notification/l8notify-enums.js"></script>
<script src="l8ui/notification/l8notify-smtp-config.js"></script>
<script src="l8ui/notification/l8notify-webhook-mgmt.js"></script>
<script src="l8ui/notification/l8notify-delivery-log.js"></script>
<script src="l8ui/notification/l8notify-target-editor.js"></script>
```

For mobile, add the same includes to `m/app.html`.

### Step 3: Use in Consumer UI

#### SMTP Config in Admin Page

```javascript
// Render SMTP config form
L8NotifySmtpConfig.render(
    container,           // DOM element to render into
    currentSmtpConfig,   // current SmtpConfig object (or null for new)
    function(data) {     // onSave callback — receives collected form data
        // POST data to your SMTP config service endpoint
    }
);

// Send test email
L8NotifySmtpConfig.sendTest(
    smtpConfig,          // SmtpConfig object to test with
    "test@example.com",  // recipient for test email
    "/myproject/10/SmtpTest"  // consumer's test endpoint URL
);
```

#### Webhook Management in Admin Page

```javascript
L8NotifyWebhookMgmt.render(
    container,           // DOM element to render into
    webhookConfigs,      // array of WebhookConfig objects
    function(data) {     // onSave — called for add and edit
        // POST/PUT data to your webhook config service
    },
    function(webhook) {  // onDelete — called when user deletes
        // DELETE webhook from your service
    },
    function(webhook) {  // onTest — called when user clicks Test
        // Send test ping to webhook.url
    }
);
```

#### Delivery Log in Detail Popup or Admin Page

```javascript
L8NotifyDeliveryLog.render(
    container,           // DOM element to render into
    deliveryResults,     // array of DeliveryResult objects
    {
        showChannel: true,   // show Channel column (default: true)
        showTarget: true,    // show Endpoint column (default: true)
        pageSize: 25         // rows per page (default: 25)
    }
);
```

Row click opens a detail popup showing all fields (status, channel, endpoint, HTTP status, attempt #, error message, timestamp).

#### Target Editor in Policy/Rule Forms

Use in consumer form definitions to let users edit the `repeated NotifyTarget` array inline:

```javascript
const targetDef = L8NotifyTargetEditor.getInlineTableDef();
// Returns: { key: 'targets', label: 'Notification Targets', columns: [...] }

// In your rule/policy form definition:
MyModule.forms = {
    NotificationRule: f.form('Notification Rule', [
        f.section('Rule Details', [
            ...f.text('ruleId', 'Rule ID', true),
            ...f.text('name', 'Name', true),
            ...f.checkbox('enabled', 'Enabled'),
            // ... project-specific filter fields ...
        ]),
        f.section('Targets', [
            ...f.inlineTable(targetDef.key, targetDef.label, targetDef.columns)
        ]),
        f.section('Throttling', [
            ...f.number('cooldownSeconds', 'Cooldown (seconds)'),
            ...f.number('maxPerHour', 'Max Per Hour')
        ])
    ])
};
```

#### Shared Enums in Column/Form Definitions

```javascript
// In consumer column definitions
...col.enum('channel', 'Channel', null, L8NotifyEnums.render.channel)
...col.status('status', 'Status', null, L8NotifyEnums.render.deliveryStatus)

// In consumer form definitions (select dropdown)
...f.select('channel', 'Channel', L8NotifyEnums.NOTIFY_CHANNEL)
```

---

## Testing

All four Go packages have unit tests. Run them with:

```bash
cd go && ./test.sh
```

The `test.sh` script fetches dependencies, runs all tests with `-v` and `-failfast`, collects coverage across all packages (`channel`, `template`, `throttle`, `escalation`), and opens an HTML coverage report.

To run tests without the interactive prompt or coverage browser:

```bash
cd go && go test ./... -v --failfast
```

### Test Coverage

| Package | Test File | Key Cases |
|---------|-----------|-----------|
| `channel` | `channel_test.go` | Dispatch routing per channel, custom sender registration, nil/error handling |
| `template` | `template_test.go` | Placeholder substitution, missing keys, empty/nil inputs, `RenderWithDefault` |
| `throttle` | `throttle_test.go` | Per-key cooldown, hourly rate limits, cross-key isolation, `Reset()` |
| `escalation` | `scheduler_test.go` | Empty steps, single/multi-step chains, `Cancel()`, `Active()` count |

---

## Dependencies

**Go**: `google.golang.org/protobuf` only. No l8orm, l8services, l8bus, l8web, l8reflect, or other Layer 8 infrastructure dependencies.

**l8ui components**: Require the l8ui shared library already present in the consumer project:
- `Layer8DTable` — table rendering
- `Layer8DPopup` — detail popups
- `Layer8DForms` / `Layer8FormFactory` — form generation and data collection
- `Layer8ColumnFactory` — column definitions
- `Layer8EnumFactory` — enum map creation
- `Layer8DRenderers` — `createStatusRenderer`, `renderEnum`
- `Layer8DNotification` — toast notifications (used by SMTP test)
- `--layer8d-*` CSS custom properties from `layer8d-theme.css`
