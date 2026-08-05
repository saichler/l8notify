# l8notify

Notification library and system service for Layer 8 applications. Provides reusable Go packages (dispatch, template
rendering, throttling, escalation scheduling) **and** an activatable backend service — `Notify` (delivery log,
dispatches on `POST`) and `IntegCfg` (editable integration endpoint configuration) — plus l8ui components for the
admin UI, following the same shape as `l8events`.

l8notify is **not** a standalone binary — it has no `main.go`, no Dockerfile, no k8s manifests. Consumer projects
import its Go packages, activate its two services from their own backend `main.go`, and copy its l8ui components
into their own web directories.

---

## Directory Structure

```
l8notify/
├── go/
│   ├── go.mod                      # Module: github.com/saichler/l8notify/go
│   ├── test.sh                     # Runs all tests with coverage report
│   ├── channel/
│   │   ├── channel.go              # Dispatch router + Sender interface + custom sender registry
│   │   ├── email.go                # SMTP email sender (plain + TLS)
│   │   ├── webhook.go              # Webhook sender (HMAC-SHA256 signing via l8common + retry with backoff)
│   │   ├── slack.go                # Slack incoming webhook sender
│   │   └── channel_test.go         # Tests for dispatch routing and custom senders
│   ├── template/
│   │   ├── template.go             # Generic {{key}} placeholder renderer
│   │   └── template_test.go        # Tests for template rendering edge cases
│   ├── throttle/
│   │   ├── throttle.go             # Per-key cooldown + hourly rate limiter
│   │   └── throttle_test.go        # Tests for cooldown and hourly limits
│   ├── escalation/
│   │   ├── scheduler.go            # Time-based escalation chain scheduler
│   │   └── scheduler_test.go       # Tests for escalation scheduling and cancellation
│   └── services/
│       ├── IntegrationConfigService.go  # ActivateIntegrationConfig() — editable CRUD for IntegrationConfig
│       └── NotifyRecordService.go       # ActivateNotify() — persists NotifyRecord, dispatches on POST
├── l8ui/notification/
│   ├── l8notify-enums.js               # NotifyChannel + DeliveryStatus + IntegrationType enums with renderers
│   ├── l8notify-integration-mgmt.js    # IntegrationConfig CRUD component (columns + form, data-only)
│   ├── l8notify-delivery-log.js        # Delivery log viewer (columns + form, data-only, read-only)
│   ├── l8notify-target-editor.js       # Inline table definition for NotifyTarget arrays
│   └── l8notify-notification.css       # Shared styles using --layer8d-* theme tokens
└── plans/                          # Implementation plans
```

Shared proto types (`NotifyChannel`, `DeliveryStatus`, `IntegrationType`, `NotifyTarget`, `SmtpConfig`,
`WebhookConfig`, `DeliveryResult`, `EscalationStep`, `NotifyRecord`, `IntegrationConfig`) live in
**`l8types/go/types/l8notifysvc`**, not in this repo — l8notify has no `proto/` directory of its own. This avoided
a naming collision with `l8types/go/types/l8notify`, an unrelated, pre-existing framework package (the internal
service change-notification mechanism).

---

## Protobuf Types (`l8types/go/types/l8notifysvc`)

| Type | Kind | Purpose |
|------|------|---------|
| `NotifyChannel` | Enum | EMAIL, WEBHOOK, SLACK, PAGERDUTY, CUSTOM |
| `DeliveryStatus` | Enum | PENDING, SENT, FAILED, RETRYING |
| `IntegrationType` | Enum | SMTP, WEBHOOK, SLACK, PAGERDUTY, CUSTOM |
| `NotifyTarget` | Embedded/child | Delivery target (channel + endpoint + template) |
| `SmtpConfig` | Embedded/child | SMTP connection settings (resolved at dispatch time, never persisted with secrets) |
| `WebhookConfig` | Embedded/child | Webhook endpoint with HMAC secret, retry count, timeout |
| `DeliveryResult` | Embedded/child | Result of a delivery attempt (status, HTTP code, error, attempt #) |
| `EscalationStep` | Embedded/child | Single step in an escalation chain (delay, channel, endpoint, template) |
| **`NotifyRecord`** | **Prime Object** | Immutable delivery-log row. `POST` triggers real dispatch, then persists the outcome. `PUT` is rejected. |
| **`IntegrationConfig`** | **Prime Object** | A configured integration endpoint (SMTP server, webhook, Slack, ...). Fully editable — created/updated/deleted by an admin. Non-secret fields only; `credential_key` is a lookup key into the consumer's own security config JSON `credentials` map, never the secret itself. |

`NotifyRecord` and `IntegrationConfig` are both served by `l8notify`'s own two services (below) — they are **not**
embedded into consumer protos the way the child types are. Everything else in the table stays a plain
building-block type consumers can embed in their own policy/rule protos, exactly as before.

---

## Go Integration

### Step 1: Add the Dependency

```bash
cd go
GOPROXY=direct GOPRIVATE=github.com go get github.com/saichler/l8notify/go@latest
go mod vendor
```

### Step 2: Activate the Services

**Both calls below must run in exactly one process within a given consumer project** — same constraint
`evtservices.ActivateEvents` already relies on (`single-owner-database-table.md`). Activating either service on
more than one node causes divergent in-memory caches and silently stale data.

```go
import notifyservices "github.com/saichler/l8notify/go/services"

// In your backend main.go, after services.ActivateAllServices(...):
notifyservices.ActivateNotify(dbcred, dbname, nic)
notifyservices.ActivateIntegrationConfig(dbcred, dbname, nic)
```

`Notify` (ServiceArea 78) persists `NotifyRecord` and dispatches on `POST`. `IntegCfg` (same ServiceArea 78) owns
CRUD for `IntegrationConfig`. Both go through `l8common.ActivateService`, so both automatically expose
`POST`/`PUT`/`PATCH`/`DELETE`/`GET` web endpoints at `/<prefix>/78/Notify` and `/<prefix>/78/IntegCfg` — the
respective `*ServiceCallback`s reject the operations that don't apply (`NotifyRecord` rejects `PUT`;
`IntegrationConfig` accepts everything).

### Step 3: Register Types in Your UI `main.go`

```go
import (
    l8c "github.com/saichler/l8common/go/common"
    "github.com/saichler/l8types/go/types/l8notifysvc"
)

l8c.RegisterType(resources, &l8notifysvc.NotifyRecord{}, &l8notifysvc.NotifyRecordList{}, "NotifyId")
l8c.RegisterType(resources, &l8notifysvc.IntegrationConfig{}, &l8notifysvc.IntegrationConfigList{}, "IntegrationId")
```

### Step 4: Set Up Credentials (deploy-time, not code)

The consumer's own security config JSON `credentials` map gets one entry per integration's secret. The non-secret
routing data (host, port, URL, retry count, etc.) is entered through the admin UI (`L8NotifyIntegrationMgmt`, see
below) as regular `IntegrationConfig` rows — never as deploy-time config.

```json
"credentials": {
  "smtp": { "zside": "smtp-username", "yside": "smtp-password" },
  "ops-alerts": { "zside": "hmac-secret-value" }
}
```

`Security().Credential(name, type, resources)` returns `(aside, zside, yside, name, err)`.
`NotifyRecordService.go`'s `resolveSmtpConfig`/`resolveWebhookSecrets` read `zside` as the username (or the whole
secret, for a single-value webhook credential) and `yside` as the password — the same destructuring pattern
`l8common.ActivateService` itself uses for DB credentials, verified directly against `OpenDBConection`. `aside` and
`name` (4th return) are unused for these two credential types.

### Step 5: Dispatch From Any Service

Any service in the ecosystem — not just consumers of `l8notify`'s own admin UI — can send a notification through
`IResources`, mirroring `Events().PostSystemEvent(...)`:

```go
result := vnic.Resources().Notify().Send(
    l8notifysvc.NotifyChannel_NOTIFY_CHANNEL_EMAIL,
    "user@example.com", "Order Confirmed", "Your order SO-001 has shipped.",
    nil, // attributes, forwarded onto the persisted NotifyRecord
)
```

This POSTs a `NotifyRecord` to the `Notify` service over the vnic and returns the resolved `*DeliveryResult`
synchronously (unlike `Events`, which is fire-and-forget — `Send` needs the dispatch outcome back).

### Alternative: Direct Package Use (no service, no persistence)

The underlying `channel`/`template`/`throttle`/`escalation` packages are still plain Go libraries — usable directly
without activating either service, e.g. for a policy-evaluation flow where the consumer manages its own persistence:

```go
import (
    "github.com/saichler/l8notify/go/channel"
    "github.com/saichler/l8notify/go/template"
    "github.com/saichler/l8notify/go/throttle"
    "github.com/saichler/l8notify/go/escalation"
    ntf "github.com/saichler/l8types/go/types/l8notifysvc"
)
```

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
        vars := map[string]string{
            "entityId": entity.Id, "name": entity.Name,
            "status": entity.Status.String(), "actionType": actionLabel(action),
        }

        for _, target := range rule.Targets {
            throttleKey := entity.Id + "+" + rule.RuleId
            groupKey := rule.RuleId
            if cb.throttler.IsThrottled(throttleKey, groupKey, rule.CooldownSeconds, rule.MaxPerHour) {
                continue
            }

            msg := template.Render(target.Template, vars)
            result := channel.Dispatch(target, msg, cb.smtpCfg, cb.webhookSecrets)

            if result.Status == ntf.DeliveryStatus_DELIVERY_STATUS_SENT {
                cb.throttler.Record(throttleKey, groupKey)
            }
            cb.logDeliveryResult(rule.RuleId, target, result)
        }
    }
}
```

Prefer `vnic.Resources().Notify().Send(...)` (Step 5) for anything that should show up in the shared delivery log —
this direct-package path is for consumers that need their own bespoke persistence/policy model instead.

---

## Go Package API Reference

### services — Activatable Backend Services

```go
import "github.com/saichler/l8notify/go/services"
```

| Function | Service | ServiceArea | PrimaryKey | Notes |
|----------|---------|-------------|-----------|-------|
| `ActivateNotify(creds, dbname, vnic)` | `Notify` | 78 | `NotifyId` | Dispatches on `POST` (`Before` hook), immutable (`PUT` rejected) |
| `ActivateIntegrationConfig(creds, dbname, vnic)` | `IntegCfg` | 78 | `IntegrationId` | Fully editable CRUD |

L8Query `from` clauses use the **protobuf type name** (`NotifyRecord`, `IntegrationConfig`), not the ServiceName
(`Notify`, `IntegCfg`) — e.g. `select * from NotifyRecord where status=2`.

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

Posts JSON to a webhook endpoint with HMAC-SHA256 signing (via `l8common.ComputeHMACSHA256`) and retry with
exponential backoff.

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

Extend dispatch with custom channels. Consumer registers at startup; Dispatch tries all registered senders for
`NOTIFY_CHANNEL_CUSTOM`.

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

Unchanged from before this transformation — stays a plain in-memory library, exactly like `l8events` leaves
`state`/`archive`/`maintenance` as plain libraries. Out of scope for the system-service migration.

#### StepHandler

Callback type invoked when an escalation step fires. The consumer implements this to perform the actual delivery
(look up SMTP config, call `channel.Dispatch`, log the result, etc.).

```go
type StepHandler func(entityID string, step *ntf.EscalationStep, message string) error
```

#### New

Creates a new Scheduler with the given step handler.

```go
func New(handler StepHandler) *Scheduler
```

#### Schedule

Starts an escalation chain for the given entity. Steps are sorted by `StepOrder` automatically. Each step fires
after its `DelayMinutes` via a goroutine timer. The step's `MessageTemplate` is rendered using
`template.Render(tmpl, vars)` before calling the handler.

If an escalation is already active for this entity, it is cancelled and replaced.

```go
func (s *Scheduler) Schedule(
    entityID string,                  // entity being escalated
    steps []*ntf.EscalationStep,     // escalation steps (sorted by StepOrder)
    vars map[string]string,           // template variables for message rendering
)
```

#### Cancel

Stops all pending escalation timers for the entity. Call this when the entity is acknowledged, resolved, or
deleted.

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

| Responsibility | l8notify | Consumer |
|----------------|----------|----------|
| `NotifyRecord`/`IntegrationConfig` persistence | **Provides** (`Notify`/`IntegCfg` services) | Activates both once, in one process |
| SMTP/webhook secret storage | Never stores secrets | Own security config JSON `credentials` map |
| Delivery dispatch + logging | **Provides** (`Notify.Before(POST)` + `channel.Dispatch`) | -- |
| Admin UI for integration config | Provides `L8NotifyIntegrationMgmt` | Wires into `Layer8DTable`/`Layer8DForms` CRUD |
| Delivery log viewer | Provides `L8NotifyDeliveryLog` | Wires into `Layer8DTable` + `Layer8DForms.openViewForm` |
| Policy/rule types with filter criteria | -- | Defines and persists, embeds `NotifyTarget`/`EscalationStep` |
| Template rendering | `template.Render()` | Builds vars map from entity fields |
| Throttling | `throttle.IsThrottled/Record()` | Holds `*Throttler` instance, defines keys (opt-in, not wired into the `Notify` service) |
| Escalation scheduling | `escalation.Schedule/Cancel()` | Holds `*Scheduler` instance, provides `StepHandler` |
| Event emission (when to notify) | -- | ServiceCallback `After()` hooks, or direct `Notify().Send(...)` calls |
| Custom channels | `RegisterCustomSender()` | Implements `Sender` interface |

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
<script src="l8ui/notification/l8notify-integration-mgmt.js"></script>
<script src="l8ui/notification/l8notify-delivery-log.js"></script>
<script src="l8ui/notification/l8notify-target-editor.js"></script>
```

For mobile: `l8notify` has never shipped `Layer8M*` equivalents of any of its components — no consumer currently
exposes these on mobile. A mobile build-out is a separable follow-up using the same `Layer8MTable`/`Layer8MForms`
data-only pattern.

### Step 3: Use in Consumer UI

All three components are **data-only** — `getColumns()`/`getFormDefinition()` return plain definitions; the
consumer wires them into the standard `Layer8DTable`/`Layer8DForms` flow. None of them render DOM directly.

#### Integration Config CRUD (Admin Page)

```javascript
const table = new Layer8DTable({
    containerId: 'integration-config-table',
    endpoint: '/<prefix>/78/IntegCfg',
    modelName: 'IntegrationConfig',
    primaryKey: 'integrationId',
    columns: L8NotifyIntegrationMgmt.getColumns(),
    onAdd: () => Layer8DForms.openAddForm(
        { endpoint: '/<prefix>/78/IntegCfg', primaryKey: 'integrationId', modelName: 'IntegrationConfig' },
        L8NotifyIntegrationMgmt.getFormDefinition()
    ),
    onEdit: (id) => Layer8DForms.openEditForm(
        { endpoint: '/<prefix>/78/IntegCfg', primaryKey: 'integrationId', modelName: 'IntegrationConfig' },
        L8NotifyIntegrationMgmt.getFormDefinition(), id
    ),
    onDelete: (id) => Layer8DForms.confirmDelete(
        { endpoint: '/<prefix>/78/IntegCfg', primaryKey: 'integrationId', modelName: 'IntegrationConfig' }, id
    )
});
table.init();
```

#### Delivery Log (Read-Only Admin Page)

`NotifyRecord` rejects `PUT` server-side — the detail popup MUST use `openViewForm`, never `openEditForm`
(`immutability-ui-alignment.md`).

```javascript
const table = new Layer8DTable({
    containerId: 'delivery-log-table',
    endpoint: '/<prefix>/78/Notify',
    modelName: 'NotifyRecord',
    primaryKey: 'notifyId',
    columns: L8NotifyDeliveryLog.getColumns({ showChannel: true, showTarget: true }),
    onRowClick: (item) => Layer8DForms.openViewForm(
        { endpoint: '/<prefix>/78/Notify', primaryKey: 'notifyId', modelName: 'NotifyRecord' },
        L8NotifyDeliveryLog.getFormDefinition(), item
    ),
    onAdd: null, onEdit: null, onDelete: null // read-only — no CRUD controls
});
table.init();
```

#### Target Editor in Policy/Rule Forms

Use in consumer form definitions to let users edit the `repeated NotifyTarget` array inline — unchanged, still an
embedded/child type editor, not backed by its own service:

```javascript
const targetDef = L8NotifyTargetEditor.getInlineTableDef();
// Returns: { key: 'targets', label: 'Notification Targets', columns: [...] }

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
...col.enum('type', 'Type', null, L8NotifyEnums.render.integrationType)

// In consumer form definitions (select dropdown) — note .enum, NOT the whole factory-return wrapper
...f.select('channel', 'Channel', L8NotifyEnums.NOTIFY_CHANNEL.enum)
```

---

## Testing

All four plain-library Go packages have unit tests. Run them with:

```bash
cd go && ./test.sh
```

The `test.sh` script fetches dependencies, runs all tests with `-v` and `-failfast`, collects coverage across all
packages (`channel`, `template`, `throttle`, `escalation`), and opens an HTML coverage report.

To run tests without the interactive prompt or coverage browser:

```bash
cd go && go test ./... -v --failfast
```

### Test Coverage

| Package | Test File | Key Cases |
|---------|-----------|-----------|
| `channel` | `channel_test.go` | Dispatch routing per channel, custom sender registration, nil/error handling, HMAC signature presence |
| `template` | `template_test.go` | Placeholder substitution, missing keys, empty/nil inputs, `RenderWithDefault` |
| `throttle` | `throttle_test.go` | Per-key cooldown, hourly rate limits, cross-key isolation, `Reset()` |
| `escalation` | `scheduler_test.go` | Empty steps, single/multi-step chains, `Cancel()`, `Active()` count |

### Testing the `Notify`/`IntegCfg` Services

`services` (`Notify`/`IntegCfg`) has no tests in this repo — per `test-location-and-approach.md`, tests for
activatable services belong in the *consumer's* `go/tests/`, exercised through the system's HTTP API, not here.
`l8notify` itself has no `go/tests/` directory — same exemption `l8events` takes.

Recipe for the consumer's own integration test:

1. Test file lives in the consumer's `go/tests/integration/notify_test.go`, not inside `l8notify`.
2. Seed the consumer's `credentials` map with a `smtp` entry pointed at a local SMTP catcher (e.g. `smtp4dev`) —
   see the credentials JSON shape in [Step 4 above](#step-4-set-up-credentials-deploy-time-not-code).
3. Stand up the consumer's `IVNic`, call `ActivateNotify`/`ActivateIntegrationConfig`.
4. `POST /<prefix>/78/IntegCfg` an SMTP `IntegrationConfig` row (`type: SMTP`, `credentialKey: "smtp"`, host/port
   pointed at the catcher).
5. `POST /<prefix>/78/Notify` with `channel: EMAIL`, assert `status: DELIVERY_STATUS_SENT`; `GET
   /<prefix>/78/Notify?body=...` (L8Query `select * from NotifyRecord`) returns the delivery log; `PUT
   /<prefix>/78/Notify` is rejected.

Assert on HTTP responses only — never by calling `NotifyCallback.Before()` or `IntegrationConfigCallback.Before()`
directly (`test-location-and-approach.md`: tests exercise the system the same way a real client would, not
unexported internals).

---

## Dependencies

**Go**: `google.golang.org/protobuf`, `github.com/saichler/l8types/go` (for `l8notifysvc` types and `INotify`/
`IIntegration` interfaces), `github.com/saichler/l8common/go` (for `ActivateService`, `GenerateID`,
`ComputeHMACSHA256`, `RegisterType`). No `l8orm`, `l8services`, `l8bus`, or `l8web` direct dependencies — those come
in transitively through `l8common`.

**l8ui components**: Require the l8ui shared library already present in the consumer project:
- `Layer8DTable` — table rendering
- `Layer8DForms` / `Layer8FormFactory` — form generation, `openAddForm`/`openEditForm`/`openViewForm`/`confirmDelete`
- `Layer8ColumnFactory` — column definitions
- `Layer8EnumFactory` — enum map creation
- `Layer8DRenderers` — `createStatusRenderer`, `renderEnum`
- `--layer8d-*` CSS custom properties from `layer8d-theme.css`
