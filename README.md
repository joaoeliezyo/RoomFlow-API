# RoomFlow API

[![CI](https://github.com/joaoeliezyo/RoomFlow-API/actions/workflows/ci.yml/badge.svg)](https://github.com/joaoeliezyo/RoomFlow-API/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/joaoeliezyo/RoomFlow-API)](https://github.com/joaoeliezyo/RoomFlow-API/releases/tag/v1.0.0) ![Go](https://img.shields.io/badge/Go-1.27-00ADD8) ![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1) ![OpenAPI](https://img.shields.io/badge/OpenAPI-3.1-6BA539)

**A concurrency-safe room reservation REST API built with Go and PostgreSQL.**

**English** · [Português (Brasil)](README.pt-BR.md)

RoomFlow API is a backend portfolio project for managing rooms and their reservations. Its central engineering challenge is preventing overlapping bookings **even when requests arrive concurrently**. Rather than trusting a read-before-write application check, PostgreSQL enforces this invariant using a partial GiST exclusion constraint.

## Highlights

- Room CRUD: create, list, retrieve, partially update, and delete rooms.
- Reservation lifecycle: create, list, retrieve, and cancel without losing history.
- Database-enforced exclusion of overlapping **active** bookings for the same room.
- RFC3339 timestamps with explicit time zones; responses normalized to UTC.
- Explicit SQL with `pgx/v5`; no ORM or third-party HTTP framework.
- HTTP and PostgreSQL integration tests, concurrent-write tests, and GitHub Actions CI.
- OpenAPI 3.1 contract and reproducible Docker Compose development setup.

## Architecture at a glance

```mermaid
flowchart TD
    Client[HTTP client] --> HTTP[net/http router & handlers]
    HTTP --> Validation[Input validation & JSON responses]
    Validation --> Rooms[Room store]
    Validation --> Reservations[Reservation store]
    Rooms --> PG[(PostgreSQL 17)]
    Reservations --> PG
    PG --> Rules[FK, CHECK & partial GiST exclusion constraint]
```

Handlers are in `internal/httpapi`, SQL stores are in `internal/rooms` and `internal/reservations`, and dependency composition lives in `cmd/api/main.go`. See [architecture and design decisions](docs/architecture.md) for the data model, concurrency flow, and test strategy.

## Why the database constraint matters

Two concurrent requests for an overlapping time slot in the same room cannot both succeed. PostgreSQL enforces the rule:

```sql
CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
    room_id WITH =,
    tstzrange(starts_at, ends_at, '[)') WITH &&
) WHERE (cancelled_at IS NULL)
```

The `[)` interval includes the start and excludes the end: **09:00–10:00** and **10:00–11:00** are allowed back to back. Cancelling a reservation sets `cancelled_at`, preserves its record, and frees the interval. A room with reservation history, including cancelled bookings, cannot be deleted.

## Tech stack

Go 1.27 · `net/http` · `pgx/v5` · PostgreSQL 17 · Docker / Docker Compose · Go `testing` · GitHub Actions · OpenAPI 3.1.

## Quick start (Docker Compose)

Requirements: Docker and Docker Compose. For a **fresh database**, apply migrations in order:

```sh
docker compose up -d --wait db
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose up -d --build api
curl -i http://localhost:8080/healthz
```

If migration 001 is already applied, run only 002. Migrations are **manual**; do not reapply an existing version. They are not run on API startup. The default credentials and ports in Compose are for local development only.

### Sample requests

Create a room (save the returned ID, shown in the `Location` header):

```sh
curl -i -X POST http://localhost:8080/rooms \
  -H 'Content-Type: application/json' \
  -d '{"name":"Blue Room","capacity":8}'
```

Create a reservation (replace `1` with your room ID):

```sh
curl -i -X POST http://localhost:8080/reservations \
  -H 'Content-Type: application/json' \
  -d '{"room_id":1,"starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}'
```

Submitting an overlapping active booking for the same room returns `409 Conflict`:

```json
{"error":{"code":"reservation_conflict","message":"reservation overlaps an active reservation"}}
```

Cancel with `POST /reservations/{id}/cancel` and then retry the timeslot. You can explore the complete requests and responses in [OpenAPI 3.1](docs/openapi.yaml).

## Endpoints

| Method | Route | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Check API and database connectivity |
| `POST` | `/rooms` | Create room (`201`, `Location`) |
| `GET` | `/rooms` | List rooms |
| `GET` | `/rooms/{id}` | Get room |
| `PATCH` | `/rooms/{id}` | Update room fields |
| `DELETE` | `/rooms/{id}` | Delete room without reservation history |
| `POST` | `/reservations` | Create reservation (`201`, `Location`; `409` on overlap) |
| `GET` | `/reservations` | List reservations, including cancelled records |
| `GET` | `/reservations/{id}` | Get reservation |
| `POST` | `/reservations/{id}/cancel` | Cancel reservation (`409` if already cancelled) |

Success responses use `{"data":...}` (except `204 No Content`). Errors use `{"error":{"code":"...","message":"..."}}`. Invalid or duplicate JSON fields are rejected; request bodies are limited to 1 MiB. Read the [API specification](docs/openapi.yaml) for full status codes and schemas.

## Running locally without Docker

Requires Go 1.27 and PostgreSQL. Create a database and apply both SQL migrations with `psql -v ON_ERROR_STOP=1 -f ...`; set the environment variables (see [.env.example](.env.example)):

```sh
export DATABASE_URL='postgres://roomflow:roomflow@localhost:5432/roomflow?sslmode=disable'
export APP_PORT=8080
go run ./cmd/api
```

`DATABASE_URL` is required, and `APP_PORT` defaults to 8080. `.env.example` is not loaded automatically.

## Testing

The CI workflow [runs on pushes and pull requests to `main`](.github/workflows/ci.yml), starts a PostgreSQL 17 service, applies both migrations, and runs `go test ./...`, `go vet ./...`, and `go test -race ./...`. It sets `REQUIRE_INTEGRATION_TESTS=1` so integration tests cannot silently skip because of a missing database URL.

To run the isolated test database locally with Compose:

```sh
docker compose --profile test up -d --wait testdb
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/001_create_rooms.up.sql
docker compose --profile test exec -T testdb psql -U roomflow -d roomflow_test -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.up.sql
docker compose --profile test run --rm test
docker compose --profile test run --rm test go test -race ./...
docker compose --profile test run --rm test go vet ./...
docker compose --profile test down
```

`testdb` uses a temporary filesystem and is separate from the development database. Never point `TEST_DATABASE_URL` at your development or production database. Without this variable, integration tests are skipped outside CI.

## Repository layout

```text
cmd/api/                 startup, dependency wiring, graceful shutdown
internal/config/         environment configuration
internal/httpapi/        routing, validation, HTTP handlers and HTTP tests
internal/rooms/          room persistence and tests
internal/reservations/   reservation persistence and concurrency tests
db/migrations/           versioned up/down SQL migrations
docs/openapi.yaml        OpenAPI 3.1 contract
docs/architecture.md     architecture and design decisions
.github/workflows/ci.yml continuous integration
```

## Migrations and limitations

- Migrations are explicitly run in order (001, then 002), once per database. Reverting 002 **deletes all reservations**; reverting 001 **deletes all rooms**. Back up important data first. The `btree_gist` extension remains installed after rolling back 002.
- To roll back 002, stop the API and execute `docker compose exec -T db psql -U roomflow -d roomflow -v ON_ERROR_STOP=1 -f /migrations/002_create_reservations.down.sql`. To roll back 001, run its corresponding `.down.sql` command afterward.
- This is an MVP, **not a production-ready public service**: there is no authentication, authorization, pagination, user interface, reservation-duration cap, or restriction on past bookings.

## More documentation

[Architecture (English)](docs/architecture.md) · [Arquitetura (Português)](docs/architecture.pt-BR.md) · [OpenAPI](docs/openapi.yaml) · [Changelog](CHANGELOG.md) · [v1.0.0 release](https://github.com/joaoeliezyo/RoomFlow-API/releases/tag/v1.0.0)
