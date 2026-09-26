package repository

import (
	"aurora-bank/core-banking/internal/model"
	"aurora-bank/core-banking/internal/service"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"testing"
	"time"
)

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("transfer_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); _ = admin.Close(ctx) })
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
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
	return model.Request{Source: "AURORA-TEST-000001", Destination: "AURORA-TEST-000002", Amount: "100000.00", Currency: "IDR", Key: key}
}
func TestPostgresPostingAndConcurrency(t *testing.T) {
	pool := database(t)
	ctx := context.Background()
	s := service.Service{Repository: Postgres{pool}}
	var wg sync.WaitGroup
	results := make(chan model.Result, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r, e := s.Transfer(ctx, request("same-key")); results <- r; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first model.Result
	for r := range results {
		if first.Reference == "" {
			first = r
		}
		if first != r {
			t.Fatal("replay differs")
		}
	}
	if first.SourceBalance != "400000.00" || first.DestinationBalance != "200000.00" {
		t.Fatal(first)
	}
	for table, want := range map[string]int{"transactions": 1, "transaction_accounts": 2, "journal_entries": 1, "journal_lines": 2, "ledger_entries": 2} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var balanced, posted bool
	if err := pool.QueryRow(ctx, `SELECT sum(CASE entry_side WHEN 'DEBIT' THEN amount ELSE -amount END)=0 FROM journal_lines`).Scan(&balanced); err != nil || !balanced {
		t.Fatal("unbalanced journal", err)
	}
	if err := pool.QueryRow(ctx, `SELECT t.status='POSTED' AND j.status='POSTED' AND t.posted_at=j.posted_at FROM transactions t JOIN journal_entries j ON j.transaction_id=t.id`).Scan(&posted); err != nil || !posted {
		t.Fatal("not posted", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ledger_entries SET amount=1`); err == nil {
		t.Fatal("ledger mutable")
	}
	// Opposing transfers exercise the same deterministic row lock order.
	errs = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := request(fmt.Sprintf("opposite-%d", i))
			r.Amount = "1.00"
			if i%2 == 0 {
				r.Source, r.Destination = r.Destination, r.Source
			}
			_, e := s.Transfer(ctx, r)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var total string
	if err := pool.QueryRow(ctx, `SELECT sum(available_balance)::text FROM accounts`).Scan(&total); err != nil || total != "600000.00" {
		t.Fatal(total, err)
	}
}
func TestPostgresRollback(t *testing.T) {
	pool := database(t)
	ctx := context.Background()
	// A test-only trigger fails the final write, after balances and ledger inserts.
	_, err := pool.Exec(ctx, `CREATE FUNCTION reject_post() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected posting failure'; END $$; CREATE TRIGGER reject_post BEFORE UPDATE ON transactions FOR EACH ROW EXECUTE FUNCTION reject_post()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (service.Service{Repository: Postgres{pool}}).Transfer(ctx, request("rollback")); err == nil {
		t.Fatal("expected failure")
	}
	for _, table := range []string{"transactions", "transaction_accounts", "journal_entries", "journal_lines", "ledger_entries"} {
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	var balance string
	if err = pool.QueryRow(ctx, `SELECT available_balance::text FROM accounts WHERE account_number=$1`, request("").Source).Scan(&balance); err != nil || balance != "500000.00" {
		t.Fatal(balance, err)
	}
}

func TestPostgresConcurrentSpending(t *testing.T) {
	pool := database(t)
	ctx := context.Background()
	s := service.Service{Repository: Postgres{pool}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := request(fmt.Sprintf("spend-%d", i))
			r.Amount = "400000.00"
			_, err := s.Transfer(ctx, r)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success, insufficient := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if err == model.ErrInsufficient {
			insufficient++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || insufficient != 1 {
		t.Fatalf("success=%d insufficient=%d", success, insufficient)
	}
	var balance string
	if err := pool.QueryRow(ctx, `SELECT available_balance::text FROM accounts WHERE account_number=$1`, request("").Source).Scan(&balance); err != nil || balance != "100000.00" {
		t.Fatal(balance, err)
	}
}
