package model

import (
	"encoding/json"
	"errors"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Money is a canonical decimal in major currency units, never a float.
// big.Int arithmetic covers the entire PostgreSQL numeric(19,2) range.
type Money string

var decimal = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]{1,2})?$`)

func ParseMoney(s string) (Money, error) {
	if !decimal.MatchString(s) {
		return "", ErrInvalid
	}
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	parts := strings.Split(s, ".")
	if len(parts[0]) > 17 {
		return "", ErrInvalid
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = (parts[1] + "0")[:2]
	}
	result := parts[0] + "." + fraction
	if negative && result != "0.00" {
		result = "-" + result
	}
	return Money(result), nil
}
func (m *Money) UnmarshalJSON(b []byte) error {
	v, e := ParseMoney(string(b))
	if e == nil {
		*m = v
	}
	return e
}
func (m Money) MarshalJSON() ([]byte, error) {
	v, e := ParseMoney(string(m))
	if e != nil {
		return nil, e
	}
	return []byte(v), nil
}
func (m Money) cents() *big.Int {
	n, ok := new(big.Int).SetString(strings.ReplaceAll(string(m), ".", ""), 10)
	if !ok {
		return new(big.Int)
	}
	return n
}
func (m Money) Compare(n Money) int { return m.cents().Cmp(n.cents()) }
func (m Money) Add(n Money) (Money, error) {
	v := new(big.Int).Add(m.cents(), n.cents())
	negative := v.Sign() < 0
	v.Abs(v)
	s := v.String()
	for len(s) < 3 {
		s = "0" + s
	}
	s = s[:len(s)-2] + "." + s[len(s)-2:]
	if negative {
		s = "-" + s
	}
	return ParseMoney(s)
}
func (m Money) Negate() Money {
	if strings.HasPrefix(string(m), "-") {
		return Money(strings.TrimPrefix(string(m), "-"))
	}
	if m == "0.00" {
		return m
	}
	return "-" + m
}

var (
	ErrInvalid      = errors.New("invalid request fields or amount")
	ErrSameAccount  = errors.New("source and destination must differ")
	ErrNotFound     = errors.New("account not found")
	ErrInactive     = errors.New("both accounts must be active")
	ErrCurrency     = errors.New("account currencies must match the requested currency")
	ErrInsufficient = errors.New("insufficient available balance")
	ErrConflict     = errors.New("idempotency key already used for another request")
	ErrBalanceLimit = errors.New("resulting balance exceeds supported range")
)

type Request struct {
	Source      string `json:"source_account_number"`
	Destination string `json:"destination_account_number"`
	Amount      Money  `json:"amount"`
	Currency    string `json:"currency"`
	Key         string `json:"idempotency_key"`
}

func (r Request) Validate() error {
	validText := func(s string, max int) bool {
		return strings.TrimSpace(s) != "" && !strings.ContainsRune(s, 0) && utf8.ValidString(s) && utf8.RuneCountInString(s) <= max
	}
	a, e := ParseMoney(string(r.Amount))
	if e != nil || a.Compare("0.00") <= 0 || !validText(r.Source, 34) || !validText(r.Destination, 34) || !validText(r.Key, 128) || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(r.Currency) {
		return ErrInvalid
	}
	if r.Source == r.Destination {
		return ErrSameAccount
	}
	return nil
}

type Result struct {
	Reference          string    `json:"transaction_reference"`
	Status             string    `json:"status"`
	Amount             Money     `json:"amount"`
	Currency           string    `json:"currency"`
	Source             string    `json:"source_account_number"`
	Destination        string    `json:"destination_account_number"`
	SourceBalance      Money     `json:"source_balance_after"`
	DestinationBalance Money     `json:"destination_balance_after"`
	PostedAt           time.Time `json:"posted_at"`
}
type Account struct {
	ID, Number, Status, Currency string
	Available, Ledger            Money
}
type Posting struct {
	Request             Request
	Result              Result
	Source, Destination Account
}
type Stored struct {
	Request Request `json:"request"`
	Result  Result  `json:"result"`
}

func (s Stored) Matches(r Request) bool {
	return s.Request.Source == r.Source && s.Request.Destination == r.Destination && s.Request.Currency == r.Currency && s.Request.Amount.Compare(r.Amount) == 0
}

var _ json.Marshaler = Money("")
