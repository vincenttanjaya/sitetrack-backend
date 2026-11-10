# SiteTrack backend

A Go and PostgreSQL backend for the SiteTrack construction tracker. It provides accounts, projects, invitations, task dependencies, inventory, and two-way deliveries.

[Assignment brief](https://seronlabs-my.sharepoint.com/:b:/p/sarvesh/IQAwwLqfMWocQ77RmM6JC1bKAcjZIeX511GvRqigg2tu8aE?e=y1DHJG) · [API reference](docs/API.md) · [Design decisions](docs/DECISIONS.md)

## Run locally

```sh
docker compose up
```

The API starts at `http://127.0.0.1:8080`. Compose starts PostgreSQL 18, the API applies migrations, and `GET /readyz` becomes healthy. The database lives in a named volume. Change the host port with `API_PORT` if needed.

The default signing key is random and process-local. Set `JWT_SECRET` to a strong random value of at least 32 bytes if access tokens must survive an API restart. A valid refresh token can issue a new access token after a restart. [.env.example](.env.example) lists the Compose options.

To run against your own PostgreSQL instance, use Go 1.27 or later and set `DATABASE_URL`.

```sh
export DATABASE_URL='postgres://sitetrack:your-password@127.0.0.1:5432/sitetrack?sslmode=disable'
go run ./cmd/api
```

`APP_ADDR` defaults to `:8080`. `BUSINESS_TIMEZONE` accepts a named IANA zone or `UTC` and defaults to `Asia/Muscat`. Access and refresh durations default to `15m` and `720h` and can be set through `ACCESS_TOKEN_TTL` and `REFRESH_TOKEN_TTL`.

## Load sample data

Load a small demo into a fresh Compose database.

```sh
docker compose run --rm api sitetrack-seed --preset small --anchor 2026-11-10
```

Use `--preset full` for the evaluation dataset. With a local PostgreSQL connection, run `go run ./cmd/seed --preset full --anchor 2026-11-10` using the same `DATABASE_URL`. The generator reports row counts as it loads set-based batches in one transaction. The anchor defaults to today's UTC date. Pass the same anchor for repeatable dates and fixture data.

The seed refuses a database with application data unless given `--reset`, which creates a new fixture dataset and discards previous records and sessions. Run the full seed and any reset on a dedicated database or with the API stopped, since seeding holds database write locks.

| Dataset | Users | Projects | Invitations | Tasks | Edges | Materials | Deliveries | Delivery lines |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Small | 80 | 8 | 64 | 160 | 320 | 120 | 96 | 336 |
| Full | 5,000 | 2,000 | 50,000 | 500,000 | 1,000,000 | 20,000 | 400,000 | 3,000,000 |

The full preset includes a project with 3,000 tasks and 50 members. The local full load and consistency checks are recorded in [performance verification](docs/PERFORMANCE.md). Each fixture login uses the password `SiteTrack-demo-2026!`. Use demo data only in a local or isolated evaluation database.

| Demo email | Access |
| --- | --- |
| `site-admin@sitetrack.test` | SuperAdmin with every project and the company store |
| `warehouse@sitetrack.test` | Company store manager |
| `member@sitetrack.test` | User and admin of `PRJ-0001` |
| `invitee@sitetrack.test` | User with a pending invitation to `PRJ-0001` |

## Tests

Integration tests use real PostgreSQL. Set `TEST_DATABASE_URL` to an isolated database whose role can create schemas. Each test creates and cleans up its own migrated schema.

```sh
go vet ./...
go test -race -count=1 ./...
go build ./...
docker compose config --quiet
```

Without `TEST_DATABASE_URL`, the database integration tests skip. GitHub Actions runs them on PostgreSQL 18 and starts a clean Compose stack for smoke tests.

## Architecture

`internal/domain` defines the business types and rules. `internal/application` implements use cases against interfaces in `internal/ports`. The HTTP adapter translates JSON and HTTP requests. The PostgreSQL adapter uses GORM for straightforward CRUD and parameterized SQL for graph traversal, scoped aggregates, and atomic stock movements. `cmd/api` wires the application and applies the embedded migrations.

Public selection of all three app roles, including SuperAdmin, is part of the assignment. A production rollout would provision elevated roles through a privileged workflow and add TLS, rate limits, session cleanup, operational monitoring, and a deployment-managed signing key. See [design decisions](docs/DECISIONS.md) for data integrity, query, and operational trade-offs.
