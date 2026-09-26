SELECT json_build_object(
 'accounts', (SELECT json_agg(a) FROM (SELECT account_number,available_balance,ledger_balance FROM accounts WHERE account_number IN ('AURORA-TEST-000001','AURORA-TEST-000002') ORDER BY account_number) a),
 'matching_transactions', (SELECT count(*) FROM transactions WHERE channel='SYSTEM' AND idempotency_key='e2e-insufficient-balance-001'),
 'transactions', (SELECT count(*) FROM transactions),
 'transaction_accounts', (SELECT count(*) FROM transaction_accounts),
 'journal_entries', (SELECT count(*) FROM journal_entries),
 'journal_lines', (SELECT count(*) FROM journal_lines),
 'ledger_entries', (SELECT count(*) FROM ledger_entries)
);
