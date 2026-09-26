# Evidence: insufficient balance

Result: **PASS**

Captured at: 2026-09-26T06:33:54.175719+00:00

Endpoint: `POST http://127.0.0.1:8080/api/v1/transfers/internal`

Channel: `SYSTEM`  
Idempotency key: `e2e-insufficient-balance-001`

The source account had IDR 400000.00 available. A transfer of IDR 400001.00 to AURORA-TEST-000002 was submitted with a new key.

## Verified results

- HTTP 422 with `{"error":"insufficient available balance"}`.
- Source available and ledger balances remain IDR 400000.00.
- Destination available and ledger balances remain IDR 200000.00.
- Zero transactions exist for the tested key.
- Database counts remain unchanged: one transaction, two transaction-account records, one journal, two journal lines, two ledger entries (all pre-existing).
- All captured before/after database fields are identical; assertions used exact decimal parsing.

## Raw evidence

- [Request](request.json)
- [Response headers](response-headers.txt)
- [Response body](response.json)
- [Database before](before.json)
- [Database after](after.json)
- [Verification SQL](verification.sql)

No application code or database schema was changed.
