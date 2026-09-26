package httpapi

import (
	"aurora-bank/core-banking/internal/model"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type TransferService interface {
	Transfer(context.Context, model.Request) (model.Result, error)
}

func New(s TransferService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/transfers/internal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fail := func(status int, message string) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			fail(405, "method not allowed")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var request model.Request
		if err := decoder.Decode(&request); err != nil {
			fail(400, "invalid JSON request")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			fail(400, "request must contain one JSON object")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		result, err := s.Transfer(ctx, request)
		if err != nil {
			switch {
			case errors.Is(err, model.ErrInvalid), errors.Is(err, model.ErrSameAccount):
				fail(400, err.Error())
			case errors.Is(err, model.ErrNotFound):
				fail(404, err.Error())
			case errors.Is(err, model.ErrConflict):
				fail(409, err.Error())
			case errors.Is(err, model.ErrInactive), errors.Is(err, model.ErrCurrency), errors.Is(err, model.ErrInsufficient), errors.Is(err, model.ErrBalanceLimit):
				fail(422, err.Error())
			default:
				slog.Error("internal transfer failed", "error", err)
				fail(500, "transfer could not be completed; retry with the same idempotency key")
			}
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	return mux
}
