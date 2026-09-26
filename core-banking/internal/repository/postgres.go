package repository

import (
	"aurora-bank/core-banking/internal/model"
	"aurora-bank/core-banking/internal/service"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Postgres struct{ Pool *pgxpool.Pool }
type transaction struct{ pgx.Tx }

func (p Postgres) WithinTransaction(ctx context.Context, fn func(service.Transaction) error) error {
	tx, err := p.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	if err = fn(transaction{tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (t transaction) Find(ctx context.Context, key string) (*model.Stored, error) {
	// Serialize same-channel/key requests, including those concerning different accounts.
	// Hash collisions only serialize unrelated requests; the unique constraint remains authoritative.
	if _, err := t.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "SYSTEM:"+key); err != nil {
		return nil, err
	}
	var metadata []byte
	var kind, status string
	err := t.QueryRow(ctx, `SELECT metadata, transaction_type, status FROM transactions WHERE channel='SYSTEM' AND idempotency_key=$1`, key).Scan(&metadata, &kind, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if kind != "INTERNAL_TRANSFER" || status != "POSTED" {
		return nil, model.ErrConflict
	}
	var stored model.Stored
	if err = json.Unmarshal(metadata, &stored); err != nil {
		return nil, err
	}
	if stored.Result.Reference == "" {
		return nil, model.ErrConflict
	}
	return &stored, nil
}
func (t transaction) LockAccount(ctx context.Context, number string) (model.Account, error) {
	var a model.Account
	var available, ledger string
	err := t.QueryRow(ctx, `SELECT id::text,account_number,status,currency_code,available_balance::text,ledger_balance::text FROM accounts WHERE account_number=$1 FOR UPDATE`, number).Scan(&a.ID, &a.Number, &a.Status, &a.Currency, &available, &ledger)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, model.ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.Available, err = model.ParseMoney(available)
	if err != nil {
		return a, err
	}
	a.Ledger, err = model.ParseMoney(ledger)
	return a, err
}
func (t transaction) Post(ctx context.Context, p model.Posting) error {
	r, out := p.Request, p.Result
	metadata, err := json.Marshal(model.Stored{Request: r, Result: out})
	if err != nil {
		return err
	}
	var transactionID, journalID string
	err = t.QueryRow(ctx, `INSERT INTO transactions(transaction_reference,idempotency_key,transaction_type,channel,status,currency_code,amount,metadata) VALUES($1,$2,'INTERNAL_TRANSFER','SYSTEM','PROCESSING',$3,$4::numeric,$5) RETURNING id::text`, out.Reference, r.Key, r.Currency, string(r.Amount), metadata).Scan(&transactionID)
	if err != nil {
		return err
	}
	for _, side := range []struct {
		account model.Account
		role    string
	}{{p.Source, "DEBIT"}, {p.Destination, "CREDIT"}} {
		if _, err = t.Exec(ctx, `INSERT INTO transaction_accounts(transaction_id,account_id,role,amount) VALUES($1,$2,$3,$4::numeric)`, transactionID, side.account.ID, side.role, string(r.Amount)); err != nil {
			return err
		}
	}
	err = t.QueryRow(ctx, `INSERT INTO journal_entries(journal_number,transaction_id,entry_date) VALUES($1,$2,$3) RETURNING id::text`, "J-"+out.Reference, transactionID, out.PostedAt.Format("2006-01-02")).Scan(&journalID)
	if err != nil {
		return err
	}
	for i, side := range []struct {
		account model.Account
		role    string
	}{{p.Source, "DEBIT"}, {p.Destination, "CREDIT"}} {
		var lineID string
		err = t.QueryRow(ctx, `INSERT INTO journal_lines(journal_entry_id,line_number,account_id,entry_side,amount,currency_code) VALUES($1,$2,$3,$4,$5::numeric,$6) RETURNING id::text`, journalID, i+1, side.account.ID, side.role, string(r.Amount), r.Currency).Scan(&lineID)
		if err != nil {
			return err
		}
		_, err = t.Exec(ctx, `INSERT INTO ledger_entries(ledger_reference,account_id,transaction_id,journal_entry_id,journal_line_id,posting_type,amount,currency_code,balance_after,effective_at,posted_at) VALUES($1,$2,$3,$4,$5,$6,$7::numeric,$8,$9::numeric,$10,$10)`, "L-"+out.Reference+"-"+side.role, side.account.ID, transactionID, journalID, lineID, side.role, string(r.Amount), r.Currency, string(side.account.Ledger), out.PostedAt)
		if err != nil {
			return err
		}
		_, err = t.Exec(ctx, `UPDATE accounts SET available_balance=$2::numeric,ledger_balance=$3::numeric WHERE id=$1`, side.account.ID, string(side.account.Available), string(side.account.Ledger))
		if err != nil {
			return err
		}
	}
	if _, err = t.Exec(ctx, `UPDATE journal_entries SET status='POSTED',posted_at=$2 WHERE id=$1`, journalID, out.PostedAt); err != nil {
		return err
	}
	_, err = t.Exec(ctx, `UPDATE transactions SET status='POSTED',posted_at=$2 WHERE id=$1`, transactionID, out.PostedAt)
	return err
}
