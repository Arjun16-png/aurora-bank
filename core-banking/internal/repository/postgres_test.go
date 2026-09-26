package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aurora-bank/core-banking/internal/httpapi"
	"aurora-bank/core-banking/internal/model"
	"aurora-bank/core-banking/internal/service"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSuccessfulTransfer(t *testing.T) {
	pool := database(t)
	result := postHTTP(t, pool, request("success"), http.StatusOK, "")
	assertPosting(t, pool, result, request("success"))
	assertBalances(t, pool, map[string][2]string{source: {"400000.00", "400000.00"}, destination: {"200000.00", "200000.00"}})
	assertCounts(t, pool, 1)
}

func TestPostgresRejectedTransfers(t *testing.T) {
	cases := []struct {
		name    string
		change  func(*model.Request)
		setup   string
		status  int
		message string
	}{
		{"insufficient_balance", func(r *model.Request) { r.Amount = "500000.01" }, "", 422, model.ErrInsufficient.Error()},
		{"same_account", func(r *model.Request) { r.Destination = r.Source }, "", 400, model.ErrSameAccount.Error()},
		{"zero_amount", func(r *model.Request) { r.Amount = "0.00" }, "", 400, model.ErrInvalid.Error()},
		{"negative_amount", func(r *model.Request) { r.Amount = "-1.00" }, "", 400, model.ErrInvalid.Error()},
		{"missing_account", func(r *model.Request) { r.Destination = "MISSING" }, "", 404, model.ErrNotFound.Error()},
		{"currency_mismatch", func(r *model.Request) { r.Currency = "USD" }, "", 422, model.ErrCurrency.Error()},
		{"balance_overflow", func(*model.Request) {}, `UPDATE accounts SET ledger_balance=99999999999999999.99 WHERE account_number='AURORA-TEST-000002'`, 422, model.ErrBalanceLimit.Error()},
	}
	// INACTIVE is not a schema-valid status. Cover every allowed non-ACTIVE status,
	// on both sides, including BLOCKED.
	for _, status := range []string{"PENDING", "DORMANT", "BLOCKED", "CLOSED"} {
		for _, number := range []string{source, destination} {
			cases = append(cases, struct {
				name    string
				change  func(*model.Request)
				setup   string
				status  int
				message string
			}{
				name: status + "_" + number, change: func(*model.Request) {}, setup: fmt.Sprintf("UPDATE accounts SET status='%s' WHERE account_number='%s'", status, number), status: 422, message: model.ErrInactive.Error(),
			})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := database(t)
			if tc.setup != "" {
				execSQL(t, pool, tc.setup)
			}
			before := snapshot(t, pool)
			r := request(tc.name)
			tc.change(&r)
			postHTTP(t, pool, r, tc.status, tc.message)
			assertUnchanged(t, pool, before)
			assertCounts(t, pool, 0)
		})
	}
}

func TestPostgresDuplicateIdempotencyKey(t *testing.T) {
	pool := database(t)
	r := request("duplicate")
	first := postHTTP(t, pool, r, 200, "")
	before := snapshot(t, pool)
	second := postHTTP(t, pool, r, 200, "")
	if first != second {
		t.Fatalf("retry changed response: first=%+v second=%+v", first, second)
	}
	assertUnchanged(t, pool, before)
	assertCounts(t, pool, 1)
	assertPosting(t, pool, first, r)
	assertBalances(t, pool, map[string][2]string{source: {"400000.00", "400000.00"}, destination: {"200000.00", "200000.00"}})
	for _, field := range []string{"amount", "source", "destination", "currency"} {
		t.Run("conflicting_"+field, func(t *testing.T) {
			changed := r
			switch field {
			case "amount":
				changed.Amount = "1.00"
			case "source":
				changed.Source = "OTHER"
			case "destination":
				changed.Destination = "OTHER"
			case "currency":
				changed.Currency = "USD"
			}
			postHTTP(t, pool, changed, 409, model.ErrConflict.Error())
			assertUnchanged(t, pool, before)
		})
	}
	// A replay must return historical balances, even after subsequent activity.
	later := request("later")
	later.Amount = "1.00"
	postHTTP(t, pool, later, 200, "")
	execSQL(t, pool, `UPDATE accounts SET status='BLOCKED' WHERE account_number=$1`, source)
	before = snapshot(t, pool)
	if replay := postHTTP(t, pool, r, 200, ""); replay != first {
		t.Fatalf("historical replay changed: %+v", replay)
	}
	assertUnchanged(t, pool, before)
	assertCounts(t, pool, 2)
}

func TestPostgresRollback(t *testing.T) {
	for _, stage := range []string{"after_transaction_insert", "after_all_financial_writes"} {
		t.Run(stage, func(t *testing.T) {
			pool := database(t)
			before := snapshot(t, pool)
			table, event, condition := "transaction_accounts", "INSERT", `(SELECT count(*) FROM transactions)=1`
			if stage == "after_all_financial_writes" {
				table, event = "transactions", "UPDATE"
				condition = `(SELECT count(*) FROM transactions)=1
     AND (SELECT count(*) FROM transaction_accounts)=2
     AND (SELECT count(*) FROM journal_entries WHERE status='POSTED')=1
     AND (SELECT count(*) FROM journal_lines)=2
     AND (SELECT count(*) FROM ledger_entries)=2
     AND (SELECT available_balance=400000 AND ledger_balance=400000 FROM accounts WHERE account_number='AURORA-TEST-000001')
     AND (SELECT available_balance=200000 AND ledger_balance=200000 FROM accounts WHERE account_number='AURORA-TEST-000002')`
			}
			// Test-only trigger, installed solely inside this test's isolated schema.
			execSQL(t, pool, fmt.Sprintf(`CREATE FUNCTION reject_post() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
   IF NOT (%s) THEN RAISE EXCEPTION 'failure injection precondition not reached'; END IF;
   RAISE EXCEPTION 'injected posting failure'; END $$;
   CREATE TRIGGER reject_post BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION reject_post()`, condition, event, table))
			r := request("rollback")
			out, err := (service.Service{Repository: Postgres{Pool: pool}}).Transfer(context.Background(), r)
			var dbError *pgconn.PgError
			if !errors.As(err, &dbError) || dbError.Message != "injected posting failure" {
				t.Fatalf("did not reach intended failure: %v", err)
			}
			if out != (model.Result{}) {
				t.Fatalf("failed transfer returned result: %+v", out)
			}
			assertUnchanged(t, pool, before)
			assertCounts(t, pool, 0)
			// Rollback must also release locks and leave the same idempotency key reusable.
			execSQL(t, pool, "DROP TRIGGER reject_post ON "+table)
			result := postHTTP(t, pool, r, 200, "")
			assertPosting(t, pool, result, r)
			assertCounts(t, pool, 1)
		})
	}
}

func TestPostgresConcurrentDoubleSpend(t *testing.T) {
	pool := database(t)
	execSQL(t, pool, `UPDATE accounts SET available_balance=100000,ledger_balance=100000 WHERE account_number=$1`, source)
	execSQL(t, pool, `UPDATE accounts SET available_balance=0,ledger_balance=0 WHERE account_number=$1`, destination)
	execSQL(t, pool, `INSERT INTO accounts(account_number,customer_id,product_code,currency_code,account_name,status,available_balance,ledger_balance)
  SELECT $1,customer_id,product_code,currency_code,'Concurrent destination','ACTIVE',0,0 FROM accounts WHERE account_number=$2`, thirdAccount, source)
	// Fail if any attempted UPDATE makes either balance negative, including writes
	// that would otherwise be rolled back and invisible to an external observer.
	execSQL(t, pool, `CREATE FUNCTION reject_negative_balance() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
  IF NEW.available_balance<0 OR NEW.ledger_balance<0 THEN RAISE EXCEPTION 'negative balance attempted'; END IF; RETURN NEW; END $$;
  CREATE TRIGGER reject_negative_balance BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION reject_negative_balance()`)
	first, second := request("spend-1"), request("spend-2")
	first.Amount = "80000.00"
	second.Amount = "80000.00"
	second.Destination = thirdAccount
	outcomes := contendedTransfers(t, pool, []model.Request{first, second})
	wins, losses := 0, 0
	var winner outcome
	for _, o := range outcomes {
		if o.err == nil {
			wins++
			winner = o
		} else if errors.Is(o.err, model.ErrInsufficient) {
			losses++
			if o.result != (model.Result{}) {
				t.Fatal("failed request returned posted result")
			}
		} else {
			t.Fatalf("unexpected failure: %v", o.err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("success=%d insufficient=%d", wins, losses)
	}
	balances := map[string][2]string{source: {"20000.00", "20000.00"}, destination: {"0.00", "0.00"}, thirdAccount: {"0.00", "0.00"}}
	balances[winner.request.Destination] = [2]string{"80000.00", "80000.00"}
	assertBalances(t, pool, balances)
	assertCounts(t, pool, 1)
	assertPosting(t, pool, winner.result, winner.request)
	var consistent bool
	err := pool.QueryRow(context.Background(), `SELECT bool_and(a.available_balance=a.ledger_balance AND
   a.ledger_balance=(CASE WHEN a.account_number=$1 THEN 100000 ELSE 0 END)+
   COALESCE((SELECT sum(CASE WHEN posting_type='CREDIT' THEN amount ELSE -amount END) FROM ledger_entries WHERE account_id=a.id),0))
   AND sum(a.available_balance)=100000 FROM accounts a`, source).Scan(&consistent)
	if err != nil || !consistent {
		t.Fatalf("account/ledger inconsistency: %v", err)
	}
}

func TestPostgresConcurrentIdempotency(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflict_%t", conflict), func(t *testing.T) {
			pool := database(t)
			requests := make([]model.Request, 8)
			for i := range requests {
				requests[i] = request("same-key")
				if conflict && i%2 == 1 {
					requests[i].Amount = "200000.00"
				}
			}
			outcomes := contendedTransfers(t, pool, requests)
			var first *outcome
			successes, conflicts := 0, 0
			for i := range outcomes {
				o := &outcomes[i]
				if o.err == nil {
					successes++
					if first == nil {
						first = o
					} else if first.result != o.result {
						t.Fatal("duplicate response differs")
					}
				} else if errors.Is(o.err, model.ErrConflict) {
					conflicts++
				} else {
					t.Fatal(o.err)
				}
			}
			if (!conflict && (successes != 8 || conflicts != 0)) || (conflict && (successes != 4 || conflicts != 4)) {
				t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
			}
			assertCounts(t, pool, 1)
			assertPosting(t, pool, first.result, first.request)
			wantSource, wantDestination := "400000.00", "200000.00"
			if first.request.Amount == "200000.00" {
				wantSource, wantDestination = "300000.00", "300000.00"
			}
			assertBalances(t, pool, map[string][2]string{source: {wantSource, wantSource}, destination: {wantDestination, wantDestination}})
		})
	}
}

func TestPostgresOpposingTransfers(t *testing.T) {
	pool := database(t)
	requests := make([]model.Request, 8)
	for i := range requests {
		requests[i] = request(fmt.Sprintf("opposing-%d", i))
		requests[i].Amount = "1.00"
		if i%2 == 1 {
			requests[i].Source, requests[i].Destination = requests[i].Destination, requests[i].Source
		}
	}
	for _, o := range contendedTransfers(t, pool, requests) {
		if o.err != nil {
			t.Fatal(o.err)
		}
		assertPosting(t, pool, o.result, o.request)
	}
	assertCounts(t, pool, 8)
	assertBalances(t, pool, map[string][2]string{source: {"500000.00", "500000.00"}, destination: {"100000.00", "100000.00"}})
}

func TestPostgresLedgerImmutable(t *testing.T) {
	pool := database(t)
	postHTTP(t, pool, request("immutable"), 200, "")
	before := snapshot(t, pool)
	for _, sql := range []string{`UPDATE ledger_entries SET amount=1`, `DELETE FROM ledger_entries`} {
		_, err := pool.Exec(context.Background(), sql)
		var dbError *pgconn.PgError
		if !errors.As(err, &dbError) || !strings.Contains(dbError.Message, "append-only") {
			t.Fatalf("ledger mutation not rejected by immutability trigger: %v", err)
		}
		assertUnchanged(t, pool, before)
	}
}

// Exercise the real handler, business service and PostgreSQL repository together.
func postHTTP(t *testing.T, pool *pgxpool.Pool, r model.Request, status int, message string) model.Result {
	t.Helper()
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	httpapi.New(service.Service{Repository: Postgres{Pool: pool}}).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/transfers/internal", strings.NewReader(string(body))))
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	if status != http.StatusOK {
		var response map[string]string
		if err = json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response["error"] != message {
			t.Fatalf("error response=%s err=%v", recorder.Body.String(), err)
		}
		return model.Result{}
	}
	var result model.Result
	if err = json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
