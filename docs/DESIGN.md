# SiteTrack backend design

## Source and scope

Implement the Backend Engineer Take-Home Assignment dated 6 November 2026. The authoritative brief is [SiteTrack-Backend-Assignment.pdf](https://seronlabs-my.sharepoint.com/:b:/p/sarvesh/IQAwwLqfMWocQ77RmM6JC1bKAcjZIeX511GvRqigg2tu8aE?e=y1DHJG). Its full text was retrieved before implementation. The brief permits decisions where it is silent; decisions below are implementation choices, not extra assignment requirements.

Backend only: Go, PostgreSQL, JWT authentication, projects/members/invitations, tasks/dependencies, materials/stores/deliveries, all eleven screen APIs, and a realistic scale-data generator. A clean `docker compose up` must start a usable application and database without manual setup.

## Chosen approach

The user selected Go, hexagonal architecture, and an ORM/raw-SQL combination. Use GORM for straightforward persistence and parameterized SQL for authorization-scoped aggregates, graph traversal, locking and stock movements. A SQL-only adapter would make simple CRUD more verbose; putting ORM models in the domain would couple business rules to persistence. Keep persistence models and GORM types inside adapters.

`internal/domain` contains business types, errors and pure rules. `internal/ports` contains interfaces expressed in domain types. `internal/application` orchestrates use cases and authorization. HTTP and PostgreSQL adapters implement external boundaries. `cmd/api` wires dependencies; `internal/platform` provides configuration and lifecycle utilities. Versioned SQL migrations own the schema; do not use AutoMigrate.

## Binding behavior

- Registration accepts name, email, password, and one app role: User, Company store manager, or SuperAdmin. Public selection of any of these roles is explicitly required by this take-home and must be documented as an assignment-only production trade-off.
- Login errors must not disclose whether an email exists. Passwords are hashed. JWTs identify an independently revocable session; expiry, signature algorithm and issuer/audience must be checked. Refresh keeps active users logged in. Logout invalidates that session immediately, including already-issued access tokens. Every authenticated request loads current account/session state; do not trust stale role claims.
- Users access only projects they belong to; store managers additionally access the company store; SuperAdmins access every project and the company store. Scope list/search/count/report queries, not only individual fetches. Inaccessible project resources return 404. Membership/role changes apply on the next request.
- Creating a project makes its creator a project admin. Only project admins manage invitations/members/admin flags. Accepting an invitation creates membership; only its recipient can accept/reject. Pending invitations can be withdrawn. A project always retains at least one admin, including under concurrent removals. Notifications are newest-first and expose an unread count.
- Project IDs are monotonically allocated as PRJ-0001 and task IDs as PRJ-0001-T-0001 with task numbering per project. Numbers must not collide concurrently or be reused after deletion. Four digits are a minimum width, not a cap at 9,999.
- Dependencies are same-project directed edges. Reject self-links and cycles, including concurrent cycle creation. Start changes Not started to In progress only when every reachable prerequisite is Done; Done is only reachable from In progress. Reopen changes Done to In progress only when no transitive dependent has started. Reject all other changes. Late and Blocked are derived, may coexist, and cannot be written by clients.
- Late means not Done with end date before today, or Not started with start date before today. Blocked means not Done with any direct or transitive prerequisite not Done. Task details show direct/transitive prerequisites and dependents with distance and currently allowed actions.
- Materials have name/category/unit. The catalogue is shared by authenticated users. Each project has a store; there is one company store. Only authorized company-store actors add new stock or set reorder points.
- Deliveries have one or more positive-quantity lines, company-to-project or project-to-company only. Initiation atomically deducts sender stock and creates In transit delivery. Receiver records or rejects; sender cancels while In transit. Record credits receiver; reject/cancel refunds sender. Terminal transitions must not double-credit or refund under races. Lines are immutable. Never permit negative stock. All multi-line effects are atomic.
- Company stock status: zero = Out of stock; positive and at/below reorder point = Low stock; above reorder = Available. Company stock includes quantity in transit to projects. Project stock includes date of last delivery in/out.
- Support every screen's fields, filters, search and sorting from the brief; document request/response contracts. Lists must have bounded pagination and deterministic ordering. Never silently ignore unsupported query parameters that alter filtering or sorting.

## Decisions to implement and document

- Date-only inputs use YYYY-MM-DD; one explicit business timezone governs today (default Asia/Muscat). Use an injectable clock in rules/tests.
- Separate short-lived JWT access tokens from rotating, hashed, opaque refresh tokens. Keep session validation in PostgreSQL for immediate revocation.
- Return stable machine-readable errors and avoid exposing SQL/internal exception details.
- Quantities use exact fixed precision (three decimal places), represented as decimal strings at the API boundary; never use binary floating point for inventory. Check overflow and reject excess precision.
- Graph mutations/status transitions serialize within a project so cycle and reopen/start checks cannot race. Stock transitions lock their delivery and affected stock rows in deterministic order. Membership/admin operations lock the relevant project when maintaining the last-admin invariant.
- Versioned migrations, uniqueness/FK/check constraints and transaction boundaries enforce durable invariants. No distributed services or cache are necessary.
- Protect history on deletion: document policies for deleting referenced materials/tasks and projects that hold stock or active deliveries. Preserve issued IDs and historical delivery lines.
- For paths with multiple dependency routes, distance means shortest edge count. For large list screens, avoid per-row queries and unbounded recursive path enumeration.

## Scale and proof

Generator target: 5,000 users; 2,000 projects (up to 50 members each); 50,000 invitations; 500,000 tasks (up to 3,000 per project); 1,000,000 dependency edges; 20,000 materials; 400,000 deliveries with 3,000,000 material lines. Generate valid realistic graphs, sessions/credentials suitable for inspection, mixed statuses/dates, and conserved nonnegative stock. Batch SQL/COPY and deterministic data are preferable to millions of ORM calls. Include a small preset and a full preset, progress and clear rerun behavior. Do not fabricate large-dataset measurements.

Verification covers real PostgreSQL transactions, immediate access changes, graph and admin races, concurrent terminal delivery transitions, stock conservation, filtered authorization-safe reads, pagination, migrations, seed execution and HTTP flows. Use isolated PostgreSQL containers and verify the complete Compose startup without changing unrelated local services. Validate configuration and repeat the relevant checks in CI.
