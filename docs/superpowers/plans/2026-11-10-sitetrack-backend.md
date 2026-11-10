# SiteTrack Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Use checkbox steps to track completion.

**Goal:** Deliver the complete SiteTrack take-home backend with documented APIs, reproducible data at the specified scale and a usable Docker Compose setup.

**Architecture:** Go hexagonal architecture with domain/application/ports isolated from HTTP, GORM and PostgreSQL. GORM implements ordinary CRUD; SQL handles complex queries and transactional invariants. Database-backed sessions enforce immediate revocation and current permissions.

**Tech stack:** Go 1.27.3 (module minimum 1.27.0), net/http, PostgreSQL, GORM, PostgreSQL driver, JWT/v5, bcrypt, versioned SQL migrations.

**Spec:** `docs/DESIGN.md` plus the authoritative full brief at the source link. Controller's local extracted copy: `/Users/vincent/Documents/Codex/2026-11-10/hey-folks-i-m-sarvesh-head/work/assignment/assignment.txt`.

## Global Constraints

- User requested GPT-6.1 Sol implementation agents, with the root orchestrating. Work on `feat/sitetrack-backend`; preserve `main` until review.
- Do not start Docker or its daemon. Native PostgreSQL is available for isolated tests. No deployment, application submission or email to the recruiter.
- Read the full brief before choosing contracts. Every listed screen/scale requirement is in scope. Every unspecified behavior must have a documented decision.
- Domain and ports must not import GORM, HTTP, SQL drivers or concrete adapters.
- Client input must never control SQL identifiers without a fixed allowlist; bind all values. Enforce bounded request sizes and list limits.
- Feature code requires meaningful tests of behavior; concurrency invariants require real PostgreSQL tests. Never replace PostgreSQL with SQLite to claim those properties.
- Maintain a meaningful commit history. Never put secrets, local test database files or generated large datasets in Git.

## Review Focus

1. Cross-project ID guessing/list aggregation leaks and stale JWT roles after membership removal.
2. Racing graph changes, task reopen/start, or last-admin removals violating otherwise-correct sequential checks.
3. Multi-line delivery failure partially changing stock, or racing record/reject/cancel crediting twice.
4. Repeated sessions, refresh rotation and logout failing to revoke outstanding access tokens.
5. Empty/large pages, tied sorts, fractional quantities, deleted references and generated full-scale data producing slow or invalid results.

## Task 1: Contracts, persistence, authentication and runtime

**Files:** `internal/domain/{errors,auth,pagination,quantity}.go`, `internal/ports/*.go`, `internal/application/auth*.go`, `internal/adapters/postgres/{db,auth}*.go`, `internal/adapters/http/{router,auth,middleware,response}*.go`, `internal/platform/*.go`, `migrations/*.sql`, `migrations/*.go`, `cmd/api/main.go`, `go.mod`, `go.sum`, `Dockerfile`, `compose.yaml`, `.dockerignore`, `.env.example`, `.github/workflows/ci.yml`.

**Interfaces:** Define actor/session/domain error/typed pagination contracts before parallel feature work. Adapter constructors own dependencies; HTTP handlers consume application services, and application services consume ports. Store exact public signatures in a small implementation contract file under the local plan workspace and give it to both feature workers before they start.

- [ ] Write tests for register/login/refresh/logout, invalid token algorithms/expiry, current-role loading and generic bad credentials; observe failures before implementing.
- [ ] Implement configuration, explicit connection pool limits, versioned migrations, bounded health/readiness, graceful shutdown, app wiring and sessions. Bootstrap has no hidden manual prerequisite for Compose.
- [ ] Verify Go unit/race/build/vet checks and native PostgreSQL migration/session tests. Report exact commands/results, including skips.
- [ ] Commit the reviewed foundation and contracts.

## Task 2: Projects, access, invitations and notifications

**Files:** domain/ports/application/PostgreSQL/HTTP files named `project*`, `membership*` and `invitation*`, and their tests. Update migrations only through the lead to avoid concurrent edits.

**Interfaces:** Consume the Task 1 actor/errors/pagination contracts. Expose services for project CRUD, authorized project summaries, memberships, invitations, invitation responses and notification unread state. Access predicates must be reusable by task/inventory adapters without leaking concrete persistence into application code.

- [ ] Write tests for non-member 404s on every child type, scoped counts/search, accepting/withdrawing invitations, live membership changes, concurrent last-admin protection and monotonically allocated project IDs.
- [ ] Implement CRUD and all project-list fields/search/sorts/late-only filter using bounded queries. Enforce project-admin-only membership operations and recipient-only invitation responses.
- [ ] Test permission checks with User, company store manager, SuperAdmin, project admin and ordinary project member actors. Test pagination under ties and empty results.
- [ ] Review behavior against the source brief, then commit.

## Task 3: Task graph, actions and screen queries

**Files:** feature-specific `task*` and `dependency*` files across domain, ports, application, PostgreSQL and HTTP; tests under the same ownership.

**Interfaces:** Consume Task 1 shared contracts and Task 2 project authorization. Expose task CRUD, direct dependency CRUD, start/done/reopen actions, list filters/sorts, and details with transitive distances and allowed actions.

- [ ] Write tests for same-project edges, self/cyclic edges, diamond/chained prerequisites, illegal transitions, late/blocked coexistence, shortest distances, concurrent cycle creation and reopen/start serialization.
- [ ] Implement monotonically allocated project-local task numbers without reuse, graph/state transaction boundaries and derived flags.
- [ ] Implement every task-screen filter/sort and detail field using bounded batch queries; explicitly test status/date/late/blocked combinations.
- [ ] Run native PostgreSQL feature tests and review graph/authorization invariants; commit.

## Task 4: Materials, inventory and delivery lifecycle

**Files:** feature-specific `material*`, `stock*`, `store*`, `delivery*` files across domain, ports, application, PostgreSQL and HTTP; corresponding tests.

**Interfaces:** Consume shared actor/errors/exact-quantity/pagination contracts and project authorization. Expose catalogue CRUD, company stock receipt/reorder, stock list queries, immutable delivery initiation and record/reject/cancel actions.

- [ ] Write tests for fractional parsing, multi-line insufficient-stock rollback, concurrent outgoing sends, sender/receiver permissions, concurrent terminal transitions, stock conservation and immutable delivered history.
- [ ] Implement transaction-bound stock debits/credits, deterministic locking, legal state transitions and store direction constraints. Never leak global company deliveries to ordinary project users.
- [ ] Implement all company/project stock and delivery filters/sorts/fields, including outbound in-transit totals and last-delivery timestamps.
- [ ] Verify against native PostgreSQL and review transaction/authorization behavior; commit.

## Task 5: Scale data, API documentation and integration

**Files:** `cmd/seed/main.go`, supporting seed package/SQL, `docs/API.md` or OpenAPI schema, `docs/DECISIONS.md`, `README.md`, integration tests/scripts and CI.

**Interfaces:** Use the actual schema/services from Tasks 1–4; never introduce a separate toy schema or bypass invariants in the resulting data. Documentation must exactly match implemented routes and errors.

- [ ] Implement deterministic small/full presets, full target counts, valid acyclic graphs, nonnegative consistent stocks and auditable delivery lines. Require explicit opt-in for destructive reset of a nonempty database.
- [ ] Test small seed, repeat/rerun behavior, known user login, every screen query and representative query plans; provide full-scale command and honest measured results.
- [ ] Document endpoints, auth/refresh/logout, all query options, role matrix, decisions/trade-offs, test and seed commands, demo credentials and remaining limits.
- [ ] Verify `go test -race ./...`, `go vet ./...`, builds, native PostgreSQL integration suite, clean diff and `docker compose config`. Use CI to validate Compose execution without starting local Docker.
- [ ] Have the root run independent final review, fix concrete findings, push the feature branch and open a reviewable PR. Do not submit the job application.
