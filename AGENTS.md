# Agent / Contributor Instructions

These rules apply to anyone (human or AI agent) working in this repo.

1. **Read first:** `docs/README.md`, then the doc relevant to your task. `docs/decisions.md` is binding.
2. **Scope guard:** Do not add features listed as *Out of Scope* in `docs/01-requirements.md` (integrations, external DB connectors, SSO, enterprise audit, data sync, self-registration, public sharing, API tokens).
3. **Decisions:** Any change to a recorded decision, or any new non-trivial technical choice, must be added to `docs/decisions.md` (new `D-xxx` entry; never rewrite history — mark old entry *Superseded by D-yyy*).
4. **Progress:** When a task from `docs/04-implementation-plan.md` is started/finished, update `docs/progress.md` (status + date + short note).
5. **Acceptance:** A task is *Done* only when its linked acceptance tests (`AT-xxx` in `docs/acceptance-tests.md`) pass.
6. **Conventions:** Follow `docs/conventions.md` (layout, naming, API errors, i18n keys, commits).
7. **i18n:** Never hard-code user-visible strings in the frontend; add keys to both `en` and `zh-CN`.
8. **Resources:** Target host is 2 vCPU / 4 GB RAM. Avoid heavy dependencies; justify any new dependency in the PR/commit message.

## AI agent efficiency (cost control)

Every model call re-sends the whole conversation. A long session costs more on every step, even on low-cost models, so cut the number of calls and keep context small.

- **Batch:** Put independent reads, searches and status checks in one response. Do not run one tool per turn when several are independent.
- **Read once:** Read each file once, in one large range. Do not re-read it in small slices. After an edit succeeds, trust it; re-read only if the file changed outside your edits.
- **Large output:** When a tool saves output to a temp file, search it for failures (`failed`, `Error`, `✘`, `expected`). Do not page through it. Trim command output, e.g. `| Select-Object -Last 80`.
- **Long commands:** `go run .\tools\ptdev e2e` rebuilds the production image and takes 6–25 min; `ptdev check` takes about 3 min. Run them with a long wait (`initial_wait` 600), or start them in the background and wait for the completion notice. Never poll at short intervals: each poll is a full-price model call.
- **Narrow test loop:** Reproduce with the smallest target (`ptdev e2e -- e2e/<spec>.ts`, `go test -run <Name> ./internal/<pkg>`, `pnpm --dir web exec vitest run <file>`). Run the full `ptdev e2e` at most once per fix batch, as the final gate.
- **Retry budget:** If the same failure survives two targeted fix attempts, stop. Write down the evidence and change approach, or ask the user, instead of rerunning.
- **Environment preflight (before long runs):**
  - `podman machine list` shows the VM running.
  - Set the build mirrors when upstream hosts are slow (they were from this network; see D-156 and the T1.23 record): `$env:APT_MIRROR`, `$env:PGDG_MIRROR`, `$env:NPM_REGISTRY`, `$env:GOPROXY`. A build stuck in `apt-get update` or `go mod download` is a network stall, not a test hang.
  - Write long gate output to a log file (`*> $log`) and search it; piping through `Select-Object -Last N` hides all progress until the command exits.
  - `podman ps -a --external` shows no leftover buildah containers from killed builds.
  - Fix environment problems first; do not rerun to diagnose them.
- **Phase large tasks:** Split work into implement → stabilize gates → docs/commit. Write a short handoff in `docs/task-prompts/<task>.md` at each phase end. When context grows large (about 200K tokens) or a phase finishes, recommend a fresh session instead of continuing.
- **Report, don't loop:** In autonomous mode, give a long-running task a stop condition (gate green, or retry budget exhausted). Never let it loop on a flaky gate.
