# SiteTrack Backend

Repository scaffold for the Seron Labs SiteTrack backend take-home. The planned stack is Go and PostgreSQL, with startup via `docker compose up`. API and Docker Compose setup are pending.

## Assignment scope

- JWT authentication and role-based access
- Projects, tasks, and task dependencies
- Material transfers between the central store and project stores
- Support for datasets with millions of rows

[Assignment brief](https://seronlabs-my.sharepoint.com/:b:/p/sarvesh/IQAwwLqfMWocQ77RmM6JC1bKAcjZIeX511GvRqigg2tu8aE?e=y1DHJG)

## Hexagonal architecture

- `internal/domain` holds domain types and rules and depends only on the Go standard library.
- `internal/application` orchestrates use cases through domain types and ports.
- `internal/ports` defines interfaces using domain types and the standard library.
- `internal/adapters/http` calls application use cases. `internal/adapters/postgres` implements persistence ports.
- `cmd/api` is the composition root. `internal/platform` holds configuration and bootstrap code.
- `migrations` holds PostgreSQL schema migrations.

## Persistence approach (planned)

- Use GORM for CRUD and parameterized raw SQL for complex queries and stock transfers. Keep GORM models, tags, and SQL in the PostgreSQL adapter so the domain and ports stay independent.
- Run every write in a stock transfer through the same transaction handle and lock the relevant rows before updating stock.
- Manage schema changes with versioned SQL migrations. Use indexes that match query patterns, cursor pagination for large result sets, and query-plan checks against representative data.

[GORM SQL builder](https://gorm.io/docs/sql_builder.html) · [GORM transactions](https://gorm.io/docs/transactions.html)
