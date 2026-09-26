# Evidence: internal transfer idempotency

Result: **PASS**

Captured at: 2026-09-26T06:29:33.789280+00:00

Endpoint: `POST http://127.0.0.1:8080/api/v1/transfers/internal`

Channel: `SYSTEM`  
Idempotency key: `e2e-transfer-001`

## Scope

This captures a retry of an existing successful transaction, not a new initial transfer. The initial transaction response was supplied in the conversation; the reference and posting time below were compared against that response. Before/after database snapshots and the retry HTTP response were captured live for this evidence.

## Verified results

- HTTP 200; status POSTED.
- Original reference retained: `TRF-c4611216e369eaa0ce44db6c8949fbf8`.
- Original posting timestamp retained: `2026-09-26T06:22:28.013245Z`.
- Amount: IDR 100000.00.
- Source available and ledger balances remain IDR 400000.00.
- Destination available and ledger balances remain IDR 200000.00.
- Exactly one transaction, two transaction-account records, one POSTED journal, and two ledger entries for this key.
- Journal debit-minus-credit total: 0.00.
- All captured database fields are identical before and after the retry.

## Raw evidence

- [Request](request.json)
- [HTTP response headers](response-headers.txt)
- [HTTP response body](response.json)
- [Database before retry](before.json)
- [Database after retry](after.json)
- [Verification SQL](verification.sql)

Assertions were checked with Python using exact decimal parsing. No application code or database schema was changed. This evidence covers identical-request replay; it does not claim changed-payload or concurrent-request testing in this live run.
