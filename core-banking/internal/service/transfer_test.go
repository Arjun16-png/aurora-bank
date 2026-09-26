package service

import (
	"aurora-bank/core-banking/internal/model"
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRepository struct {
	accounts map[string]model.Account
	stored   *model.Stored
	locks    []string
	posts    int
	failure  error
}

func (f *fakeRepository) WithinTransaction(ctx context.Context, fn func(Transaction) error) error {
	return fn(f)
}
func (f *fakeRepository) Find(context.Context, string) (*model.Stored, error) { return f.stored, nil }
func (f *fakeRepository) LockAccount(_ context.Context, n string) (model.Account, error) {
	f.locks = append(f.locks, n)
	a, ok := f.accounts[n]
	if !ok {
		return a, model.ErrNotFound
	}
	return a, nil
}
func (f *fakeRepository) Post(_ context.Context, p model.Posting) error {
	if f.failure != nil {
		return f.failure
	}
	f.posts++
	f.stored = &model.Stored{Request: p.Request, Result: p.Result}
	f.accounts[p.Source.Number] = p.Source
	f.accounts[p.Destination.Number] = p.Destination
	return nil
}
func fixture() (*fakeRepository, model.Request) {
	return &fakeRepository{accounts: map[string]model.Account{
		"B": {ID: "b", Number: "B", Status: "ACTIVE", Currency: "IDR", Available: "500000.00", Ledger: "550000.00"},
		"A": {ID: "a", Number: "A", Status: "ACTIVE", Currency: "IDR", Available: "100000.00", Ledger: "110000.00"},
	}}, model.Request{Source: "B", Destination: "A", Amount: "100000.00", Currency: "IDR", Key: "test-001"}
}
func TestTransfer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fakeRepository, *model.Request)
		want   error
	}{
		{"success", func(*fakeRepository, *model.Request) {}, nil},
		{"insufficient balance", func(_ *fakeRepository, r *model.Request) { r.Amount = "500000.01" }, model.ErrInsufficient},
		{"same account", func(_ *fakeRepository, r *model.Request) { r.Destination = r.Source }, model.ErrSameAccount},
		{"zero amount", func(_ *fakeRepository, r *model.Request) { r.Amount = "0.00" }, model.ErrInvalid},
		{"negative amount", func(_ *fakeRepository, r *model.Request) { r.Amount = "-1.00" }, model.ErrInvalid},
		{"fractional precision", func(_ *fakeRepository, r *model.Request) { r.Amount = "1.001" }, model.ErrInvalid},
		{"inactive account", func(f *fakeRepository, _ *model.Request) {
			a := f.accounts["A"]
			a.Status = "BLOCKED"
			f.accounts["A"] = a
		}, model.ErrInactive},
		{"currency mismatch", func(_ *fakeRepository, r *model.Request) { r.Currency = "USD" }, model.ErrCurrency},
		{"missing account", func(_ *fakeRepository, r *model.Request) { r.Destination = "missing" }, model.ErrNotFound},
		{"overflow", func(f *fakeRepository, _ *model.Request) {
			a := f.accounts["A"]
			a.Ledger = "99999999999999999.99"
			f.accounts["A"] = a
		}, model.ErrBalanceLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, r := fixture()
			tc.change(f, &r)
			result, err := (Service{f}).Transfer(context.Background(), r)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if tc.want != nil {
				if f.posts != 0 {
					t.Fatal("posted failed transfer")
				}
				return
			}
			if result.SourceBalance != "400000.00" || result.DestinationBalance != "200000.00" || f.accounts["B"].Ledger != "450000.00" || f.accounts["A"].Ledger != "210000.00" || result.Status != "POSTED" || result.PostedAt.IsZero() {
				t.Fatalf("incorrect result: %+v", result)
			}
			if !reflect.DeepEqual(f.locks, []string{"A", "B"}) {
				t.Fatal(f.locks)
			}
		})
	}
}
func TestIdempotency(t *testing.T) {
	f, r := fixture()
	s := Service{f}
	first, err := s.Transfer(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Transfer(context.Background(), r)
	if err != nil || first != second || f.posts != 1 {
		t.Fatalf("replay: %+v %v posts=%d", second, err, f.posts)
	}
	r.Amount = "1.00"
	if _, err = s.Transfer(context.Background(), r); !errors.Is(err, model.ErrConflict) {
		t.Fatal(err)
	}
}
func TestRepositoryFailure(t *testing.T) {
	f, r := fixture()
	f.failure = errors.New("write failed")
	out, err := (Service{f}).Transfer(context.Background(), r)
	if err == nil || out.Reference != "" {
		t.Fatal("failure returned successful result")
	}
}
