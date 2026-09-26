package service

import (
	"aurora-bank/core-banking/internal/model"
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"time"
)

type Transaction interface {
	Find(context.Context, string) (*model.Stored, error)
	LockAccount(context.Context, string) (model.Account, error)
	Post(context.Context, model.Posting) error
}
type Repository interface {
	WithinTransaction(context.Context, func(Transaction) error) error
}
type Service struct{ Repository Repository }

func (s Service) Transfer(ctx context.Context, r model.Request) (model.Result, error) {
	var result model.Result
	if err := r.Validate(); err != nil {
		return result, err
	}
	// Normalize callers that construct requests directly rather than via JSON.
	r.Amount, _ = model.ParseMoney(string(r.Amount))
	err := s.Repository.WithinTransaction(ctx, func(tx Transaction) error {
		stored, err := tx.Find(ctx, r.Key)
		if err != nil {
			return err
		}
		if stored != nil {
			if !stored.Matches(r) {
				return model.ErrConflict
			}
			result = stored.Result
			return nil
		}
		numbers := []string{r.Source, r.Destination}
		sort.Strings(numbers)
		accounts := map[string]model.Account{}
		for _, number := range numbers {
			a, e := tx.LockAccount(ctx, number)
			if e != nil {
				return e
			}
			accounts[number] = a
		}
		source, destination := accounts[r.Source], accounts[r.Destination]
		if source.Status != "ACTIVE" || destination.Status != "ACTIVE" {
			return model.ErrInactive
		}
		if source.Currency != r.Currency || destination.Currency != r.Currency {
			return model.ErrCurrency
		}
		if source.Available.Compare(r.Amount) < 0 {
			return model.ErrInsufficient
		}
		for _, change := range []struct {
			account *model.Account
			amount  model.Money
		}{{&source, r.Amount.Negate()}, {&destination, r.Amount}} {
			change.account.Available, err = change.account.Available.Add(change.amount)
			if err != nil {
				return model.ErrBalanceLimit
			}
			change.account.Ledger, err = change.account.Ledger.Add(change.amount)
			if err != nil {
				return model.ErrBalanceLimit
			}
		}
		var id [16]byte
		if _, err = rand.Read(id[:]); err != nil {
			return err
		}
		result = model.Result{Reference: "TRF-" + hex.EncodeToString(id[:]), Status: "POSTED", Amount: r.Amount, Currency: r.Currency, Source: r.Source, Destination: r.Destination, SourceBalance: source.Available, DestinationBalance: destination.Available, PostedAt: time.Now().UTC().Truncate(time.Microsecond)}
		return tx.Post(ctx, model.Posting{Request: r, Result: result, Source: source, Destination: destination})
	})
	if err != nil {
		return model.Result{}, err
	}
	return result, nil
}
