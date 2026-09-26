package httpapi

import (
	"aurora-bank/core-banking/internal/model"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type stub struct{ err error }

func (s stub) Transfer(context.Context, model.Request) (model.Result, error) {
	return model.Result{Amount: "1.00", SourceBalance: "2.00", DestinationBalance: "3.00"}, s.err
}
func TestHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		err                error
		status             int
	}{
		{"success", "POST", `{"amount":1}`, nil, 200},
		{"invalid decimal", "POST", `{"amount":1.001}`, nil, 400},
		{"unknown field", "POST", `{"unknown":1}`, nil, 400},
		{"trailing body", "POST", `{} {}`, nil, 400},
		{"method", "GET", ``, nil, 405},
		{"conflict", "POST", `{}`, model.ErrConflict, 409},
		{"balance", "POST", `{}`, model.ErrInsufficient, 422},
		{"missing", "POST", `{}`, model.ErrNotFound, 404},
		{"database failure", "POST", `{}`, errors.New("private database details"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			New(stub{tc.err}).ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v1/transfers/internal", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Fatal("leaked internals")
			}
		})
	}
}
