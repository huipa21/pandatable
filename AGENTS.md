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
