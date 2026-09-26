SELECT json_build_object(
 'transactions', (SELECT json_agg(t) FROM (SELECT transaction_reference,status,amount,posted_at FROM transactions WHERE channel='SYSTEM' AND idempotency_key='e2e-transfer-001') t),
 'accounts', (SELECT json_agg(a) FROM (SELECT account_number,available_balance,ledger_balance FROM accounts WHERE account_number IN ('AURORA-TEST-000001','AURORA-TEST-000002') ORDER BY account_number) a),
 'transaction_accounts', (SELECT count(*) FROM transaction_accounts a JOIN transactions t ON t.id=a.transaction_id WHERE t.channel='SYSTEM' AND t.idempotency_key='e2e-transfer-001'),
 'journals', (SELECT json_agg(j) FROM (SELECT j.journal_number,j.status,SUM(CASE WHEN l.entry_side='DEBIT' THEN l.amount ELSE -l.amount END) AS net_amount FROM journal_entries j JOIN transactions t ON t.id=j.transaction_id JOIN journal_lines l ON l.journal_entry_id=j.id WHERE t.channel='SYSTEM' AND t.idempotency_key='e2e-transfer-001' GROUP BY j.journal_number,j.status) j),
 'ledger_entries', (SELECT count(*) FROM ledger_entries l JOIN transactions t ON t.id=l.transaction_id WHERE t.channel='SYSTEM' AND t.idempotency_key='e2e-transfer-001')
);
