# How to use this file

This is a reusable prompt template for executing `PLAN-L8NOTIFY-SYSTEM-SERVICE.md` one phase at a time, each in its
own fresh session (per the plan's "Execution model" section).

To start a phase's session:
1. Open a new session in the working directory of the repo that phase targets (check the plan's Phase list —
   e.g. Phase 0 → `l8types`, Phase 0.25 → `l8utils`, Phase 3/4/5/6/7 → `l8notify`).
2. Copy everything **below the divider**, replace `{PHASE}` with the phase identifier you want executed (e.g. `0`,
   `0.25`, `0.5a`, `0.5b`, `3`, `4`, `5`, `6`, `7`, `8`), and send it as your first message.

Do not run Phases 1 or 2 this way — they're no-op markers (see the plan).

---

# Execute L8Notify Plan — Phase {PHASE}

You are executing exactly one phase of a pre-approved, multi-repo implementation plan. This plan spans five repos
and is being implemented across separate sessions, one phase per session, with no shared memory between them — this
document plus the plan file are your only context. Follow these steps in order.

## 1. Read the plan first, completely

Read the full plan at:
`/home/saichler/proj/src/github.com/saichler/l8notify/plans/PLAN-L8NOTIFY-SYSTEM-SERVICE.md`

Read it start to finish, not just the "Phase {PHASE}" section — the Purpose, Reference Pattern, and Design
Decisions sections explain *why* the phase is shaped the way it is, several phases reference proto messages or
interfaces defined in other phases, and the Rule Compliance Notes / Open Items sections contain constraints that
apply across phases.

## 2. Confirm Phase {PHASE} is actually executable

If it's marked "RETIRED, SKIP" or "SUPERSEDED, SKIP" (Phases 1 and 2 as of this writing), stop immediately and
report that no work is needed for this phase — do not invent substitute work.

## 3. Verify prerequisites before writing any code

Every phase states a **Repo** and a **Prerequisites** line. Before touching any file:
- Confirm the prerequisite code is actually present and vendored where the phase expects it (e.g. grep the target
  repo's `go.mod` for the expected dependency version; confirm an imported package/type actually exists in
  `vendor/` or via the module cache; confirm a referenced file from an earlier phase actually exists on disk).
- If something the phase depends on is missing, **stop and report exactly what's missing**. Do not work around it,
  do not implement the missing prerequisite yourself even if it looks small, and do not guess at what a later
  phase will provide. Report back and wait for the user.

## 4. Work only in the phase's stated repo

Do not edit files in any other repo, with one exception: **updating this plan's own status marker (step 7 below)
is always allowed**, even though the plan file lives in `l8notify`. Everything else stays scoped to the one repo
this phase targets. If you notice something in a different phase or a different repo while working — a bug in an
earlier phase's already-implemented code, an inconsistency, anything — do not fix it inline. Note it in your final
report instead.

## 5. Implement exactly what the phase specifies

Treat the phase's code samples, file list, and design notes as the specification. Where the plan explicitly flags
something as "verify at implementation time" (e.g. `Security().Credential()`'s exact return-value order in
Phase 4.1/4.2), actually verify it against the real interface or an existing working call site before writing code
that depends on it — the plan's snippets are illustrative, not guaranteed byte-exact.

Apply the standing project rules as you normally would (protobuf conventions, maintainability, test location,
duplication, etc.) — this plan was written to comply with them. If something in the phase text conflicts with a
rule you're independently aware of, flag the conflict in your report rather than silently resolving it one way.

## 6. Hard constraints — do not do these under any circumstances

- **No `git` commands of any kind** — no `add`, `commit`, `push`, `tag`, `checkout`, nothing. The user reviews and
  pushes every phase manually, across every repo.
- **No `go mod tidy` / `go mod vendor` / `go mod init`**, or any other command that touches `go.mod`, `go.sum`, or
  a `vendor/` directory. The user vendors dependencies between phases manually.
- **No edits outside the one repo this phase targets**, except the plan-status update in step 7.

## 7. Mark this phase's status in the plan

Once implementation is complete (or once you've stopped due to a missing prerequisite), edit
`PLAN-L8NOTIFY-SYSTEM-SERVICE.md` and change this phase's heading:
- Fully implemented: `## Phase {PHASE} — ...` → `## Phase {PHASE} [DONE] — ...`
- Stopped partway (missing prerequisite, open question, etc.): `## Phase {PHASE} [BLOCKED: <short reason>] — ...`

This status marker is the only persistent memory across sessions — whoever executes the next phase needs to be
able to tell what's already landed by reading this file alone.

## 8. Report back

End your session with:
- Which files were created/modified/deleted, in this repo only
- Any deviations from the plan's snippets, and why
- Any open questions, inconsistencies, or issues noticed in other phases/repos (not fixed, just reported)
- What the user needs to do next: review the diff, run `go build ./...` / `go vet ./...` (or `node -c` for JS)
  locally if not already done, push, and vendor this repo's new version into whichever repo(s) the *next* phase's
  Prerequisites will require

---

**Phase to execute: {PHASE}**
