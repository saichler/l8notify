# Plan: Move `l8ui/notification/` from l8notify into the canonical `../l8ui` library (with mobile parity)

**Status: Phases 1–4 [DONE]. Phase 5 (manual visual verification) remains deferred to the user, in a consuming
project, per its own text below.**

## Context

`l8notify` currently ships 5 reusable UI files at `l8notify/l8ui/notification/` (channel/status/integration-type
enums, integration-config CRUD, delivery-log viewer, target editor, CSS). Per `l8notify/README.md`'s "l8ui
Integration" section, these are designed to be **copy-distributed**: each consuming project runs
`cp -r l8notify/l8ui/notification/ <project>/.../web/l8ui/notification/` and wires the scripts into its own
`app.html`.

This plan follows the exact precedent already executed in `../l8events` — see
`../l8events/plans/PLAN-MOVE-L8UI-EVENTS-TO-L8UI.md` — which moved `l8events/l8ui/events/` into `l8ui/events/`
and is now documented at `../l8ui/rules/l8events-ui.md`. The same reasoning applies here: `l8notify` is not
required infrastructure the way `l8events` is (`events-service-required.md` has no `l8notify` equivalent), but it
*is* an activatable service any consumer wiring in notifications needs, and its UI belongs in the canonical `l8ui`
library for the same reason `l8ui/sys/*` and `l8ui/events/` do — no separate copy step, no drift, available for
free to any project that adds `l8ui` as a submodule (`l8ui-copy-to-new-project.md`).

## Verification Already Done

- **Naming collision found — target directory is `l8ui/notify/`, NOT `l8ui/notification/`.** `../l8ui/notification/`
  already exists and contains `layer8d-notification.css`/`layer8d-notification.js` — the shared **toast
  notification** component (`Layer8DNotification`, per `layer8d-api-reference.md`), a completely unrelated system
  to l8notify's admin UI. Reusing the source directory name (`notification/`) would silently merge two unrelated
  components into one directory. `l8ui/notify/` (mirroring `l8ui/events/`'s pattern: top-level category named
  after the source project, not the source directory) has no existing collision — verified: `ls ../l8ui/notify`
  returns nothing today.
- **Theme compliance**: `l8notify-notification.css` uses `var(--layer8d-*)` exclusively (23 references, 0
  hardcoded colors, no `[data-theme]` blocks) — passes `l8ui-theme-compliance.md` as-is.
- **Dead CSS found — must be cleaned during the move, not carried forward.** `l8notify-notification.css` still
  contains ~8 selectors (`.l8notify-smtp-config`, `.l8notify-smtp-actions`, `.l8notify-webhook-mgmt`,
  `.l8notify-webhook-toolbar`, `.l8notify-delivery-detail`, `.l8notify-detail-row`, `.l8notify-detail-row label`,
  `.l8notify-detail-row span`, `.l8notify-detail-error span`, `.l8notify-test-btn.*`) left over from before
  `l8notify`'s own Phase 5 UI rework retired the `render()`/`_showDetail()` methods and the two SMTP/webhook files
  that used them. None of the 4 remaining JS files reference any of these classes. Carrying 100% dead CSS into a
  permanent shared library is worse hygiene than leaving it in a per-project copy — this plan's Phase 1 removes it.
  (`.l8notify-status-*` and the `signatureHeader`-adjacent classes were checked too — also unreferenced, also dead,
  also removed.)
- **All 5 files are platform-agnostic — even more so than `l8events`'s were.** `grep`-checked every JS file for
  `Layer8DTable`, `Layer8DPopup`, `Layer8MTable`, `Layer8MPopup`, and `innerHTML` — **zero real hits** (one comment
  mentioning `Layer8DTable` as consumer-wiring guidance, not actual code). Every file is a pure
  `getColumns()`/`getFormDefinition()`/`getInlineTableDef()` data function — no DOM manipulation of any kind. This
  is a stronger position than `l8events`, which still had two files (`l8events-alarm-detail.js`,
  `l8events-state-actions.js`) doing `container.innerHTML` generation. `l8notify`'s Phase 5 UI rework (this
  project's own plan, `PLAN-L8NOTIFY-SYSTEM-SERVICE.md`) deliberately moved everything to the data-only pattern
  specifically so consumers wire it into `Layer8DTable`/`Layer8DForms` (or `Layer8MTable`/`Layer8MForms`)
  themselves — this move is the natural continuation of that decision, not a new one.
- **Mobile runtime dependency confirmed available**: the one real dependency, `Layer8DRenderers`
  (`createStatusRenderer`, `renderEnum`), is loaded on mobile pages per `mobile-script-loading-order.md` — already
  established precedent (`l8security-enums.js`, and now `l8events-enums.js` in its new home).
- **Consumer drift already exists**: `l8alarms` (`go/alm/ui/web/l8ui/notification/notification/`) carries an
  already-diverged, ad-hoc copy — note the doubled `notification/notification/` path (a pre-existing copy mistake)
  — that still includes the deleted `l8notify-smtp-config.js`/`l8notify-webhook-mgmt.js`, meaning it predates
  `l8notify`'s own Phase 5 rework and is stale even against the *current* copy-distribution model, before this
  move is even considered. Reconciling it stays **out of scope** for this plan (separate repo, separate task) —
  see "Follow-ups," same treatment `l8alarms`/`l8vendingmachine` got in the `l8events` version of this plan.
- **This closes a gap `l8notify`'s own plan explicitly deferred.** `PLAN-L8NOTIFY-SYSTEM-SERVICE.md` Phase 5.4's
  Platform Completeness Audit marked mobile support for all three UI components "**Deferred**" — reason given:
  "`l8notify` has never shipped `Layer8M*` equivalents... a mobile build-out is a separable follow-up." This plan
  *is* that follow-up, and (per the point above) achieves it with zero new files, the same way `l8events`'s did.

## Design Decision: How Mobile Parity Is Achieved (Without Forking)

Read all 5 source files in full (`l8notify-enums.js`, `l8notify-integration-mgmt.js`, `l8notify-delivery-log.js`,
`l8notify-target-editor.js`, `l8notify-notification.css`). As established above, none of them touch a desktop-only
widget class at all — they produce pure `Layer8ColumnFactory`/`Layer8FormFactory` output (platform-agnostic per
`shared-schemas.md`) or, in `l8notify-enums.js`'s case, pure enum/renderer maps.

| File | Mobile treatment |
|---|---|
| `l8notify-enums.js` | **Reused as-is.** `Layer8DRenderers` loads on mobile; same precedent as `l8security-enums.js` / the now-relocated `l8events-enums.js`. |
| `l8notify-integration-mgmt.js` | **Reused as-is** + add `primary`/`secondary` markers to `getColumns()`'s column defs. |
| `l8notify-delivery-log.js` | **Reused as-is** + add `primary`/`secondary` markers to `getColumns()`'s column defs (only to the columns always present — `sentAt`/`status`; `channel`/`endpoint` are conditionally included via the `options` param, see Phase 1). |
| `l8notify-target-editor.js` | **Reused as-is, no markers.** `getInlineTableDef()` feeds `f.inlineTable(...)` inside a *parent* form — it is never itself rendered as a top-level `Layer8MTable`/`Layer8MEditTable` card list, so the `primary`/`secondary` card-display convention doesn't apply here. Confirm this against actual inline-table mobile rendering behavior during implementation; if inline tables do render as cards on mobile, add markers then — flagged as a judgment call, not asserted as certain. |
| `l8notify-notification.css` | **Reused, with dead-selector cleanup** (see Verification section above) — not a behavior change, since nothing references the removed selectors today. |

**Net result: zero new files, zero forked logic** — the same outcome `l8events`'s move achieved, for the same
underlying reason (no file here was ever coupled to a desktop-only widget).

**Risk being carried forward, NOT resolved by this plan:** exactly as with `l8events`, `l8notify` has no web app of
its own to test in (no `app.html`, no `run-local.sh`, by design). Real, visual, in-browser verification that the
mobile card layout (once `primary`/`secondary` markers are added) actually looks right is explicitly **out of
scope** for this plan and deferred to the user, in a consuming project, once `l8ui/notify/` is wired in.

## What Moves

From `l8notify/l8ui/notification/` → `l8ui/notify/` (top-level category, parallel to `l8ui/events/`, `l8ui/sys/`,
`l8ui/chart/`, etc.):

```
l8notify-enums.js               (must load first — others depend on it)   — unchanged
l8notify-integration-mgmt.js                                              — + primary/secondary markers
l8notify-delivery-log.js                                                  — + primary/secondary markers
l8notify-target-editor.js                                                 — unchanged (see mobile table above)
l8notify-notification.css                                                 — dead-selector cleanup, otherwise unchanged
```

Global names (`window.L8NotifyEnums`, `window.L8NotifyIntegrationMgmt`, `window.L8NotifyDeliveryLog`,
`window.L8NotifyTargetEditor`) and script load order are preserved exactly. No mobile-specific load order is
needed — the same 5 files serve both `app.html` and `m/app.html` includes.

## Platform Audit

| Component | Desktop File | Desktop Status | Mobile Status |
|---|---|---|---|
| Channel/status/integration-type enums | `l8notify-enums.js` | Exists — relocating unchanged | Reused directly — `Layer8DRenderers` confirmed available on mobile pages |
| Integration config CRUD | `l8notify-integration-mgmt.js` | Exists — relocating + primary/secondary markers | Reused directly via the same file |
| Delivery log viewer | `l8notify-delivery-log.js` | Exists — relocating + primary/secondary markers | Reused directly via the same file |
| Target editor (inline table def) | `l8notify-target-editor.js` | Exists — relocating unchanged | Reused directly via the same file; card markers likely not applicable (inline-table context, not a top-level list) |
| Styling | `l8notify-notification.css` | Exists — relocating, dead selectors removed | Reused directly via the same file; visual mobile-viewport check deferred to the user (Phase 5) |

## Traceability Matrix

| # | Section | Gap / Action Item | Platform | Phase |
|---|---------|-------------------|----------|-------|
| 1 | Verification already done | Resolve `l8ui/notification/` naming collision — target `l8ui/notify/` instead | Both | Phase 1 |
| 2 | What Moves | Copy 5 files from `l8notify/l8ui/notification/` to `../l8ui/notify/` | Both | Phase 1 |
| 3 | What Moves | Diff-verify `l8notify-enums.js` and `l8notify-target-editor.js` are byte-for-byte identical to source | Both | Phase 1 |
| 4 | Design Decision | Add `primary`/`secondary` markers to `l8notify-integration-mgmt.js` | Mobile | Phase 1 |
| 5 | Design Decision | Add `primary`/`secondary` markers to `l8notify-delivery-log.js` | Mobile | Phase 1 |
| 6 | Verification already done | Remove dead CSS selectors from `l8notify-notification.css` | Both | Phase 1 |
| 7 | Verification already done | Document the component (desktop + mobile usage) in `l8ui`'s own README (`Shared & Utilities` list) | Both | Phase 2 |
| 8 | Verification already done | Add `../l8ui/rules/l8notify-ui.md` (prerequisites, load order, API surface) | Both | Phase 2 |
| 9 | Verification already done | Add files to `desktop-script-loading-order.md`/`mobile-script-loading-order.md`, conditional on those rule files existing in the l8ui repo (they don't today — same gap `l8events`'s move flagged and skipped) | Both | Phase 2 |
| 10 | Context | Delete `l8notify/l8ui/` (now redundant at the source) | Both | Phase 3 |
| 11 | Context | Rewrite `l8notify/README.md` §"l8ui Integration" to point at the `l8ui` submodule instead of copy-distribution, covering both surfaces | Both | Phase 3 |
| 12 | Context | Update `plans/PLAN-L8NOTIFY-SYSTEM-SERVICE.md` Phase 5.4's Platform Completeness Audit table — mobile is no longer "Deferred," it's closed by this plan | Both | Phase 3 |
| 13 | Design Decision (risk) | Confirm in an actual mobile viewport that the new `primary`/`secondary` card layouts look right, and that `l8notify-target-editor.js` doesn't need markers after all | Mobile | Phase 5 — deferred to the user, in a separate project; any fix comes back here first |
| 14 | Follow-ups | Reconcile `l8alarms`'s drifted, doubled-path local copy (`.../l8ui/notification/notification/`) onto the new canonical source | Both | Deferred — separate repo, separate task, not requested |

Every row above resolves to a phase in this plan or an explicitly-deferred follow-up; nothing is unaccounted for.

## Steps

### Phase 1 — Copy into `../l8ui`, add mobile card markers, remove dead CSS
1. Copy all 5 files from `l8notify/l8ui/notification/` to `../l8ui/notify/` (new directory).
2. Diff-verify `l8notify-enums.js` and `l8notify-target-editor.js` are byte-for-byte identical to the pre-move
   originals.
3. In `l8notify-integration-mgmt.js`'s `getColumns()`: add `primary: true` to `name`, `secondary: true` to `type`
   (confirm during implementation which pairing reads best as a card title/subtitle — these are the working
   defaults, not locked in, same caveat the `l8events` version of this plan carried).
4. In `l8notify-delivery-log.js`'s `getColumns()`: add `primary: true` to `status` or `endpoint` (endpoint is
   conditionally included via the `options.showTarget` param — decide during implementation whether the primary
   marker should live on the always-present `sentAt`/`status` columns instead, to avoid a card with no title when
   `showTarget: false` is passed) and `secondary: true` to whichever of the remaining always-present columns fits.
5. In `l8notify-notification.css`: remove the dead selectors listed in "Verification Already Done" above
   (`.l8notify-smtp-config`, `.l8notify-smtp-actions`, `.l8notify-webhook-mgmt`, `.l8notify-webhook-toolbar`,
   `.l8notify-delivery-detail`, `.l8notify-detail-row` and its `label`/`span` children, `.l8notify-detail-error
   span`, `.l8notify-test-btn.*`). Keep `.l8notify-status-*` only if a future column renderer references it —
   verify via grep across the 4 remaining JS files before deciding; remove if still unreferenced.

### Phase 2 — Document the component in `l8ui`
6. Add an entry to `../l8ui/README.md` under **"Shared & Utilities"**, alongside `l8events-ui.md`:
   `- [l8notify-ui.md](rules/l8notify-ui.md) — Notification delivery-log and integration-config UI components (desktop + mobile)`
7. Add `../l8ui/rules/l8notify-ui.md` documenting: prerequisites (`Layer8DRenderers`, `Layer8EnumFactory`,
   `Layer8ColumnFactory`, `Layer8FormFactory`), the mandatory script load order, the `window.L8NotifyEnums` /
   `L8NotifyIntegrationMgmt` / `L8NotifyDeliveryLog` / `L8NotifyTargetEditor` API surface, and an explicit note
   that these files are used unforked on both desktop and mobile (so a future editor doesn't "fix" this by
   splitting them) — model the structure directly on `../l8ui/rules/l8events-ui.md`.
8. Add `l8notify-notification.css` and the 4 JS files to `desktop-script-loading-order.md`'s and
   `mobile-script-loading-order.md`'s canonical lists — **only if** those rule files exist in the `l8ui` repo
   itself; they don't today (`../l8ui/rules/` currently only has `l8events-ui.md` and `layer8m-auth.md`), so skip
   and rely on step 7's dedicated doc instead, same as the `l8events` move did.

### Phase 3 — Remove from `l8notify`, update its docs
9. Delete `l8notify/l8ui/` (the whole directory — it only ever contained `notification/`).
10. Rewrite `l8notify/README.md` §"l8ui Integration" (Step 1: Copy Files / Step 2: Add to app.html / Step 3: Use in
    Consumer UI): replace the `cp -r l8notify/l8ui/notification/ ...` instruction and the `<script
    src="l8ui/notification/...">` include list with: "These components ship as part of the `l8ui` library at
    `l8ui/notify/`, used identically on both desktop and mobile. Add `l8ui` to your project via
    `setup-l8ui-submodule.sh` (see `l8ui-copy-to-new-project.md`) and they're available automatically." Keep the
    usage examples (`Layer8DTable`/`Layer8DForms` wiring) — still accurate, just update the `<script src>` paths
    from `l8ui/notification/*` to `l8ui/notify/*` and drop the "For mobile: l8notify has never shipped..." caveat
    now that this plan closes it.
11. Update `plans/PLAN-L8NOTIFY-SYSTEM-SERVICE.md` Phase 5.4's Platform Completeness Audit table: change all three
    "**Deferred**" mobile-status cells to point at this plan (e.g. "Closed — see
    `PLAN-MOVE-L8UI-NOTIFICATION-TO-L8UI.md`"), and update the "Reason for deferral" prose accordingly. Historical
    plan docs shouldn't silently go stale without a marker.

### Phase 4 — File-Level Verification (everything checkable without a running app)

- [ ] `l8notify/l8ui/` no longer exists in the l8notify repo
- [ ] `../l8ui/notify/` contains all 5 files
- [ ] `l8notify-enums.js`, `l8notify-target-editor.js` are byte-for-byte identical to the pre-move originals
- [ ] `l8notify-integration-mgmt.js`, `l8notify-delivery-log.js` carry `primary`/`secondary` markers and are
      otherwise unchanged
- [ ] `l8notify-notification.css` no longer contains any of the dead selectors listed above
- [ ] `../l8ui/README.md` lists the new component under "Shared & Utilities"
- [ ] `../l8ui/rules/l8notify-ui.md` exists and documents prerequisites, script load order, the API surface, and
      the "used unforked on both platforms" note
- [ ] `l8notify/README.md` §"l8ui Integration" no longer instructs consumers to `cp -r` anything and covers both
      desktop and mobile script includes, pointing at `l8ui/notify/`
- [ ] `plans/PLAN-L8NOTIFY-SYSTEM-SERVICE.md` Phase 5.4's audit table no longer says "Deferred" for mobile
- [ ] `grep -rn "l8ui/notification" l8notify/README.md` returns no hits implying self-hosting or copy-distribution
      (note: `l8ui/notification/` is a *different, real* component — `layer8d-notification.css`/`.js`, the toast
      system — so this grep must be read carefully; it's checking for l8notify's own old path, not flagging the
      unrelated toast component)
- [ ] Re-read `l8ui-no-project-specific-code.md` against the final `l8ui/notify/` content: confirm the files read
      as a **built-in module** (same class as `sys/security`, `events/`) rather than app-specific code — nothing
      hardcodes an endpoint prefix, project name, or service area outside the generic notify/integration domain

### Phase 5 — Manual Visual Verification (deferred to the user, in a separate project)

Not performed as part of this plan. `l8notify` has no runnable UI of its own to test in, so this checklist is
carried forward as an explicit TODO for whichever consuming project wires `l8ui/notify/` in and actually runs it —
covering **both** platforms, not just the mobile-specific card-layout risk this plan's Design Decision section
carries forward:

- [ ] **Desktop**: `L8NotifyIntegrationMgmt`/`L8NotifyDeliveryLog`/`L8NotifyTargetEditor` render identically to
      before the move — expected low risk (same files, same global names, same script load order, nothing about
      desktop rendering changed), but not yet confirmed in a browser. Click through: integration config table
      loads and its add/edit/delete forms work; delivery log table loads and its (now `openViewForm`-based) detail
      popup shows all fields read-only; a policy/rule form using `L8NotifyTargetEditor.getInlineTableDef()` still
      renders and edits the inline target rows correctly.
- [ ] **Mobile**: the new `primary`/`secondary` mobile card layouts on `L8NotifyIntegrationMgmt`/
      `L8NotifyDeliveryLog` look right in an actual mobile viewport, and confirm whether
      `l8notify-target-editor.js` genuinely doesn't need markers (per the open question in the Design Decision
      table) once its inline table is actually exercised on a mobile card/form.

**If that check finds a real problem**, the fix comes back here first — no unilateral in-flight decision to fork a
file or patch the CSS without checking in. This plan's core bet (reuse over fork) stays open until that manual
check happens; it is not being marked "done" by this plan.

## Follow-ups (explicitly out of scope here)

- `l8alarms` (`go/alm/ui/web/l8ui/notification/notification/`) currently carries an independent, already-diverged,
  desktop-only, pre-Phase-5 copy of these files (still has the deleted `l8notify-smtp-config.js`/
  `l8notify-webhook-mgmt.js`). Once `l8ui/notify/` is canonical, this project should be migrated to a real `l8ui`
  submodule per `l8ui-copy-to-new-project.md`, have its ad-hoc copy (and the doubled `notification/notification/`
  path) removed, and pick up mobile support and the current Phase-5 component shape for free — but that touches
  another repo and is a separate, explicitly-requested task.

## Git Handling

Two separate git repositories are touched (`l8notify` and `l8ui`). Per `vendor-and-git.md`, no `git
add`/`commit`/`push` will be run in either repo as part of this plan — file changes only. The user handles staging
and committing in each repo separately once the plan is approved.

## Rule Compliance Notes

- `mobile-rules.md` Rule 2 (Desktop/Mobile Functional Parity) — satisfied without forking, on an even stronger
  footing than `l8events`'s move: zero files here ever did DOM manipulation, so there's no "does this HTML work
  inside a mobile popup" risk to carry forward — only the `primary`/`secondary` card-layout visual check remains,
  deferred to Phase 5.
- `plan-requirements.md` Platform Completeness — satisfied via the Platform Audit table and the Platform column on
  the Traceability Matrix; Phase 4's checklist covers what's checkable without a running app, Phase 5 explicitly
  carries the remaining visual check as deferred rather than silently dropping it. This plan also directly closes
  the mobile deferral `PLAN-L8NOTIFY-SYSTEM-SERVICE.md` Phase 5.4 left open.
- `plan-requirements.md` Duplication Audit — not triggered: no file is forked, zero new duplicated code.
- `l8ui-no-project-specific-code.md` — satisfied per Phase 4's final check; reclassifies `l8notify` UI as a
  built-in domain module (same precedent as `l8ui/sys/*`, `l8ui/events/`), not per-app code.
- `l8ui-theme-compliance.md` — desktop CSS already satisfied, verified pre-move; dead selectors removed rather than
  carried forward into the shared library.
- `shared-schemas.md` — mobile column definitions add `primary`/`secondary` markers per the documented mobile
  column schema extension, in-place on the existing shared files.
- `l8ui-copy-to-new-project.md` — this move is what makes that rule's promise ("add the l8ui submodule, get
  everything") actually true for `l8notify` consumers going forward, on both platforms, with zero extra
  mobile-specific setup.
- `test-location-and-approach.md` — not applicable; no Go code changes in this plan.
- `desktop-script-loading-order.md` / `mobile-script-loading-order.md` — Phase 2 step 8 adds the new files to both
  canonical lists **only if** those rule files exist in the `l8ui` repo; verified they don't today (`../l8ui/rules/`
  currently has only `l8events-ui.md` and `layer8m-auth.md`), so this step is skipped and `l8notify-ui.md` (step 7)
  is the sole source of load-order documentation instead — same resolution the `l8events` move used for the
  identical gap, not a new decision.
