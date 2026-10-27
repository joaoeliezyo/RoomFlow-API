# Changelog

All notable changes to RoomFlow API are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-10-27

Initial release of RoomFlow API, a RESTful room and reservation management service built with Go and PostgreSQL.

### Added

- Room management endpoints to create, list, retrieve, partially update, and delete rooms.
- Reservation endpoints to create, list, retrieve, and cancel reservations while retaining cancellation history.
- Database-enforced prevention of overlapping active reservations for the same room using a PostgreSQL partial GiST exclusion constraint and half-open `[start, end)` time ranges.
- UTC-normalized reservation timestamps and RFC 3339 timestamp validation.
- Referential integrity that prevents deleting rooms with reservation history, including cancelled reservations.
- PostgreSQL schema migrations for rooms and reservations, with explicit up/down scripts.
- Health check endpoint (`GET /healthz`) that checks database connectivity.
- Environment-based configuration, graceful shutdown, and Docker multi-stage build with Docker Compose support.
- Unit, persistence integration, HTTP integration, and concurrent reservation/cancellation tests.
- GitHub Actions CI with PostgreSQL, migration setup, `go test ./...`, `go vet ./...`, and `go test -race ./...`.
- OpenAPI 3.1 specification in `docs/openapi.yaml` and setup instructions in `README.md`.

### Validation and error handling

- Consistent JSON response envelopes (`data` on success, `error` on failure).
- Input validation for room names and capacities, reservation IDs and periods, unknown or duplicate JSON keys, and invalid payloads.
- Appropriate HTTP error responses for missing resources, reservation conflicts, repeated cancellation, and rooms with reservation history.
- Generic internal error responses that do not expose database details.

### Known limitations

- Database migrations must be run manually; applying down migrations removes the corresponding data.
- The API does not include authentication, authorization, pagination, or a graphical interface.
- Reservation creation does not impose a maximum duration or reject past timestamps.

[1.0.0]: https://github.com/joaoeliezyo/RoomFlow-API/releases/tag/v1.0.0
