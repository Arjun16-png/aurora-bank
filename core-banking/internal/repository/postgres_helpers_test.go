package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"aurora-bank/core-banking/internal/model"
	"aurora-bank/core-banking/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	source       = "AURORA-TEST-000001"
	destination  = "AURORA-TEST-000002"
	thirdAccount = "AURORA-TEST-000003"
)

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("transfer_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+quoted+" CASCADE"); err != nil {
			t.Errorf("cleanup schema: %v", err)
		}
		if err := admin.Close(ctx); err != nil {
			t.Errorf("close admin: %v", err)
		}
	})
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 12
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	config.ConnConfig.RuntimeParams["application_name"] = schema
	config.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, file := range []string{"../../../database/schema/001_init.sql", "../../../database/seeds/001_core_banking_seed.sql"} {
		sql, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func request(key string) model.Request {
	return model.Request{Source: source, Destination: destination, Amount: "100000.00", Currency: "IDR", Key: key}
}
func execSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

// Include every row and column, not just counts: this detects changes to either
// balance, timestamps, metadata, and pre-existing financial records.
func snapshot(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, table := range []string{"accounts", "transactions", "transaction_accounts", "journal_entries", "journal_lines", "ledger_entries"} {
		var rows string
		if err := pool.QueryRow(context.Background(), "SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY r.id), '[]'::jsonb)::text FROM "+table+" r").Scan(&rows); err != nil {
			t.Fatal(err)
		}
		state[table] = rows
	}
	return state
}
func assertUnchanged(t *testing.T, pool *pgxpool.Pool, before map[string]string) {
	t.Helper()
	for table, after := range snapshot(t, pool) {
		if before[table] != after {
			t.Errorf("%s changed\nbefore=%s\nafter=%s", table, before[table], after)
		}
	}
}
func assertCounts(t *testing.T, pool *pgxpool.Pool, transfers int) {
	t.Helper()
	for table, multiplier := range map[string]int{"transactions": 1, "transaction_accounts": 2, "journal_entries": 1, "journal_lines": 2, "ledger_entries": 2} {
		var count int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != transfers*multiplier {
			t.Errorf("%s count=%d want=%d", table, count, transfers*multiplier)
		}
	}
}
func assertBalances(t *testing.T, pool *pgxpool.Pool, want map[string][2]string) {
	t.Helper()
	for number, balances := range want {
		var available, ledger string
		if err := pool.QueryRow(context.Background(), `SELECT available_balance::text,ledger_balance::text FROM accounts WHERE account_number=$1`, number).Scan(&available, &ledger); err != nil {
			t.Fatal(err)
		}
		if available != balances[0] || ledger != balances[1] {
			t.Errorf("%s available=%s ledger=%s want=%v", number, available, ledger, balances)
		}
	}
}
func assertPosting(t *testing.T, pool *pgxpool.Pool, result model.Result, r model.Request) {
	t.Helper()
	if result.Status != "POSTED" || result.Reference == "" || result.PostedAt.IsZero() || result.Amount != r.Amount || result.Currency != r.Currency || result.Source != r.Source || result.Destination != r.Destination {
		t.Fatalf("unexpected result: %+v", result)
	}
	var valid bool
	err := pool.QueryRow(context.Background(), `SELECT
  t.status='POSTED' AND t.channel='SYSTEM' AND t.transaction_type='INTERNAL_TRANSFER'
  AND t.idempotency_key=$2 AND t.amount=$3::numeric AND t.currency_code=$4 AND t.posted_at=$5
  AND (SELECT count(*)=1 AND bool_and(status='POSTED' AND posted_at=t.posted_at) FROM journal_entries WHERE transaction_id=t.id)
  AND (SELECT count(*)=2 AND count(*) FILTER(WHERE entry_side='DEBIT')=1 AND count(*) FILTER(WHERE entry_side='CREDIT')=1
    AND sum(CASE WHEN entry_side='DEBIT' THEN amount ELSE -amount END)=0
    AND bool_and(l.amount=t.amount AND l.currency_code=t.currency_code AND a.account_number=CASE WHEN l.entry_side='DEBIT' THEN $6 ELSE $7 END)
    FROM journal_lines l JOIN journal_entries j ON j.id=l.journal_entry_id JOIN accounts a ON a.id=l.account_id WHERE j.transaction_id=t.id)
  AND (SELECT count(*)=2 AND count(*) FILTER(WHERE role='DEBIT')=1 AND count(*) FILTER(WHERE role='CREDIT')=1
    AND bool_and(ta.amount=t.amount AND a.account_number=CASE WHEN ta.role='DEBIT' THEN $6 ELSE $7 END)
    FROM transaction_accounts ta JOIN accounts a ON a.id=ta.account_id WHERE ta.transaction_id=t.id)
  AND (SELECT count(*)=2 AND count(*) FILTER(WHERE posting_type='DEBIT')=1 AND count(*) FILTER(WHERE posting_type='CREDIT')=1
    AND bool_and(le.amount=t.amount AND le.currency_code=t.currency_code AND le.posted_at=t.posted_at AND le.effective_at=t.posted_at
      AND le.balance_after=CASE WHEN le.posting_type='DEBIT' THEN $8::numeric ELSE $9::numeric END
      AND le.journal_entry_id=jl.journal_entry_id AND le.account_id=jl.account_id AND le.posting_type=jl.entry_side
      AND a.account_number=CASE WHEN le.posting_type='DEBIT' THEN $6 ELSE $7 END)
    FROM ledger_entries le JOIN journal_lines jl ON jl.id=le.journal_line_id JOIN accounts a ON a.id=le.account_id WHERE le.transaction_id=t.id)
  FROM transactions t WHERE t.transaction_reference=$1`, result.Reference, r.Key, string(r.Amount), r.Currency, result.PostedAt, r.Source, r.Destination, string(result.SourceBalance), string(result.DestinationBalance)).Scan(&valid)
	if err != nil || !valid {
		t.Fatalf("invalid financial posting for %s: %v", result.Reference, err)
	}
	// These fixtures have equal available and ledger balances, so the historical
	// ledger balance_after must equal the corresponding response balance.
}

type outcome struct {
	request model.Request
	result  model.Result
	err     error
}

// Force actual overlap without timing sleeps: hold the shared source row, start
// all requests, and observe PostgreSQL lock waits before releasing the row.
func contendedTransfers(t *testing.T, pool *pgxpool.Pool, requests []model.Request) []outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = blocker.Rollback(ctx)
	}()
	if _, err = blocker.Exec(ctx, `SELECT id FROM accounts WHERE account_number=$1 FOR UPDATE`, source); err != nil {
		t.Fatal(err)
	}
	replies := make(chan outcome, len(requests))
	start := make(chan struct{})
	for _, r := range requests {
		go func(r model.Request) {
			<-start
			result, err := (service.Service{Repository: Postgres{Pool: pool}}).Transfer(ctx, r)
			replies <- outcome{r, result, err}
		}(r)
	}
	close(start)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waitCtx, stopWait := context.WithTimeout(ctx, 5*time.Second)
	defer stopWait()
	for {
		var waiting int
		err = pool.QueryRow(waitCtx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock'`, pool.Config().ConnConfig.RuntimeParams["application_name"]).Scan(&waiting)
		if err != nil {
			t.Fatalf("observe lock contention: %v", err)
		}
		if waiting >= len(requests) {
			break
		}
		select {
		case early := <-replies:
			t.Fatalf("request completed before lock release: %+v", early)
		case <-waitCtx.Done():
			t.Fatalf("only %d/%d requests reached lock wait", waiting, len(requests))
		case <-ticker.C:
		}
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	results := make([]outcome, 0, len(requests))
	for range requests {
		select {
		case r := <-replies:
			results = append(results, r)
		case <-ctx.Done():
			t.Fatal("concurrent transfers did not finish:", ctx.Err())
		}
	}
	return results
}
