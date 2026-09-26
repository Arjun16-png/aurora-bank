# Internal transfer hardening results

Result: **PASS**

All changes are inside `core-banking/`. Production Go code and the existing database schema are unchanged. No implementation bug was reproduced; the prior suite had coverage gaps in rejection side effects, full rollback assertions, and the specified different-destination double-spend scenario.

## Validation

- `gofmt -w cmd internal` completed.
- `go test ./...` passed with `TEST_DATABASE_URL` set; PostgreSQL integration tests executed, not skipped. See [full-suite output](go-test.txt).
- `go test -race -count=25 -run 'TestPostgres(Concurrent|Opposing)' ./internal/repository` passed. See [repeated concurrency output](concurrency-race.txt).
- `git diff --check` passed.

Tests ran against an isolated local PostgreSQL cluster on port 55439, with a fresh temporary schema per test. The live Aurora database was not used or changed. No mocks are used in the database correctness/concurrency tests.

## Tests added or completed

1. `TestPostgresSuccessfulTransfer`: real HTTP request, both available/ledger balances, POSTED transaction, exactly one POSTED journal, equal debit/credit totals, exactly two linked ledger entries and historical balance values.
2. `TestPostgresRejectedTransfers`: insufficient funds; identical accounts; zero and negative amounts; missing account; currency mismatch; balance overflow; PENDING, DORMANT, BLOCKED and CLOSED statuses on each side. All cases assert full database snapshots unchanged and no financial records.
3. `TestPostgresDuplicateIdempotencyKey`: identical sequential replay without writes; changed amount, source, destination or currency rejected; historical replay preserved after further activity and account blocking.
4. `TestPostgresRollback`: injected failures after transaction creation and after all financial writes; validates that the intended failure stage was reached, every row/balance rolls back, and the same key can succeed after removing the fault.
5. `TestPostgresConcurrentDoubleSpend`: 100000 source balance, two concurrent 80000 transfers to different destination accounts; exactly one posts and one fails with insufficient funds, source ends at 20000, no negative balance update attempted, and ledger movements reconcile to all final balances.
6. `TestPostgresConcurrentIdempotency`: eight identical requests share one posting; eight same-key requests with conflicting amounts produce four successful identical responses and four conflicts with only one financial posting.
7. `TestPostgresOpposingTransfers`: eight opposite-direction requests complete under observed lock contention without deadlock or balance loss.
8. `TestPostgresLedgerImmutable`: both UPDATE and DELETE fail through the existing append-only trigger without changing rows.

Concurrency tests observe PostgreSQL lock waits before releasing a held source row, establishing real contention. Each contention scenario passed 25 repetitions with Go's race detector. Existing money, service and handler unit tests were retained.
