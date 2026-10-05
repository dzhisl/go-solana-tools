package solanatools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
)

type httpFunc func(*http.Request) (*http.Response, error)

func (f httpFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestHelius(t *testing.T) {
	for _, status := range []int{200, 404, 429} {
		h, err := NewHelius(HeliusConfig{APIKey: "test-secret", HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("api-key") != "test-secret" || !strings.HasSuffix(r.URL.Path, "/funded-by") {
				t.Error("incorrect request")
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"amount":12.500001,"amountRaw":"12500001"}`))}, nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		info, err := h.FundingSource(context.Background(), solana.SystemProgramID)
		switch status {
		case 200:
			if err != nil || info.Amount.String() != "12.500001" {
				t.Fatal(info, err)
			}
		case 404:
			if !errors.Is(err, ErrFundingNotFound) {
				t.Fatal(err)
			}
		case 429:
			if err == nil {
				t.Fatal("missing HTTP error")
			}
		}
	}
	h, _ := NewHelius(HeliusConfig{APIKey: "test-secret", HTTPClient: httpFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New(r.URL.String()) })})
	_, err := h.FundingSource(context.Background(), solana.SystemProgramID)
	if err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatal("transport error leaked key", err)
	}
}
