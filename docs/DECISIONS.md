# Design decisions

## Access and sessions

- App roles and project membership are separate. SuperAdmin can work with task, store, and delivery data across projects. An actual project-admin membership is required to administer a project's metadata, memberships, and invitations.
- The company store is a distinct delivery scope. A warehouse actor can send to or receive from any project on the company route and sees the project label on that delivery. The project's routes still require project access.
- The assignment permits registering as any app role. Production would restrict elevated roles to a privileged provisioning flow. Catalogue CRUD is shared by all authenticated roles.
- JWTs carry user and session identifiers. The API loads the current role from PostgreSQL and checks session revocation on every authenticated request. Refresh tokens are random, stored as SHA-256 digests and rotated in one SQL statement. The refresh expiry slides on successful refresh. Logout revokes the session immediately.
- Passwords use bcrypt. Login returns the same error for an unknown email and an incorrect password. Production would add throttling, account recovery and email verification, automated cleanup of expired sessions, stable managed signing keys, and audit logging.

## Projects and graphs

- The project sequence is database-owned. Each project allocates its next task number while holding its project row lock. Deleting a row leaves a gap and preserves all other IDs.
- The project row serializes graph writes, membership-admin changes, invitations, and inventory writes made through project routes. This trades some same-project write concurrency for straightforward locking and consistent invariants.
- Task dates are independent of project dates. A task is late when a relevant date is strictly before the current business date. The planned date range filter uses inclusive overlap.
- A started task can gain a new prerequisite only when the prerequisite is Done. Combined with start and reopen rules and acyclicity, every Done prerequisite has a fully Done chain. This lets task and project list queries derive Blocked with an indexed direct-prerequisite existence check. Task details and status checks traverse the task's reachable prerequisites and dependents with indexed edge lookups. Distances are shortest edge counts.
- A task with incoming dependency references requires explicit unlinking before deletion. A project with any delivery history or outstanding inventory cannot be deleted. Empty projects can be deleted along with their tasks, memberships, invitations, and store.
- Notification read status is explicit. Viewing the list leaves it unchanged. Responding marks the invitation read. The frontend can fetch the unread count separately on every screen.

## Inventory

- Quantities use signed 64-bit integer thousandths with nonnegative checks in PostgreSQL. Decimal strings avoid precision loss at the JSON boundary.
- Initiation deducts available stock and reserves refund headroom for the sender. Record releases the reservation and credits the receiver. Reject and cancel release the reservation and restore the sender. A receiver near the numeric limit can reject or make space before retrying a record.
- Delivery actions are scoped to the acting store and lock the delivery and affected stock rows. Project-scoped actions also lock the project to serialize against membership changes. Stock rows are touched in deterministic order. Delivery line descriptions and units are snapshotted so catalogue edits preserve past records.
- Company stock shows every catalogue item and computes availability from its on-hand quantity. Project stores show materials already handled by the store. The last-delivery timestamp uses sent time on dispatch and recorded time on receipt. Reject and cancel preserve the dispatch history.
- Materials with inventory, reorder points or delivery lines cannot be removed or have their unit changed. A receipt adds stock directly to the company store. A production inventory audit could persist supplier receipts and adjustments as a separate ledger.

## Scale and operation

- Cursor pagination uses allowlisted ordering and unique ties. Pages reflect current data rather than a frozen snapshot. Delivery lines are hydrated in one batch for the page. Non-metric project sorting pages projects before calculating task counts and graph flags.
- Completion and late-count project sorts compute totals at read time. Completion uses indexed per-project status counts. Late ranking groups tasks from visible projects in one query. Project lists disable PostgreSQL JIT for the read transaction to avoid compilation overhead. If production traffic makes these sorts expensive, measure and maintain a project summary table with an explicit refresh strategy.
- PostgreSQL handles migrations and persistence. The API uses a bounded connection pool and request timeouts. The Compose setup uses local database credentials and publishes only the HTTP API on loopback. Production would add managed PostgreSQL backups, observability, and rollout migration controls.
- The seed generates valid dependency graphs and chronological deliveries using set-based SQL. Company supply is provisioned to cover gross sends. Project returns follow earlier successful deliveries of the same materials. Stock and reservations are derived from the generated history.
