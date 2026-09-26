# Core Banking: internal transfers

Go `net/http` service using pgx and the existing PostgreSQL schema. No authentication is implemented; the default listener is `127.0.0.1:8080`.

## Run locally

Requirements: Go 1.26.2 or newer, PostgreSQL (the project Compose configuration uses PostgreSQL 17), and `psql` for loading seed data.

From the repository root:

```sh
docker compose up -d postgres
# Wait until PostgreSQL is ready. The schema is installed on first initialization.
psql 'postgres://postgres:postgres@localhost:5432/aurorabank?sslmode=disable' \
  -f database/seeds/001_core_banking_seed.sql
cd core-banking
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/aurorabank?sslmode=disable'
go run ./cmd/server
```

For an existing database without the schema, apply `database/schema/001_init.sql` once before loading seeds. The service does not migrate or modify the schema. Set `HTTP_ADDR` to override the listen address.

```sh
curl -i http://127.0.0.1:8080/api/v1/transfers/internal \
  -H 'Content-Type: application/json' \
  -d '{
    "source_account_number": "AURORA-TEST-000001",
    "destination_account_number": "AURORA-TEST-000002",
    "amount": 100000,
    "currency": "IDR",
    "idempotency_key": "test-transfer-001"
  }'
```

Success returns HTTP 200 with `transaction_reference`, `status`, `amount`, `currency`, `source_account_number`, `destination_account_number`, `source_balance_after`, `destination_balance_after`, and `posted_at`. With fresh seeds, balances after this transfer are 400000.00 and 200000.00. Response balances are **available balances**; ledger entries record the resulting **ledger balance**.

Amounts are JSON numbers in major currency units (100000 means IDR 100000), with at most two decimal places. Quoted numbers, exponent notation, zero, negative amounts, and values beyond `numeric(19,2)` are rejected. Calculations use integer arithmetic with arbitrary precision, so the full schema range is supported without floating point. Clients must also preserve decimal precision.

Errors return `{"error":"message"}`: 400 for invalid input, 404 for missing accounts, 409 for idempotency conflicts, 422 for inactive accounts, currency mismatch, insufficient funds or balance overflow, and 500 for unexpected failures. Database details are only logged server-side.

## Atomicity and retries

Every operation uses channel `SYSTEM`. A transaction-scoped advisory lock serializes requests for the same channel/key before checking the existing unique `(channel, idempotency_key)` constraint. Identical retries return the original committed response, including historical balances and posting time; changed transfer details return 409. If a commit outcome is uncertain, retry with the **same key**.

Both account rows are locked individually with `SELECT ... FOR UPDATE` in sorted account-number order. All validations, transaction roles, journal lines, immutable ledger inserts, balance changes and POSTED transitions run in one database transaction. Any failure rolls back the entire operation. The existing ledger mutation trigger provides append-only enforcement. No database schema changes are required; the request and original response are stored in transaction metadata for replay.

## Tests

```sh
gofmt -w cmd internal
go test ./...
```

Unit tests cover successful transfers, insufficient funds, identical accounts, invalid amounts, inactive accounts, currency mismatches, missing accounts, overflow, idempotent replay/conflict and HTTP error handling.

Integration tests require a disposable PostgreSQL database and a role allowed to create schemas and install the existing `pgcrypto` extension:

```sh
TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/aurorabank_test?sslmode=disable' \
  go test -race ./...
```

Each test creates an isolated schema, applies the unchanged project schema and seeds, then drops that test schema. Integration tests check real postings, balanced journals, immutable ledger entries, concurrent same-key retries, opposing transfers and full rollback after an injected final-write failure. Without `TEST_DATABASE_URL`, integration tests are explicitly skipped.

## Layout

- `cmd/server`: configuration, database connection and HTTP lifecycle
- `internal/httpapi`: JSON parsing and public HTTP responses
- `internal/service`: validation, locking order and transfer business logic
- `internal/repository`: PostgreSQL transactions and postings
- `internal/model`: request/result models and exact monetary arithmetic
